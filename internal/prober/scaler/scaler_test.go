// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build kind_tests

package scaler

import (
	"context"
	"reflect"
	"testing"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	kind "github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	kindTestEnv      kind.KindCluster
	scalerTestLogger = logr.Discard()
)

const (
	namespace                                        = "default"
	deploymentImageName                              = "nginx:1.14.2"
	ignoreScaleAnnotationKey                         = "dependency-watchdog.gardener.cloud/ignore-scaling"
	defaultTestResourceCheckTimeout                  = 1 * time.Minute
	defaultTestResourceCheckInterval                 = 1 * time.Second
	defaultTestScaleResourceBackoff                  = 100 * time.Millisecond
	expectedSpecReplicasAfterSuccessfulScaleDownTest = 0
)

func TestScalerSuite(t *testing.T) {
	g := NewWithT(t)
	tearDownScalerTests := setUpScalerTests(g)
	defer tearDownScalerTests(g)
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"test getting scale subresource times out", testGettingScaleSubResourceTimesOut},
		{"test scaling when kind of a resource is invalid", testScalingWhenKindOfResourceIsInvalid},
		//{"test waitTillMinTargetReplicasReached returns an error", testWaitTillMinTargetReplicasReachedReturnsError},
		{"test scaling when mandatory resource(optional is false in resourceInfo) is not found", testScalingWhenMandatoryResourceNotFound},
		{"test scaling when optional resource(optional is true in resourceInfo) is not found", testScalingWhenOptionalResourceNotFound},
		{"test scale down then scale up when ignore scaling annotation is not present", testScaleDownThenScaleUpWhenIgnoreScalingAnnotationIsNotPresent},
		{"test scale down then scale up when ignore scaling annotation is present", testScaleDownThenScaleUpWhenIgnoreScalingAnnotationIsPresent},
		{"test scale up should not happen if current replica count is positive", testResourceShouldNotScaleUpIfCurrentReplicaCountIsPositive},
		{"test scale up when replica annotation has invalid value", testScaleUpShouldReturnErrorWhenReplicasAnnotationsHasInvalidValue},
	}
	for _, test := range tests {
		test := test
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
		err := kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
}

func testScaleDownThenScaleUpWhenIgnoreScalingAnnotationIsNotPresent(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)

	table := []struct {
		mcmReplicas                 int32
		kcmReplicas                 int32
		caReplicas                  int32
		expectedScaledUpMCMReplicas int32
		expectedScaledUpKCMReplicas int32
		expectedScaledUpCAReplicas  int32
	}{
		{0, 0, 0, 1, 1, 1},
		{1, 1, 1, 1, 1, 1},
		{2, 2, 2, 2, 2, 2},
		{0, 1, 2, 1, 1, 2},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		err := ds.ScaleDown(context.Background())
		g.Expect(err).ToNot(HaveOccurred())
		checkScaleSuccess(g, scaleDown, namespace, caObjectRef.Name, entry.caReplicas, expectedSpecReplicasAfterSuccessfulScaleDownTest)
		checkScaleSuccess(g, scaleDown, namespace, mcmObjectRef.Name, entry.mcmReplicas, expectedSpecReplicasAfterSuccessfulScaleDownTest)
		checkScaleSuccess(g, scaleDown, namespace, kcmObjectRef.Name, entry.kcmReplicas, expectedSpecReplicasAfterSuccessfulScaleDownTest)

		err = ds.ScaleUp(context.Background())
		g.Expect(err).ToNot(HaveOccurred())
		checkScaleSuccess(g, scaleUp, namespace, mcmObjectRef.Name, entry.mcmReplicas, entry.expectedScaledUpMCMReplicas)
		checkScaleSuccess(g, scaleUp, namespace, caObjectRef.Name, entry.caReplicas, entry.expectedScaledUpCAReplicas)
		checkScaleSuccess(g, scaleUp, namespace, kcmObjectRef.Name, entry.kcmReplicas, entry.expectedScaledUpKCMReplicas)

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
	t.Log("scale down then scale up test finished")
}

