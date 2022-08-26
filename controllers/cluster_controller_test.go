package controllers

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	proberpackage "github.com/gardener/dependency-watchdog/internal/prober"
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

func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	localSchemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		gardenerv1alpha1.AddToScheme,
	)
	utilruntime.Must(localSchemeBuilder.AddToScheme(scheme))
	return scheme
}

func setupProberEnv(t *testing.T, g *WithT, ctx context.Context) (client.Client, *envtest.Environment, *ClusterReconciler, manager.Manager) {
	t.Log("setting up the test Env for Prober")
	scheme := buildScheme()
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("testdata", "crd", "prober")},
		ErrorIfCRDPathMissing: false,
		Scheme:                scheme,
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).NotTo(BeNil())

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	g.Expect(err).ToNot(HaveOccurred())

	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).To(BeNil())

	probeConfigPath := filepath.Join(testdataPath, "config", "prober-config.yaml")
	validateIfFileExists(probeConfigPath, g)
	proberConfig, err := proberpackage.LoadConfig(probeConfigPath)
	g.Expect(err).To(BeNil())

	clusterReconciler := &ClusterReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		ScaleGetter:             scalesGetter,
		ProberMgr:               proberpackage.NewManager(),
		ProbeConfig:             proberConfig,
		MaxConcurrentReconciles: maxConcurrentReconcilesProber,
	}
	err = clusterReconciler.SetupWithManager(mgr)
	g.Expect(err).To(BeNil())

	go func() {
		err = mgr.Start(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	}()

	return k8sClient, testEnv, clusterReconciler, mgr
}

func teardownEnv(t *testing.T, g *WithT, testEnv *envtest.Environment, cancelFn context.CancelFunc) {
	t.Log("destroying the test Env")
	cancelFn()
	err := testEnv.Stop()
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterControllerSuite(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"tests with common environment", testCommonEnvTest},
		{"tests with dedicated environment for each test", testDedicatedEnvTest},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
}

func testDedicatedEnvTest(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		title string
		run   func(t *testing.T, envTest *envtest.Environment, ctx context.Context, k8sClient client.Client, reconciler *ClusterReconciler, mgr manager.Manager, cancelFn context.CancelFunc)
	}{
		{"calling reconciler after shutting down API server", testReconciliationAfterAPIServerIsDown},
	}
	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		k8sClient, testEnv, reconciler, mgr := setupProberEnv(t, g, ctx)
		t.Run(test.title, func(t *testing.T) {
			test.run(t, testEnv, ctx, k8sClient, reconciler, mgr, cancelFn)
		})
		teardownEnv(t, g, testEnv, cancelFn)
	}
}

func testReconciliationAfterAPIServerIsDown(t *testing.T, testEnv *envtest.Environment, ctx context.Context, k8sClient client.Client, reconciler *ClusterReconciler, mgr manager.Manager, cancelFn context.CancelFunc) {
	g := NewWithT(t)
	cluster, _ := createClusterResource()
	cancelFn()
	err := testEnv.ControlPlane.APIServer.Stop()
	g.Expect(err).To(BeNil())
	_, err = reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: cluster.ObjectMeta.Name, Namespace: ""}})
	g.Expect(err).ToNot(BeNil())
	err = testEnv.ControlPlane.APIServer.Start()
	g.Expect(err).To(BeNil())
}

func testCommonEnvTest(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	k8sClient, testEnv, reconciler, _ := setupProberEnv(t, g, ctx)
	defer teardownEnv(t, g, testEnv, cancelFn)

	tests := []struct {
		title string
		run   func(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler)
	}{
		{"changing hibernation spec", testChangingHibernationSpec},
		{"changing hibernation status", testChangingHibernationStatus},
		{"invalid shoot in cluster spec", testInvalidShootInClusterSpec},
		{"deletion time stamp check", testProberShouldBeRemovedIfDeletionTimeStampIsSet},
		{"no prober if shoot creation is not successful", testShootCreationNotComplete},
		{"no prober if shoot control plane is migrating", testShootIsMigrating},
		{"start prober if last operation is restore", testLastOperationIsRestore},
		{"start prober if last operation is reconciliation of shoot", testLastOperationIsShootReconciliation},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t, k8sClient, reconciler)
		})
		deleteAllClusters(g, k8sClient)
	}
}

func deleteAllClusters(g *WithT, k8sClient client.Client) {
	err := k8sClient.DeleteAllOf(context.Background(), &gardenerv1alpha1.Cluster{})
	g.Expect(err).To(BeNil())
}

func createClusterAndCheckProber(g *WithT, k8sClient client.Client, reconciler *ClusterReconciler,
	cluster *gardenerv1alpha1.Cluster, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	err := k8sClient.Create(context.Background(), cluster)
	g.Expect(err).To(BeNil())
	time.Sleep(2 * time.Second)
	if checkProber != nil {
		checkProber(g, reconciler, cluster)
	}
}

