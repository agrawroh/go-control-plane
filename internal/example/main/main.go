package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	v3Cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
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

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS Server Debug Logging")

	// The port that this xDS server listens on
	flag.UintVar(&port, "port", 18000, "xDS Management Server Port")

	// Tell Envoy to use this Node ID
	flag.StringVar(&nodeID, "nodeID", "test-id", "Node ID")
}

// parseYaml takes in a yaml envoy config string and returns a typed version
func parseYaml(yamlString string) ([]byte, error) {
	l.Debugf("***** YAML ---> JSON *****")
	jsonString, err := yaml.YAMLToJSON([]byte(yamlString))
	if err != nil {
		return nil, err
	}
	return jsonString, nil
}

func convertJsonToPb(jsonString string) (*v3.RouteConfiguration, error) {
	l.Debugf("***** Converting JSON ---> PB *****")
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

func watchForChanges(clientset *kubernetes.Clientset, snapshotCache *cache.SnapshotCache, configmapKey, configmap *string, clustersResource, routesResource *[]types.Resource, mutex *sync.Mutex) {
	for {
		watcher, err := clientset.CoreV1().ConfigMaps(corev1.NamespaceAll).Watch(context.TODO(), metav1.ListOptions{})
		if err != nil {
			l.Errorf("Error occurred while creating the watcher: %s", err)
			panic("Unable to create the watcher!")
		}
		updateCurrentConfigmap(watcher.ResultChan(), snapshotCache, configmapKey, configmap, clustersResource, routesResource, mutex)
	}
}

func updateRoutesConfig(routesResource *[]types.Resource, configMap *corev1.ConfigMap, configmapKey, configmap *string) error {
	var err error
	for key, value := range configMap.Data {
		l.Debugf("ConfigMap Key: %s", key)
		xzString, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			l.Errorf("Error decoding string: %s ", err.Error())
			return err
		}

		r, err := xz.NewReader(bytes.NewReader(xzString))
		if err != nil {
			l.Errorf("Error decompressing string: %s", err.Error())
			return err
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
		routesJson := extractData(envoyConfigString, ".route_config")
		l.Debugf("Successfully extracted routes from the Envoy ConfigMap!")
		pb, err := convertJsonToPb(routesJson)
		*routesResource = make([]types.Resource, 1)
		(*routesResource)[0] = pb
		*configmapKey = key
		*configmap = routesJson
		l.Debugf("*** Final PB: %s", pb)
	}
	return err
}

func updateClustersConfig(clustersResource *[]types.Resource, configMap *corev1.ConfigMap) error {
	var err error
	for key, value := range configMap.Data {
		l.Debugf("ConfigMap Key: %s", key)
		xzString, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			l.Errorf("Error decoding string: %s ", err.Error())
			return err
		}

		r, err := xz.NewReader(bytes.NewReader(xzString))
		if err != nil {
			l.Errorf("Error decompressing string: %s", err.Error())
			return err
		}
		result, _ := ioutil.ReadAll(r)
		envoyConfigString := string(result)
		clustersJson := extractData(envoyConfigString, ".cluster_config")
		l.Debugf("*** Clusters: %s", clustersJson)
		var clustersJsonArray []interface{}
		err = json.Unmarshal([]byte(clustersJson), &clustersJsonArray)
		if err != nil {
			l.Debugf("Error occurred while unmarshalling clusters array: %s", err)
		}
		*clustersResource = make([]types.Resource, len(clustersJsonArray))
		for index, clusterJson := range clustersJsonArray {
			clusterJsonString, err := json.Marshal(clusterJson)
			if err != nil {
				l.Debugf("Error occurred while marshalling: %s", err)
			}
			pb, _ := convertClustersJsonToPb(string(clusterJsonString))
			(*clustersResource)[index] = pb
			l.Debugf("*** PB: %s", pb)
		}
	}
	return err
}

