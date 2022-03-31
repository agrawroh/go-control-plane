package utils

import (
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

/**
 * Converts the given YAML to JSON so that proto json could be used.
 */
func getJsonBytes(logger Logger, yamlBytes []byte) ([]byte, error) {
	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		logger.Errorf("Error while converting JSON -> YAML: %s", err.Error())
		return nil, err
	}
	return jsonBytes, nil
}

// ConvertYamlToRouteConfigurationProto This method is used to convert the parse the given YAML string to v3
// RouteConfiguration proto.
func ConvertYamlToRouteConfigurationProto(logger Logger, yamlBytes []byte) (*v3.RouteConfiguration, error) {
	config := &v3.RouteConfiguration{}
	jsonBytes, err := getJsonBytes(logger, yamlBytes)
	if err != nil {
		return nil, err
	}
	err = protojson.Unmarshal(jsonBytes, config)
	if err != nil {
		logger.Errorf("Error while converting YAML -> RouteConfiguration PB: %s", err.Error())
		return nil, err
	}
	return config, nil
}

// ConvertYamlToRouteProto This method is used to convert the parse the given YAML string to v3 Route proto.
func ConvertYamlToRouteProto(logger Logger, yamlBytes []byte) (*v3.Route, error) {
	config := &v3.Route{}
	jsonBytes, err := getJsonBytes(logger, yamlBytes)
	if err != nil {
		return nil, err
	}
	err = protojson.Unmarshal(jsonBytes, config)
	if err != nil {
		logger.Errorf("Error while converting YAML -> Route PB: %s", err.Error())
		return nil, err
	}
	return config, nil
}

// ConvertYamlToVirtualClusterProto This method is used to convert the parse the given YAML string to v3 VirtualCluster
// proto.
func ConvertYamlToVirtualClusterProto(logger Logger, yamlBytes []byte) (*v3.VirtualCluster, error) {
	config := &v3.VirtualCluster{}
	jsonBytes, err := getJsonBytes(logger, yamlBytes)
	if err != nil {
		return nil, err
	}
	err = protojson.Unmarshal(jsonBytes, config)
	if err != nil {
		logger.Errorf("Error while converting YAML -> VirtualCluster PB: %s", err.Error())
		return nil, err
	}
	return config, nil
}
