package utils

import (
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"sigs.k8s.io/yaml"

	v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
)

/**
 * getJsonBytes convert the yamlBytes to json and return bytes array.
 */
func getJSONBytes(yamlBytes []byte) ([]byte, error) {
	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while converting JSON -> YAML")
	}
	return jsonBytes, nil
}

// ConvertYamlToClusterProto convert yamlBytes to v3.Cluster proto.
func ConvertYamlToClusterProto(yamlBytes []byte) (*v3.Cluster, error) {
	config := &v3.Cluster{}
	jsonBytes, err := getJSONBytes(yamlBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred while creating v3.Cluster proto")
	}
	if err = protojson.Unmarshal(jsonBytes, config); err != nil {
		return nil, errors.Wrap(err, "error occurred while converting YAML -> v3.Cluster protobuf")
	}
	return config, nil
}