func updateCurrentConfigmap(eventChannel <-chan watch.Event, snapshotCache *cache.SnapshotCache, configmapKey, configmap *string, clustersResource, routesResource *[]types.Resource, mutex *sync.Mutex) {
	for {
		event, open := <-eventChannel
		if open {
			switch event.Type {
			case watch.Added:
				fallthrough
			case watch.Modified:
				l.Debugf("*** Envoy ConfigMap Modified ***")
				mutex.Lock()
				// Update our configmap
				if configMap, ok := event.Object.(*corev1.ConfigMap); ok {
					configmapName := configMap.Name
					//configmapNamespace := configMap.Namespace
					var err error
					if strings.Contains(configmapName, "-envoy-routes-config") {
						l.Debugf("*** Updating Routes ***")
						err = updateRoutesConfig(routesResource, configMap, configmapKey, configmap)
					}
					if strings.Contains(configmapName, "-envoy-clusters-config") {
						l.Debugf("*** Updating Clusters ***")
						err = updateClustersConfig(clustersResource, configMap)
					}
					if err == nil {
						newSnapshot := cache.NewSnapshot(
							time.Now().Format("2006-01-02-15-04-05"),
							[]types.Resource{}, // endpoints
							*clustersResource,  // clusters
							*routesResource,    // routes
							[]types.Resource{}, // HCM
							[]types.Resource{}, // runtimes
							[]types.Resource{}, // secrets
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

func extractData(jsonConfig, queryString string) string {
	query, err := gojq.Parse(queryString)
	if err != nil {
		l.Debugf("Error occurred while extracting Data: %s", err)
	}
	jsonMap := make(map[string]interface{})
	err = json.Unmarshal([]byte(jsonConfig), &jsonMap)
	if err != nil {
		l.Debugf("Error occurred while unmarshalling JSON: %s", err)
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
	pods, err := clientset.CoreV1().Pods(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		l.Debugf("Error occurred while listing pods.", err.Error())
		panic(err.Error())
	}
	l.Debugf("There are %d pods across all the namespace(s).", len(pods.Items))

	// Return Clientset
	return clientset
}

func setupWatcher(clientset *kubernetes.Clientset, snapshotCache *cache.SnapshotCache) {
	var (
		currentConfigmapKey string
		currentConfigmap    string
		clustersResource    []types.Resource
		routesResource      []types.Resource
		mutex               *sync.Mutex
	)
	currentConfigmapKey = DEFAULT_CONFIGMAP_KEY
	currentConfigmap = DEFAULT_CONFIGMAP

	l.Debugf("Start setting up the config-map watcher...")
	mutex = &sync.Mutex{}
	go watchForChanges(clientset, snapshotCache, &currentConfigmapKey, &currentConfigmap, &clustersResource, &routesResource, mutex)
}

func convertClustersJsonToPb(jsonString string) (*v3Cluster.Cluster, error) {
	l.Debugf("Converting JSON ---> PB ***")
	config := &v3Cluster.Cluster{}
	err := protojson.Unmarshal([]byte(jsonString), config)
	// err := yaml.Unmarshal([]byte(envoyYaml), config)
	if err != nil {
		l.Errorf("Error while converting JSON -> PB: %s ", err.Error())
		return nil, err
	}
	l.Debugf("*** SUCCESS ***")
	return config, nil
}

/*
func test(c cache.SnapshotCache) {
	result, err := ioutil.ReadFile("/Users/rohit.agrawal/universe/apiproxy/deploy/multitenant/zzz_JSONNET_GENERATED/dev/oregon-dev/mt-shard/apiproxy.yaml")
	if err == nil {
		envoyConfigString := string(result)
		clustersJson := extractData(envoyConfigString, ".cluster_config")
		l.Debugf("*** Clusters: %s", clustersJson)
		var clustersJsonArray []interface{}
		err = json.Unmarshal([]byte(clustersJson), &clustersJsonArray)
		if err != nil {
			l.Debugf("Error occurred while unmarshalling clusters array: %s", err)
		}
		clusters := make([]types.Resource, len(clustersJsonArray))
		for index, clusterJson := range clustersJsonArray {
			clusterJsonString, err := json.Marshal(clusterJson)
			if err != nil {
				l.Debugf("Error occurred while marshalling: %s", err)
			}
			pb, _ := convertClustersJsonToPb(string(clusterJsonString))
			clusters[index] = pb
			l.Debugf("*** PB: %s", pb)
		}
		snapshot := cache.NewSnapshot(
			"1",
			[]types.Resource{}, // endpoints
			clusters,           // clusters
			[]types.Resource{}, // routes
			[]types.Resource{}, // HCM
			[]types.Resource{}, // runtimes
			[]types.Resource{}, // secrets
			//[]types.Resource{}, // extension configs
		)
		l.Debugf("***** !!! DONE !!! *****")
		if err := c.SetSnapshot(nodeID, snapshot); err != nil {
			l.Errorf("Snapshot error %q for %+v", err, snapshot)
			os.Exit(1)
		}
	} else {
		l.Debugf("*** ERROR: %s", err)
	}
}
*/

func main() {
	flag.Parse()
	l.Debugf("Start Initializing xDS Main Server...")

	// Create Snapshot Cache
	// true = ADS Mode
	snapshotCache := cache.NewSnapshotCache(false, cache.IDHash{}, l)
	//test(snapshotCache)

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