func updateClusterAndCheckProber(g *WithT, k8sClient client.Client, reconciler *ClusterReconciler,
	cluster *gardenerv1alpha1.Cluster, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	err := k8sClient.Update(context.Background(), cluster)
	g.Expect(err).To(BeNil())
	if checkProber != nil {
		checkProber(g, reconciler, cluster)
	}
}

func deleteClusterAndCheckIfProberRemoved(g *WithT, k8sClient client.Client, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster) {
	err := k8sClient.Delete(context.Background(), cluster)
	g.Expect(err).To(BeNil())
	proberShouldNotBePresent(g, reconciler, cluster)
}

func updateShootHibernationSpecAndCheckProber(g *WithT, k8sClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, isHibernationEnabled *bool,
	reconciler *ClusterReconciler, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	shoot.Spec.Hibernation.Enabled = isHibernationEnabled
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, checkProber)
}

func testChangingHibernationSpec(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	enableHibernation := true
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	updateShootHibernationSpecAndCheckProber(g, k8sClient, cluster, shoot, &enableHibernation, reconciler, proberShouldNotBePresent)

	disableHibernation := false
	updateShootHibernationSpecAndCheckProber(g, k8sClient, cluster, shoot, &disableHibernation, reconciler, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func updateShootHibernationStatus(g *WithT, k8sClient client.Client, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster,
	shoot *gardencorev1beta1.Shoot, IsHibernated bool, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	shoot.Status.IsHibernated = IsHibernated
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, checkProber)
}

func testChangingHibernationStatus(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	updateShootHibernationStatus(g, k8sClient, reconciler, cluster, shoot, true, nil)
	updateShootHibernationStatus(g, k8sClient, reconciler, cluster, shoot, false, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func testInvalidShootInClusterSpec(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, _ := createClusterResource()
	cluster.Spec.Shoot.Object = nil
	cluster.Spec.Shoot.Raw = []byte(`{"apiVersion": 8}`)
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func updateShootDeletionTimeStamp(g *WithT, k8sClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot) {
	deletionTimeStamp, _ := time.Parse(time.RFC3339, "2022-05-05T08:34:05Z")
	shoot.DeletionTimestamp = &metav1.Time{
		Time: deletionTimeStamp,
	}
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	err := k8sClient.Update(context.Background(), cluster)
	g.Expect(err).To(BeNil())
}

func testProberShouldBeRemovedIfDeletionTimeStampIsSet(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	updateShootDeletionTimeStamp(g, k8sClient, cluster, shoot)
	proberShouldNotBePresent(g, reconciler, cluster)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
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

func testShootCreationNotComplete(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, shoot := createClusterResource()
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateProcessing)
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStatePending)
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateFailed)
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateError)
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateAborted)
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded)
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func testShootIsMigrating(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldNotBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func testLastOperationIsRestore(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, shoot := createClusterResource()
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, "")
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func testLastOperationIsShootReconciliation(t *testing.T, k8sClient client.Client, reconciler *ClusterReconciler) {
	g := NewWithT(t)
	cluster, shoot := createClusterResource()
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeReconcile, "")
	createClusterAndCheckProber(g, k8sClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, k8sClient, reconciler, cluster)
}

func proberShouldBePresent(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster) {
	g.Eventually(func() int { return len(reconciler.ProberMgr.GetAllProbers()) }, 10*time.Second, 1*time.Second).Should(Equal(1))
	prober, ok := reconciler.ProberMgr.GetProber(cluster.ObjectMeta.Name)
	g.Expect(ok).To(BeTrue())
	g.Expect(prober.IsClosed()).To(BeFalse())
}

func proberShouldNotBePresent(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster) {
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

func createClusterResource() (*gardenerv1alpha1.Cluster, *gardencorev1beta1.Shoot) {
	falseVal := false
	end := "00 08 * * 1,2,3,4,5"
	start := "30 19 * * 1,2,3,4,5"
	location := "Asia/Calcutta"

	cloudProfile := gardencorev1beta1.CloudProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws",
		},
	}
	seed := gardencorev1beta1.Seed{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws",
		},
	}
	shoot := gardencorev1beta1.Shoot{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shoot--test",
		},
		Spec: gardencorev1beta1.ShootSpec{
			Hibernation: &gardencorev1beta1.Hibernation{
				Enabled: &falseVal,
				Schedules: []gardencorev1beta1.HibernationSchedule{
					{End: &end, Start: &start, Location: &location},
				},
			},
		},
		Status: gardencorev1beta1.ShootStatus{
			IsHibernated: false,
			SeedName:     &seed.ObjectMeta.Name,
			LastOperation: &gardencorev1beta1.LastOperation{
				Type:  gardencorev1beta1.LastOperationTypeCreate,
				State: gardencorev1beta1.LastOperationStateSucceeded,
			},
		},
	}
	cluster := gardenerv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shoot--test",
		},
		Spec: gardenerv1alpha1.ClusterSpec{
			CloudProfile: runtime.RawExtension{
				Object: &cloudProfile,
			},
			Seed: runtime.RawExtension{
				Object: &seed,
			},
			Shoot: runtime.RawExtension{
				Object: &shoot,
			},
		},
	}
	return &cluster, &shoot
}
