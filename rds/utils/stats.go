package utils

import (
	"fmt"

	stats "github.com/lyft/gostats"
)

type PerConfigMapRdsStats struct {
	configMapFetchErrorCount       stats.Counter
	configMapParseErrorCount       stats.Counter
	configMapParseSuccessCount     stats.Counter
	configMapDataErrorCount        stats.Counter
	configMapProcessedSuccessCount stats.Counter
	configMapValuesCount           stats.Counter
}

type PerRouteTableRdsStats struct {
	aggregatedRoutesCount          stats.Counter
	aggregatedVirtualClustersCount stats.Counter
	totalRoutesCount               stats.Counter
	totalVirtualClustersCount      stats.Counter
	createErrorCount               stats.Counter
	createSuccessCount             stats.Counter
}

type PerNodeRdsStats struct {
	snapshotCacheUpdateSuccessCount stats.Counter
	snapshotCacheUpdateErrorCount   stats.Counter
}

var (
	perConfigMapCounters  = make(map[string]interface{})
	perRouteTableCounters = make(map[string]interface{})
	perNodeCounters       = make(map[string]interface{})
	statsStore            = stats.NewDefaultStore()
	serviceScope          = statsStore.Scope("rds_service")
)

/**
 * Returns the counters with additional tags including the ConfigMap name.
 */
func getPerConfigMapCounters(configMapName string) PerConfigMapRdsStats {
	if perConfigMapCounters[configMapName] == nil {
		perConfigMapCounters[configMapName] = PerConfigMapRdsStats{
			configMapFetchErrorCount:       serviceScope.NewCounterWithTags("config_map_fetch_error_count", map[string]string{"configMapName": configMapName}),
			configMapParseErrorCount:       serviceScope.NewCounterWithTags("config_map_parse_error_count", map[string]string{"configMapName": configMapName}),
			configMapParseSuccessCount:     serviceScope.NewCounterWithTags("config_map_parse_success_count", map[string]string{"configMapName": configMapName}),
			configMapDataErrorCount:        serviceScope.NewCounterWithTags("config_map_data_error_count", map[string]string{"configMapName": configMapName}),
			configMapProcessedSuccessCount: serviceScope.NewCounterWithTags("config_map_processed_success_count", map[string]string{"configMapName": configMapName}),
			configMapValuesCount:           serviceScope.NewCounterWithTags("config_map_values_count", map[string]string{"configMapName": configMapName}),
		}
	}
	return perConfigMapCounters[configMapName].(PerConfigMapRdsStats)
}

/**
 * Returns the counters with additional tags including the Route Table name.
 */
func getPerRouteTableCounters(routeTableName string) PerRouteTableRdsStats {
	if perRouteTableCounters[routeTableName] == nil {
		perRouteTableCounters[routeTableName] = PerRouteTableRdsStats{
			aggregatedRoutesCount:          serviceScope.NewCounterWithTags("route_table_aggregated_routes_count", map[string]string{"routeTableName": routeTableName}),
			aggregatedVirtualClustersCount: serviceScope.NewCounterWithTags("route_table_aggregated_vc_count", map[string]string{"routeTableName": routeTableName}),
			totalRoutesCount:               serviceScope.NewCounterWithTags("route_table_total_routes_count", map[string]string{"routeTableName": routeTableName}),
			totalVirtualClustersCount:      serviceScope.NewCounterWithTags("route_table_total_vc_count", map[string]string{"routeTableName": routeTableName}),
			createErrorCount:               serviceScope.NewCounterWithTags("route_table_create_error_count", map[string]string{"routeTableName": routeTableName}),
			createSuccessCount:             serviceScope.NewCounterWithTags("route_table_create_success_count", map[string]string{"routeTableName": routeTableName}),
		}
	}
	return perRouteTableCounters[routeTableName].(PerRouteTableRdsStats)
}

/**
 * Returns the counters with additional tags including the NodeId & Snapshot Version.
 */
