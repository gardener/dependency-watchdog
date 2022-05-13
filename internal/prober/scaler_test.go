package prober

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/gardener/pkg/utils/flow"
	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	defaultInitialDelay = 10 * time.Millisecond
	defaultTimeout      = 10 * time.Second
	mcmRef              = &autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "machine-controller-manager", APIVersion: "apps/v1"}
	kcmRef              = &autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "kube-controller-manager", APIVersion: "apps/v1"}
	caRef               = &autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "cluster-autoscaler", APIVersion: "apps/v1"}
	kindTestEnv         test.KindCluster
	sLogger             = log.FromContext(context.Background()).WithName("scalerLogger")
)

const (
	namespace                = "default"
	deploymentImageName      = "nginx:1.14.2"
	ignoreScaleAnnotationKey = "dependency-watchdog.gardener.cloud/ignore-scaling"
)

func beforeAllScalerEnvTests(g *WithT) func(g *WithT) {
	var err error
	kindTestEnv, err = test.CreateKindCluster(test.KindConfig{Name: "test"})
	g.Expect(err).To(BeNil())
	return func(g *WithT) {
		err := kindTestEnv.Delete()
		g.Expect(err).To(BeNil())
	}
}

func createDeploymentScaler(g *WithT, probeCfg *Config) DeploymentScaler {
	cfg := kindTestEnv.GetRestConfig()
	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).To(BeNil())
	ds := NewDeploymentScaler(namespace, probeCfg, kindTestEnv.GetClient(), scalesGetter, sLogger,
		withDependentResourceCheckTimeout(10*time.Second), withDependentResourceCheckInterval(100*time.Millisecond))
	return ds
}

