package utils

import (
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

/**
 * getJsonBytes convert the yamlBytes to json and return bytes array.
 */
func getJsonBytes(yamlBytes []byte) ([]byte, error) {
	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while converting JSON -> YAML")
	}
	return jsonBytes, nil
}

// ConvertYamlToRouteConfigurationProto convert yamlBytes to v3 RouteConfiguration proto.
func ConvertYamlToRouteConfigurationProto(yamlBytes []byte) (*v3.RouteConfiguration, error) {
	config := &v3.RouteConfiguration{}
	jsonBytes, err := getJsonBytes(yamlBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while creating v3.RouteConfiguration proto")
	}
	if err = protojson.Unmarshal(jsonBytes, config); err != nil {
		return nil, errors.Wrap(err, "error occurred while converting YAML -> v3.RouteConfiguration protobuf")
	}
	return config, nil
}

// ConvertYamlToRouteProto convert yamlBytes to v3 Route proto.
func ConvertYamlToRouteProto(yamlBytes []byte) (*v3.Route, error) {
	config := &v3.Route{}
	jsonBytes, err := getJsonBytes(yamlBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while creating v3.Route proto")
	}
	if err = protojson.Unmarshal(jsonBytes, config); err != nil {
		return nil, errors.Wrap(err, "error occurred while converting YAML -> v3.Route protobuf")
	}
	return config, nil
}

// ConvertYamlToVirtualClusterProto convert yamlBytes to v3 VirtualCluster proto.
func ConvertYamlToVirtualClusterProto(yamlBytes []byte) (*v3.VirtualCluster, error) {
	config := &v3.VirtualCluster{}
	jsonBytes, err := getJsonBytes(yamlBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while creating v3.VirtualCluster proto")
	}
	if err = protojson.Unmarshal(jsonBytes, config); err != nil {
		return nil, errors.Wrap(err, "error occurred while converting YAML -> v3.VirtualCluster protobuf")
	}
	return config, nil
}
