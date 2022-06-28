package main

import (
	"bytes"
	"encoding/base64"
	"io"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/ulikunitz/xz"
	coreV1 "k8s.io/api/core/v1"
)

func TestSkipUpdtae(t *testing.T) {
	lastGoodVersion = "abcd"
	cm := &coreV1.ConfigMap{Data: map[string]string{}}
	cm.ObjectMeta.Labels = map[string]string{}
	// % is invalid base64 character. If update wasn't skipped, it would result in
	// failure.
	cm.Data["bad-data"] = "%%%"

	// First test versionHash == lastGoodVersion. Update will be skipped and
	// invalid data won't be parsed or trigger error.
	cm.Labels["versionHash"] = lastGoodVersion
	err := doUpdate(nil, "unused-namespace", cm)
	if err != nil {
		t.Errorf("Failed to skip update: %s", err.Error())
	}

	// Next, test versionHash != lastGoodVersion. Update will NOT be skipped and
	// invalid data triggers parsing errors.
	cm.Labels["versionHash"] = "some-other-version"
	err = doUpdate(nil, "unused-namespace", cm)
	if err == nil {
		t.Errorf("Expected error, got none.")
	}
}

func TestParseServiceImportOrderConfigMapFail(t *testing.T) {
	compressedConfigMap := coreV1.ConfigMap{
		Data: map[string]string{"bar": "foo"},
	}
	_, err := ParseServiceImportOrderConfigMap(&compressedConfigMap)
	expectedErrorMsg := "error occurred while parsing service import order ConfigMap: error occurred while decoding the base64 data: illegal base64 data at input byte 0"
	assert.EqualErrorf(t, err, expectedErrorMsg, "Error should be: %v, got: %v", expectedErrorMsg, err)
}

func TestParseServiceImportOrderConfigMapSuccess(t *testing.T) {
	text := `---
    - foo
    - bar
    `
	data, err := getTestData(text)
	if err != nil {
		t.Errorf("expected no errors, got '%s'", err.Error())
	}
	compressedConfigMap := coreV1.ConfigMap{
		Data: map[string]string{"svc-names": data},
	}
	serviceNames, err := ParseServiceImportOrderConfigMap(&compressedConfigMap)
	// We expect ParseServiceImportOrderConfigMap function to return two service i.e. `foo` and `bar`
	assert.Equal(t, 2, len(serviceNames))
	assert.True(t, reflect.DeepEqual(serviceNames, []string{"foo", "bar"}))
}

/*
 * getTestData compresses the given text using XZ and then Base64 encode it. The data inside the ConfigMap(s) we get
 * is compressed using XZ and Kubernetes stores everything as a Base64 encoded string so this lines up with what's
 * expected while reading the ConfigMap(s) data.
 */
func getTestData(text string) (string, error) {
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
