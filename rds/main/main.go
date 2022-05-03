package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/envoyproxy/go-control-plane/rds/stats"

	"github.com/pkg/errors"

	"github.com/envoyproxy/go-control-plane/rds/env"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	resourcesV3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	rdsServer "github.com/envoyproxy/go-control-plane/rds/server"
	"github.com/envoyproxy/go-control-plane/rds/utils"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

var (
	logger        utils.Logger
	snapshotsVal  atomic.Value
	snapshotCache cache.SnapshotCache
	settings      env.Settings
)

// Initialize Variables
func init() {
	// Initialize Logger
	logger = utils.Logger{
		Debug: settings.DebugLogging,
	}
	// Initialize Settings Object
	settings = env.NewSettings()
	logger.Infof("settings: %s", settings)
	// Initialize Snapshot
	snapshotsVal = atomic.Value{}
	// Initialize Snapshot Cache
	snapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, logger)
}

/**
 * parseServiceImportOrderConfigMap parses the service import order ConfigMap and return an array of service names which
 * is used while aggregating the per-service routes and virtual clusters.
 */
func parseServiceImportOrderConfigMap(configMap *coreV1.ConfigMap) ([]string, error) {
	svcImportOrderConfigMapName := configMap.Name
	logger.Debugf("service import order ConfigMap name: %s", svcImportOrderConfigMapName)
	configMapDataMap, err := utils.Decompress(configMap)
	if err != nil {
		stats.RecordConfigMapParseError(svcImportOrderConfigMapName)
		return nil, errors.Wrap(err, "error occurred while parsing service import order ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(svcImportOrderConfigMapName)
	var serviceNames []string
	for _, yamlBytes := range configMapDataMap {
		if err := yaml.Unmarshal(yamlBytes, &serviceNames); err != nil {
			stats.RecordConfigMapDataError(svcImportOrderConfigMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the service import order ConfigMap")
		}
		stats.RecordConfigMapProcessedSuccessError(svcImportOrderConfigMapName)
	}
	return serviceNames, nil
}

/**
 * getRoutesImportOrder parse the route import order ConfigMap to get the routes & virtual clusters import order which
 * is used to aggregate the routes & virtual clusters in a specific order while creating the route tables.
 */
func getRoutesImportOrder(clientSet *kubernetes.Clientset, k8sNamespace, configMapRequiredVersion string) (map[string]map[string][]string, error) {
	routesImportOrderConfigMapName := settings.EnvoyRoutesImportOrderConfigName
	routesImportOrderConfigMap, err := getVersionedConfigMap(clientSet, routesImportOrderConfigMapName, k8sNamespace, configMapRequiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(routesImportOrderConfigMapName)
		return nil, errors.Wrap(err, "error occurred while fetching routes import order ConfigMap")
	}
	configMapDataMap, err := utils.Decompress(routesImportOrderConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(routesImportOrderConfigMapName)
		return nil, errors.Wrap(err, "error occurred while parsing routes import order ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(routesImportOrderConfigMapName)
	routesImportOrderMap := make(map[string]map[string][]string)
	for _, yamlBytes := range configMapDataMap {
		if err := yaml.Unmarshal(yamlBytes, &routesImportOrderMap); err != nil {
			stats.RecordConfigMapDataError(routesImportOrderConfigMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling routes import order ConfigMap")
		}
		stats.RecordConfigMapProcessedSuccessError(routesImportOrderConfigMapName)
	}
	return routesImportOrderMap, nil
}

/**
 * getVersionedConfigMap get the ConfigMap and compare the version hash on it with the master version retrieved from the
 * service import order ConfigMap. If there is a hash/version mismatch then we wait for some time hoping that there is
 * an ongoing sync, and we would get the expected version once the sync finishes. If we don't get the correct/required
 * version hash even after waiting then we give up and return an error.
 */
func getVersionedConfigMap(clientSet *kubernetes.Clientset, configMapName, k8sNamespace, configMapRequiredVersion string, shouldWaitForSync bool) (*coreV1.ConfigMap, error) {
	configMap, err := clientSet.CoreV1().ConfigMaps(k8sNamespace).Get(context.TODO(), configMapName, metaV1.GetOptions{})
	if err != nil {
		stats.RecordConfigMapFetchError(configMapName)
		return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetching ConfigMap: %s", configMapName))
	}
	configMapVersion, versionLabelFound := configMap.Labels["versionHash"]
	if !versionLabelFound {
		return nil, fmt.Errorf("failed to get version label from the ConfigMap: %s", configMapName)
	}
	// If the version hash retrieved from the ConfigMap is not same as the one we got from the main import order then,
	// we'll wait for some time and see if an update is pending.
	if configMapVersion != configMapRequiredVersion {
		if shouldWaitForSync {
			time.Sleep(time.Duration(settings.SyncDelayTimeSeconds) * time.Second)
			return getVersionedConfigMap(clientSet, configMapName, k8sNamespace, configMapRequiredVersion, false)
		}
		return nil, fmt.Errorf("version hash mismatch on the ConfigMap: %s. Required: %s, Received: %s", configMapName, configMapRequiredVersion, configMapVersion)
	}
	return configMap, nil
}

/**
 * getRouteConfigurations This method does the following:
 * 1. It reads all the route table configurations from the ConfigMap named `envoy-route-configurations-config`.
 * 2. It reads the route import order for each of these route table configurations.
 * 3. It aggregates all the per-service routes and virtual clusters following the import order and append the data back
 *    to the route table to create a final route table with all the routes & virtual clusters.
 */
func getRouteConfigurations(clientSet *kubernetes.Clientset, k8sNamespace, configMapRequiredVersion string, serviceNames []string) ([]*v3.RouteConfiguration, error) {
	routeConfigurationConfigMapName := settings.EnvoyRouteConfigurationsConfigName
	routeConfigurationsConfigMap, err := getVersionedConfigMap(clientSet, routeConfigurationConfigMapName, k8sNamespace, configMapRequiredVersion, true)
	var routeTables []*v3.RouteConfiguration
	if err != nil {
		stats.RecordConfigMapFetchError(routeConfigurationConfigMapName)
		return nil, errors.Wrap(err, "error occurred while fetching routes configuration ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(routeConfigurationConfigMapName)
	routesImportOrderMap, err := getRoutesImportOrder(clientSet, k8sNamespace, configMapRequiredVersion)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while getting routes import order")
	}
	configMapDataMap, err := utils.Decompress(routeConfigurationsConfigMap)
	if err != nil {
		stats.RecordConfigMapDataError(routeConfigurationConfigMapName)
		return nil, errors.Wrap(err, "error occurred while parsing routes configuration ConfigMap")
	}
	// Read all the route table configs
	for name, yamlBytes := range configMapDataMap {
		logger.Infof("start creating route table '%s' with version hash '%s'", name, configMapRequiredVersion)
		routeTablePb, err := utils.ConvertYamlToRouteConfigurationProto(yamlBytes)
		if err != nil {
			stats.RecordConfigMapDataError(routeConfigurationConfigMapName)
			return nil, errors.Wrap(err, "error occurred while creating v3.RouteConfiguration proto from the ConfigMap data")
		}
		// Append Routes
		aggregatedRoutes, err := aggregateRoutes(clientSet, k8sNamespace, configMapRequiredVersion, serviceNames, routesImportOrderMap[name]["routes"])
		if err != nil {
			stats.RecordRouteTableCreateError(name)
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while aggregating the routes for route table: %s", name))
		}
		stats.RecordRouteTableAggregatedRoutes(name, uint64(len(aggregatedRoutes)))
		routeTablePb.VirtualHosts[0].Routes = append(routeTablePb.VirtualHosts[0].Routes, aggregatedRoutes...)
		stats.RecordRouteTableTotalRoutes(name, uint64(len(routeTablePb.VirtualHosts[0].Routes)))
		aggregatedRoutes = nil
		// Append Virtual Clusters
		aggregatedVirtualClusters, err := aggregateVirtualClusters(clientSet, k8sNamespace, configMapRequiredVersion, serviceNames, routesImportOrderMap[name]["virtualClusters"])
		if err != nil {
			stats.RecordRouteTableCreateError(name)
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while aggregating the virtual clusters for route table: %s", name))
		}
		stats.RecordRouteTableAggregatedVirtualClusters(name, uint64(len(aggregatedVirtualClusters)))
		routeTablePb.VirtualHosts[0].VirtualClusters = append(routeTablePb.VirtualHosts[0].VirtualClusters, aggregatedVirtualClusters...)
		stats.RecordRouteTableTotalVirtualClusters(name, uint64(len(routeTablePb.VirtualHosts[0].VirtualClusters)))
		aggregatedVirtualClusters = nil
		logger.Infof("finished creating the route table: %s", name)
		stats.RecordRouteTableCreateSuccess(name)
		routeTables = append(routeTables, routeTablePb)
		routeTablePb = nil
	}
	stats.RecordConfigMapProcessedSuccessError(routeConfigurationConfigMapName)
	return routeTables, nil
}

/**
 * aggregateRoutes aggregate all the routes following the routes import order.
 */
func aggregateRoutes(clientSet *kubernetes.Clientset, k8sNamespace, configMapRequiredVersion string, serviceNames, routesImportOrder []string) (aggregatedRoutes []*v3.Route, err error) {
	for _, importConfigName := range routesImportOrder {
		serviceRoutesSuffix := "-svc-routes"
		serviceVirtualClusterSuffix := "-vc-svc-routes"
		if strings.Contains(importConfigName, serviceRoutesSuffix) && !strings.Contains(importConfigName, serviceVirtualClusterSuffix) {
			prefix := strings.Split(importConfigName, serviceRoutesSuffix)[0]
			svcRoutes, err := parseServiceRoutesConfigMap(clientSet, k8sNamespace, configMapRequiredVersion, prefix, serviceNames)
			if err != nil {
				return nil, errors.Wrap(err, "error occurred while parsing per-service route ConfigMap(s)")
			}
			aggregatedRoutes = append(aggregatedRoutes, svcRoutes...)
			svcRoutes = nil
		} else {
			routes, err := getRoutes(clientSet, k8sNamespace, importConfigName, configMapRequiredVersion)
			if err != nil {
				return nil, errors.Wrap(err, "error occurred while parsing the route ConfigMap")
			}
			aggregatedRoutes = append(aggregatedRoutes, routes...)
			routes = nil
		}
	}
	return aggregatedRoutes, nil
}

/**
 * aggregateVirtualClusters aggregate all the virtual clusters following the vc import order.
 */
func aggregateVirtualClusters(clientSet *kubernetes.Clientset, k8sNamespace, configMapRequiredVersion string, serviceNames, vcImportOrder []string) (aggregatedVirtualClusters []*v3.VirtualCluster, err error) {
	for _, importConfigName := range vcImportOrder {
		serviceRoutesSuffix := "-svc-routes"
		serviceVirtualClusterSuffix := "-vc-svc-routes"
		if strings.Contains(importConfigName, serviceVirtualClusterSuffix) {
			prefix := strings.Split(importConfigName, serviceRoutesSuffix)[0]
			svcVirtualClusters, err := parseServiceVirtualClustersConfigMap(clientSet, k8sNamespace, configMapRequiredVersion, prefix, serviceNames)
			if err != nil {
				return nil, errors.Wrap(err, "error occurred while parsing per-service virtual clusters ConfigMap(s)")
			}
			aggregatedVirtualClusters = append(aggregatedVirtualClusters, svcVirtualClusters...)
			svcVirtualClusters = nil
		} else {
			virtualClusters, err := getVirtualClusters(clientSet, k8sNamespace, importConfigName, configMapRequiredVersion)
			if err != nil {
				return nil, errors.Wrap(err, "error occurred while parsing the virtual cluster ConfigMap")
			}
			aggregatedVirtualClusters = append(aggregatedVirtualClusters, virtualClusters...)
			virtualClusters = nil
		}
	}
	return aggregatedVirtualClusters, nil
}

/**
 * parseServiceRoutesConfigMap read the per-service routes k8s ConfigMap(s) and returns an aggregated array of v3.Route.
 */
func parseServiceRoutesConfigMap(clientSet *kubernetes.Clientset, k8sNamespace, configMapRequiredVersion, configMapNamePrefix string, serviceNames []string) (allSvcRoutes []*v3.Route, err error) {
	// Service Routes
	for _, serviceName := range serviceNames {
		configMapName := fmt.Sprintf("%s-%s", configMapNamePrefix, serviceName)
		serviceRoutes, err := getRoutes(clientSet, k8sNamespace, configMapName, configMapRequiredVersion)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetchig the route ConfigMap: '%s'", configMapName))
		} else if len(serviceRoutes) > 0 {
			allSvcRoutes = append(allSvcRoutes, serviceRoutes...)
		} else {
			logger.Debugf(fmt.Sprintf("no routes defined in: '%s'", configMapName))
		}
		serviceRoutes = nil
	}
	logger.Infof("total number of service routes found: %d", len(allSvcRoutes))
	return allSvcRoutes, nil
}

/**
 * parseServiceVirtualClustersConfigMap read the per-service virtual clusters k8s ConfigMap(s) and returns an aggregated
 * array of v3.VirtualCluster.
 */
func parseServiceVirtualClustersConfigMap(clientSet *kubernetes.Clientset, k8sNamespace, configMapRequiredVersion, configMapNamePrefix string, serviceNames []string) (allSvcVirtualClusters []*v3.VirtualCluster, err error) {
	// Service Virtual Clusters
	for _, serviceName := range serviceNames {
		configMapName := fmt.Sprintf("%s-%s", configMapNamePrefix, serviceName)
		serviceVirtualClusters, err := getVirtualClusters(clientSet, k8sNamespace, configMapName, configMapRequiredVersion)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetchig the virtual cluster ConfigMap: '%s'", configMapName))
		} else if len(serviceVirtualClusters) > 0 {
			allSvcVirtualClusters = append(allSvcVirtualClusters, serviceVirtualClusters...)
		} else {
			logger.Debugf(fmt.Sprintf("no virtual clusters defined in: '%s'", configMapName))
		}
		serviceVirtualClusters = nil
	}
	logger.Infof("total number of virtual clusters found: %d", len(allSvcVirtualClusters))
	return allSvcVirtualClusters, nil
}

/**
 * getRoutes read the routes k8s ConfigMap and returns an array of v3.Route.
 */
func getRoutes(clientSet *kubernetes.Clientset, k8sNamespace, routesConfigMapName, configMapRequiredVersion string) ([]*v3.Route, error) {
	routesConfigMap, err := getVersionedConfigMap(clientSet, routesConfigMapName, k8sNamespace, configMapRequiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(routesConfigMapName)
		return nil, errors.Wrap(err, "error occurred while fetching the routes ConfigMap")
	}
	configMapDataMap, err := utils.Decompress(routesConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(routesConfigMapName)
		return nil, errors.Wrap(err, "error occurred while parsing the routes ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(routesConfigMapName)
	for _, yamlBytes := range configMapDataMap {
		var routesMap []map[string]interface{}
		if err := yaml.Unmarshal(yamlBytes, &routesMap); err != nil {
			stats.RecordConfigMapDataError(routesConfigMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the routes array")
		}
		var routesArray []*v3.Route
		for _, route := range routesMap {
			routeBytes, err := json.Marshal(route)
			if err != nil {
				stats.RecordConfigMapDataError(routesConfigMapName)
				return nil, errors.Wrap(err, "error occurred while marshaling the route data")
			}
			routePb, _ := utils.ConvertYamlToRouteProto(routeBytes)
			if routePb != nil && routePb.ValidateAll() == nil {
				routesArray = append(routesArray, routePb)
			}
		}
		stats.RecordConfigMapProcessedSuccessError(routesConfigMapName)
		stats.RecordConfigMapItems(routesConfigMapName, uint64(len(routesArray)))
		return routesArray, nil
	}
	return nil, err
}

/**
 * getVirtualClusters read the virtual clusters k8s ConfigMap and returns an array of v3.VirtualCluster.
 */
func getVirtualClusters(clientSet *kubernetes.Clientset, k8sNamespace, vcConfigMapName, configMapRequiredVersion string) ([]*v3.VirtualCluster, error) {
	vcConfigMap, err := getVersionedConfigMap(clientSet, vcConfigMapName, k8sNamespace, configMapRequiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(vcConfigMapName)
		return nil, errors.Wrap(err, "error occurred while fetching the virtual clusters ConfigMap")
	}
	configMapDataMap, err := utils.Decompress(vcConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(vcConfigMapName)
		return nil, errors.Wrap(err, "error occurred while parsing the virtual clusters ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(vcConfigMapName)
	for _, yamlBytes := range configMapDataMap {
		var vcMap []map[string]interface{}
		if err := yaml.Unmarshal(yamlBytes, &vcMap); err != nil {
			stats.RecordConfigMapDataError(vcConfigMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the virtual clusters array")
		}
		var virtualClustersArray []*v3.VirtualCluster
		for _, vc := range vcMap {
			vcBytes, err := json.Marshal(vc)
			if err != nil {
				stats.RecordConfigMapDataError(vcConfigMapName)
				return nil, errors.Wrap(err, "error occurred while marshaling the virtual cluster data")
			}
			virtualClusterPb, _ := utils.ConvertYamlToVirtualClusterProto(vcBytes)
			if virtualClusterPb != nil && virtualClusterPb.ValidateAll() == nil {
				virtualClustersArray = append(virtualClustersArray, virtualClusterPb)
			}
		}
		stats.RecordConfigMapProcessedSuccessError(vcConfigMapName)
		stats.RecordConfigMapItems(vcConfigMapName, uint64(len(virtualClustersArray)))
		return virtualClustersArray, nil
	}
	return nil, err
}

/**
 * doUpdate start the update process by parsing different ConfigMap(s), aggregating all the resources and update the
 * snapshot cache.
 */
func doUpdate(clientSet *kubernetes.Clientset, namespace string, svcImportOrderConfigMap *coreV1.ConfigMap) error {
	configMapVersion, versionLabelFound := svcImportOrderConfigMap.Labels["versionHash"]
	if !versionLabelFound {
		return errors.New("failed to get version label from the import order ConfigMap")
	}
	serviceNames, err := parseServiceImportOrderConfigMap(svcImportOrderConfigMap)
	if err != nil {
		return errors.Wrap(err, "failed to get service names from the import order")
	}
	// It can take a while to sync all the ConfigMap(s) and hence wait for some time before start
	// the aggregation.
	time.Sleep(time.Duration(settings.SyncDelayTimeSeconds) * time.Second)
	routeTables, err := getRouteConfigurations(clientSet, namespace, configMapVersion, serviceNames)
	if err != nil {
		return errors.Wrap(err, "failed to get route table(s)")
	}
	routesResource := make([]types.Resource, len(routeTables))
	for index, routeTable := range routeTables {
		routesResource[index] = routeTable
	}
	versionID := time.Now().Format("2006-01-02T15-04-05")
	logger.Debugf("creating new snapshot with version: %s", versionID)
	newSnapshot, err := cache.NewSnapshot(
		versionID,
		map[resourcesV3.Type][]types.Resource{
			resourcesV3.RouteType: routesResource,
		},
	)
	if err != nil {
		return errors.Wrap(err, "error occurred while updating the cache snapshot")
	}
	snapshotsVal.Store(*newSnapshot)
	logger.Infof("successfully updated snapshot with version: %s", versionID)
	return nil
}

/**
 * updateSnapshot read all ConfigMap(s) to create final route tables and update the cache snapshot when a new version of
 * the service order ConfigMap gets detected.
 */
func updateSnapshot(clientSet *kubernetes.Clientset, watchEventChannel <-chan watch.Event, mutex *sync.Mutex) {
	for {
		event, open := <-watchEventChannel
		if open {
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				logger.Debugf("*** Envoy Service Import Order ConfigMap Modified ***")
				mutex.Lock()
				namespace := settings.ConfigMapNamespace
				if configMap, ok := event.Object.(*coreV1.ConfigMap); ok {
					if err := doUpdate(clientSet, namespace, configMap); err != nil {
						logger.Errorf("error occurred while processing update", err.Error())
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
	var (
		mutex *sync.Mutex
	)
	mutex = &sync.Mutex{}
	go watchForChanges(clientSet, mutex)
}

/**
 * watchForChanges watcher implementation which would look at any state changes on the Envoy service import order
 * ConfigMap i.e. `envoy-svc-import-order-config` and invokes `updateCurrentConfigmap()` for every change.
 */
func watchForChanges(clientSet *kubernetes.Clientset, mutex *sync.Mutex) {
	for {
		namespace := settings.ConfigMapNamespace
		watcher, err := clientSet.CoreV1().ConfigMaps(namespace).Watch(context.TODO(), metaV1.ListOptions{FieldSelector: "metadata.name=" + settings.EnvoyServiceImportOrderConfigName})
		if err != nil {
			logger.Errorf("error occurred while creating the watcher", err.Error())
			panic(err.Error())
		}
		// Detect changes & update cache snapshot
		updateSnapshot(clientSet, watcher.ResultChan(), mutex)
	}
}

/**
 * setupSnapshotUpdater set up a new snapshot updater which would set the latest snapshot to all the client nodes.
 */
func setupSnapshotUpdater() {
	var (
		mutex *sync.Mutex
	)
	mutex = &sync.Mutex{}
	go updateSnapshotCache(mutex)
}

/**
 * updateSnapshotCache update the snapshot cache for the client nodes with the most recent snapshot.
 */
func updateSnapshotCache(mutex *sync.Mutex) {
	for {
		latestSnapshotEntry := snapshotsVal.Load()
		if latestSnapshotEntry != nil {
			latestSnapshot := latestSnapshotEntry.(cache.Snapshot)
			latestSnapshotVersion := latestSnapshot.GetVersion(resourcesV3.RouteType)
			mutex.Lock()
			nodesIdsSet := snapshotCache.GetStatusKeys()
			for _, nodeID := range nodesIdsSet {
				snapshot, err := snapshotCache.GetSnapshot(nodeID)
				if err != nil {
					logger.Debugf("unable to get the existing snapshot for nodeID: %s", nodeID, err.Error())
					setSnapshot(nodeID, latestSnapshotVersion, &latestSnapshot)
				} else if snapshot.GetVersion(resourcesV3.RouteType) != latestSnapshotVersion {
					setSnapshot(nodeID, latestSnapshotVersion, &latestSnapshot)
				}
			}
			mutex.Unlock()
		}
		// Add some delay
		time.Sleep(1 * time.Second)
	}
}

/**
 * setSnapshot set the snapshot for the given nodeID in the snapshots cache.
 */
func setSnapshot(nodeID, latestSnapshotVersion string, latestSnapshot *cache.Snapshot) {
	logger.Infof("start setting snapshot for nodeID: %s", nodeID)
	if err := snapshotCache.SetSnapshot(context.Background(), nodeID, latestSnapshot); err != nil {
		logger.Errorf("error occurred while updating the snapshot cache", err.Error())
		stats.RecordSnapshotCacheUpdateError(nodeID, latestSnapshotVersion)
	} else {
		stats.RecordSnapshotCacheUpdateSuccess(nodeID, latestSnapshotVersion)
		logger.Infof("successfully updated the snapshot cache for nodeID: %s", nodeID)
	}
}

/**
 * main bootstrap the RDS server and initializes the kubernetes client, sets up watcher etc.
 */
func main() {
	flag.Parse()
	logger.Infof("start initializing the main server...")

	// Initialize Kubernetes Client
	clientSet := initKubernetesClient()

	// Setup ConfigMap Watcher
	setupWatcher(clientSet)

	// Setup Cache Snapshot Updater
	setupSnapshotUpdater()

	// Run the RDS server
	logger.Infof("running RDS server...")
	ctx := context.Background()
	gRPCServer := server.NewServer(ctx, snapshotCache, nil)
	rdsServer.RunServer(&settings, gRPCServer, logger)
}
