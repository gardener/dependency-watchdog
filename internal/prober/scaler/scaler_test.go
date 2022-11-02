// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package scaler

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"

	kind "github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	kindTestEnv      kind.KindCluster
	scalerTestLogger = log.FromContext(context.Background()).WithName("scalerTestLogger")
)

const (
	namespace                = "default"
	deploymentImageName      = "nginx:1.14.2"
	ignoreScaleAnnotationKey = "dependency-watchdog.gardener.cloud/ignore-scaling"
)

func setUpScalerEnvTests(g *WithT) func(g *WithT) {
	var err error
	kindTestEnv, err = kind.CreateKindCluster(kind.KindConfig{Name: "test"})
	g.Expect(err).To(BeNil())
	return func(g *WithT) {
		err := kindTestEnv.Delete()
		g.Expect(err).To(BeNil())
	}
}

func createScaler(g *WithT, probeCfg *papi.Config) Scaler {
	cfg := kindTestEnv.GetRestConfig()
	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).To(BeNil())
	ds := NewScaler(namespace, probeCfg, kindTestEnv.GetClient(), scalesGetter, scalerTestLogger,
		withDependentResourceCheckTimeout(1*time.Minute), withDependentResourceCheckInterval(1*time.Second))
	return ds
}

func TestScalerSuite(t *testing.T) {
	g := NewWithT(t)
	tearDownScalerTests := setUpScalerEnvTests(g)
	defer tearDownScalerTests(g)
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		//{"test updateResourceAndScale returns an error", testDoScaleReturnsError},
		//{"test scaling when KCM deployment(shouldExist is true in resourceInfo) is not found", testScalingWhenKCMDeploymentNotFound},
		//{"test scaling when CA deployment(shouldExist is false in resourceInfo) is not found", testScalingWhenCADeploymentNotFound},
		{"test scale down then scale up", testScaleDownThenScaleUp},
		//{"test scale up should not happen if current replica count is positive", testScaleUpShouldNotHappenIfCurrentReplicaCountIsPositive},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
		err := kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func testScaleDownThenScaleUp(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createScaler(g, probeCfg)
	table := []struct {
		mcmReplicas                       int32
		kcmReplicas                       int32
		caReplicas                        int32
		expectedScaledUpMCMReplicas       int32
		expectedScaledUpKCMReplicas       int32
		expectedScaledUpCAReplicas        int32
		applyIgnoreScalingAnnotationOnKCM bool
	}{
		{0, 0, 0, 1, 1, 1, false},
		{1, 1, 1, 1, 1, 1, false},
		{2, 2, 2, 2, 2, 2, false},
		{2, 2, 2, 2, 2, 2, true},
		{0, 1, 2, 1, 1, 2, false},
		{0, 2, 2, 1, 2, 2, true},
		{2, 1, 1, 2, 1, 1, true},
		{2, 0, 2, 2, 0, 2, true},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
		if entry.applyIgnoreScalingAnnotationOnKCM {
			createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, map[string]string{ignoreScaleAnnotationKey: "true"})
		} else {
			createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)
		}

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmObjectRef.Name, entry.mcmReplicas) }, 1*time.Minute, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, caObjectRef.Name, entry.caReplicas) }, 1*time.Minute, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, kcmObjectRef.Name, entry.kcmReplicas) }, 1*time.Minute, time.Second).Should(BeTrue())

		err := ds.ScaleDown(context.Background())
		g.Expect(err).To(BeNil())
		matchSpecReplicas(g, namespace, caObjectRef.Name, 0)
		matchStatusReplicas(g, namespace, mcmObjectRef.Name, 0)
		if entry.applyIgnoreScalingAnnotationOnKCM {
			matchStatusReplicas(g, namespace, kcmObjectRef.Name, entry.kcmReplicas)
		} else {
			matchStatusReplicas(g, namespace, kcmObjectRef.Name, 0)
		}

		err = ds.ScaleUp(context.Background())
		g.Expect(err).To(BeNil())
		matchSpecReplicas(g, namespace, mcmObjectRef.Name, entry.expectedScaledUpMCMReplicas)
		matchStatusReplicas(g, namespace, caObjectRef.Name, entry.expectedScaledUpCAReplicas)
		if entry.applyIgnoreScalingAnnotationOnKCM {
			matchStatusReplicas(g, namespace, kcmObjectRef.Name, entry.kcmReplicas)
		} else {
			matchStatusReplicas(g, namespace, kcmObjectRef.Name, entry.expectedScaledUpKCMReplicas)
		}

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func testScalingWhenKCMDeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createScaler(g, probeCfg)
	table := []struct {
		mcmReplicas               int32
		caReplicas                int32
		expectedScaledMCMReplicas int32
		expectedScaledCAReplicas  int32
		scalingFn                 func(context.Context) error
	}{
		{0, 0, 0, 1, ds.ScaleUp},
		{2, 2, 0, 2, ds.ScaleDown},
	}
	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmObjectRef.Name, entry.mcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, caObjectRef.Name, entry.caReplicas) }, 10*time.Second, time.Second).Should(BeTrue())

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("\"" + kcmObjectRef.Name + "\" not found"))
		matchSpecReplicas(g, namespace, mcmObjectRef.Name, entry.expectedScaledMCMReplicas)
		matchSpecReplicas(g, namespace, caObjectRef.Name, entry.expectedScaledCAReplicas)
		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func testScalingWhenCADeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createScaler(g, probeCfg)
	table := []struct {
		mcmReplicas               int32
		kcmReplicas               int32
		expectedScaledMCMReplicas int32
		expectedScaledCAReplicas  int32
		scalingFn                 func(context.Context) error
	}{
		{0, 0, 1, 1, ds.ScaleUp},
		{2, 2, 0, 0, ds.ScaleDown},
	}
	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmObjectRef.Name, entry.mcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, kcmObjectRef.Name, entry.kcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())

		err := entry.scalingFn(context.Background())
		g.Expect(err).To(BeNil())
		matchSpecReplicas(g, namespace, mcmObjectRef.Name, entry.expectedScaledMCMReplicas)
		matchSpecReplicas(g, namespace, kcmObjectRef.Name, entry.expectedScaledCAReplicas)
		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

