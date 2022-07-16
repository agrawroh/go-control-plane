package env

import (
	"crypto/tls"

	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	// Environment Configuration
	Port               int    `envconfig:"SERVICE_PORT_NUMBER" default:"11000"`
	HTTPPort           int    `envconfig:"SERVICE_HTTP_PORT_NUMBER" default:"8080"`
	DebugPort          int    `envconfig:"SERVICE_DEBUG_PORT_NUMBER" default:"8081"`
	StatsDPort         string `envconfig:"STATSD_PORT" default:"9125"`
	StatsDHost         string `envconfig:"STATSD_HOST" default:"localhost"`
	DebugServerEnabled bool   `envconfig:"DEBUG_SERVER_ENABLED" default:"false"`

	// Config Canary Configuration

	/*
	 * This is the time (in milliseconds) for which RDS would wait after canary-ing the latest config to certain
	 * percentage of clients. This time period should be in sync with the HMR and ideally is long enough for HMR to
	 * get some health signals from the connected clients and trigger a rollback if the health metrics aren't looking
	 * good.
	 */
	ConfigCanaryTimeInMilliseconds int64 `envconfig:"CONFIG_CANARY_TIME_IN_MILLISECONDS" default:"600000"`

	/*
	 * This is the ratio of the clients to which RDS would start canary-ing the new config update after aggregating a
	 * newly received set of ConfigMap(s).
	 */
	ConfigCanaryRatio float32 `envconfig:"CONFIG_CANARY_RATIO" default:"0.3"`

	// Kubernetes Client Configuration
	ClientQPS   float32 `envconfig:"K8S_CLIENT_QPS" default:"100"`
	ClientBurst int     `envconfig:"K8S_CLIENT_BURST" default:"150"`

	// ConfigMap(s)
	ConfigMapNamespace                 string `envconfig:"CONFIG_MAP_NAMESPACE" default:"default"`
	SyncDelayTimeSeconds               int    `envconfig:"CONFIG_MAP_SYNC_DELAY_TIME_SECONDS" default:"60"`
	ConfigMapPollInterval              string `envconfig:"CONFIG_MAP_POLL_INTERVAL" default:"10s"`
	EnvoyRoutesImportOrderConfigName   string `envconfig:"ENVOY_ROUTES_IMPORT_ORDER_CONFIG_NAME" default:"envoy-routes-import-order-config"`
	EnvoyRouteConfigurationsConfigName string `envconfig:"ENVOY_ROUTE_CONFIGURATIONS_CONFIG_NAME" default:"envoy-route-configurations-config"`
	EnvoyServiceImportOrderConfigName  string `envconfig:"ENVOY_SERVICE_IMPORT_ORDER_CONFIG_NAME" default:"envoy-svc-import-order-config"`

	/*
	 * This is the time (in milliseconds) for which RDS would delay the check for the connected clients and start doing
	 * canary or reconcile the existing canary state.
	 */
	SnapshotCacheUpdateDelayMilliseconds int64 `envconfig:"SNAPSHOT_)CACHE_UPDATE_DELAY_MILLISECONDS" default:"5000"`

	// gRPC Server Configuration
	GrpcKeepaliveTimeSeconds    int    `envconfig:"GRPC_KEEPALIVE_TIME_SECONDS" default:"30"`
	GrpcKeepaliveTimeoutSeconds int    `envconfig:"GRPC_KEEPALIVE_TIMEOUT_SECONDS" default:"5"`
	GrpcKeepaliveMinTimeSeconds int    `envconfig:"GRPC_KEEPALIVE_MIN_TIME_SECONDS" default:"30"`
	GrpcMaxConcurrentStreams    uint32 `envconfig:"GRPC_MAX_CONCURRENT_STREAMS" default:"1000000"`

	// Logging Settings
	LogLevel  string `envconfig:"LOG_LEVEL" default:"DEBUG"`
	LogFormat string `envconfig:"LOG_FORMAT" default:"text"`

	// TLS Configuration
	MinTLSVersion         string `envconfig:"MIN_TLS_VERSION" default:"TLSv1.2"`
	MaxTLSVersion         string `envconfig:"MAX_TLS_VERSION" default:"TLSv1.3"`
	RequireClientCert     bool   `envconfig:"REQUIRE_CLIENT_CERT" default:"true"`
	ServerCaPath          string `envconfig:"SERVER_CA_PATH" default:""`
	ServerCertificatePath string `envconfig:"SERVER_CERT_PATH" default:""`
	ServerKeyPath         string `envconfig:"SERVER_KEY_PATH" default:""`
	ServerTLS             bool   `envconfig:"SERVER_TLS" default:"true"`
	ServerTLSConfig       *tls.Config
}

// NewSettings Parse environment variables and return settings object.
func NewSettings() Settings {
	var settings Settings
	err := envconfig.Process("", &settings)
	if err != nil {
		panic(err)
	}
	return settings
}
