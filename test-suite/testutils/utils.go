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

// Based on https://github.com/operator-framework/operator-sdk/blob/master/internal/testutils/utils.go with minor changes
// Unable to import as Go module due to being in an internal directory

package testutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kbtestutils "sigs.k8s.io/kubebuilder/v3/test/e2e/utils"
)

const (
	BinaryName                  = "operator-sdk"
	MetricsServerYAMLPath       = "../templates/metrics-server-insecure.yaml"
	AdditionalScrapeConfigsPath = "../templates/additional-scrape-configs.yaml"
	PrometheusInstancePath      = "../templates/prometheus.yaml"
	KubeStateMetricsVersion     = "v2.5.0"
	DefaultOSDKTag              = "v1.20.0"
	OperatorSDKGitUrl           = "https://github.com/operator-framework/operator-sdk.git"
)

// TestContext wraps kubebuilder's e2e TestContext.
type TestContext struct {
	*kbtestutils.TestContext
	// BundleImageName store the image to use to build the bundle
	BundleImageName string
	// ProjectName store the project name
	ProjectName string
	// isPrometheusManagedBySuite is true when the suite tests is installing/uninstalling the Prometheus
	isPrometheusManagedBySuite bool
	// isOLMManagedBySuite is true when the suite tests is installing/uninstalling the OLM
	isOLMManagedBySuite bool
}

// NewTestContext returns a TestContext containing a new kubebuilder TestContext.
// Construct if your environment is connected to a live cluster, ex. for e2e tests.
func NewTestContext(binaryName string, env ...string) (tc TestContext, err error) {
	if tc.TestContext, err = kbtestutils.NewTestContext(binaryName, env...); err != nil {
		return tc, err
	}
	tc.ProjectName = strings.ToLower(filepath.Base(tc.Dir))
	tc.ImageName = makeImageName(tc.ProjectName)
	tc.BundleImageName = makeBundleImageName(tc.ProjectName)
	tc.isOLMManagedBySuite = true
	tc.isPrometheusManagedBySuite = true
	return tc, nil
}

func makeImageName(projectName string) string {
	return fmt.Sprintf("quay.io/example/%s:v0.0.1", projectName)
}

func makeBundleImageName(projectName string) string {
	return fmt.Sprintf("quay.io/example/%s-bundle:v0.0.1", projectName)
}

// LoadImageToKindClusterWithName loads a local docker image with the name informed to the kind cluster
func (tc TestContext) LoadImageToKindClusterWithName(image string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", "--name", cluster, image}
	cmd := exec.Command("kind", kindOptions...)
	_, err := tc.Run(cmd)
	return err
}

// InstallPrerequisites will install OLM and Prometheus
// when the cluster kind is Kind and when they are not present on the Cluster
func (tc TestContext) InstallPrerequisites() {
	By("checking API resources applied on Cluster")
	output, err := tc.Kubectl.Command("api-resources")
	Expect(err).NotTo(HaveOccurred())
	if strings.Contains(output, "servicemonitors") {
		tc.isPrometheusManagedBySuite = false
	}
	if strings.Contains(output, "clusterserviceversions") {
		tc.isOLMManagedBySuite = false
	}

	if tc.isPrometheusManagedBySuite {
		By("installing Prometheus")
		Expect(tc.InstallPrometheusOperManager()).To(Succeed())

		By("ensuring provisioned Prometheus Manager Service")
		Eventually(func() error {
			_, err := tc.Kubectl.Get(
				false,
				"Service", "prometheus-operator")
			return err
		}, 3*time.Minute, time.Second).Should(Succeed())
	}

	By("installing metrics service")
	_, err = tc.Kubectl.Apply(false, "-f", MetricsServerYAMLPath)
	Expect(err).NotTo(HaveOccurred())

	// Install a prometheus instance and kube state metrics to scrape cluster and operator metrics
	if os.Getenv("SCRAPE_METRICS") == "true" {
		By("prometheus instance")
		_, err = tc.Kubectl.Apply(false, "-f", AdditionalScrapeConfigsPath)
		Expect(err).NotTo(HaveOccurred())
		_, err = tc.Kubectl.Apply(false, "-f", PrometheusInstancePath)
		Expect(err).NotTo(HaveOccurred())

		By("installing kube-state-metrics")
		Expect(tc.InstallKubeStateMetrics()).To(Succeed())
	}
}