func testScaleDownThenScaleUpWhenIgnoreScalingAnnotationIsPresent(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)
	validIgnoreScalingAnnotationMap := map[string]string{ignoreScaleAnnotationKey: "true"}
	invalidIgnoreScalingAnnotationMap := map[string]string{ignoreScaleAnnotationKey: "foo"}

	table := []struct {
		mcmReplicas                 int32
		kcmReplicas                 int32
		caReplicas                  int32
		expectedScaledUpMCMReplicas int32
		expectedScaledUpKCMReplicas int32
		expectedScaledUpCAReplicas  int32
		annotationsOnKCM            map[string]string
	}{
		{2, 2, 2, 2, 2, 2, validIgnoreScalingAnnotationMap},
		{0, 2, 2, 1, 2, 2, validIgnoreScalingAnnotationMap},
		{2, 1, 1, 2, 1, 1, validIgnoreScalingAnnotationMap},
		{2, 0, 2, 2, 0, 2, validIgnoreScalingAnnotationMap},
		{1, 2, 0, 1, 2, 1, invalidIgnoreScalingAnnotationMap},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, entry.annotationsOnKCM)

		err := ds.ScaleDown(context.Background())
		g.Expect(err).ToNot(HaveOccurred())
		checkScaleSuccess(g, scaleDown, namespace, caObjectRef.Name, entry.caReplicas, expectedSpecReplicasAfterSuccessfulScaleDownTest)
		checkScaleSuccess(g, scaleDown, namespace, mcmObjectRef.Name, entry.mcmReplicas, expectedSpecReplicasAfterSuccessfulScaleDownTest)
		if reflect.DeepEqual(entry.annotationsOnKCM, validIgnoreScalingAnnotationMap) {
			matchSpecReplicas(g, namespace, kcmObjectRef.Name, entry.kcmReplicas)
		} else {
			checkScaleSuccess(g, scaleDown, namespace, kcmObjectRef.Name, entry.kcmReplicas, expectedSpecReplicasAfterSuccessfulScaleDownTest)
		}

		err = ds.ScaleUp(context.Background())
		g.Expect(err).ToNot(HaveOccurred())
		checkScaleSuccess(g, scaleUp, namespace, mcmObjectRef.Name, entry.mcmReplicas, entry.expectedScaledUpMCMReplicas)
		checkScaleSuccess(g, scaleUp, namespace, caObjectRef.Name, entry.caReplicas, entry.expectedScaledUpCAReplicas)
		if reflect.DeepEqual(entry.annotationsOnKCM, validIgnoreScalingAnnotationMap) {
			matchSpecReplicas(g, namespace, kcmObjectRef.Name, entry.kcmReplicas)
		} else {
			checkScaleSuccess(g, scaleUp, namespace, kcmObjectRef.Name, entry.kcmReplicas, entry.expectedScaledUpKCMReplicas)
		}

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
	t.Log("scale down then scale up when ignore scaling annotation is present test finished")
}

func testScalingWhenMandatoryResourceNotFound(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)
	table := []struct {
		mcmReplicas                          int32
		caReplicas                           int32
		scalingFn                            func(ctx context.Context) error
		op                                   operation
		unscaledResourceName                 string
		scaledResourceName                   string
		expectedUnscaledResourceSpecReplicas int32
		expectedScaledResourceSpecReplicas   int32
	}{
		{0, 0, ds.ScaleUp, scaleUp, mcmObjectRef.Name, caObjectRef.Name, 0, 1},
		{2, 2, ds.ScaleDown, scaleDown, caObjectRef.Name, mcmObjectRef.Name, 2, expectedSpecReplicasAfterSuccessfulScaleDownTest},
	}
	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("\"" + kcmObjectRef.Name + "\" not found"))
		matchSpecReplicas(g, namespace, entry.unscaledResourceName, entry.expectedUnscaledResourceSpecReplicas)
		checkScaleSuccess(g, entry.op, namespace, entry.scaledResourceName, entry.expectedUnscaledResourceSpecReplicas, entry.expectedScaledResourceSpecReplicas)

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
	t.Log("scaling when mandatory resource not found test finished")
}

