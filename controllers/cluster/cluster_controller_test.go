// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package cluster

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/utils/pointer"

	proberpackage "github.com/gardener/dependency-watchdog/internal/prober"
	testutil "github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenerv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testdataPath                  = "testdata"
	maxConcurrentReconcilesProber = 1
)

var (
	shootKCMNodeMonitorGracePeriod   = &metav1.Duration{Duration: 80 * time.Second}
	defaultKCMNodeMonitorGracePeriod = metav1.Duration{Duration: proberpackage.DefaultKCMNodeMonitorGraceDuration}
)

func setupProberEnv(ctx context.Context, g *WithT) (client.Client, *envtest.Environment, *Reconciler, manager.Manager) {
	scheme := buildScheme()
	crdDirectoryPaths := []string{testdataPath}
	opts := zap.Options{
		Development: true,
		Level:       zapcore.DebugLevel,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	testLogger := ctrl.Log.WithName("test-logger")

	controllerTestEnv, err := testutil.CreateControllerTestEnv(scheme, crdDirectoryPaths, nil)
	g.Expect(err).ToNot(HaveOccurred())

	testEnv := controllerTestEnv.GetEnv()
	cfg := controllerTestEnv.GetConfig()
	crClient := controllerTestEnv.GetClient()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Logger: testLogger,
	})
	g.Expect(err).ToNot(HaveOccurred())

	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).ToNot(HaveOccurred())

	probeConfigPath := filepath.Join(testdataPath, "prober-config.yaml")
	validateIfFileExists(probeConfigPath, g)
	proberConfig, err := proberpackage.LoadConfig(probeConfigPath, scheme)
	g.Expect(err).ToNot(HaveOccurred())

	clusterReconciler := &Reconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		ScaleGetter:             scalesGetter,
		ProberMgr:               proberpackage.NewManager(),
		DefaultProbeConfig:      proberConfig,
		MaxConcurrentReconciles: maxConcurrentReconcilesProber,
	}
	err = clusterReconciler.SetupWithManager(mgr)
	g.Expect(err).ToNot(HaveOccurred())

	go func() {
		err = mgr.Start(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	}()

	return crClient, testEnv, clusterReconciler, mgr
}

func TestClusterControllerSuite(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"tests with common environment", testProberSharedEnvTest},
		{"tests with dedicated environment for each test", testProberDedicatedEnvTest},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
}

// testProberDedicatedEnvTest creates a new envTest at the start of each subtest and destroys it at the end of each subtest.
func testProberDedicatedEnvTest(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		title string
		run   func(ctx context.Context, t *testing.T, envTest *envtest.Environment, crClient client.Client, reconciler *Reconciler, mgr manager.Manager, cancelFn context.CancelFunc)
	}{
		{"calling reconciler after shutting down API server", testReconciliationAfterAPIServerIsDown},
	}
	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		crClient, testEnv, reconciler, mgr := setupProberEnv(ctx, g)
		t.Run(test.title, func(t *testing.T) {
			test.run(ctx, t, testEnv, crClient, reconciler, mgr, cancelFn)
		})
		testutil.TeardownEnv(g, testEnv, cancelFn)
	}
}

func testReconciliationAfterAPIServerIsDown(ctx context.Context, t *testing.T, testEnv *envtest.Environment, _ client.Client, reconciler *Reconciler, _ manager.Manager, cancelFn context.CancelFunc) {
	var err error
	g := NewWithT(t)
	cluster, _, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	cancelFn()
	err = testEnv.ControlPlane.APIServer.Stop()
	g.Expect(err).ToNot(HaveOccurred())
	_, err = reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: cluster.ObjectMeta.Name, Namespace: ""}})
	g.Expect(err).To(HaveOccurred())
	err = testEnv.ControlPlane.APIServer.Start()
	g.Expect(err).ToNot(HaveOccurred())
}

