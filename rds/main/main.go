package main

import (
	"context"
	"encoding/json"
	"flag"
	"strings"
	"time"

	"github.com/envoyproxy/go-control-plane/rds/env"

	"github.com/envoyproxy/go-control-plane/rds/utils"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	logger utils.Logger
	// settings contains the config flag values which are either the declared default or the values set by using the
	// environment variables.
	settings env.Settings
)

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

	// Create ClientSet
	clientSet, err := kubernetes.NewForConfig(inClusterConfig)
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
	// Get the namespace from the settings
	namespace := settings.ConfigMapNamespace
	// Get the list of ConfigMaps
	configMapsList, _ := clientSet.CoreV1().ConfigMaps(namespace).List(context.TODO(), metaV1.ListOptions{})
	numItems := len(configMapsList.Items)
	resSequential := make([]string, numItems)

	nanoSecBefore := time.Now().UnixNano()
	for index, configMapItem := range configMapsList.Items {
		configMap, _ := clientSet.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapItem.Name, metaV1.GetOptions{})
		for key := range configMap.Data {
			resSequential[index] = key
		}
	}
	nanoSecAfter := time.Now().UnixNano()

	logger.Infof("nanoseconds Elapsed (Sequential): %d", nanoSecAfter-nanoSecBefore)
	logger.Infof("results: %s", resSequential)

	// ***** Benchmark Parallel *****
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
