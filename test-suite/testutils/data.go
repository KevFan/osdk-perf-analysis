package testutils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	"os"
	"reflect"
	"sort"
	"time"
)

const (
	Namespace         = "memcached-operator-system"
	tickerInterval    = time.Second
	GoType            = "go/v3"
	AnsibleType       = "ansible"
	HelmType          = "helm"
	DefaultResultsDir = "results"
	OperatorPodLabel  = "control-plane=controller-manager"
)

// GatherMetricsForDuration Gather operator pod metrics for a specific duration
func GatherMetricsForDuration(metricsClient *metricsv.Clientset, tickerDuration time.Duration) []v1beta1.PodMetrics {
	var metrics []v1beta1.PodMetrics

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()
	done := make(chan bool)

	go func() {
		time.Sleep(tickerDuration)
		done <- true
	}()

	for {
		select {
		case <-done:
			return metrics
		case <-ticker.C:
			podMetricsList, err := metricsClient.MetricsV1beta1().PodMetricses(Namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: OperatorPodLabel,
			})
			if err != nil {
				continue
			}

			for _, podMetric := range podMetricsList.Items {
				sortContainersByName(podMetric.Containers)
			}

			metrics = append(metrics, podMetricsList.Items...)
		}
	}
}

// GatherMetricsToChannel Gather Pod metrics and send to channel
func GatherMetricsToChannel(metricsClient *metricsv.Clientset, tickerDuration time.Duration, metricsChannel chan []v1beta1.PodMetrics) {
	metricsChannel <- GatherMetricsForDuration(metricsClient, tickerDuration)
	println("Sent gathered metrics to channel")
}

// sortContainersByName Sort container metrics by name
func sortContainersByName(elems []v1beta1.ContainerMetrics) {
	sort.Slice(elems, func(i, j int) bool {
		return elems[i].Name < elems[j].Name
	})
}

// SaveAsJsonToDir Marshal object to json and save to directory
func SaveAsJsonToDir(dir string, object interface{}) error {
	// Create result directory
	resultsDir := os.Getenv("RESULTS_DIR")
	if resultsDir == "" {
		resultsDir = DefaultResultsDir
	}

	path := fmt.Sprintf("%s/%s", resultsDir, dir)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return err
		}
	}

	// Marshal to json
	if reflect.TypeOf(object).String() != "string" {
		bytes, err := json.MarshalIndent(object, "", "  ")
		if err != nil {
			return err
		}

		return SaveJSONStringToDir(path, string(bytes))
	}

	return SaveJSONStringToDir(path, object.(string))
}

// SaveJSONStringToDir Save JSON string to directory
func SaveJSONStringToDir(path string, jsonString string) error {
	// Create json file
	f, err := os.Create(fmt.Sprintf("%s/%d.json", path, time.Now().UnixMicro()))
	if err != nil {
		return err
	}
	defer f.Close()

	// Write results to file
	_, err = f.WriteString(jsonString)

	return err
}