func getPerNodeCounters(nodeId, snapshotVersion string) PerNodeRdsStats {
	key := fmt.Sprintf("%s-%s", nodeId, snapshotVersion)
	if perNodeCounters[key] == nil {
		perNodeCounters[key] = PerNodeRdsStats{
			snapshotCacheUpdateSuccessCount: serviceScope.NewCounterWithTags("snapshot_update_success_count", map[string]string{"nodeId": nodeId, "snapshotVersion": snapshotVersion}),
			snapshotCacheUpdateErrorCount:   serviceScope.NewCounterWithTags("snapshot_update_error_count", map[string]string{"nodeId": nodeId, "snapshotVersion": snapshotVersion}),
		}
	}
	return perNodeCounters[key].(PerNodeRdsStats)
}

// RecordConfigMapFetchError It records the number of times k8s failed to fetch a given ConfigMap.
func RecordConfigMapFetchError(configMapName string) {
	getPerConfigMapCounters(configMapName).configMapFetchErrorCount.Inc()
}

// RecordConfigMapParseError It records the number of times we failed to parse a given ConfigMap.
func RecordConfigMapParseError(configMapName string) {
	getPerConfigMapCounters(configMapName).configMapParseErrorCount.Inc()
}

// RecordConfigMapParseSuccess It records the number of times we successfully parsed the given ConfigMap.
func RecordConfigMapParseSuccess(configMapName string) {
	getPerConfigMapCounters(configMapName).configMapParseSuccessCount.Inc()
}

// RecordConfigMapDataError It records the number of times we were unable to unmarshall the contents of the given
// ConfigMap.
func RecordConfigMapDataError(configMapName string) {
	getPerConfigMapCounters(configMapName).configMapDataErrorCount.Inc()
}

// RecordConfigMapProcessedSuccessError It records the number of times we successfully processed the given ConfigMap
// end-to-end.
func RecordConfigMapProcessedSuccessError(configMapName string) {
	getPerConfigMapCounters(configMapName).configMapProcessedSuccessCount.Inc()
}

// RecordConfigMapValues It records the number of routes or virtual cluster resources extracted from the given ConfigMap.
func RecordConfigMapValues(configMapName string, count uint64) {
	getPerConfigMapCounters(configMapName).configMapValuesCount.Add(count)
}

// RecordSnapshotCacheUpdateSuccess It records the number of times RDS was able to successfully update the snapshot.
func RecordSnapshotCacheUpdateSuccess(nodeId, snapshotVersion string) {
	getPerNodeCounters(nodeId, snapshotVersion).snapshotCacheUpdateSuccessCount.Inc()
}

// RecordSnapshotCacheUpdateError It records the number of times RDS was able to failed to update the snapshot cache.
func RecordSnapshotCacheUpdateError(nodeId, snapshotVersion string) {
	getPerNodeCounters(nodeId, snapshotVersion).snapshotCacheUpdateErrorCount.Inc()
}

// RecordRouteTableAggregatedRoutes It records the number of aggregated routes in the given Route Table.
func RecordRouteTableAggregatedRoutes(routeTableName string, count uint64) {
	getPerRouteTableCounters(routeTableName).aggregatedRoutesCount.Add(count)
}

// RecordRouteTableAggregatedVirtualClusters It records the number of aggregated VCs in the given Route Table.
func RecordRouteTableAggregatedVirtualClusters(routeTableName string, count uint64) {
	getPerRouteTableCounters(routeTableName).aggregatedVirtualClustersCount.Add(count)
}

// RecordRouteTableTotalRoutes It records the number of total routes in the given Route Table after adding new routes.
func RecordRouteTableTotalRoutes(routeTableName string, count uint64) {
	getPerRouteTableCounters(routeTableName).totalRoutesCount.Add(count)
}

// RecordRouteTableTotalVirtualClusters It records the number of total VCs in the given Route Table after adding new VCs.
func RecordRouteTableTotalVirtualClusters(routeTableName string, count uint64) {
	getPerRouteTableCounters(routeTableName).totalVirtualClustersCount.Add(count)
}

// RecordRouteTableCreateSuccess It records the times the creation of given Route Table succeeded.
func RecordRouteTableCreateSuccess(routeTableName string) {
	getPerRouteTableCounters(routeTableName).createSuccessCount.Inc()
}

// RecordRouteTableCreateError It records the times the creation of given Route Table failed.
func RecordRouteTableCreateError(routeTableName string) {
	getPerRouteTableCounters(routeTableName).createErrorCount.Inc()
}
