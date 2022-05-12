package stats

import (
	"fmt"

	stats "github.com/lyft/gostats"
)

type RdsStatsByConfigMap struct {
	configMapFetchErrorCount       stats.Counter
	configMapParseErrorCount       stats.Counter
	configMapParseSuccessCount     stats.Counter
	configMapDataErrorCount        stats.Counter
	configMapProcessedSuccessCount stats.Counter
	configMapItemsCount            stats.Counter
}

type RdsStatsByRouteTable struct {
	// aggregatedRoutesCount consists of the total number of routes which are aggregated by the RDS from different
	// sources.
	aggregatedRoutesCount stats.Counter
	// aggregatedVirtualClustersCount count consists of the total number of virtual clusters which are aggregated by the
	// RDS from different sources.
	aggregatedVirtualClustersCount stats.Counter
	// totalRoutesCount consists of the total number of routes which are aggregated by the RDS from different sources
	// and the routes which are statically received from the configuration ConfigMap.
	// NOTE: totalRoutesCount >= aggregatedRoutesCount
	totalRoutesCount stats.Counter
	// totalVirtualClustersCount consists of the total number of virtual clusters which are aggregated by the RDS from
	// different sources and the virtual clusters which are statically received from the configuration ConfigMap.
	// NOTE: totalVirtualClustersCount >= aggregatedVirtualClustersCount
	totalVirtualClustersCount stats.Counter
	createErrorCount          stats.Counter
	createSuccessCount        stats.Counter
}

type RdsStatsBySnapshot struct {
	snapshotCacheUpdateSuccessCount stats.Counter
	snapshotCacheUpdateErrorCount   stats.Counter
}

var (
	countersByConfigMap  = make(map[string]RdsStatsByConfigMap)
	countersByRouteTable = make(map[string]RdsStatsByRouteTable)
	countersBySnapshot   = make(map[string]RdsStatsBySnapshot)
	serviceScope         = stats.NewDefaultStore().Scope("rds_service")

	makeCounterWithConfigMapNameTagFunc = func(counterName, configMapName string) stats.Counter {
		return serviceScope.NewCounterWithTags(counterName, map[string]string{"configMapName": configMapName})
	}
	makeCounterWithRouteTableNameTagFunc = func(counterName, routeTableName string) stats.Counter {
		return serviceScope.NewCounterWithTags(counterName, map[string]string{"routeTableName": routeTableName})
	}
	makeCounterWithNodeAndSnapshotTagsFunc = func(counterName, nodeID, snapshotVersion string) stats.Counter {
		return serviceScope.NewCounterWithTags(counterName, map[string]string{"nodeId": nodeID, "snapshotVersion": snapshotVersion})
	}
)

/**
 * getCountersByConfigMap returns the counters with tag including the ConfigMap name.
 */
func getCountersByConfigMap(configMapName string) RdsStatsByConfigMap {
	if _, exists := countersByConfigMap[configMapName]; !exists {

		countersByConfigMap[configMapName] = RdsStatsByConfigMap{
			configMapFetchErrorCount:       makeCounterWithConfigMapNameTagFunc("config_map_fetch_error_count", configMapName),
			configMapParseErrorCount:       makeCounterWithConfigMapNameTagFunc("config_map_parse_error_count", configMapName),
			configMapParseSuccessCount:     makeCounterWithConfigMapNameTagFunc("config_map_parse_success_count", configMapName),
			configMapDataErrorCount:        makeCounterWithConfigMapNameTagFunc("config_map_data_error_count", configMapName),
			configMapProcessedSuccessCount: makeCounterWithConfigMapNameTagFunc("config_map_processed_success_count", configMapName),
			configMapItemsCount:            makeCounterWithConfigMapNameTagFunc("config_map_items_count", configMapName),
		}
	}
	return countersByConfigMap[configMapName]
}

/**
 * getCountersByRouteTable returns the counters with tag including the route table name.
 */
func getCountersByRouteTable(routeTableName string) RdsStatsByRouteTable {
	if _, exists := countersByRouteTable[routeTableName]; !exists {
		countersByRouteTable[routeTableName] = RdsStatsByRouteTable{
			aggregatedRoutesCount:          makeCounterWithRouteTableNameTagFunc("route_table_aggregated_routes_count", routeTableName),
			aggregatedVirtualClustersCount: makeCounterWithRouteTableNameTagFunc("route_table_aggregated_vc_count", routeTableName),
			totalRoutesCount:               makeCounterWithRouteTableNameTagFunc("route_table_total_routes_count", routeTableName),
			totalVirtualClustersCount:      makeCounterWithRouteTableNameTagFunc("route_table_total_vc_count", routeTableName),
			createErrorCount:               makeCounterWithRouteTableNameTagFunc("route_table_create_error_count", routeTableName),
			createSuccessCount:             makeCounterWithRouteTableNameTagFunc("route_table_create_success_count", routeTableName),
		}
	}
	return countersByRouteTable[routeTableName]
}

