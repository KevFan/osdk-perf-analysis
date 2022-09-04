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

// Based from https://github.com/operator-framework/operator-sdk/blob/master/test/e2e/go/suite_test.go

package _go

import (
	"fmt"
	"os"
	"os/exec"
	"osdk-go-perf/testutils"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestPerformance ensures the Go projects built with the SDK tool by using its binary.
func TestPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Operator SDK Performance Suite testing in short mode")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Suite")
}

var (
	tc    testutils.TestContext
	oType string
)

// BeforeSuite run before any specs are run to perform the required actions for all e2e Go tests.
var _ = BeforeSuite(func() {
	By("destroying kind cluster")
	Expect(tc.DeleteKindCluster()).To(Succeed())

	By("creating kind cluster")
	Expect(tc.CreateKindCluster()).To(Succeed())

	var err error

	By("creating a new test context")
	tc, err = testutils.NewTestContext(testutils.BinaryName, "GO111MODULE=on")
	Expect(err).NotTo(HaveOccurred())

	tc.Domain = "example.com"
	tc.Group = "cache"
	tc.Version = "v1alpha1"
	tc.Kind = "Memcached"
	tc.Resources = "memcacheds"
	tc.ProjectName = "memcached-operator"
	tc.Kubectl.Namespace = fmt.Sprintf("%s-system", tc.ProjectName)
	tc.Kubectl.ServiceAccount = fmt.Sprintf("%s-controller-manager", tc.ProjectName)

	oskVersion := tc.GetOSDKVersion()

	By(fmt.Sprintf("cloning OperatorSDK repository: %s", oskVersion))
	Expect(tc.CloneOperatorSDK(oskVersion)).To(Succeed())

	By("getting operator type from env")
	oType = testutils.GetType()
	By(oType)

	By("copying sample to a temporary e2e directory")
	Expect(exec.Command("cp", "-r", fmt.Sprintf("operator-sdk/testdata/%s/memcached-operator", oType), tc.Dir).Run()).To(Succeed())

	By("preparing the prerequisites on cluster")
	tc.InstallPrerequisites()

	By("building the project image")
	err = tc.Make("docker-build", "IMG="+tc.ImageName)
	Expect(err).NotTo(HaveOccurred())

	onKind, err := tc.IsRunningOnKind()
	Expect(err).NotTo(HaveOccurred())
	if onKind {
		By("loading the required images into Kind cluster")
		Expect(tc.LoadImageToKindCluster()).To(Succeed())
	}

	By("installing cert manager bundle")
	Expect(tc.InstallCertManager(false)).To(Succeed())
})

// AfterSuite run after all the specs have run, regardless of whether any tests have failed to ensures that
// all be cleaned up
var _ = AfterSuite(func() {
	By("destroying container image and work dir")
	tc.Destroy()

	// Destroy KIND cluster
	destroyCluster := os.Getenv("DESTROY_CLUSTER")
	if destroyCluster == "true" {
		By("destroying kind cluster")
		Expect(tc.DeleteKindCluster()).To(Succeed())
	}
})
