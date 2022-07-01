package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/envoyproxy/go-control-plane/rds/env"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/ulikunitz/xz"
	coreV1 "k8s.io/api/core/v1"
	testClient "k8s.io/client-go/kubernetes/fake"
)

func TestSkipUpdate(t *testing.T) {
	lastGoodVersion = "abcd"
	cm := &coreV1.ConfigMap{Data: map[string]string{}}
	cm.ObjectMeta.Labels = map[string]string{}
	// % is invalid base64 character. If update wasn't skipped, it would result in
	// failure.
	cm.Data["bad-data"] = "%%%"

	// First test versionHash == lastGoodVersion. Update will be skipped and
	// invalid data won't be parsed or trigger error.
	cm.Labels["versionHash"] = lastGoodVersion
	client := K8s{
		ClientSet: testClient.NewSimpleClientset(),
	}
	settings = env.NewSettings()
	err := client.doUpdate("unused-namespace", cm)
	if err != nil {
		t.Errorf("Failed to skip update: %s", err.Error())
	}

	// Next, test versionHash != lastGoodVersion. Update will NOT be skipped and
	// invalid data triggers parsing errors.
	cm.Labels["versionHash"] = "some-other-version"
	err = client.doUpdate("unused-namespace", cm)
	if err == nil {
		t.Errorf("Expected error, got none.")
	}
}

func TestWatcherCreationFailOnInvalidDuration(t *testing.T) {
	client := K8s{
		ClientSet: testClient.NewSimpleClientset(),
	}
	settings = env.Settings{
		ConfigMapPollInterval: "foo",
	}
	assertPanic(t, client.setupWatcher)
}

// Tests that watchForChanges() can see pre-existing ConfigMaps and update
// snapshotVal.
func TestWatchForChanges(t *testing.T) {
	settings.ConfigMapNamespace = "route-discovery-service"
	settings.SyncDelayTimeSeconds = 1
	settings.ConfigMapPollInterval = "1s"

	cms := []coreV1.ConfigMap{}
	configMapFile := "testdata/rds-config.yaml"

	bytes, err := ioutil.ReadFile(configMapFile)
	assert.Nil(t, err, "Failed to read %s", configMapFile)

	err = yaml.Unmarshal(bytes, &cms)
	assert.Nil(t, err, "Failed to unmarshal content in %s: %s", configMapFile, err)
	assert.NotEqual(t, 0, "%s file contains 0 ConfigMaps", configMapFile)

	k := K8s{
		ClientSet: testClient.NewSimpleClientset(),
	}
	v1cms := k.ClientSet.CoreV1().ConfigMaps(settings.ConfigMapNamespace)
	for _, cm := range cms {
		_, err := v1cms.Create(context.TODO(), &cm, metaV1.CreateOptions{})
		assert.Nil(t, err, "configMap creation failed with an error: %s", err)
	}

	_, err = v1cms.Get(context.TODO(), settings.EnvoyServiceImportOrderConfigName, metaV1.GetOptions{})
	assert.Nil(t, err, "Failed to get configMap %s: %s", settings.EnvoyServiceImportOrderConfigName, err)

	go k.watchForChanges(time.NewTicker(1 * time.Second))
	for i := 0; i < 10; i++ {
		t.Logf("Loading snapshot for the %d-th time", i)
		s := snapshotVal.Load()
		if s == nil {
			time.Sleep(1 * time.Second)
		}
	}
	assert.NotNil(t, snapshotVal, "Failed to update snapshotVal.")
}

func TestGetConfigMap(t *testing.T) {
	client := K8s{
		ClientSet: testClient.NewSimpleClientset(),
	}
	// Create a ConfigMap called `config-name` in the kubernetes namespace called `namespace` with a label `versionHash` = `1`
	_, err := client.ClientSet.CoreV1().ConfigMaps("namespace").Create(context.TODO(), &coreV1.ConfigMap{ObjectMeta: metaV1.ObjectMeta{Name: "config-name", Labels: map[string]string{"versionHash": "1"}}}, metaV1.CreateOptions{})
	if err != nil {
		assert.Fail(t, "configMap creation failed with an error.")
	}

	// This should succeed as we'll be able to find this ConfigMap with version = 1
	configMap, _ := client.getConfigMap("config-name", "namespace", "1", false)
	assert.NotNil(t, configMap)
	assert.Equal(t, "1", configMap.Labels["versionHash"])

	// This should fail as we won't be able to find this ConfigMap with version = 2
	configMap, _ = client.getConfigMap("config-name", "namespace", "2", false)
	assert.Nil(t, configMap)
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

/*
 * assertPanic asserts whether the code under test panics or not. The test would fail if the function doesn't panic.
 */
func assertPanic(t *testing.T, f func()) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	f()
}
