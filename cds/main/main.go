package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/envoyproxy/go-control-plane/cds/stats"

	"github.com/pkg/errors"

	"github.com/envoyproxy/go-control-plane/cds/env"

	cdsServer "github.com/envoyproxy/go-control-plane/cds/server"
	"github.com/envoyproxy/go-control-plane/cds/utils"
	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	resourcesV3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/http/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/tcp/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// SnapshotCache stores the snapshot for the cluster resources against a specific Envoy NodeID. Different nodes can
// technically have different versions of snapshots but, in our case they are all same. It's possible for a new Envoy
// replica to get added after the latest snapshot gets generated and hence we update SnapshotCache separately using
// the value from the snapshotVal which points to the latest snapshot.
type SnapshotCache struct {
	mu            sync.Mutex
	snapshotCache cache.SnapshotCache
}

var (
	logger utils.Logger
	// snapshotVal stores the latest snapshot generated from the latest clusters slice which is created by aggregating
	// all the clusters from different ConfigMap(s). Upon detecting a new version of the cluster ConfigMap, k8s watcher
	// triggers the update function which then reads this ConfigMap and add/update the clusters slice, generate a new
	// snapshot, and update this value.
	snapshotVal atomic.Value
	// settings contains the config flag values which are either the declared default or the values set by using the
	// environment variables.
	settings env.Settings
)

// Initialize Variables
func init() {
	// Initialize Settings Object
	settings = env.NewSettings()
	// Initialize Logger
	logger = utils.Logger{
		Debug: settings.DebugLogging,
		Info:  settings.DebugLogging,
	}
	logger.Infof("settings: %s", settings)
	// Initialize Snapshot
	snapshotVal = atomic.Value{}
}

// ParseClusterConfigMap parses the cluster ConfigMap and return the v3.Cluster.
func ParseClusterConfigMap(compressedConfigMap *coreV1.ConfigMap) (*v3.Cluster, error) {
	configMapName := compressedConfigMap.Name
	logger.Infof("cluster ConfigMap name: %s", configMapName)
	configMap, err := utils.Decompress(compressedConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(configMapName)
		return nil, errors.Wrap(err, "error occurred while parsing cluster ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(configMapName)
	for _, yamlBytes := range configMap {
		proto, err := utils.ConvertYamlToClusterProto(yamlBytes)
		if err != nil {
			stats.RecordConfigMapDataError(configMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the cluster ConfigMap")
		}
		stats.RecordConfigMapProcessedSuccessError(configMapName)
		return proto, nil
	}
	return nil, nil
}

/**
 * doUpdate start the update process by parsing different ConfigMap(s), aggregating all the resources and update the
 * snapshot cache.
 */
func doUpdate(clusterConfigMap *coreV1.ConfigMap, clusters []*v3.Cluster) error {
	clusterPb, err := ParseClusterConfigMap(clusterConfigMap)
	if err != nil {
		return errors.Wrap(err, "failed to get service names from the import order")
	}
	clusters = append(clusters, clusterPb)

	clusterResource := make([]types.Resource, len(clusters))
	for index, cluster := range clusters {
		clusterResource[index] = cluster
	}
	versionID := time.Now().Format("2006-01-02T15-04-05")
	logger.Debugf("creating new snapshot with version: %s", versionID)
	newSnapshot, err := cache.NewSnapshot(
		versionID,
		map[resourcesV3.Type][]types.Resource{
			resourcesV3.ClusterType: clusterResource,
		},
	)
	if err != nil {
		return errors.Wrap(err, "error occurred while updating the cache snapshot")
	}
	snapshotVal.Store(*newSnapshot)
	logger.Infof("successfully updated snapshot with version: %s", versionID)
	return nil
}

/**
 * updateSnapshot read all ConfigMap(s) to create the cluster resources and update the cache snapshot when a new version
 * of the service order ConfigMap gets detected.
 */
func updateSnapshot(watchEventChannel <-chan watch.Event, mutex *sync.Mutex, clusters []*v3.Cluster) {
	for {
		event, open := <-watchEventChannel
		if open {
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				mutex.Lock()
				if configMap, ok := event.Object.(*coreV1.ConfigMap); ok {
					if strings.HasPrefix(configMap.Name, settings.EnvoyClusterConfigNamePrefix) {
						logger.Debugf("*** Envoy Cluster ConfigMap Modified ***")
						if err := doUpdate(configMap, clusters); err != nil {
							logger.Errorf("error occurred while processing update", err.Error())
						}
					} else {
						logger.Debugf(fmt.Sprintf("ignoring non-cluster ConfigMap: %s", configMap.Name))
					}
				}
				mutex.Unlock()
			case watch.Deleted:
				// Do Nothing
			default:
				// Do Nothing
			}
		} else {
			// If eventChannel is closed, it means the server has closed the connection
			return
		}
	}
}

/**
 * initKubernetesClient initialize the kubernetes client which would be used to fetch the ConfigMap resources.
 */
func initKubernetesClient() *kubernetes.Clientset {
	logger.Debugf("start initializing the kubernetes client...")

	// Create In-Cluster Config
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("error occurred while creating in-cluster config", err.Error())
		panic(err.Error())
	}

	// Create ClientSet
	clientSet, err := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		logger.Errorf("error occurred while creating the new kubernetes client-set", err.Error())
		panic(err.Error())
	}

	// Sanity Check(s)
	namespace := settings.ConfigMapNamespace
	timeoutSeconds := int64(10)
	if _, err = clientSet.CoreV1().ConfigMaps(namespace).List(context.TODO(), metaV1.ListOptions{Limit: 1, TimeoutSeconds: &timeoutSeconds}); err != nil {
		logger.Errorf("error occurred while listing ConfigMap(s). Please check k8s permissions", err.Error())
		panic(err.Error())
	}

	// Return ClientSet
	return clientSet
}

