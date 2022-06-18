package utils

import (
	"reflect"
	"testing"

	coreV1 "k8s.io/api/core/v1"
)

func TestBadKey(t *testing.T) {
	c := NewConfigMapCache()
	_, exists := c.Get("bad-key", "version")
	if exists {
		t.Errorf("Expected key to be non-existent, but found it.")
	}
}

func TestBadVersion(t *testing.T) {
	c := NewConfigMapCache()
	key := "test-key"
	version := "v1"
	badVersion := "v2"
	c.Put(key, version, &coreV1.ConfigMap{})

	_, exists := c.Get(key, badVersion)
	if exists {
		t.Errorf("Expected badVersion \"%s\" to not exist, but found", badVersion)
	}
}

func TestPut(t *testing.T) {
	c := NewConfigMapCache()
	key := "test-key"
	v1 := "v1"
	v2 := "v2"
	data := map[string]string{"key": "value"}
	c.Put(key, v1, &coreV1.ConfigMap{Data: data})

	cm, exists := c.Get(key, v1)
	if !exists {
		t.Errorf("Expected key \"%s\" to exists, but it doesn't", key)
	}
	if !reflect.DeepEqual(cm.Data, data) {
		t.Errorf("Expected equal data, but different")
	}

	// After update with new version and content, the next Get() should reflect
	// that.
	newData := map[string]string{"key": "new-value"}
	c.Put(key, v2, &coreV1.ConfigMap{Data: newData})

	cm, exists = c.Get(key, v2)
	if !exists {
		t.Errorf("Expected key \"%s\" to exists, but it doesn't", key)
	}
	if !reflect.DeepEqual(cm.Data, newData) {
		t.Errorf("Expected equal data, but different")
	}
}
