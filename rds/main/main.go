package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"strings"
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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

/**
 * This file has the main logic to watch for the Envoy RouteConfiguration ConfigMap(s) and aggregate the Envoy route
 * and virtual cluster definitions to create final route tables. These route table resources are consumed by the client
 * Envoy.
 *
 * There are two main threads/go-routines:
 * 1. Kubernetes Watcher: k8s watcher watches for any updates on the ConfigMap envoy-svc-import-order-config and invokes
 *                        updateCurrentConfigmap() for every add/change event. updateCurrentConfigmap() aggregates all
 *                        other routes and virtual clusters ConfigMap(s) to create a snapshot and updates snapshotVal.
 * 2. Snapshot Updater: snapshot updater reads the snapshotVal and updates snapshotCache for all the client (Envoy)
 *                      nodes. This happens in a separate thread because snapshotCache has 1:1 mapping between nodeId
 *                      to the snapshot, and it's possible to get requests from the client (Envoys) even after the k8s
 *                      watcher thread finishes updating the snapshotVal.
 *
 * We don't need any mutex to sync in either of the threads as snapshotVal is the only shared variable, and it's already
 * atomic. The k8s watcher would process the incoming events on the channel for addition/modification of the import
 * order ConfigMap one-by-one and hence we don't need to lock/unlock when we write to snapshotVal.
 */

// SnapshotCache stores the snapshot for the route resources against a specific Envoy NodeID. Different nodes can
// technically have different versions of snapshots but, in our case they are all same. It's possible for a new Envoy
// replica to get added after the latest snapshot gets generated and hence we update SnapshotCache separately using
// the value from the snapshotVal which points to the latest snapshot.
type SnapshotCache struct {
	snapshotCache cache.SnapshotCache
}

// K8s is used to make API calls to the K8S API Server for fetching ConfigMap(s) etc.
type K8s struct {
	ClientSet kubernetes.Interface
}

// ClientMetadata holds the metadata for a single client (Envoy).
type ClientMetadata struct {
	snapshotVersion      string
	inCanary             bool
	lastUpdatedTimestamp int64
}

var (
	logger utils.Logger
	// snapshotVal stores the latest snapshot generated from the latest version of route tables which are created by
	// aggregating all the routes and virtual clusters from different ConfigMap(s). Upon detecting a new version of the
	// service import order ConfigMap, k8s watcher triggers the update function which then reads all other ConfigMap(s)
	// to create new route tables, generate a new snapshot, and update this value.
	snapshotVal atomic.Value
	// settings contains the config flag values which are either the declared default or the values set by using the
	// environment variables.
	settings       env.Settings
	configMapCache *utils.ConfigMapCache
	// lastFetchedVersion is the latest version which got fetched after reading all the ConfigMap(s). This may or may
	// not be the LKG version depending on if we are still actively canary-ing the configs or not. Upon finishing
	// canary, LKG version would be updated to this version.
	lastFetchedVersion string
	// Map to store the canary status of each client connected to this RDS.
	canaryStatusMap map[string]ClientMetadata
)

// Initialize Variables
func init() {
	// Initialize Settings Object
	settings = env.NewSettings()

	// Initialize Logger
	logger = utils.Logger{
		Debug: strings.EqualFold("DEBUG", settings.LogLevel),
		Info:  strings.EqualFold("DEBUG", settings.LogLevel) || strings.EqualFold("INFO", settings.LogLevel),
	}

	if s, err := json.Marshal(&settings); err == nil {
		logger.Infof("settings: %s", string(s))
	}

	// Initialize Snapshot
	snapshotVal = atomic.Value{}
	configMapCache = utils.NewConfigMapCache()
	lastFetchedVersion = ""
	canaryStatusMap = make(map[string]ClientMetadata)
}