func TestScalerSuite(t *testing.T) {
	g := NewWithT(t)
	afterAllScalerTests := beforeAllScalerEnvTests(g)
	defer afterAllScalerTests(g)
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"test doScale returns an error", testDoScaleReturnsError},
		{"test scaling when KCM deployment is not found", testScalingWhenKCMDeploymentNotFound},
		{"test scaling when all deployments are found", testScalingWhenAllDeploymentsAreFound},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
		err := kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func testScalingWhenAllDeploymentsAreFound(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDeploymentScaler(g, probeCfg)
	table := []struct {
		mcmReplicas               int32
		kcmReplicas               int32
		caReplicas                int32
		expectedScaledMCMReplicas int32
		expectedScaledKCMReplicas int32
		expectedScaledCAReplicas  int32
		applyKCMAnnotation        bool
		scalingFn                 func(context.Context) error
		isFnScaleUp               bool
	}{
		{0, 0, 0, 1, 1, 1, false, ds.ScaleUp, true},
		{0, 1, 0, 1, 1, 1, false, ds.ScaleUp, true},
		{0, 0, 0, 1, 0, 1, true, ds.ScaleUp, true},
		{0, 1, 0, 1, 1, 1, true, ds.ScaleUp, true},
		{1, 1, 1, 0, 0, 0, false, ds.ScaleDown, false},
		{0, 1, 0, 0, 0, 0, false, ds.ScaleDown, false},
		{1, 1, 1, 0, 1, 0, true, ds.ScaleDown, false},
		{1, 0, 1, 0, 0, 0, true, ds.ScaleDown, false},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caRef.Name, deploymentImageName, entry.caReplicas, nil)
		if entry.applyKCMAnnotation {
			createDeployment(g, namespace, kcmRef.Name, deploymentImageName, entry.kcmReplicas, map[string]string{ignoreScaleAnnotationKey: "true"})
		} else {
			createDeployment(g, namespace, kcmRef.Name, deploymentImageName, entry.kcmReplicas, nil)
		}

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmRef.Name, entry.mcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, caRef.Name, entry.caReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, kcmRef.Name, entry.kcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())

		err := entry.scalingFn(context.Background())
		g.Expect(err).To(BeNil())
		if entry.isFnScaleUp {
			matchStatusReplicas(g, namespace, caRef.Name, entry.expectedScaledCAReplicas)
			matchStatusReplicas(g, namespace, kcmRef.Name, entry.expectedScaledKCMReplicas)
			matchSpecReplicas(g, namespace, mcmRef.Name, entry.expectedScaledMCMReplicas)
		} else {
			matchSpecReplicas(g, namespace, caRef.Name, entry.expectedScaledCAReplicas)
			matchStatusReplicas(g, namespace, kcmRef.Name, entry.expectedScaledKCMReplicas)
			matchStatusReplicas(g, namespace, mcmRef.Name, entry.expectedScaledMCMReplicas)
		}
		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func testScalingWhenKCMDeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	probeCfg := createProbeConfig(nil)
	ds := createDeploymentScaler(g, probeCfg)
	table := []struct {
		mcmReplicas               int32
		caReplicas                int32
		expectedScaledMCMReplicas int32
		expectedScaledCAReplicas  int32
		scalingFn                 func(context.Context) error
	}{
		{0, 0, 0, 1, ds.ScaleUp},
		{1, 1, 0, 1, ds.ScaleDown},
	}
	for _, entry := range table {
		createDeployment(g, namespace, mcmRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caRef.Name, deploymentImageName, entry.caReplicas, nil)

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmRef.Name, entry.mcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, caRef.Name, entry.caReplicas) }, 10*time.Second, time.Second).Should(BeTrue())

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("\"" + kcmRef.Name + "\" not found"))
		matchSpecReplicas(g, namespace, mcmRef.Name, entry.expectedScaledMCMReplicas)
		matchSpecReplicas(g, namespace, caRef.Name, entry.expectedScaledCAReplicas)
		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func testDoScaleReturnsError(t *testing.T) {
	g := NewWithT(t)
	faultyProbeCfg := createProbeConfig(nil)
	faultyProbeCfg.DependentResourceInfos[2].Ref.Kind = "Depoyment"
	ds1 := createDeploymentScaler(g, faultyProbeCfg)
	timeout := time.Nanosecond
	probeCfg := createProbeConfig(&timeout)
	ds2 := createDeploymentScaler(g, probeCfg)
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
		{1, 1, 1, 0, 0, 1, ds1.ScaleDown, "no matches for kind \"Depoyment\" in version \"apps/v1\""},
		{0, 0, 0, 0, 0, 0, ds2.ScaleUp, "context deadline exceeded"},
		{1, 1, 1, 1, 1, 1, ds2.ScaleDown, "context deadline exceeded"},
	}

	for _, entry := range table {
		createDeployment(g, namespace, mcmRef.Name, deploymentImageName, entry.mcmReplicas, nil)
		createDeployment(g, namespace, caRef.Name, deploymentImageName, entry.caReplicas, nil)
		createDeployment(g, namespace, kcmRef.Name, deploymentImageName, entry.kcmReplicas, nil)

		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, mcmRef.Name, entry.mcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, caRef.Name, entry.caReplicas) }, 10*time.Second, time.Second).Should(BeTrue())
		g.Eventually(func() bool { return checkIfDeploymentReady(namespace, kcmRef.Name, entry.kcmReplicas) }, 10*time.Second, time.Second).Should(BeTrue())

		err := entry.scalingFn(context.Background())
		g.Expect(err).ToNot(BeNil())
		g.Expect(err.Error()).To(ContainSubstring(entry.errorString))
		matchStatusReplicas(g, namespace, caRef.Name, entry.expectedScaledCAReplicas)
		matchStatusReplicas(g, namespace, kcmRef.Name, entry.expectedScaledKCMReplicas)
		matchStatusReplicas(g, namespace, mcmRef.Name, entry.expectedScaledMCMReplicas)
		err = kindTestEnv.DeleteAllDeployments(namespace)
		g.Expect(err).To(BeNil())
	}
}

func TestCreateResourceScaleFlowParallel(t *testing.T) {
	g := NewWithT(t)

	depScaler := deploymentScaler{l: sLogger}
	var scri []scaleableResourceInfo
	scri = append(scri, scaleableResourceInfo{ref: caRef, level: 1, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0})
	scri = append(scri, scaleableResourceInfo{ref: mcmRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0})
	scri = append(scri, scaleableResourceInfo{ref: kcmRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0})

	waitOnResourceInfos := [][]scaleableResourceInfo{
		{scri[1], scri[2]},
	}
	sf := depScaler.createResourceScaleFlow(namespace, "test", scri, util.ScaleDownReplicasMismatch)
	checkCreatedFlow(g, sf, waitOnResourceInfos)
}

