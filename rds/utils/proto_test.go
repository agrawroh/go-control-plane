package utils_test

import (
	"fmt"
	"testing"

	"github.com/envoyproxy/go-control-plane/rds/utils"
	"github.com/stretchr/testify/assert"
)

func TestConvertYamlToRouteConfigurationProtoError(t *testing.T) {
	_, err := utils.ConvertYamlToRouteConfigurationProto([]byte("foo"))
	expectedErrorMsg := "error occurred while converting YAML -> v3.RouteConfiguration protobuf"
	assert.Containsf(t, err.Error(), expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
}

func TestConvertYamlToRouteConfigurationProtoSuccess(t *testing.T) {
	routeTableName := "http-routes"
	routeConfigYaml := fmt.Sprintf("name: %s", routeTableName)
	proto, err := utils.ConvertYamlToRouteConfigurationProto([]byte(routeConfigYaml))
	assert.Nil(t, err)
	assert.Equal(t, proto.Name, routeTableName)
}

func TestConvertYamlToRouteProtoError(t *testing.T) {
	_, err := utils.ConvertYamlToRouteProto([]byte("bar"))
	expectedErrorMsg := "error occurred while converting YAML -> v3.Route protobuf"
	assert.Containsf(t, err.Error(), expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
}

func TestConvertYamlToRouteProtoSuccess(t *testing.T) {
	routeName := "/api"
	yaml := `
    name: %s
    match:
      headers:
        - name: :authority
    `
	routeYaml := fmt.Sprintf(yaml, routeName)
	proto, err := utils.ConvertYamlToRouteProto([]byte(routeYaml))
	assert.Nil(t, err)
	assert.Equal(t, routeName, proto.Name)
}

func TestConvertYamlToVirtualClusterProtoError(t *testing.T) {
	_, err := utils.ConvertYamlToVirtualClusterProto([]byte("foo"))
	expectedErrorMsg := "error occurred while converting YAML -> v3.VirtualCluster protobuf"
	assert.Containsf(t, err.Error(), expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
}

func TestConvertYamlToVirtualClusterProtoSuccess(t *testing.T) {
	vcName := "/api"
	yaml := `
    name: %s
    headers:
    - name: :authority
    `
	vcYaml := fmt.Sprintf(yaml, vcName)
	proto, err := utils.ConvertYamlToVirtualClusterProto([]byte(vcYaml))
	assert.Nil(t, err)
	assert.Equal(t, proto.Name, vcName)
}