// ParseServiceImportOrderConfigMap parses the service import order ConfigMap and return an array of service names which
// is used while aggregating the per-service routes and virtual clusters.
//
// TODO: This method has been made public for testing purposes and should be refactored into a new/existing utils
// package to facilitate testing.
func ParseServiceImportOrderConfigMap(compressedConfigMap *coreV1.ConfigMap) ([]string, error) {
	configMapName := compressedConfigMap.Name
	logger.Infof("service import order ConfigMap name: %s", configMapName)
	configMap, err := utils.Decompress(compressedConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(configMapName)
		return nil, errors.Wrap(err, "error occurred while parsing service import order ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(configMapName)
	var serviceNames []string
	for _, yamlBytes := range configMap {
		if err := yaml.Unmarshal(yamlBytes, &serviceNames); err != nil {
			stats.RecordConfigMapDataError(configMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the service import order ConfigMap")
		}
		stats.RecordConfigMapProcessedSuccessError(configMapName)
	}
	return serviceNames, nil
}

/**
 * getRoutesImportOrder parses the route import order ConfigMap to get the routes & virtual clusters import order which
 * is used to aggregate the routes & virtual clusters in a specific order while creating the route tables. This and all
 * other ConfigMap(s) are defined in the universe, and they are published to the namespace where this service is running
 * by a Spinnaker Pipeline for syncing route-discovery-service-config:
 * `servicemesh-control/route-discovery-service/deploy/rds-envoy-configmaps.jsonnet.TEMPLATE`
 *
 * It returns a map of the route table name to its import order details:
 * {
 *   "extauthz-routes": { "routes": [<envoy-route>], "virtualClusters": [<envoy-virtual-cluster>] },
 *   "https-routes": { "routes": [<envoy-route>], "virtualClusters": [<envoy-virtual-cluster>] },
 *   ...
 * }
 */
func (k K8s) getRoutesImportOrder(k8sNamespace, requiredVersion string) (map[string]map[string][]string, error) {
	configMapName := settings.EnvoyRoutesImportOrderConfigName
	compressedConfigMap, err := k.getConfigMap(configMapName, k8sNamespace, requiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(configMapName)
		return nil, errors.Wrap(err, "error occurred while fetching routes import order ConfigMap")
	}
	configMap, err := utils.Decompress(compressedConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(configMapName)
		return nil, errors.Wrap(err, "error occurred while parsing routes import order ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(configMapName)
	routesImportOrderMap := make(map[string]map[string][]string)
	for _, yamlBytes := range configMap {
		if err := yaml.Unmarshal(yamlBytes, &routesImportOrderMap); err != nil {
			stats.RecordConfigMapDataError(configMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling routes import order ConfigMap")
		}
		stats.RecordConfigMapProcessedSuccessError(configMapName)
	}
	return routesImportOrderMap, nil
}

/**
 * getConfigMap gets the ConfigMap and compare the version hash on it with the master version retrieved from the
 * service import order ConfigMap.
 *
 * The idea is to have all the ConfigMap(s) with the same hash that gets added by the sjsonnet binary. We read the
 * version hash from the first ConfigMap i.e. the service import order ConfigMap and then compare that version hash with
 * all other ConfigMap(s) we read to create the route tables. If there is a hash/version mismatch then we wait for some
 * time hoping that there is an ongoing sync, and we would get the expected version once the sync finishes. If we don't
 * get the correct/required version hash even after waiting then we give up and return an error.
 */
func (k K8s) getConfigMap(configMapName, k8sNamespace, requiredVersion string, shouldWaitForSync bool) (*coreV1.ConfigMap, error) {
	configMap, cacheHit := configMapCache.Get(configMapName, requiredVersion)
	if configMap == nil {
		cm, err := k.ClientSet.CoreV1().ConfigMaps(k8sNamespace).Get(context.TODO(), configMapName, metaV1.GetOptions{})

		if err != nil {
			stats.RecordConfigMapFetchError(configMapName)
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetching ConfigMap: %s", configMapName))
		}
		configMap = cm
	}

	// This version hash is computed by the Scala JSONNET (sjsonnet) binary and is added to all the RDS ConfigMap(s)
	// i.e. Routes and Service Configurations, Import Orders, etc.
	configMapVersion, versionLabelFound := configMap.Labels["versionHash"]
	if !versionLabelFound {
		return nil, fmt.Errorf("failed to get version label from the ConfigMap: %s", configMapName)
	}
	// If the version hash retrieved from the ConfigMap is not same as the one we got from the service import order map
	// then, we'll wait for some time and see if an update is pending.
	if configMapVersion != requiredVersion {
		if shouldWaitForSync {
			time.Sleep(time.Duration(settings.SyncDelayTimeSeconds) * time.Second)
			return k.getConfigMap(configMapName, k8sNamespace, requiredVersion, false)
		}
		return nil, fmt.Errorf("version hash mismatch on the ConfigMap: %s. Required: %s, Received: %s", configMapName, requiredVersion, configMapVersion)
	}
	// Only put new config in cache after verifying it's the required version.
	if !cacheHit {
		configMapCache.Put(configMapName, requiredVersion, configMap)
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
func (k K8s) getRouteConfigurations(k8sNamespace, requiredVersion string, serviceNames []string) ([]*v3.RouteConfiguration, error) {
	configMapName := settings.EnvoyRouteConfigurationsConfigName
	compressedConfigMap, err := k.getConfigMap(configMapName, k8sNamespace, requiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(configMapName)
		return nil, errors.Wrap(err, "error occurred while fetching routes configuration ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(configMapName)
	configMap, err := utils.Decompress(compressedConfigMap)
	if err != nil {
		stats.RecordConfigMapDataError(configMapName)
		return nil, errors.Wrap(err, "error occurred while parsing routes configuration ConfigMap")
	}

	// Get routes import order
	routesImportOrderMap, err := k.getRoutesImportOrder(k8sNamespace, requiredVersion)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while getting routes import order")
	}

	// Read all the route table configs
	var routeTables []*v3.RouteConfiguration
	for name, yamlBytes := range configMap {
		logger.Infof("start creating route table '%s' with version hash '%s'", name, requiredVersion)
		routeTablePb, err := utils.ConvertYamlToRouteConfigurationProto(yamlBytes)
		if err != nil {
			stats.RecordConfigMapDataError(configMapName)
			return nil, errors.Wrap(err, "error occurred while creating v3.RouteConfiguration proto from the ConfigMap data")
		}
		// Append Routes
		aggregatedRoutes, err := k.aggregateRoutes(k8sNamespace, requiredVersion, serviceNames, routesImportOrderMap[name]["routes"])
		if err != nil {
			stats.RecordRouteTableCreateError(name)
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while aggregating the routes for route table: %s", name))
		}
		stats.RecordRouteTableAggregatedRoutes(name, uint64(len(aggregatedRoutes)))
		routeTablePb.VirtualHosts[0].Routes = append(routeTablePb.VirtualHosts[0].Routes, aggregatedRoutes...)
		stats.RecordRouteTableTotalRoutes(name, uint64(len(routeTablePb.VirtualHosts[0].Routes)))
		// Append Virtual Clusters
		aggregatedVirtualClusters, err := k.aggregateVirtualClusters(k8sNamespace, requiredVersion, serviceNames, routesImportOrderMap[name]["virtualClusters"])
		if err != nil {
			stats.RecordRouteTableCreateError(name)
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while aggregating the virtual clusters for route table: %s", name))
		}
		stats.RecordRouteTableAggregatedVirtualClusters(name, uint64(len(aggregatedVirtualClusters)))
		routeTablePb.VirtualHosts[0].VirtualClusters = append(routeTablePb.VirtualHosts[0].VirtualClusters, aggregatedVirtualClusters...)
		stats.RecordRouteTableTotalVirtualClusters(name, uint64(len(routeTablePb.VirtualHosts[0].VirtualClusters)))
		logger.Infof("finished creating the route table: %s", name)
		stats.RecordRouteTableCreateSuccess(name)
		routeTables = append(routeTables, routeTablePb)
	}
	stats.RecordConfigMapProcessedSuccessError(configMapName)
	return routeTables, nil
}

/**
 * aggregateRoutes aggregate all the routes following the routes import order.
 */
func (k K8s) aggregateRoutes(k8sNamespace, requiredVersion string, serviceNames, routesImportOrder []string) (aggregatedRoutes []*v3.Route, err error) {
	for _, importConfigName := range routesImportOrder {
		routes, err := k.parseRoutesConfigMap(k8sNamespace, requiredVersion, importConfigName, serviceNames)
		if err != nil {
			return nil, errors.Wrap(err, "error occurred while parsing routes ConfigMap(s)")
		}
		aggregatedRoutes = append(aggregatedRoutes, routes...)
	}
	return aggregatedRoutes, nil
}

/**
 * aggregateVirtualClusters aggregate all the virtual clusters following the vc import order.
 */
func (k K8s) aggregateVirtualClusters(k8sNamespace, requiredVersion string, serviceNames, vcImportOrder []string) (aggregatedVirtualClusters []*v3.VirtualCluster, err error) {
	for _, importConfigName := range vcImportOrder {
		virtualClusters, err := k.parseVirtualClustersConfigMap(k8sNamespace, requiredVersion, importConfigName, serviceNames)
		if err != nil {
			return nil, errors.Wrap(err, "error occurred while parsing virtual clusters ConfigMap(s)")
		}
		aggregatedVirtualClusters = append(aggregatedVirtualClusters, virtualClusters...)
	}
	return aggregatedVirtualClusters, nil
}

/**
 * getSvcRoutePrefix returns a service route prefix based on the passed config name. This prefix would be used
 *                   to fetch per-service ConfigMap(s) for routes. For example,
 *                   prefix = envoy-main-route-
 *                   configMapName = envoy-main-route-authN, envoy-main-route-mlflow, envoy-main-route-jobs, ...
 */
func getSvcRoutePrefix(configName string) (prefix string, ok bool) {
	serviceRoutesSuffix := "-svc-routes"
	serviceVirtualClusterSuffix := "-vc-svc-routes"
	// service route and virtual cluster route share the same suffix, so need to verify both.
	if strings.Contains(configName, serviceRoutesSuffix) && !strings.Contains(configName, serviceVirtualClusterSuffix) {
		return strings.Split(configName, serviceRoutesSuffix)[0], true
	}
	return "", false
}

/**
 * parseRoutesConfigMap read all the routes k8s ConfigMap(s) and returns an aggregated slice of v3.Route.
 */
func (k K8s) parseRoutesConfigMap(k8sNamespace, requiredVersion, importConfigName string, serviceNames []string) (routes []*v3.Route, err error) {
	svcRoutePrefix, ok := getSvcRoutePrefix(importConfigName)
	if ok {
		// Service Routes
		for _, serviceName := range serviceNames {
			configMapName := fmt.Sprintf("%s-%s", svcRoutePrefix, serviceName)
			serviceRoutes, err := k.getRoutes(k8sNamespace, configMapName, requiredVersion)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetchig the route ConfigMap: '%s'", configMapName))
			} else if len(serviceRoutes) > 0 {
				routes = append(routes, serviceRoutes...)
			} else {
				logger.Infof(fmt.Sprintf("no routes defined in: '%s'", configMapName))
			}
		}
		logger.Infof("total number of routes found for '%s': %d", importConfigName, len(routes))
	} else {
		routes, err = k.getRoutes(k8sNamespace, importConfigName, requiredVersion)
		if err != nil {
			return nil, err
		}
	}
	return routes, nil
}

/**
 * getVcRoutePrefix returns a service virtual cluster prefix based on the passed config name. This prefix would be used
 *                  to fetch per-service ConfigMap(s) for virtual clusters. For example,
 *                  prefix = envoy-main-vc-
 *                  configMapName = envoy-main-vc-authN, envoy-main-vc-cluster-manager, envoy-main-vc-jobs, ...
 */
func getVcRoutePrefix(configName string) (prefix string, ok bool) {
	serviceRoutesSuffix := "-svc-routes"
	serviceVirtualClusterSuffix := "-vc-svc-routes"
	if strings.Contains(configName, serviceVirtualClusterSuffix) {
		return strings.Split(configName, serviceRoutesSuffix)[0], true
	}
	return "", false
}

/**
 * parseVirtualClustersConfigMap read all the virtual clusters k8s ConfigMap(s) and returns an aggregated slice of
 * v3.VirtualCluster.
 */
func (k K8s) parseVirtualClustersConfigMap(k8sNamespace, requiredVersion, importConfigName string, serviceNames []string) (virtualClusters []*v3.VirtualCluster, err error) {
	vcRoutePrefix, ok := getVcRoutePrefix(importConfigName)
	if ok {
		// Service Virtual Clusters
		for _, serviceName := range serviceNames {
			configMapName := fmt.Sprintf("%s-%s", vcRoutePrefix, serviceName)
			serviceVirtualClusters, err := k.getVirtualClusters(k8sNamespace, configMapName, requiredVersion)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetchig the virtual cluster ConfigMap: '%s'", configMapName))
			} else if len(serviceVirtualClusters) > 0 {
				virtualClusters = append(virtualClusters, serviceVirtualClusters...)
			} else {
				logger.Infof(fmt.Sprintf("no virtual clusters defined in: '%s'", configMapName))
			}
		}
		logger.Infof("total number of virtual clusters found for '%s': %d", importConfigName, len(virtualClusters))
	} else {
		virtualClusters, err = k.getVirtualClusters(k8sNamespace, importConfigName, requiredVersion)
		if err != nil {
			return nil, err
		}
	}
	return virtualClusters, nil
}

/**
 * getRoutes read the routes k8s ConfigMap(s) and returns a slice of v3.Route.
 */
func (k K8s) getRoutes(k8sNamespace, configMapName, requiredVersion string) ([]*v3.Route, error) {
	compressedConfigMap, err := k.getConfigMap(configMapName, k8sNamespace, requiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(configMapName)
		return nil, errors.Wrap(err, "error occurred while fetching the routes ConfigMap")
	}
	configMap, err := utils.Decompress(compressedConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(configMapName)
		return nil, errors.Wrap(err, "error occurred while parsing the routes ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(configMapName)
	for _, yamlBytes := range configMap {
		var routesMap []map[string]interface{}
		if err := yaml.Unmarshal(yamlBytes, &routesMap); err != nil {
			stats.RecordConfigMapDataError(configMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the routes array")
		}
		var routes []*v3.Route
		for _, route := range routesMap {
			routeBytes, err := json.Marshal(route)
			if err != nil {
				stats.RecordConfigMapDataError(configMapName)
				return nil, errors.Wrap(err, "error occurred while marshaling the route data")
			}
			routePb, _ := utils.ConvertYamlToRouteProto(routeBytes)
			if routePb != nil && routePb.ValidateAll() == nil {
				routes = append(routes, routePb)
			}
		}
		stats.RecordConfigMapProcessedSuccessError(configMapName)
		stats.RecordConfigMapItems(configMapName, uint64(len(routes)))
		return routes, nil
	}
	return nil, err
}

/**
 * getVirtualClusters read the virtual clusters k8s ConfigMap(s) and returns a slice of v3.VirtualCluster.
 */
func (k K8s) getVirtualClusters(k8sNamespace, configMapName, requiredVersion string) ([]*v3.VirtualCluster, error) {
	compressedConfigMap, err := k.getConfigMap(configMapName, k8sNamespace, requiredVersion, true)
	if err != nil {
		stats.RecordConfigMapFetchError(configMapName)
		return nil, errors.Wrap(err, "error occurred while fetching the virtual clusters ConfigMap")
	}
	configMap, err := utils.Decompress(compressedConfigMap)
	if err != nil {
		stats.RecordConfigMapParseError(configMapName)
		return nil, errors.Wrap(err, "error occurred while parsing the virtual clusters ConfigMap")
	}
	stats.RecordConfigMapParseSuccess(configMapName)
	for _, yamlBytes := range configMap {
		var vcMap []map[string]interface{}
		if err := yaml.Unmarshal(yamlBytes, &vcMap); err != nil {
			stats.RecordConfigMapDataError(configMapName)
			return nil, errors.Wrap(err, "error occurred while unmarshalling the virtual clusters array")
		}
		var virtualClusters []*v3.VirtualCluster
		for _, vc := range vcMap {
			vcBytes, err := json.Marshal(vc)
			if err != nil {
				stats.RecordConfigMapDataError(configMapName)
				return nil, errors.Wrap(err, "error occurred while marshaling the virtual cluster data")
			}
			virtualClusterPb, _ := utils.ConvertYamlToVirtualClusterProto(vcBytes)
			if virtualClusterPb != nil && virtualClusterPb.ValidateAll() == nil {
				virtualClusters = append(virtualClusters, virtualClusterPb)
			}
		}
		stats.RecordConfigMapProcessedSuccessError(configMapName)
		stats.RecordConfigMapItems(configMapName, uint64(len(virtualClusters)))
		return virtualClusters, nil
	}
	return nil, err
}

/**
 * doUpdate starts the update process by parsing different ConfigMap(s), aggregating all the ConfigMap(s) and update the
 * Snapshot.
 */
func (k K8s) doUpdate(namespace string, svcImportOrderConfigMap *coreV1.ConfigMap) error {
	configMapVersion, versionLabelFound := svcImportOrderConfigMap.Labels["versionHash"]
	if !versionLabelFound {
		return errors.New("failed to get version label from the import order ConfigMap")
	}
	if configMapVersion == lastFetchedVersion {
		logger.Infof("latest version is the same as last fetched version %s. skipping update.", lastFetchedVersion)
		return nil
	}
	logger.Infof("Loaded new version %v", configMapVersion)
	serviceNames, err := ParseServiceImportOrderConfigMap(svcImportOrderConfigMap)
	if err != nil {
		return errors.Wrap(err, "failed to get service names from the import order")
	}
	// It can take a while to sync all the ConfigMap(s) and hence wait for some time before start
	// the aggregation.
	time.Sleep(time.Duration(settings.SyncDelayTimeSeconds) * time.Second)
	routeTables, err := k.getRouteConfigurations(namespace, configMapVersion, serviceNames)
	if err != nil {
		return errors.Wrap(err, "failed to get route table(s)")
	}
	routesResource := make([]types.Resource, len(routeTables))
	for index, routeTable := range routeTables {
		routesResource[index] = routeTable
	}
	logger.Infof("creating new snapshot with version: %s", configMapVersion)
	newSnapshot, err := cache.NewSnapshot(
		configMapVersion,
		map[resourcesV3.Type][]types.Resource{
			resourcesV3.RouteType: routesResource,
		},
	)
	if err != nil {
		return errors.Wrap(err, "error occurred while updating the cache snapshot")
	}
	snapshotVal.Store(*newSnapshot)
	lastFetchedVersion = configMapVersion
	logger.Infof("successfully updated snapshot with version: %s", configMapVersion)
	return nil
}

/**
 * getKubernetesClientConfig creates and returns a new kubernetes client configuration.
 */
func getKubernetesClientConfig() *rest.Config {
	// Create In-Cluster Config
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("error occurred while creating in-cluster config", err.Error())
		panic(err.Error())
	}

	// Current QPS & Burst are too low which results in the client-side throttling and hence we are increasing these
	// limits to see if it has any positive impact on the performance while fetching the ConfigMap(s) in parallel.
	inClusterConfig.QPS = settings.ClientQPS
	inClusterConfig.Burst = settings.ClientBurst

	return inClusterConfig
}

/**
 * initKubernetesClient initializes the kubernetes client which is used to fetch the ConfigMap(s).
 */
func initKubernetesClient() K8s {
	logger.Infof("start initializing the kubernetes client...")

	// Create ClientSet
	clientSet, err := kubernetes.NewForConfig(getKubernetesClientConfig())
	if err != nil {
		logger.Errorf("error occurred while creating the new kubernetes client-set", err.Error())
		panic(err.Error())
	}
	client := K8s{
		ClientSet: clientSet,
	}

	// Sanity Check(s)
	namespace := settings.ConfigMapNamespace
	timeoutSeconds := int64(10)
	if _, err = client.ClientSet.CoreV1().ConfigMaps(namespace).List(context.TODO(), metaV1.ListOptions{Limit: 1, TimeoutSeconds: &timeoutSeconds}); err != nil {
		logger.Errorf("error occurred while listing ConfigMap(s). Please check k8s permissions", err.Error())
		panic(err.Error())
	}

	// Return Client
	return client
}

/**
 * setupWatcher sets up a new watcher which would look at the ConfigMap state changes and update the snapshot cache by
 * pulling and aggregating the data from different ConfigMap(s) on changes.
 */
func (k K8s) setupWatcher() {
	duration, err := time.ParseDuration(settings.ConfigMapPollInterval)
	if err != nil {
		logger.Errorf("error occurred while parsing the poller poll interval", err.Error())
		panic(err.Error())
	}
	go k.watchForChanges(time.NewTicker(duration))
}

/**
 * watchForChanges looks at changes on the Envoy ConfigMap envoy-svc-import-order-config and invokes
 * updateCurrentConfigmap() for every change.
 *
 * Service import order ConfigMap is the first ConfigMap that we read. Update function reads all other ConfigMap(s)
 * i.e., route configurations, route import orders, etc., to aggregate all the routes & virtual clusters and to create
 * the route tables. All the ConfigMap(s) are expected to have the same version hash which is added by the sjsonnet
 * binary and hence we ignore the update if any of the ConfigMap doesn't have the same version hash.
 */
func (k K8s) watchForChanges(ticker *time.Ticker) {
	defer ticker.Stop()
	namespace := settings.ConfigMapNamespace
	v1ConfigMap := k.ClientSet.CoreV1().ConfigMaps(namespace)
	for {
		select {
		case <-ticker.C:
			cm, err := v1ConfigMap.Get(context.TODO(), settings.EnvoyServiceImportOrderConfigName, metaV1.GetOptions{})
			if err != nil {
				logger.Errorf("error occurred while creating the watcher", err.Error())
				panic(err.Error())
			}
			if err = k.doUpdate(namespace, cm); err != nil {
				logger.Errorf("error occurred while processing update", err.Error())
			}
		}
	}
}

/**
 * setupSnapshotUpdater sets up a new snapshot updater which would set the latest snapshot to all the client nodes in
 * the SnapshotCache.
 */
func setupSnapshotUpdater(sc *SnapshotCache) {
	go sc.updateSnapshotCache()
}

/**
 * updateSnapshotCache periodically updates the SnapshotCache, which the go-control-plane would deliver to the
 * connected watchers.
 */
func (sc *SnapshotCache) updateSnapshotCache() {
	for {
		latestSnapshotEntry := snapshotVal.Load()
		if latestSnapshotEntry == nil {
			continue
		}
		latestSnapshot := latestSnapshotEntry.(cache.Snapshot)
		latestSnapshotVersion := latestSnapshot.GetVersion(resourcesV3.RouteType)
		// Start performing canary rollout to update the clients with the latest snapshot gradually
		sc.doCanary(latestSnapshotVersion, &latestSnapshot)
		time.Sleep(1 * time.Second)
	}
}

/**
 * doCanary kicks off the canary process by getting the IDs of the connected clients and then checking the status of all
 * clients one-by-one.
 */
func (sc *SnapshotCache) doCanary(latestSnapshotVersion string, latestSnapshot *cache.Snapshot) {
	nodeIDs := sc.snapshotCache.GetStatusKeys()
	sort.Strings(nodeIDs)

	for _, nodeID := range nodeIDs {
		currentSnapshot, err := sc.snapshotCache.GetSnapshot(nodeID)
		if err != nil || currentSnapshot == nil {
			logger.Infof("unable to get the existing snapshot for client: '%s'", nodeID)
			updateCanaryStatusForClient(nodeID, false, time.Now().UnixMilli(), latestSnapshotVersion)
			sc.setSnapshot(nodeID, latestSnapshotVersion, latestSnapshot)
			continue
		}

		if inCanary(nodeID) && isCanaryDone(nodeID) {
			logger.Infof("finished canary rollout to client: '%s' with version: '%s'", nodeID, latestSnapshotVersion)
			updateCanaryStatusForClient(nodeID, false, time.Now().UnixMilli(), latestSnapshotVersion)
		} else if shouldCanary(latestSnapshotVersion, nodeID, nodeIDs) {
			logger.Infof("started canary rollout to client: '%s' with version: '%s'", nodeID, latestSnapshotVersion)
			updateCanaryStatusForClient(nodeID, true, time.Now().UnixMilli(), latestSnapshotVersion)
			sc.setSnapshot(nodeID, latestSnapshotVersion, latestSnapshot)
		}
	}

	// Check whether we are still actively canary-ing or not. Upon finishing we update the remaining clients to the
	// latest snapshot version.
	if isCanaryFinished(nodeIDs) {
		logger.Infof("finished delivery to all the clients in the canary set. updating remaining clients to version: %s", latestSnapshotVersion)

		for _, nodeID := range nodeIDs {
			currentSnapshot, err := sc.snapshotCache.GetSnapshot(nodeID)
			if err != nil || currentSnapshot == nil {
				logger.Infof("unable to get the existing snapshot for client: '%s'", nodeID)
				updateCanaryStatusForClient(nodeID, false, time.Now().UnixMilli(), latestSnapshotVersion)
				sc.setSnapshot(nodeID, latestSnapshotVersion, latestSnapshot)
				continue
			}

			if latestSnapshotVersion != currentSnapshot.GetVersion(resourcesV3.RouteType) {
				updateCanaryStatusForClient(nodeID, false, time.Now().UnixMilli(), latestSnapshotVersion)
				sc.setSnapshot(nodeID, latestSnapshotVersion, latestSnapshot)
			}
		}
		logger.Infof("all clients are updated to: %s", latestSnapshotVersion)
	}
}

/**
 * shouldCanary makes a decision on whether to start canary-ing for a given client or not based on how many clients are
 * in active canary phase.
 */
func shouldCanary(latestSnapshotVersion, nodeID string, nodeIDs []string) bool {
	// If the client is already on the latest version then we don't have to canary.
	if canaryStatusMap[nodeID].snapshotVersion == latestSnapshotVersion {
		return false
	}

	canaryCount := 0
	// Get the count of the clients which are currently in canary
	for _, node := range nodeIDs {
		clientMetadata, _ := canaryStatusMap[node]
		// If the client is currently canary-ing or finished canary-ing then we count it as we want the total X percent
		// of the clients to be on the new canary version before we can send the config update to other connected
		// clients.
		if clientMetadata.inCanary || clientMetadata.snapshotVersion == latestSnapshotVersion {
			canaryCount = canaryCount + 1
		}
	}

	// We should canary if less than 30% of the total clients are in canary, otherwise we should not canary
	return float32(canaryCount)/float32(len(nodeIDs)) <= settings.ConfigCanaryRatio
}

/**
 * updateCanaryStatusForClient updates the map which holds the client -> canary status mapping.
 */
func updateCanaryStatusForClient(clientID string, inCanary bool, timestamp int64, snapshotVersion string) {
	canaryStatusMap[clientID] = ClientMetadata{
		lastUpdatedTimestamp: timestamp,
		inCanary:             inCanary,
		snapshotVersion:      snapshotVersion,
	}
}

/**
 * inCanary returns the current canary status for a given client.
 */
func inCanary(clientID string) bool {
	return canaryStatusMap[clientID].inCanary
}

/**
 * isCanaryDone checks whether to canary-ing process for a given client is finished or not based on how much time has
 * been elapsed since we started the canary rollout.
 */
func isCanaryDone(clientID string) bool {
	clientMetadata, _ := canaryStatusMap[clientID]
	currentTimestamp := time.Now().UnixMilli()
	return (currentTimestamp - clientMetadata.lastUpdatedTimestamp) > settings.ConfigCanaryTimeInMilliseconds
}

/**
 * isCanaryFinished checks the active canary status of all the connected clients and returns true only if none of the
 * clients are in the active canary rollout phase.
 */
func isCanaryFinished(nodeIDs []string) bool {
	for _, nodeID := range nodeIDs {
		clientMetadata, _ := canaryStatusMap[nodeID]
		if clientMetadata.inCanary {
			return false
		}
	}
	return true
}

/**
 * setSnapshot sets the snapshot for the given nodeID in the SnapshotCache.
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
 * main bootstraps the RDS server and sets up all the dependencies like k8s watcher, snapshot updater etc.
 */
func main() {
	flag.Parse()
	logger.Infof("start initializing the main server...")

	// Initialize Kubernetes Client
	client := initKubernetesClient()

	// Setup ConfigMap Watcher
	client.setupWatcher()

	// Setup Cache Snapshot Updater
	sc := SnapshotCache{
		snapshotCache: cache.NewSnapshotCache(false, cache.IDHash{}, logger),
	}
	setupSnapshotUpdater(&sc)

	// Run the RDS server
	logger.Infof("running RDS server...")
	ctx := context.Background()
	gRPCServer := server.NewServer(ctx, sc.snapshotCache, nil)
	rdsServer.RunServer(&settings, gRPCServer, logger)
}