/**
 * setupWatcher set up a new watcher which would look at the ConfigMap state changes and update the snapshot cache by
 * pulling and aggregating the data from different ConfigMap(s) on changes.
 */
func setupWatcher(clientSet *kubernetes.Clientset) {
	mutex := &sync.Mutex{}
	var clusters []*v3.Cluster
	go watchForChanges(clientSet, mutex, clusters)
}

/**
 * watchForChanges watcher implementation which would look at any state changes on the Envoy cluster ConfigMap.
 * NOTE: In this case we don't have any `ListOptions` which means that if a new ConfigMap gets published in the
 * namespace being watched then it'll trigger the update function, but we would filter out all the ConfigMap(s) except
 * the ones with a name prefix `envoy-cluster-` indicating that it contains the cluster info.
 */
func watchForChanges(clientSet *kubernetes.Clientset, mutex *sync.Mutex, clusters []*v3.Cluster) {
	for {
		namespace := settings.ConfigMapNamespace
		watcher, err := clientSet.CoreV1().ConfigMaps(namespace).Watch(context.TODO(), metaV1.ListOptions{})
		if err != nil {
			logger.Errorf("error occurred while creating the watcher", err.Error())
			panic(err.Error())
		}
		// Detect changes & update cache snapshot
		updateSnapshot(watcher.ResultChan(), mutex, clusters)
	}
}

/**
 * setupSnapshotUpdater set up a new snapshot updater which would set the latest snapshot to all the client nodes.
 */
func setupSnapshotUpdater(sc *SnapshotCache) {
	go sc.updateSnapshotCache()
}

/**
 * updateSnapshotCache update the snapshot cache for the client nodes with the most recent snapshot.
 */
func (sc *SnapshotCache) updateSnapshotCache() {
	for {
		latestSnapshotEntry := snapshotVal.Load()
		if latestSnapshotEntry != nil {
			latestSnapshot := latestSnapshotEntry.(cache.Snapshot)
			latestSnapshotVersion := latestSnapshot.GetVersion(resourcesV3.ClusterType)
			sc.mu.Lock()
			nodesIdsSet := sc.snapshotCache.GetStatusKeys()
			for _, nodeID := range nodesIdsSet {
				snapshot, err := sc.snapshotCache.GetSnapshot(nodeID)
				if err != nil {
					logger.Debugf("unable to get the existing snapshot for nodeID: %s", nodeID, err.Error())
					sc.setSnapshot(nodeID, latestSnapshotVersion, &latestSnapshot)
				} else if snapshot.GetVersion(resourcesV3.ClusterType) != latestSnapshotVersion {
					sc.setSnapshot(nodeID, latestSnapshotVersion, &latestSnapshot)
				}
			}
			sc.mu.Unlock()
		}
		// Add some delay
		time.Sleep(1 * time.Second)
	}
}

/**
 * setSnapshot set the snapshot for the given nodeID in the snapshots cache.
 */
func (sc *SnapshotCache) setSnapshot(nodeID, version string, snapshot *cache.Snapshot) {
	logger.Infof("start setting snapshot for nodeID: %s", nodeID)
	if err := sc.snapshotCache.SetSnapshot(context.Background(), nodeID, snapshot); err != nil {
		logger.Errorf("error occurred while updating the snapshot cache", err.Error())
		stats.RecordSnapshotCacheUpdateError(nodeID, version)
	} else {
		stats.RecordSnapshotCacheUpdateSuccess(nodeID, version)
		logger.Infof("successfully updated the snapshot cache for nodeID: %s", nodeID)
	}
}

/**
 * main bootstrap the CDS server and initializes the kubernetes client, sets up watcher etc.
 */
func main() {
	flag.Parse()
	logger.Infof("start initializing the main server...")

	// Initialize Kubernetes Client
	clientSet := initKubernetesClient()

	// Setup ConfigMap Watcher
	setupWatcher(clientSet)

	// Setup Cache Snapshot Updater
	sc := SnapshotCache{
		snapshotCache: cache.NewSnapshotCache(false, cache.IDHash{}, logger),
	}
	setupSnapshotUpdater(&sc)

	// Run the CDS server
	logger.Infof("running CDS server...")
	ctx := context.Background()
	gRPCServer := server.NewServer(ctx, sc.snapshotCache, nil)
	cdsServer.RunServer(&settings, gRPCServer, logger)
}
