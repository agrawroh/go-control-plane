package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/rds/stats"
	"github.com/pkg/errors"
	coreV1 "k8s.io/api/core/v1"

	"github.com/envoyproxy/go-control-plane/rds/env"

	"github.com/envoyproxy/go-control-plane/rds/utils"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ConfigMapCacheEntry struct {
	config *coreV1.ConfigMap
}

type ConfigMapCache struct {
	// the cache of the latest config maps, keyed by config map name. When we read
	// from k8s a ConfigMap of the same name but at different version, the entry
	// will be replaced.
	cache map[string]ConfigMapCacheEntry
}

var (
	logger utils.Logger
	// settings contains the config flag values which are either the declared default or the values set by using the
	// environment variables.
	settings       env.Settings
	configMapCache ConfigMapCache
)

func (c *ConfigMapCache) Get(configMapName string) (cm *coreV1.ConfigMap, exists bool) {
	if entry, ok := c.cache[configMapName]; ok {
		return entry.config, true
	}
	return nil, false
}

func (c *ConfigMapCache) Put(name string, config *coreV1.ConfigMap) {
	c.cache[name] = ConfigMapCacheEntry{config}
}

// Initialize Variables
func init() {
	// Initialize Settings Object
	settings = env.NewSettings()

	// Initialize Logger
	logger = utils.Logger{
		Debug: strings.EqualFold("DEBUG", settings.LogLevel),
		Info:  strings.EqualFold("DEBUG", settings.LogLevel) || strings.EqualFold("INFO", settings.LogLevel),
	}

	if s, err := json.Marshal(&settings); err == nil {
		logger.Infof("settings: %s", string(s))
	}

	// Initialize the ConfigMap cache
	configMapCache = ConfigMapCache{map[string]ConfigMapCacheEntry{}}
}

/**
 * initKubernetesClient initializes the kubernetes client which is used to fetch the ConfigMap(s).
 */
func initKubernetesClient() *kubernetes.Clientset {
	logger.Debugf("start initializing the kubernetes client...")

	// Create In-Cluster Config
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("error occurred while creating in-cluster config", err.Error())
		panic(err.Error())
	}
	clientConfig := &rest.Config{
		Host:            inClusterConfig.Host,
		TLSClientConfig: inClusterConfig.TLSClientConfig,
		BearerToken:     inClusterConfig.BearerToken,
		BearerTokenFile: inClusterConfig.BearerTokenFile,
		// Current QPS & Burst are too low which results in the client-side throttling and hence we are increasing these
		// limits to see if it has any positive impact on the performance while fetching the ConfigMap(s) in parallel.
		QPS:   150,
		Burst: 200,
	}

	// Create ClientSet
	clientSet, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		logger.Errorf("error occurred while creating the new kubernetes client-set", err.Error())
		panic(err.Error())
	}

	// Sanity Check(s)
	namespace := settings.ConfigMapNamespace
	timeoutSeconds := int64(10)
	if _, err = clientSet.CoreV1().ConfigMaps(namespace).List(context.TODO(), metaV1.ListOptions{Limit: 1, TimeoutSeconds: &timeoutSeconds}); err != nil {
		logger.Errorf("error occurred while listing ConfigMap(s). Please check k8s permissions", err.Error())
		panic(err.Error())
	}

	// Return ClientSet
	return clientSet
}

/*
 * benchmarkSequentialReads read all the ConfigMap(s) present in the given namespace sequentially and measure the total
 *                          time it took.
 */
func benchmarkSequentialReads(clientSet *kubernetes.Clientset) {
	// Get the namespace from the settings
	namespace := settings.ConfigMapNamespace
	// Get the list of ConfigMaps
	configMapsList, _ := clientSet.CoreV1().ConfigMaps(namespace).List(context.TODO(), metaV1.ListOptions{})
	numItems := len(configMapsList.Items)
	resSequential := make([]string, numItems)

	nanoSecBefore := time.Now().UnixNano()
	for index, configMapItem := range configMapsList.Items {
		configMap, _ := getConfigMap(clientSet, namespace, configMapItem.Name)
		for key := range configMap.Data {
			resSequential[index] = key
		}
	}
	nanoSecAfter := time.Now().UnixNano()

	logger.Infof("nanoseconds Elapsed (Sequential): %d", nanoSecAfter-nanoSecBefore)
	logger.Infof("results: %s", resSequential)
}