// testProberSharedEnvTest creates an envTest just once and that is then shared by all the subtests. Shared envTest is destroyed once all subtests have run.
func testProberSharedEnvTest(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	crClient, testEnv, reconciler, _ := setupProberEnv(ctx, g)
	defer testutil.TeardownEnv(g, testEnv, cancelFn)

	tests := []struct {
		title string
		run   func(g *WithT, crClient client.Client, reconciler *Reconciler)
	}{
		{"changing hibernation spec and status", testShootHibernation},
		{"invalid shoot in cluster spec", testInvalidShootInClusterSpec},
		{"deletion time stamp check", testProberShouldBeRemovedIfDeletionTimeStampIsSet},
		{"no prober if shoot creation is not successful", testShootCreationNotComplete},
		{"no prober if shoot control plane is migrating", testShootIsMigrating},
		{"no prober if shoot control plane is restoring after migrate", testShootRestoringIsNotComplete},
		{"start prober if last operation is restore and successfully", testLastOperationIsRestoreAndSuccessful},
		{"start prober if last operation is reconciliation of shoot", testLastOperationIsShootReconciliation},
		{"no prober if shoot has no workers", testShootHasNoWorkers},
		{"prober should start with correct worker node conditions mapping", testShootWorkerNodeConditions},
	}

	for _, test := range tests {
		t.Run(test.title, func(_ *testing.T) {
			test.run(g, crClient, reconciler)
		})
		deleteAllClusters(g, crClient)
	}
}

func testShootWorkerNodeConditions(g *WithT, crClient client.Client, reconciler *Reconciler) {
	workerNodeConditions := [][]string{{testutil.NodeConditionDiskPressure, testutil.NodeConditionMemoryPressure}}
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).WithWorkerNodeConditions(workerNodeConditions).Build()
	g.Expect(err).ToNot(HaveOccurred())
	createCluster(g, crClient, cluster)
	expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	// update the workers
	updatedWorkerNodeConditions := []string{testutil.NodeConditionPIDPressure, testutil.NodeConditionNetworkReady}
	shoot.Spec.Provider.Workers[0].MachineControllerManagerSettings.NodeConditions = updatedWorkerNodeConditions
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateCluster(g, crClient, cluster)
	expectedWorkerNodeConditions = util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func deleteAllClusters(g *WithT, crClient client.Client) {
	err := crClient.DeleteAllOf(context.Background(), &gardenerv1alpha1.Cluster{})
	g.Expect(err).ToNot(HaveOccurred())
}

