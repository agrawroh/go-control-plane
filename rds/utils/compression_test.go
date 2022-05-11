package utils_test

import (
	"bytes"
	"encoding/base64"
	"io"
	"testing"

	"github.com/envoyproxy/go-control-plane/rds/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/ulikunitz/xz"

	coreV1 "k8s.io/api/core/v1"
)

func TestDecompressBase64Fail(t *testing.T) {
	compressedConfigMap := coreV1.ConfigMap{
		Data: map[string]string{"bar": "foo"},
	}
	_, err := utils.Decompress(&compressedConfigMap)
	expectedErrorMsg := "error occurred while decoding the base64 data: illegal base64 data at input byte 0"
	assert.EqualErrorf(t, err, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
}

func TestDecompressXZFail(t *testing.T) {
	compressedConfigMap := coreV1.ConfigMap{
		Data: map[string]string{"foo": "L1RkNldGb0FBQVRtMXJSR0FnQWhBUllBQUFCMEwrV2pBUUFNU0dWc2JHOHNJRmR2Y214a0lRQUFBQUFzZTJSYzhETUdLQUFCSlExeEdjUzJIN2J6ZlFFQUFBQUFCRmxh"},
	}
	_, err := utils.Decompress(&compressedConfigMap)
	expectedErrorMsg := "error occurred while decompressing the xz string: xz: invalid header magic bytes"
	assert.EqualErrorf(t, err, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
}

func TestDecompress(t *testing.T) {
	text := "Hello, World!"
	data, err := compress(text)
	if err != nil {
		t.Errorf("expected no errors, got '%s'", err.Error())
	}
	compressedConfigMap := coreV1.ConfigMap{
		Data: map[string]string{"foo": data},
	}
	configMap, err := utils.Decompress(&compressedConfigMap)
	assert.Equal(t, text, string(configMap["foo"]))
}

/*
 * compress applies XZ compression on the given and Base64 encodes it. The data inside the ConfigMap(s) we get is
 * compressed using XZ and Kubernetes stores everything as a Base64 encoded string so this lines up with what's
 * expected while reading the ConfigMap(s) data.
 */
func compress(text string) (string, error) {
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		return "", errors.Wrap(err, "xz.NewWriter error")
	}
	if _, err := io.WriteString(w, text); err != nil {
		return "", errors.Wrap(err, "WriteString error")
	}
	if err := w.Close(); err != nil {
		return "", errors.Wrap(err, "w.Close error")
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