func testScalingWhenOptionalResourceNotFound(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)
	table := []struct {
		mcmReplicas               int32
		kcmReplicas               int32
		expectedScaledMCMReplicas int32
		expectedScaledCAReplicas  int32
		scalingFn                 func(context.Context) error
		op                        operation
	}{
		{0, 0, 1, 1, ds.ScaleUp, scaleUp},
		{2, 2, expectedSpecReplicasAfterSuccessfulScaleDownTest, expectedSpecReplicasAfterSuccessfulScaleDownTest, ds.ScaleDown, scaleDown},
	}
	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(HaveOccurred())
		checkScaleSuccess(g, entry.op, namespace, mcmObjectRef.Name, entry.mcmReplicas, entry.expectedScaledMCMReplicas)
		checkScaleSuccess(g, entry.op, namespace, kcmObjectRef.Name, entry.kcmReplicas, entry.expectedScaledCAReplicas)

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
	t.Log("scaling when optional resource not found test finished")
}

func testGettingScaleSubResourceTimesOut(t *testing.T) {
	g := NewWithT(t)
	timeout := time.Nanosecond
	probeCfg := createProbeConfig(&timeout)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)

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
		{0, 0, 0, 0, 0, 0, ds.ScaleUp, "context deadline exceeded"},
		{1, 1, 1, 1, 1, 1, ds.ScaleDown, "context deadline exceeded"},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring(entry.errorString))
		matchSpecReplicas(g, namespace, caObjectRef.Name, entry.expectedScaledCAReplicas)
		matchSpecReplicas(g, namespace, kcmObjectRef.Name, entry.expectedScaledKCMReplicas)
		matchSpecReplicas(g, namespace, mcmObjectRef.Name, entry.expectedScaledMCMReplicas)

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
	t.Log("updateResourceAndScale times out test finished")
}

func testScalingWhenKindOfResourceIsInvalid(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	probeCfg.DependentResourceInfos[1].Ref.Kind = "Depoyment" // "Depoyment" is misspelled intentionally
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)

	table := []struct {
		mcmReplicas                          int32
		kcmReplicas                          int32
		caReplicas                           int32
		scalingFn                            func(context.Context) error
		op                                   operation
		errorString                          string
		scaledResourceName                   string
		unscaledResourceNames                []string
		expectedScaledResourceSpecReplicas   int32
		expectedUnscaledResourceSpecReplicas []int32
	}{
		{0, 0, 0, ds.ScaleUp, scaleUp, "no matches for kind \"Depoyment\" in version \"apps/v1\"", caObjectRef.Name, []string{mcmObjectRef.Name, kcmObjectRef.Name}, 1, []int32{0, 0}},
		{2, 2, 2, ds.ScaleDown, scaleDown, "no matches for kind \"Depoyment\" in version \"apps/v1\"", mcmObjectRef.Name, []string{caObjectRef.Name, kcmObjectRef.Name}, expectedSpecReplicasAfterSuccessfulScaleDownTest, []int32{2, 2}},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring(entry.errorString))
		checkScaleSuccess(g, entry.op, namespace, entry.scaledResourceName, entry.expectedScaledResourceSpecReplicas, entry.expectedScaledResourceSpecReplicas)
		for i, unscaledResName := range entry.unscaledResourceNames {
			matchSpecReplicas(g, namespace, unscaledResName, entry.expectedUnscaledResourceSpecReplicas[i])
		}

		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).ToNot(HaveOccurred())
	}
	t.Log("scaling when res has invalid kind test finished")
}