func TestCreateScaleFlowSequential(t *testing.T) {
	g := NewWithT(t)

	depScaler := deploymentScaler{l: sLogger}
	var scri []scaleableResourceInfo
	scri = append(scri, scaleableResourceInfo{ref: caRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1})
	scri = append(scri, scaleableResourceInfo{ref: kcmRef, level: 1, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1})
	scri = append(scri, scaleableResourceInfo{ref: mcmRef, level: 2, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1})

	waitOnResourceInfos := [][]scaleableResourceInfo{
		{scri[0]},
		{scri[0], scri[1]},
	}

	sf := depScaler.createResourceScaleFlow(namespace, "test", scri, util.ScaleDownReplicasMismatch)
	checkCreatedFlow(g, sf, waitOnResourceInfos)
}

func checkCreatedFlow(g *WithT, sf *scaleFlow, waitOnResourceInfos [][]scaleableResourceInfo) {
	g.Expect(len(sf.flowStepInfos)).To(Equal(len(waitOnResourceInfos) + 1))
	g.Expect(sf.flow).ToNot(BeNil())
	g.Expect(sf.flow.Len()).To(Equal(len(waitOnResourceInfos) + 1))
	g.Expect(sf.flowStepInfos[0].dependentTaskIDs.Len()).To(Equal(0))
	g.Expect(sf.flowStepInfos[0].waitOnResourceInfos).To(BeNil())
	dependentTaskIDs := flow.NewTaskIDs(sf.flowStepInfos[0].taskID)
	for i, flowStep := range sf.flowStepInfos[1:] {
		g.Expect(flowStep.dependentTaskIDs).To(Equal(dependentTaskIDs))
		g.Expect(flowStep.waitOnResourceInfos).To(Equal(waitOnResourceInfos[i]))
		dependentTaskIDs.Insert(flowStep.taskID)
	}
}

func TestSleepWithContextInScale(t *testing.T) {
	g := NewWithT(t)
	var err error
	depScaler := deploymentScaler{l: sLogger}
	cancelableCtx, cancelFn := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = depScaler.scale(cancelableCtx, scaleableResourceInfo{ref: caRef, level: 0, initialDelay: defaultInitialDelay, timeout: 100 * time.Millisecond, replicas: 1}, nil, nil)
	}()
	cancelFn()
	wg.Wait()
	g.Expect(err).To(Equal(context.Canceled))
}

func TestSortAndGetUniqueLevels(t *testing.T) {
	g := NewWithT(t)
	numResInfosByLevel := map[int]int{2: 1, 0: 2, 1: 2}
	resInfos := createScaleableResourceInfos(numResInfosByLevel)
	levels := sortAndGetUniqueLevels(resInfos)
	g.Expect(levels).ToNot(BeNil())
	g.Expect(levels).ToNot(BeEmpty())
	g.Expect(len(levels)).To(Equal(3))
	g.Expect(levels).To(Equal([]int{0, 1, 2}))
}

func TestSortAndGetUniqueLevelsForEmptyScaleableResourceInfos(t *testing.T) {
	g := NewWithT(t)
	levels := sortAndGetUniqueLevels([]scaleableResourceInfo{})
	g.Expect(levels).To(BeNil())
}

func TestCreateScaleUpResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []DependentResourceInfo
	depResInfos = append(depResInfos, createDependentResourceInfo(mcmRef.Name, 2, 0, 1, 0, nil))
	depResInfos = append(depResInfos, createDependentResourceInfo(caRef.Name, 0, 1, 1, 0, nil))
	depResInfos = append(depResInfos, createDependentResourceInfo(kcmRef.Name, 1, 0, 1, 0, nil))

	scaleUpResInfos := createScaleUpResourceInfos(depResInfos)
	g.Expect(scaleUpResInfos).ToNot(BeNil())
	g.Expect(scaleUpResInfos).ToNot(BeEmpty())
	g.Expect(len(scaleUpResInfos)).To(Equal(len(depResInfos)))

	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: mcmRef, level: 2, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleUpResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: caRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleUpResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: kcmRef, level: 1, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleUpResInfos)).To(BeTrue())
}

