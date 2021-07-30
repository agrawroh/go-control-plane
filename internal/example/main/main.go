// Copyright 2020 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/envoyproxy/go-control-plane/internal/example"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	CM_NAME               = "apiproxy-envoy-config"
	NAMESPACE             = "test-shard-seikun-kambashi-westusc2"
	DEFAULT_CONFIGMAP_KEY = "no configmap key"
	DEFAULT_CONFIGMAP     = "no configmap"
)

var (
	l example.Logger

	port     uint
	basePort uint
	mode     string

	nodeID string
)

func init() {
	l = example.Logger{}

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	// The port that this xDS server listens on
	flag.UintVar(&port, "port", 18000, "xDS management server port")

	// Tell Envoy to use this Node ID
	flag.StringVar(&nodeID, "nodeID", "test-id", "Node ID")
}

func watchForChanges(clientset *kubernetes.Clientset, configmapKey *string, configmap *string, mutex *sync.Mutex) {
	for {
		watcher, err := clientset.CoreV1().ConfigMaps(NAMESPACE).Watch(context.TODO(),
			metav1.SingleObject(metav1.ObjectMeta{Name: CM_NAME, Namespace: NAMESPACE}))
		if err != nil {
			panic("Unable to create watcher")
		}
		updateCurrentConfigmap(watcher.ResultChan(), configmapKey, configmap, mutex)
	}
}

func updateCurrentConfigmap(eventChannel <-chan watch.Event, configmapKey *string, configmap *string, mutex *sync.Mutex) {
	for {
		event, open := <-eventChannel
		if open {
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				mutex.Lock()
				l.Debugf("[databricks-envoy-cp] configmap modified")
				// Update our configmap
				if updatedMap, ok := event.Object.(*corev1.ConfigMap); ok {
					for key, value := range updatedMap.Data {
						l.Debugf("[databricks-envoy-cp] %s => %s", key, value)
						*configmapKey = key
						*configmap = value
					}
				}
				mutex.Unlock()
			case watch.Deleted:
				mutex.Lock()
				// Fall back to the default value
				*configmapKey = DEFAULT_CONFIGMAP_KEY
				*configmap = DEFAULT_CONFIGMAP
				mutex.Unlock()
			default:
				// Do nothing
			}
		} else {
			// If eventChannel is closed, it means the server has closed the connection
			return
		}
	}
}

func testClient() {
	var (
		currentConfigmapKey string
		currentConfigmap    string
		mutex               *sync.Mutex
	)
	currentConfigmapKey = DEFAULT_CONFIGMAP_KEY
	currentConfigmap = DEFAULT_CONFIGMAP

	l.Debugf("[databricks-envoy-cp] init k8s client")
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	l.Debugf("[databricks-envoy-cp] k8s client try list pods")
	// Sanity check we can list pods
	pods, err := clientset.CoreV1().Pods(NAMESPACE).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	l.Debugf("[databricks-envoy-cp] There are %d pods in the cluster\n", len(pods.Items))

	l.Debugf("[databricks-envoy-cp] setup watcher")
	mutex = &sync.Mutex{}
	go watchForChanges(clientset, &currentConfigmapKey, &currentConfigmap, mutex)

	http.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		body := []byte(fmt.Sprintf(`{"current_configmap_key": "%s"}`, currentConfigmapKey))
		mutex.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	l.Debugf("Listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}

func main() {
	flag.Parse()

	l.Debugf("[databricks-envoy-cp] init")

	// Create a cache
	cache := cache.NewSnapshotCache(false, cache.IDHash{}, l)

	// Create the snapshot that we'll serve to Envoy
	snapshot := example.GenerateSnapshot()
	if err := snapshot.Consistent(); err != nil {
		l.Errorf("snapshot inconsistency: %+v\n%+v", snapshot, err)
		os.Exit(1)
	}
	l.Debugf("will serve snapshot %+v", snapshot)

	// Add the snapshot to the cache
	if err := cache.SetSnapshot(nodeID, snapshot); err != nil {
		l.Errorf("snapshot error %q for %+v", err, snapshot)
		os.Exit(1)
	}

	l.Debugf("[databricks-envoy-cp] testing k8s client")
	testClient()

	l.Debugf("[databricks-envoy-cp] running server")
	// Run the xDS server
	ctx := context.Background()
	cb := &test.Callbacks{Debug: l.Debug}
	srv := server.NewServer(ctx, cache, cb)
	example.RunServer(ctx, srv, port)
}
