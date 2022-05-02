package env

import (
	"crypto/tls"

	"github.com/kelseyhightower/envconfig"
)

type Settings struct {
	// Environment Configuration
	Port       int    `envconfig:"SERVICE_PORT_NUMBER" default:"11000"`
	HttpPort   int    `envconfig:"SERVICE_HTTP_PORT_NUMBER" default:"8080"`
	StatsDPort string `envconfig:"STATSD_PORT" default:"9125"`
	StatsDHost string `envconfig:"STATSD_HOST" default:"localhost"`

	// ConfigMap(s)
	ConfigMapNamespace                 string `envconfig:"CONFIG_MAP_NAMESPACE" default:"default"`
	SyncDelayTimeSeconds               int    `envconfig:"CONFIG_MAP_SYNC_DELAY_TIME_SECONDS" default:"60"`
	EnvoyRoutesImportOrderConfigName   string `envconfig:"ENVOY_ROUTES_IMPORT_ORDER_CONFIG_NAME" default:"envoy-routes-import-order-config"`
	EnvoyRouteConfigurationsConfigName string `envconfig:"ENVOY_ROUTE_CONFIGURATIONS_CONFIG_NAME" default:"envoy-route-configurations-config"`
	EnvoyServiceImportOrderConfigName  string `envconfig:"ENVOY_SERVICE_IMPORT_ORDER_CONFIG_NAME" default:"envoy-svc-import-order-config"`

	// gRPC Server Configuration
	GrpcKeepaliveTimeSeconds    int    `envconfig:"GRPC_KEEPALIVE_TIME_SECONDS" default:"30"`
	GrpcKeepaliveTimeoutSeconds int    `envconfig:"GRPC_KEEPALIVE_TIMEOUT_SECONDS" default:"5"`
	GrpcKeepaliveMinTimeSeconds int    `envconfig:"GRPC_KEEPALIVE_MIN_TIME_SECONDS" default:"30"`
	GrpcMaxConcurrentStreams    uint32 `envconfig:"GRPC_MAX_CONCURRENT_STREAMS" default:"1000000"`

	// Logging Settings
	DebugLogging bool   `envconfig:"DEBUG_LOGGING_ENABLED" default:"true"`
	LogLevel     string `envconfig:"LOG_LEVEL" default:"WARN"`
	LogFormat    string `envconfig:"LOG_FORMAT" default:"text"`

	// TLS Configuration
	MinTlsVersion         string `envconfig:"MIN_TLS_VERSION" default:""`
	MaxTlsVersion         string `envconfig:"MAX_TLS_VERSION" default:""`
	RequireClientCert     bool   `envconfig:"REQUIRE_CLIENT_CERT" default:"false"`
	ServerCaPath          string `envconfig:"SERVER_CA_PATH" default:""`
	ServerCertificatePath string `envconfig:"SERVER_CERT_PATH" default:""`
	ServerKeyPath         string `envconfig:"SERVER_KEY_PATH" default:""`
	ServerTls             bool   `envconfig:"SERVER_TLS" default:"false"`
	ServerTlsConfig       *tls.Config
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