func TestCreateScaleDownResourceInfos(t *testing.T) {
	g := NewWithT(t)
	var depResInfos []DependentResourceInfo
	depResInfos = append(depResInfos, createDependentResourceInfo(mcmRef.Name, 1, 0, 1, 0, nil))
	depResInfos = append(depResInfos, createDependentResourceInfo(caRef.Name, 0, 1, 2, 1, nil))
	depResInfos = append(depResInfos, createDependentResourceInfo(kcmRef.Name, 1, 0, 1, 0, nil))

	scaleDownResInfos := createScaleDownResourceInfos(depResInfos)
	g.Expect(scaleDownResInfos).ToNot(BeNil())
	g.Expect(scaleDownResInfos).ToNot(BeEmpty())
	g.Expect(len(scaleDownResInfos)).To(Equal(len(depResInfos)))

	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: mcmRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0}, scaleDownResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: caRef, level: 1, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 1}, scaleDownResInfos)).To(BeTrue())
	g.Expect(scaleableResourceMatchFound(scaleableResourceInfo{ref: kcmRef, level: 0, initialDelay: defaultInitialDelay, timeout: defaultTimeout, replicas: 0}, scaleDownResInfos)).To(BeTrue())
}

// utility methods to be used by tests
//------------------------------------------------------------------------------------------------------------------
// createScaleableResourceInfos creates a slice of scaleableResourceInfo's taking in a map whose key is level
// and value is the number of scaleableResourceInfo's to be created at that level
func createScaleableResourceInfos(numResInfosByLevel map[int]int) []scaleableResourceInfo {
	var resInfos []scaleableResourceInfo
	for k, v := range numResInfosByLevel {
		for i := 0; i < v; i++ {
			resInfos = append(resInfos, scaleableResourceInfo{
				ref:   &autoscalingv1.CrossVersionObjectReference{Name: fmt.Sprintf("resource-%d%d", k, i)},
				level: k,
			})
		}
	}
	return resInfos
}

func createDependentResourceInfo(name string, scaleUpLevel, scaleDownLevel int, scaleUpReplicas, scaleDownReplicas int32, timeout *time.Duration) DependentResourceInfo {
	if timeout == nil {
		timeout = &defaultTimeout
	}
	return DependentResourceInfo{
		Ref: &autoscalingv1.CrossVersionObjectReference{Name: name, Kind: "Deployment", APIVersion: "apps/v1"},
		ScaleUpInfo: &ScaleInfo{
			Level:        scaleUpLevel,
			InitialDelay: &defaultInitialDelay,
			Timeout:      timeout,
			Replicas:     &scaleUpReplicas,
		},
		ScaleDownInfo: &ScaleInfo{
			Level:        scaleDownLevel,
			InitialDelay: &defaultInitialDelay,
			Timeout:      timeout,
			Replicas:     &scaleDownReplicas,
		},
	}
}

func scaleableResourceMatchFound(expected scaleableResourceInfo, resources []scaleableResourceInfo) bool {
	for _, resInfo := range resources {
		if resInfo.ref.Name == expected.ref.Name {
			// compare all values which are not nil
			return reflect.DeepEqual(expected.ref, resInfo.ref) && expected.level == resInfo.level && expected.replicas == resInfo.replicas
		}
	}
	return false
}

func createDeployment(g *WithT, namespace, name, deploymentImageName string, replicas int32, annotations map[string]string) {
	err := kindTestEnv.CreateDeployment(name, namespace, deploymentImageName, replicas, annotations)
	g.Expect(err).To(BeNil())
}

func checkIfDeploymentReady(namespace, name string, replicas int32) bool {
	deploy, err := kindTestEnv.GetDeployment(namespace, name)
	if err != nil || deploy.Status.Replicas != replicas {
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
	g.Expect(deploy.Status.Replicas).Should(Equal(expectedReplicas))
}

func createProbeConfig(timeout *time.Duration) *Config {
	dependentResourceInfos := createDepResourceInfoArray(timeout)
	return &Config{DependentResourceInfos: dependentResourceInfos}
}

// func createFaultyProbeConfig(timeout *time.Duration) *Config {
// 	dependentResourceInfos := createDepResourceInfoArray(timeout)
// 	dependentResourceInfos[2].Ref.Kind = "Depoyment"
// 	return &Config{DependentResourceInfos: dependentResourceInfos}
// }

func createDepResourceInfoArray(timeout *time.Duration) []DependentResourceInfo {
	var dependentResourceInfos []DependentResourceInfo
	dependentResourceInfos = append(dependentResourceInfos, createDependentResourceInfo(mcmRef.Name, 2, 0, 1, 0, timeout))
	dependentResourceInfos = append(dependentResourceInfos, createDependentResourceInfo(kcmRef.Name, 1, 0, 1, 0, timeout))
	dependentResourceInfos = append(dependentResourceInfos, createDependentResourceInfo(caRef.Name, 0, 1, 1, 0, timeout))
	return dependentResourceInfos
}
