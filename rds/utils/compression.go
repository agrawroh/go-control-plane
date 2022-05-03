package utils

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/ulikunitz/xz"
	coreV1 "k8s.io/api/core/v1"
)

// Decompress parse configMap, decompress it using `xz` and return a map of
// (configMap data key -> uncompressed data bytes). For example, ("envoy-main-routes" -> <yaml-bytes>)
// NOTE: configMap data is assumed to be compressed using `xz` and hence the same library is being uses to decompress it.
func Decompress(configMap *coreV1.ConfigMap) (map[string][]byte, error) {
	result := make(map[string][]byte)
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
		result[key] = uncompressedData
	}
	return result, nil
}