// TODO: (rishabh-11) Move this test to use a fake client. Commented this test as it is flaky.
//func testWaitTillMinTargetReplicasReachedReturnsError(t *testing.T) {
//	g := NewWithT(t)
//	probeCfg := createProbeConfig(nil)
//	ds := createScaler(g, probeCfg, 0*time.Millisecond, 1*time.Millisecond, 1*time.Millisecond)
//
//	table := []struct {
//		mcmReplicas               int32
//		kcmReplicas               int32
//		caReplicas                int32
//		expectedScaledMCMReplicas int32
//		expectedScaledKCMReplicas int32
//		expectedScaledCAReplicas  int32
//		scalingFn                 func(context.Context) error
//		op                        operation
//		errorString               string
//	}{
//		{0, 0, 0, 0, 0, 1, ds.ScaleUp, scaleUp, fmt.Sprintf("timed out waiting for {namespace: %s, resource: %s} to reach minTargetReplicas", namespace, caObjectRef.Name)},
//		{2, 2, 2, expectedSpecReplicasAfterSuccessfulScaleDownTest, expectedSpecReplicasAfterSuccessfulScaleDownTest, 2, ds.ScaleDown, scaleDown, "timed out waiting"}, // mcm or kcm can return error hence short string is used
//	}
//
//	for _, entry := range table {
//		createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, entry.mcmReplicas, nil)
//		createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, entry.caReplicas, nil)
//		createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, entry.kcmReplicas, nil)
//
//		err := entry.scalingFn(context.Background())
//		g.Expect(err).ToNot(BeNil())
//		g.Expect(err.Error()).To(ContainSubstring(entry.errorString))
//		matchSpecReplicas(g, namespace, caObjectRef.Name, entry.expectedScaledCAReplicas)
//		matchSpecReplicas(g, namespace, kcmObjectRef.Name, entry.expectedScaledKCMReplicas)
//		matchSpecReplicas(g, namespace, mcmObjectRef.Name, entry.expectedScaledMCMReplicas)
//
//		err = kindTestEnv.DeleteAllDeployments(namespace)
//		g.Expect(err).ToNot(HaveOccurred())
//	}
//	t.Log("WaitTillMinTargetReplicasReached returns error test finished")
//}

func testResourceShouldNotScaleUpIfCurrentReplicaCountIsPositive(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)
	createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, 0, nil)
	createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, 0, nil)
	createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, 1, map[string]string{replicasAnnotationKey: "2"})

	err := ds.ScaleUp(context.Background())
	g.Expect(err).ToNot(HaveOccurred())
	checkScaleSuccess(g, scaleUp, namespace, caObjectRef.Name, 0, 1)
	checkScaleSuccess(g, scaleUp, namespace, kcmObjectRef.Name, 1, 1)
	matchSpecReplicas(g, namespace, mcmObjectRef.Name, 1)

	err = kindTestEnv.DeleteAllDeployments(namespace)
	g.Expect(err).ToNot(HaveOccurred())
	t.Log("Resource should not scale up if current replica count is positive test finished")
}

func testScaleUpShouldReturnErrorWhenReplicasAnnotationsHasInvalidValue(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDefaultScaler(g, probeCfg.DependentResourceInfos)
	createDeployment(g, namespace, mcmObjectRef.Name, deploymentImageName, 0, nil)
	createDeployment(g, namespace, caObjectRef.Name, deploymentImageName, 0, nil)
	createDeployment(g, namespace, kcmObjectRef.Name, deploymentImageName, 0, map[string]string{replicasAnnotationKey: "foo"})

	err := ds.ScaleUp(context.Background())
	g.Expect(err).ToNot(BeNil())
	checkScaleSuccess(g, scaleUp, namespace, caObjectRef.Name, 0, 1)
	matchSpecReplicas(g, namespace, kcmObjectRef.Name, 0)
	matchSpecReplicas(g, namespace, mcmObjectRef.Name, 0)

	err = kindTestEnv.DeleteAllDeployments(namespace)
	g.Expect(err).ToNot(HaveOccurred())
	t.Log("Res should not scale up if replica annotation is incorrect test finished")
}

// utility methods to be used by tests
// ------------------------------------------------------------------------------------------------------------------

func setUpScalerTests(g *WithT) func(g *WithT) {
	var err error
	kindTestEnv, err = kind.CreateKindCluster(kind.KindConfig{Name: "scaler-test"})
	g.Expect(err).ToNot(HaveOccurred())
	return func(g *WithT) {
		err := kindTestEnv.Delete()
		g.Expect(err).ToNot(HaveOccurred())
	}
}

func createDefaultScaler(g *WithT, dependentResourceInfos []papi.DependentResourceInfo) Scaler {
	return createScaler(g, dependentResourceInfos, defaultTestResourceCheckTimeout, defaultTestResourceCheckInterval, defaultTestScaleResourceBackoff)
}