/**
 * getCountersBySnapshot returns the counters with tags including the client node id and snapshot cache version.
 */
func getCountersBySnapshot(nodeID, snapshotVersion string) RdsStatsBySnapshot {
	key := fmt.Sprintf("%s-%s", nodeID, snapshotVersion)
	if _, exists := countersBySnapshot[key]; !exists {
		countersBySnapshot[key] = RdsStatsBySnapshot{
			snapshotCacheUpdateSuccessCount: makeCounterWithNodeAndSnapshotTagsFunc("snapshot_update_success_count", nodeID, snapshotVersion),
			snapshotCacheUpdateErrorCount:   makeCounterWithNodeAndSnapshotTagsFunc("snapshot_update_error_count", nodeID, snapshotVersion),
		}
	}
	return countersBySnapshot[key]
}

// RecordConfigMapFetchError records the number of times k8s failed to fetch a given ConfigMap.
func RecordConfigMapFetchError(configMapName string) {
	getCountersByConfigMap(configMapName).configMapFetchErrorCount.Inc()
}

// RecordConfigMapParseError records the number of times we failed to parse a given ConfigMap.
func RecordConfigMapParseError(configMapName string) {
	getCountersByConfigMap(configMapName).configMapParseErrorCount.Inc()
}

// RecordConfigMapParseSuccess records the number of times we successfully parsed the given ConfigMap.
func RecordConfigMapParseSuccess(configMapName string) {
	getCountersByConfigMap(configMapName).configMapParseSuccessCount.Inc()
}

// RecordConfigMapDataError records the number of times we were unable to unmarshall the contents of the given
// ConfigMap.
func RecordConfigMapDataError(configMapName string) {
	getCountersByConfigMap(configMapName).configMapDataErrorCount.Inc()
}

// RecordConfigMapProcessedSuccessError records the number of times we successfully processed the given ConfigMap
// end-to-end.
func RecordConfigMapProcessedSuccessError(configMapName string) {
	getCountersByConfigMap(configMapName).configMapProcessedSuccessCount.Inc()
}

// RecordConfigMapItems records the number of routes or virtual cluster resources extracted from the given ConfigMap.
func RecordConfigMapItems(configMapName string, count uint64) {
	getCountersByConfigMap(configMapName).configMapItemsCount.Add(count)
}

// RecordSnapshotCacheUpdateSuccess records the number of times RDS was able to successfully update the snapshot.
func RecordSnapshotCacheUpdateSuccess(nodeID, snapshotVersion string) {
	getCountersBySnapshot(nodeID, snapshotVersion).snapshotCacheUpdateSuccessCount.Inc()
}

// RecordSnapshotCacheUpdateError records the number of times RDS was able to failed to update the snapshot cache.
func RecordSnapshotCacheUpdateError(nodeID, snapshotVersion string) {
	getCountersBySnapshot(nodeID, snapshotVersion).snapshotCacheUpdateErrorCount.Inc()
}

// RecordRouteTableAggregatedRoutes records the number of aggregated routes in the given Route Table.
func RecordRouteTableAggregatedRoutes(routeTableName string, count uint64) {
	getCountersByRouteTable(routeTableName).aggregatedRoutesCount.Add(count)
}

// RecordRouteTableAggregatedVirtualClusters records the number of aggregated VCs in the given Route Table.
func RecordRouteTableAggregatedVirtualClusters(routeTableName string, count uint64) {
	getCountersByRouteTable(routeTableName).aggregatedVirtualClustersCount.Add(count)
}

// RecordRouteTableTotalRoutes records the number of total routes in the given Route Table after adding new routes.
func RecordRouteTableTotalRoutes(routeTableName string, count uint64) {
	getCountersByRouteTable(routeTableName).totalRoutesCount.Add(count)
}

// RecordRouteTableTotalVirtualClusters records the number of total VCs in the given Route Table after adding new VCs.
func RecordRouteTableTotalVirtualClusters(routeTableName string, count uint64) {
	getCountersByRouteTable(routeTableName).totalVirtualClustersCount.Add(count)
}

// RecordRouteTableCreateSuccess records the times the creation of given Route Table succeeded.
func RecordRouteTableCreateSuccess(routeTableName string) {
	getCountersByRouteTable(routeTableName).createSuccessCount.Inc()
}

// RecordRouteTableCreateError records the times the creation of given Route Table failed.
func RecordRouteTableCreateError(routeTableName string) {
	getCountersByRouteTable(routeTableName).createErrorCount.Inc()
}