func createCluster(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster) {
	err := crClient.Create(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
	time.Sleep(2 * time.Second) // giving some time for controller to take action
}

func updateCluster(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster) {
	err := crClient.Update(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
	time.Sleep(2 * time.Second) // giving some time for controller to take action
}

func deleteClusterAndCheckIfProberRemoved(g *WithT, crClient client.Client, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster) {
	err := crClient.Delete(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
	proberShouldNotBePresent(g, reconciler, cluster)
}

func updateShootHibernationSpec(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, isHibernationEnabled *bool) {
	shoot.Spec.Hibernation.Enabled = isHibernationEnabled
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateCluster(g, crClient, cluster)
}

func testShootHibernation(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	createCluster(g, crClient, cluster)
	expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	// update spec to indicate start of hibernation
	updateShootHibernationSpec(g, crClient, cluster, shoot, pointer.Bool(true))
	proberShouldNotBePresent(g, reconciler, cluster)
	// update the status to show hibernation is finished
	updateShootHibernationStatus(g, crClient, cluster, shoot, true)
	// update spec to indicate cluster is waking up
	updateShootHibernationSpec(g, crClient, cluster, shoot, pointer.Bool(false))
	proberShouldNotBePresent(g, reconciler, cluster)
	// update status to indicate cluster has successfully woken up
	updateShootHibernationStatus(g, crClient, cluster, shoot, false)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func updateShootHibernationStatus(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, IsHibernated bool) {
	shoot.Status.IsHibernated = IsHibernated
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateCluster(g, crClient, cluster)
}

func testInvalidShootInClusterSpec(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, _, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	cluster.Spec.Shoot.Object = nil
	cluster.Spec.Shoot.Raw = []byte(`{"apiVersion": 8}`)
	createCluster(g, crClient, cluster)
	proberShouldNotBePresent(g, reconciler, cluster)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func updateShootDeletionTimeStamp(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot) {
	deletionTimeStamp, _ := time.Parse(time.RFC3339, "2022-05-05T08:34:05Z")
	shoot.DeletionTimestamp = &metav1.Time{
		Time: deletionTimeStamp,
	}
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	err := crClient.Update(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
}

func testProberShouldBeRemovedIfDeletionTimeStampIsSet(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	createCluster(g, crClient, cluster)
	expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	updateShootDeletionTimeStamp(g, crClient, cluster, shoot)
	proberShouldNotBePresent(g, reconciler, cluster)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func setShootLastOperationStatus(cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, opType gardencorev1beta1.LastOperationType, opState gardencorev1beta1.LastOperationState) {
	shoot.Status.LastOperation = &gardencorev1beta1.LastOperation{
		Type:  opType,
		State: opState,
	}
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
}

func testShootCreationNotComplete(g *WithT, crClient client.Client, reconciler *Reconciler) {
	testCases := []struct {
		lastOpState        gardencorev1beta1.LastOperationState
		shouldCreateProber bool
	}{
		{gardencorev1beta1.LastOperationStateProcessing, false},
		{gardencorev1beta1.LastOperationStatePending, false},
		{gardencorev1beta1.LastOperationStateError, false},
		{gardencorev1beta1.LastOperationStateAborted, false},
		{gardencorev1beta1.LastOperationStateSucceeded, true},
	}

	for _, testCase := range testCases {
		cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).WithNodeMonitorGracePeriod(shootKCMNodeMonitorGracePeriod).Build()
		g.Expect(err).ToNot(HaveOccurred())
		setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, testCase.lastOpState)
		createCluster(g, crClient, cluster)
		if testCase.shouldCreateProber {
			expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
			proberShouldBePresent(g, reconciler, cluster, *shootKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
		} else {
			proberShouldNotBePresent(g, reconciler, cluster)
		}
		deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
	}
}

func testShootIsMigrating(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	createCluster(g, crClient, cluster)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateCluster(g, crClient, cluster)
	proberShouldNotBePresent(g, reconciler, cluster)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testShootRestoringIsNotComplete(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	createCluster(g, crClient, cluster)
	expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	// cluster migration starts
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateCluster(g, crClient, cluster)
	proberShouldNotBePresent(g, reconciler, cluster)
	// cluster migrate done, restore in progress
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, gardencorev1beta1.LastOperationStateProcessing)
	updateCluster(g, crClient, cluster)
	proberShouldNotBePresent(g, reconciler, cluster)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testLastOperationIsRestoreAndSuccessful(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, gardencorev1beta1.LastOperationStateSucceeded)
	createCluster(g, crClient, cluster)
	expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testLastOperationIsShootReconciliation(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.NewClusterBuilder().WithWorkerCount(1).Build()
	g.Expect(err).ToNot(HaveOccurred())
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeReconcile, "")
	createCluster(g, crClient, cluster)
	expectedWorkerNodeConditions := util.GetEffectiveNodeConditionsForWorkers(shoot)
	proberShouldBePresent(g, reconciler, cluster, defaultKCMNodeMonitorGracePeriod, expectedWorkerNodeConditions)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testShootHasNoWorkers(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, _, err := testutil.NewClusterBuilder().Build()
	g.Expect(err).ToNot(HaveOccurred())
	createCluster(g, crClient, cluster)
	proberShouldNotBePresent(g, reconciler, cluster)
}

func proberShouldBePresent(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster, expectedKCMNodeMonitorGraceDuration metav1.Duration, expectedWorkerNodeConditions map[string][]string) {
	g.Eventually(func() bool {
		prober, ok := reconciler.ProberMgr.GetProber(cluster.ObjectMeta.Name)
		return ok && reflect.DeepEqual(*prober.GetConfig().KCMNodeMonitorGraceDuration, expectedKCMNodeMonitorGraceDuration) && !prober.AreWorkerNodeConditionsStale(expectedWorkerNodeConditions) && !prober.IsClosed()
	}, 10*time.Second, 1*time.Second).Should(BeTrue())
}

func proberShouldNotBePresent(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster) {
	g.Eventually(func() int { return len(reconciler.ProberMgr.GetAllProbers()) }, 10*time.Second, 1*time.Second).Should(Equal(0))
	prober, ok := reconciler.ProberMgr.GetProber(cluster.ObjectMeta.Name)
	g.Expect(ok).To(BeFalse())
	g.Expect(prober).To(Equal(proberpackage.Prober{}))
}

func validateIfFileExists(file string, g *WithT) {
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	g.Expect(err).ToNot(HaveOccurred(), "File at path %v should exist")
}

func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	localSchemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		gardenerv1alpha1.AddToScheme,
	)
	utilruntime.Must(localSchemeBuilder.AddToScheme(scheme))
	return scheme
}
