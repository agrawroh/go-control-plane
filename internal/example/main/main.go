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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"sync"

	_ "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/decompressor/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/ext_authz/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/internal/example"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	_ "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/itchyny/gojq"
	"github.com/ulikunitz/xz"
	"google.golang.org/protobuf/encoding/protojson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

const (
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

// parseYaml takes in a yaml envoy config string and returns a typed version
func parseYaml(yamlString string) ([]byte, error) {
	l.Debugf("[databricks-envoy-cp] *** YAML ---> JSON ***")
	jsonString, err := yaml.YAMLToJSON([]byte(yamlString))
	if err != nil {
		return nil, err
	}
	return jsonString, nil
}

func convertJsonToPb(jsonString string) (*v3.RouteConfiguration, error) {
	l.Debugf("Converting JSON ---> PB ***")
	config := &v3.RouteConfiguration{}
	err := protojson.Unmarshal([]byte(jsonString), config)
	// err := yaml.Unmarshal([]byte(envoyYaml), config)
	if err != nil {
		l.Errorf("Error while converting JSON -> PB: %s ", err.Error())
		return nil, err
	}
	l.Debugf("*** SUCCESS ***")
	return config, nil
}

func watchForChanges(clientset *kubernetes.Clientset, snapshotCache *cache.SnapshotCache, configmapKey *string, configmap *string, mutex *sync.Mutex) {
	for {
		namespace := os.Getenv("CONFIG_MAP_NAMESPACE")
		watcher, err := clientset.CoreV1().ConfigMaps(namespace).Watch(context.TODO(),
			metav1.SingleObject(metav1.ObjectMeta{
				Name:      os.Getenv("CONFIG_MAP_NAME"),
				Namespace: namespace,
			}))
		if err != nil {
			l.Errorf("Error occurred while creating the watcher: %s", err)
			panic("Unable to create watcher")
		}
		updateCurrentConfigmap(watcher.ResultChan(), snapshotCache, configmapKey, configmap, mutex)
	}
}

func updateCurrentConfigmap(eventChannel <-chan watch.Event, snapshotCache *cache.SnapshotCache, configmapKey *string, configmap *string, mutex *sync.Mutex) {
	for {
		event, open := <-eventChannel
		if open {
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				mutex.Lock()
				l.Debugf("*** Envoy ConfigMap Modified ***")
				// Update our configmap
				if updatedMap, ok := event.Object.(*corev1.ConfigMap); ok {
					for key, value := range updatedMap.Data {
						l.Debugf("ConfigMap Name: %s", key)
						xzString, err := base64.StdEncoding.DecodeString(value)
						if err != nil {
							l.Errorf("Error decoding string: %s ", err.Error())
							return
						}

						r, err := xz.NewReader(bytes.NewReader(xzString))
						if err != nil {
							l.Errorf("Error decompressing string: %s", err.Error())
							return
						}
						result, _ := ioutil.ReadAll(r)
						envoyConfigString := string(result)
						/*
							config, err := parseYaml(envoyConfigString)
							if err != nil {
								l.Errorf("Error parsing yaml string: %s ", err.Error())
								return
							}
						*/
						l.Debugf("Extracting Routes From JSON ConfigMap...")
						routesJson := extractRoutes(envoyConfigString)
						l.Debugf("Successfully extracted routes from the Envoy ConfigMap!")
						pb, err := convertJsonToPb(routesJson)
						*configmapKey = key
						*configmap = routesJson
						l.Debugf("*** Final PB: %s", pb)
						if err == nil {
							newSnapshot := cache.NewSnapshot(
								"1",
								[]types.Resource{},   // endpoints
								[]types.Resource{},   // clusters
								[]types.Resource{pb}, // routes
								[]types.Resource{},   // HCM
								[]types.Resource{},   // runtimes
								[]types.Resource{},   // secrets
								//[]types.Resource{}, // extension configs
							)
							l.Debugf("*** SETTING SNAPSHOT ***")
							if err := (*snapshotCache).SetSnapshot(nodeID, newSnapshot); err != nil {
								l.Errorf("Snapshot Error %q for %+v", err, newSnapshot)
								os.Exit(1)
							}
							l.Debugf("*** DONE ***")
						} else {
							l.Errorf("Unmarshalling Error: %s", err.Error())
						}
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

func extractRoutes(jsonConfig string) string {
	query, err := gojq.Parse(".route_config")
	if err != nil {
		l.Debugf("Error occurred while extracting Routes: %s", err)
	}
	jsonMap := make(map[string]interface{})
	err = json.Unmarshal([]byte(jsonConfig), &jsonMap)
	if err != nil {
		l.Debugf("Error occurred while unmarshalling Routes JSON: %s", err)
	}
	iter := query.Run(jsonMap) // or query.RunWithContext
	routes, ok := iter.Next()
	if !ok {
		l.Debugf("Not OK!")
		return ""
	}
	if err, ok := routes.(error); ok {
		l.Debugf("Error occurred: %s", err)
	}
	routesString, err := json.Marshal(routes)
	if err != nil {
		l.Debugf("Error occurred while marshalling: %s", err)
	}
	return string(routesString)
}

func initKubernetesClient() *kubernetes.Clientset {
	l.Debugf("Initializing Kubernetes Client...")
	// Create In-Cluster Config
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		l.Debugf("Error occurred while creating in-cluster config. %s", err)
		panic(err.Error())
	}
	// Create ClientSet
	clientset, err := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		l.Debugf("Error occurred while creating new client-set. %s", err)
		panic(err.Error())
	}

	// Sanity Check(s) [We can list pods]
	namespace := os.Getenv("CONFIG_MAP_NAMESPACE")
	l.Debugf("Listing existing K8S pods in the namespace: %s", namespace)
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		l.Debugf("Error occurred while listing pods.", err.Error())
		panic(err.Error())
	}
	l.Debugf("There are %d pods in the given namespace.", len(pods.Items))

	// Return Clientset
	return clientset
}

func setupWatcher(clientset *kubernetes.Clientset, snapshotCache *cache.SnapshotCache) {
	var (
		currentConfigmapKey string
		currentConfigmap    string
		mutex               *sync.Mutex
	)
	currentConfigmapKey = DEFAULT_CONFIGMAP_KEY
	currentConfigmap = DEFAULT_CONFIGMAP

	l.Debugf("Start setting up the config-map watcher...")
	mutex = &sync.Mutex{}
	go watchForChanges(clientset, snapshotCache, &currentConfigmapKey, &currentConfigmap, mutex)
}

/*
func test(c cache.SnapshotCache) {
	result, err := ioutil.ReadFile("/Users/rohit.agrawal/universe/apiproxy/deploy/multitenant/zzz_JSONNET_GENERATED/dev/oregon-dev/mt-shard/apiproxy.yaml")
	if err == nil {
		envoyConfigString := string(result)
		routesJson := extractRoutes(envoyConfigString)
		l.Debugf("[databricks-envoy-cp] *** Routes: %s", routesJson)
		pb, _ := convertJsonToPb(routesJson)
		snapshot := cache.NewSnapshot(
			"1",
			[]types.Resource{},   // endpoints
			[]types.Resource{},   // clusters
			[]types.Resource{pb}, // routes
			[]types.Resource{},   // HCM
			[]types.Resource{},   // runtimes
			[]types.Resource{},   // secrets
			//[]types.Resource{},   // extension configs
		)
		if err := c.SetSnapshot(nodeID, snapshot); err != nil {
			l.Errorf("Snapshot error %q for %+v", err, snapshot)
			os.Exit(1)
		}
	} else {
		l.Debugf("[databricks-envoy-cp] *** ERROR: %s", err)
	}
}
*/

func main() {
	flag.Parse()
	l.Debugf("Start Initializing xDS Main Server...")

	// Create Snapshot Cache
	// true = ADS Mode
	snapshotCache := cache.NewSnapshotCache(false, cache.IDHash{}, l)

	// Initialize Kubernetes Client
	clientset := initKubernetesClient()

	// Setup ConfigMap Watcher
	setupWatcher(clientset, &snapshotCache)

	// Run the xDS server
	l.Debugf("Running xDS Server...")
	ctx := context.Background()
	// cb := &test.Callbacks{Debug: l.Debug}
	srv := server.NewServer(ctx, snapshotCache, nil)
	example.RunServer(ctx, srv, port)
}