func createScaler(g *WithT, dependentResourceInfos []papi.DependentResourceInfo, resCheckTimeout time.Duration, resCheckInterval time.Duration, scaleResBackoff time.Duration) Scaler {
	cfg := kindTestEnv.GetRestConfig()
	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).ToNot(HaveOccurred())
	ds := NewScaler(namespace, dependentResourceInfos, kindTestEnv.GetClient(), scalesGetter, scalerTestLogger,
		withResourceCheckTimeout(resCheckTimeout), withResourceCheckInterval(resCheckInterval), withScaleResourceBackOff(scaleResBackoff))
	return ds
}

func createDeployment(g *WithT, namespace, name, deploymentImageName string, replicas int32, annotations map[string]string) {
	err := kindTestEnv.CreateDeployment(name, namespace, deploymentImageName, replicas, annotations)
	g.Expect(err).ToNot(HaveOccurred())
	g.Eventually(func() bool { return checkIfDeploymentReady(namespace, name, replicas) }, 1*time.Minute, time.Second).Should(BeTrue())
}

func checkIfDeploymentReady(namespace, name string, replicas int32) bool {
	deploy, err := kindTestEnv.GetDeployment(namespace, name)
	if err != nil {
		return false
	}
	var podList corev1.PodList
	err = kindTestEnv.GetClient().List(context.Background(), &podList, &client.ListOptions{Namespace: namespace})
	if err != nil || deploy.Status.ReadyReplicas != replicas {
		return false
	}
	return true
}

func checkScaleSuccess(g *WithT, opType operation, namespace, name string, initialReplicas, expectedSpecReplicas int32) {
	deploy := matchSpecReplicas(g, namespace, name, expectedSpecReplicas)
	if opType == scaleUp {
		g.Eventually(func() bool {
			return checkIfDeploymentReady(namespace, name, expectedSpecReplicas)
		}, 1*time.Minute, time.Second).Should(BeTrue())
		g.Expect(deploy.ObjectMeta.Annotations).ToNot(HaveKey(papi.MeltdownProtectionActive))
	} else {
		g.Eventually(func() bool {
			return checkIfDeploymentReady(namespace, name, expectedSpecReplicasAfterSuccessfulScaleDownTest)
		}, 1*time.Minute, time.Second).Should(BeTrue())
		//the check for initial replicas is done as when initial replicas is 0, no scale down happens, so no annotations are set.
		if initialReplicas != 0 {
			g.Expect(deploy.ObjectMeta.Annotations).To(HaveKey(papi.MeltdownProtectionActive))
			g.Expect(deploy.ObjectMeta.Annotations).To(HaveKey(replicasAnnotationKey))
		}
	}
}

func matchSpecReplicas(g *WithT, namespace string, name string, expectedReplicas int32) *v1.Deployment {
	deploy, err := kindTestEnv.GetDeployment(namespace, name)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deploy).ToNot(BeNil())
	g.Expect(*deploy.Spec.Replicas).Should(Equal(expectedReplicas))
	return deploy
}

func createProbeConfig(timeout *time.Duration) *papi.Config {
	dependentResourceInfos := createDepResourceInfoArray(timeout)
	return &papi.Config{DependentResourceInfos: dependentResourceInfos}
}

func createDepResourceInfoArray(timeout *time.Duration) []papi.DependentResourceInfo {
	var dependentResourceInfos []papi.DependentResourceInfo
	dependentResourceInfos = append(dependentResourceInfos, createTestDeploymentDependentResourceInfo(mcmObjectRef.Name, 2, 0, timeout, pointer.Duration(0*time.Second), false))
	dependentResourceInfos = append(dependentResourceInfos, createTestDeploymentDependentResourceInfo(kcmObjectRef.Name, 1, 0, timeout, pointer.Duration(0*time.Second), false))
	dependentResourceInfos = append(dependentResourceInfos, createTestDeploymentDependentResourceInfo(caObjectRef.Name, 0, 1, timeout, pointer.Duration(0*time.Second), true))
	return dependentResourceInfos
}
