package utils

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/ulikunitz/xz"
	coreV1 "k8s.io/api/core/v1"
)

// UnzipConfigMapData This function is used to parse the given ConfigMap data and unzip it. We assume that the ConfigMap
// data is compressed using xz and use the same library while decompressing it.
func UnzipConfigMapData(logger Logger, configMap *coreV1.ConfigMap) (map[string][]byte, error) {
	configMapData := make(map[string][]byte)
	for key, value := range configMap.Data {
		// Base64 Decode
		xzString, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, errors.Wrap(err, "error occurred while decoding the base64 data")
		}
		// Decompress
		data, err := xz.NewReader(bytes.NewReader(xzString))
		if err != nil {
			return nil, errors.Wrap(err, "error occurred while decompressing the xz string")
		}
		uncompressedData, _ := ioutil.ReadAll(data)
		configMapData[key] = uncompressedData
	}
	return configMapData, nil
}