// add one more test for invalid kind during scaleup.
func testDoScaleReturnsError(t *testing.T) {
	g := NewWithT(t)
	faultyProbeCfg := createProbeConfig(nil)
	faultyProbeCfg.DependentResourceInfos[2].Ref.Kind = "Depoyment"
	ds1 := createScaler(g, faultyProbeCfg)
	timeout := time.Nanosecond
	probeCfg := createProbeConfig(&timeout)
	ds2 := createScaler(g, probeCfg)
	table := []struct {
		mcmReplicas               int32
		kcmReplicas               int32
		caReplicas                int32
		expectedScaledMCMReplicas int32
		expectedScaledKCMReplicas int32
		expectedScaledCAReplicas  int32
		scalingFn                 func(context.Context) error
		errorString               string
	}{
		{0, 0, 0, 0, 0, 0, ds1.ScaleUp, "no matches for kind \"Depoyment\" in version \"apps/v1\""},
		{2, 2, 2, 0, 0, 2, ds1.ScaleDown, "no matches for kind \"Depoyment\" in version \"apps/v1\""},
		{0, 0, 0, 0, 0, 0, ds2.ScaleUp, "context deadline exceeded"},
		{1, 1, 1, 1, 1, 1, ds2.ScaleDown, "context deadline exceeded"},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmObjectRef.Name, entry.mcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, caObjectRef.Name, entry.caReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, kcmObjectRef.Name, entry.kcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring(entry.errorString))
		matchStatusReplicas(g, namespace, caObjectRef.Name, entry.expectedScaledCAReplicas)
		matchStatusReplicas(g, namespace, kcmObjectRef.Name, entry.expectedScaledKCMReplicas)
		matchStatusReplicas(g, namespace, mcmObjectRef.Name, entry.expectedScaledMCMReplicas)
		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

// utility methods to be used by tests
// ------------------------------------------------------------------------------------------------------------------

func createDeployment(g *WithT, namespace, name, deploymentImageName string, replicas int32, annotations map[string]string) {
	err := kindTestEnv.CreateDeployment(name, namespace, deploymentImageName, replicas, annotations)
	g.Expect(err).To(BeNil())
}

func checkIfDeploymentReady(namespace, name string, replicas int32) bool {
	deploy, err := kindTestEnv.GetDeployment(namespace, name)
	var podList corev1.PodList
	err = kindTestEnv.GetClient().List(context.Background(), &podList, &client.ListOptions{Namespace: namespace})
	if err != nil || deploy.Status.ReadyReplicas != replicas {
		return false
	}
	return true
}

func matchSpecReplicas(g *WithT, namespace string, name string, expectedReplicas int32) {
	deploy, err := kindTestEnv.GetDeployment(namespace, name)
	g.Expect(err).To(BeNil())
	g.Expect(deploy).ToNot(BeNil())
	g.Expect(*deploy.Spec.Replicas).Should(Equal(expectedReplicas))
}

func matchStatusReplicas(g *WithT, namespace string, name string, expectedReplicas int32) {
	deploy, err := kindTestEnv.GetDeployment(namespace, name)
	g.Expect(err).To(BeNil())
	g.Expect(deploy).ToNot(BeNil())
	g.Expect(deploy.Status.ReadyReplicas).Should(Equal(expectedReplicas))
}

func createProbeConfig(timeout *time.Duration) *papi.Config {
	dependentResourceInfos := createDepResourceInfoArray(timeout)
	return &papi.Config{DependentResourceInfos: dependentResourceInfos}
}

func createDepResourceInfoArray(timeout *time.Duration) []papi.DependentResourceInfo {
	var dependentResourceInfos []papi.DependentResourceInfo
	dependentResourceInfos = append(dependentResourceInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 2, 0, timeout, pointer.Duration(0*time.Second), true))
	dependentResourceInfos = append(dependentResourceInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 1, 0, timeout, pointer.Duration(0*time.Second), true))
	dependentResourceInfos = append(dependentResourceInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 0, 1, timeout, pointer.Duration(0*time.Second), false))
	return dependentResourceInfos
}