// IsRunningOnKind returns true when the tests are executed in a Kind Cluster
func (tc TestContext) IsRunningOnKind() (bool, error) {
	kubectx, err := tc.Kubectl.Command("config", "current-context")
	if err != nil {
		return false, err
	}
	return strings.Contains(kubectx, "kind"), nil
}

// UninstallPrerequisites will uninstall all prerequisites installed via InstallPrerequisites()
func (tc TestContext) UninstallPrerequisites() {
	if tc.isPrometheusManagedBySuite {
		By("uninstalling Prometheus")
		tc.UninstallPrometheusOperManager()
	}
}

// JSONPatchDeployment Patch a deployment with the --type=json flag
func (tc TestContext) JSONPatchDeployment(deploymentName, nameSpace, jsonPatch string) error {
	_, err := tc.Kubectl.Command("patch", "deployments", deploymentName, "-n", nameSpace, "--type=json", "-p",
		jsonPatch)

	return err
}

// PatchDeployment Patch a deployment with the -p flag
func (tc TestContext) PatchDeployment(deploymentName, nameSpace, jsonPatch string) error {
	_, err := tc.Kubectl.Command("patch", "deployments", deploymentName, "-n", nameSpace, "-p",
		jsonPatch)

	return err
}

// InstallKubeStateMetrics Install Kube-state-metrics
func (tc TestContext) InstallKubeStateMetrics() error {
	_, err := tc.Kubectl.Apply(false, "-f", fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kube-state-metrics/%s/examples/standard/cluster-role-binding.yaml", KubeStateMetricsVersion))
	if err != nil {
		return err
	}

	_, err = tc.Kubectl.Apply(false, "-f", fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kube-state-metrics/%s/examples/standard/cluster-role.yaml", KubeStateMetricsVersion))
	if err != nil {
		return err
	}
	_, err = tc.Kubectl.Apply(false, "-f", fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kube-state-metrics/%s/examples/standard/deployment.yaml", KubeStateMetricsVersion))
	if err != nil {
		return err
	}
	_, err = tc.Kubectl.Apply(false, "-f", fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kube-state-metrics/%s/examples/standard/service-account.yaml", KubeStateMetricsVersion))
	if err != nil {
		return err
	}
	_, err = tc.Kubectl.Apply(false, "-f", fmt.Sprintf("https://raw.githubusercontent.com/kubernetes/kube-state-metrics/%s/examples/standard/service.yaml", KubeStateMetricsVersion))

	return err
}

// CreateKindCluster Create local kind cluster
func (tc TestContext) CreateKindCluster() error {
	return exec.Command("kind", "create", "cluster").Run()
}

// DeleteKindCluster delete local kind cluster
func (tc TestContext) DeleteKindCluster() error {
	return exec.Command("kind", "delete", "cluster").Run()
}

func GetType() string {
	oType := os.Getenv("TYPE")

	if oType == AnsibleType {
		return AnsibleType
	} else if oType == HelmType {
		return HelmType
	}

	return GoType
}

// CloneOperatorSDK clone operator sdk at a specific tag
func (tc TestContext) CloneOperatorSDK(oskVersion string) error {
	if err := exec.Command("rm", "-rf", "operator-sdk").Run(); err != nil {
		return err
	}

	return exec.Command("git", "clone", OperatorSDKGitUrl, "--branch", oskVersion).Run()
}

// GetOSDKVersion get the operator sdk version from environment, otherwise use default
func (tc TestContext) GetOSDKVersion() string {
	oSdkVersion := os.Getenv("OSDKVersion")

	if oSdkVersion != "" {
		return oSdkVersion
	}

	return DefaultOSDKTag
}