/*
 * benchmarkParallelReads read all the ConfigMap(s) present in the given namespace in parallel and measure the total
 *                        time it took.
 */
func benchmarkParallelReads(clientSet *kubernetes.Clientset) {
	// Get the namespace from the settings
	namespace := settings.ConfigMapNamespace
	// Get the list of ConfigMaps
	configMapsListParallel, _ := clientSet.CoreV1().ConfigMaps(namespace).List(context.TODO(), metaV1.ListOptions{})
	numItemsParallel := len(configMapsListParallel.Items)
	semaphore := make(chan struct{}, numItemsParallel)
	resParallel := make([]string, numItemsParallel)

	nanoSecBeforeStartParallel := time.Now().UnixNano()
	for index, configMapItem := range configMapsListParallel.Items {
		configMapItem := configMapItem
		go func(index int) {
			// Read ConfigMap
			configMap, _ := clientSet.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapItem.Name, metaV1.GetOptions{})
			for key := range configMap.Data {
				// Store the key of ConfigMap as the result
				resParallel[index] = key
			}
			semaphore <- struct{}{}
		}(index)
	}

	// Wait for all the goroutines to finish
	for i := 0; i < numItemsParallel; i++ {
		<-semaphore
	}

	nanoSecAfterParallelFinish := time.Now().UnixNano()
	logger.Infof("nanoseconds Elapsed (Parallel): %d", nanoSecAfterParallelFinish-nanoSecBeforeStartParallel)
	logger.Infof("results: %s", resParallel)
}

/**
 * getConfigMap get the ConfigMap from either the local in-memory cache that we maintain or fetch it from the K8S.
 */
func getConfigMap(clientSet *kubernetes.Clientset, k8sNamespace, configMapName string) (*coreV1.ConfigMap, error) {
	configMap, cacheHit := configMapCache.Get(configMapName)
	if configMap == nil {
		cm, err := clientSet.CoreV1().ConfigMaps(k8sNamespace).Get(context.TODO(), configMapName, metaV1.GetOptions{})
		if err != nil {
			stats.RecordConfigMapFetchError(configMapName)
			return nil, errors.Wrap(err, fmt.Sprintf("error occurred while fetching ConfigMap: %s", configMapName))
		}
		configMap = cm
	}
	// In case of the cache miss, store the ConfigMap fetched in the cache for faster subsequent lookups
	if !cacheHit {
		configMapCache.Put(configMapName, configMap)
	}
	return configMap, nil
}

/**
 * main bootstraps the RDS server and sets up all the dependencies like k8s watcher, snapshot updater etc.
 */
func main() {
	flag.Parse()
	logger.Infof("start initializing the main server...")

	// Initialize Kubernetes Client
	clientSet := initKubernetesClient()

	logger.Infof("sleep for a minute to let all ConfigMaps get synced...")
	time.Sleep(time.Duration(settings.SyncDelayTimeSeconds) * time.Second)
	logger.Infof("start benchmarking...")

	// ***** Benchmark Sequential *****
	// Cleanup the existing in-memory cache which stores the already fetched ConfigMap(s) before we start benchmarking
	configMapCache = ConfigMapCache{map[string]ConfigMapCacheEntry{}}
	benchmarkSequentialReads(clientSet)

	// ***** Benchmark Parallel *****
	// Cleanup the existing in-memory cache which stores the already fetched ConfigMap(s) before we start benchmarking
	configMapCache = ConfigMapCache{map[string]ConfigMapCacheEntry{}}
	benchmarkParallelReads(clientSet)

	// Prevent the server from getting killed
	select {}
}
