// Copyright 2020 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Modified from https://github.com/kubernetes-sigs/kubebuilder/tree/39224f0/test/e2e/v3

// Based on https://github.com/operator-framework/operator-sdk/blob/master/test/e2e/go/cluster_test.go

package _go

import (
	"context"
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	"os"
	"osdk-go-perf/testutils"
	"path/filepath"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"strings"
	"time"

	kbutil "sigs.k8s.io/kubebuilder/v3/pkg/plugin/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type Timings struct {
	TimeForPodsRunning int64 `json:"timeForPodsRunning"`
	TimeForPodsDeleted int64 `json:"timeForPodsDeleted"`
}

const (
	CRNameInYaml           = "memcached-sample"
	NumberOfCRToCreate     = 15
	OperatorDeploymentName = "memcached-operator-controller-manager"
)

var _ = Describe("operator-sdk", func() {
	var controllerPodName string

	Context("built with operator-sdk", func() {

		BeforeEach(func() {
			By("deploying project on the cluster")
			Expect(tc.Make("deploy", "IMG="+tc.ImageName)).To(Succeed())
		})

		It("should run correctly in a cluster", func() {
			// Ansible and Helm defaults to number of logical CPUs usable by the current process
			// Go defaults to 1
			maxConcurrentReconcile := os.Getenv("MAX_CONCURRENT_RECONCILE")
			if maxConcurrentReconcile != "" && (oType == testutils.HelmType || oType == testutils.AnsibleType) {
				By("set max concurrent reconciles")
				err := tc.JSONPatchDeployment(OperatorDeploymentName, testutils.Namespace,
					fmt.Sprintf(`[{"op": "add", "path": "/spec/template/spec/containers/1/args/-", "value": "--max-concurrent-reconciles=%s" }]`, maxConcurrentReconcile))
				Expect(err).NotTo(HaveOccurred())
			} else if oType == testutils.GoType {
				maxConcurrentReconcile = "1" // TODO - Get from prometheus metric - Go default is one and can't be changed via container flag
			} else {
				maxConcurrentReconcile = "4" // TODO - Get from prometheus metric - Was the default on the server used to test
			}

			cpuLimit := os.Getenv("CPU_LIMIT")
			isDefaultCpuLimit := false
			if cpuLimit != "" {
				By("setting cpu limit on operator deployment")
				err := tc.PatchDeployment(OperatorDeploymentName, testutils.Namespace,
					fmt.Sprintf(`{"spec":{"template": {"spec":{"containers":[{"name":"manager","resources":{"limits":{"cpu": "%s"}}}]}}}}`, cpuLimit))
				Expect(err).NotTo(HaveOccurred())
			} else {
				cpuLim, err := tc.Kubectl.Get(true, "deployment", OperatorDeploymentName, "-o", "jsonpath={.spec.template.spec.containers[1].resources.limits.cpu}")
				Expect(err).NotTo(HaveOccurred())
				cpuLimit = cpuLim
				isDefaultCpuLimit = true
			}

			memoryLimit := os.Getenv("MEMORY_LIMIT")
			isDefaultMemoryLimit := false
			if memoryLimit != "" {
				By("setting memory limit on operator deployment")
				err := tc.PatchDeployment(OperatorDeploymentName, testutils.Namespace,
					fmt.Sprintf(`{"spec":{"template": {"spec":{"containers":[{"name":"manager","resources":{"limits":{"memory": "%s"}}}]}}}}`, memoryLimit))
				Expect(err).NotTo(HaveOccurred())
			} else {
				memLim, err := tc.Kubectl.Get(true, "deployment", OperatorDeploymentName, "-o", "jsonpath={.spec.template.spec.containers[1].resources.limits.memory}")
				Expect(err).NotTo(HaveOccurred())
				memoryLimit = memLim
				isDefaultMemoryLimit = true
			}

			resultsDir := fmt.Sprintf("%s-%s-%s-%s", strings.Split(oType, "/")[0], maxConcurrentReconcile, memoryLimit, cpuLimit)
			if isDefaultMemoryLimit && isDefaultCpuLimit {
				resultsDir = fmt.Sprintf("%s-D", resultsDir)
			}

			By("checking if the Operator project Pod is running")
			verifyControllerUp := func() error {
				// Get the controller-manager pod name
				podOutput, err := tc.Kubectl.Get(
					true,
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}")
				if err != nil {
					return fmt.Errorf("could not get pods: %v", err)
				}
				podNames := kbutil.GetNonEmptyLines(podOutput)
				if len(podNames) != 1 {
					return fmt.Errorf("expecting 1 pod, have %d", len(podNames))
				}
				controllerPodName = podNames[0]
				if !strings.Contains(controllerPodName, "controller-manager") {
					return fmt.Errorf("expecting pod name %q to contain %q", controllerPodName, "controller-manager")
				}

				// Ensure the controller-manager Pod is running.
				status, err := tc.Kubectl.Get(
					true,
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}")
				if err != nil {
					return fmt.Errorf("failed to get pod status for %q: %v", controllerPodName, err)
				}
				if status != "Running" {
					return fmt.Errorf("controller pod in %s status", status)
				}
				return nil
			}
			Eventually(verifyControllerUp, 2*time.Minute, time.Second).Should(Succeed())

			By("ensuring the created ServiceMonitor for the manager")
			_, err := tc.Kubectl.Get(
				true,
				"ServiceMonitor",
				fmt.Sprintf("%s-controller-manager-metrics-monitor", tc.ProjectName))
			Expect(err).NotTo(HaveOccurred())

			By("ensuring the created metrics Service for the manager")
			_, err = tc.Kubectl.Get(
				true,
				"Service",
				fmt.Sprintf("%s-controller-manager-metrics-service", tc.ProjectName))
			Expect(err).NotTo(HaveOccurred())

			By("wait until metrics available")
			restConfig := controllerruntime.GetConfigOrDie()
			metricsClient, err := metricsv.NewForConfig(restConfig)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				podMetricsList, err := metricsClient.MetricsV1beta1().PodMetricses(testutils.Namespace).List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					return err
				}
				if len(podMetricsList.Items) != 1 {
					return errors.New("metrics not available yet")
				}

				return nil
			}, 3*time.Minute, time.Second).Should(Succeed())
			By("metrics available from pods")

			// Block to gather baseline metrics for 2 minutes once metrics are available
			By("gathering baseline cpu and memory metrics")
			metricsBefore := testutils.GatherMetricsForDuration(metricsClient, 2*time.Minute)

			// Unblocking call to gather metrics during creation / processing of CRs
			metricsChannel := make(chan []v1beta1.PodMetrics, 2)
			go func(chan []v1beta1.PodMetrics) {
				By("start gathering metrics for load creation")
				testutils.GatherMetricsToChannel(metricsClient, 3*time.Minute, metricsChannel)
			}(metricsChannel)

			By("creating CR instances")
			// currently controller-runtime doesn't provide a readiness probe, we retry a few times
			// we can change it to probe the readiness endpoint after CR supports it.
			sampleFile := filepath.Join("config", "samples",
				fmt.Sprintf("%s_%s_%s.yaml", tc.Group, tc.Version, strings.ToLower(tc.Kind)))

			// For helm, default CR is set to 3 replica count - set to 1
			if oType == testutils.HelmType {
				err = kbutil.ReplaceInFile(fmt.Sprintf("%s/%s", tc.Dir, sampleFile), "3", "1")
				Expect(err).NotTo(HaveOccurred())
			}

			timeBeforeCreatingCR := time.Now()
			for i := 0; i < NumberOfCRToCreate; i++ {
				initialName := CRNameInYaml
				if i > 0 {
					initialName = fmt.Sprintf("%v%02d", CRNameInYaml, i-1)
				}
				err = kbutil.ReplaceInFile(fmt.Sprintf("%s/%s", tc.Dir, sampleFile),
					initialName,
					fmt.Sprintf("%v%02d", CRNameInYaml, i),
				)

				Expect(err).NotTo(HaveOccurred())

				Eventually(func() error {
					_, err = tc.Kubectl.Apply(true, "-f", sampleFile)
					return err
				}, time.Minute, time.Second).Should(Succeed())
			}

			By("measuring time for all pods to be running")
			getPodStatus := func() error {
				var status string
				// Helm has different labels
				if oType == testutils.HelmType {
					status, err = tc.Kubectl.Get(true, "pods", "-l", "app.kubernetes.io/name=memcached", "-o", "jsonpath={.items[*].status.phase}")
				} else {
					status, err = tc.Kubectl.Get(true, "pods", "-l", "app=memcached", "-o", "jsonpath={.items[*].status.phase}")
				}
				if err == nil && strings.TrimSpace(status) == "" {
					return errors.New("empty status, continue")
				}
				nodes := strings.Split(status, " ")

				for i := 0; i < len(nodes); i++ {
					if nodes[i] != "Running" {
						return errors.New("not all pods are running yet")
					}
				}

				if len(nodes) != NumberOfCRToCreate {
					return errors.New("not reached the number of pods yet")
				}

				return nil

			}
			Eventually(getPodStatus, 15*time.Minute, time.Second).Should(Succeed())
			timeForPodsRunning := time.Now().Sub(timeBeforeCreatingCR).Milliseconds()
			By(fmt.Sprintf("time for all pods to be running: %d", timeForPodsRunning))

			// Eventually metrics from during the run will be returned
			Eventually(func() error {
				if len(metricsChannel) == 0 {
					return errors.New("waiting for metrics to be available in channel")
				}
				return nil
			}, 5*time.Minute, time.Second).Should(Succeed())

			By("save all CRs in operator namespace")
			status, err := tc.Kubectl.Get(true, "memcacheds", "-o", "json")
			Expect(err).NotTo(HaveOccurred())
			Expect(testutils.SaveAsJsonToDir(fmt.Sprintf("%s/memcacheds", resultsDir), status)).To(Succeed())

			By("save all pods in operator namespace")
			status, err = tc.Kubectl.Get(true, "pods", "-o", "json")
			Expect(err).NotTo(HaveOccurred())
			Expect(testutils.SaveAsJsonToDir(fmt.Sprintf("%s/pods", resultsDir), status)).To(Succeed())

			if oType == testutils.HelmType {
				By("save all statefulsets in operator namespace for helm type")
				status, err = tc.Kubectl.Get(true, "statefulsets", "-o", "json")
				Expect(err).NotTo(HaveOccurred())
				Expect(testutils.SaveAsJsonToDir(fmt.Sprintf("%s/statefulsets", resultsDir), status)).To(Succeed())
			}

			By("save all deployments in operator namespace")
			status, err = tc.Kubectl.Get(true, "deployments", "-o", "json")
			Expect(err).NotTo(HaveOccurred())
			Expect(testutils.SaveAsJsonToDir(fmt.Sprintf("%s/deployments", resultsDir), status)).To(Succeed())

			go func(chan []v1beta1.PodMetrics) {
				By("start gathering metrics for before deleting CRs")
				testutils.GatherMetricsToChannel(metricsClient, 5*time.Minute, metricsChannel)
			}(metricsChannel)

			By("deleting CR instances")
			timeBeforeDeletion := time.Now()
			for i := 0; i < NumberOfCRToCreate; i++ {
				initialName := fmt.Sprintf("%v%02d", CRNameInYaml, i)

				Eventually(func() error {
					_, err = tc.Kubectl.Delete(true, "memcacheds", initialName)
					return err
				}, time.Minute, time.Second).Should(Succeed())
			}

			getAllPodStatus := func() error {
				if oType == testutils.HelmType {
					status, err = tc.Kubectl.Get(true, "pods", "-l", "app.kubernetes.io/name=memcached", "-o", "jsonpath={.items[*]}")
				} else {
					status, err = tc.Kubectl.Get(true, "pods", "-l", "app=memcached", "-o", "jsonpath={.items[*]}")
				}
				if err == nil && strings.TrimSpace(status) == "" {
					return nil
				}

				return errors.New("waiting for pods to be terminated")
			}
			Eventually(getAllPodStatus, 5*time.Minute, time.Second).Should(Succeed())

			timeForPodsDeleted := time.Now().Sub(timeBeforeDeletion).Milliseconds()
			By(fmt.Sprintf("time for all pods to be deleted: %d", timeForPodsDeleted))

			By("saving timings to file")
			timings := Timings{
				TimeForPodsRunning: timeForPodsRunning,
				TimeForPodsDeleted: timeForPodsDeleted,
			}
			Expect(testutils.SaveAsJsonToDir(fmt.Sprintf("%s/timings", resultsDir), timings)).To(Succeed())

			// Save all the metrics to file
			// Eventually metrics from during the run will be returned
			Eventually(func() error {
				if len(metricsChannel) == 1 {
					return errors.New("waiting for metrics to be available in channel")
				}
				return nil
			}, 10*time.Minute, time.Second).Should(Succeed())
			allMetrics := append(append(metricsBefore, <-metricsChannel...), <-metricsChannel...)
			Expect(testutils.SaveAsJsonToDir(fmt.Sprintf("%s/cpuMemory", resultsDir), allMetrics)).To(Succeed())
		})
	})
})
