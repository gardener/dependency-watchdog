// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !kind_tests

package controllers

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/utils/pointer"

	proberpackage "github.com/gardener/dependency-watchdog/internal/prober"
	testenv "github.com/gardener/dependency-watchdog/internal/test"
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

func setupProberEnv(ctx context.Context, g *WithT) (client.Client, *envtest.Environment, *ClusterReconciler, manager.Manager) {
	scheme := buildScheme()
	crdDirectoryPaths := []string{filepath.Join("testdata", "crd", "prober")}

	controllerTestEnv, err := testenv.CreateControllerTestEnv(scheme, crdDirectoryPaths)
	g.Expect(err).To(BeNil())

	testEnv := controllerTestEnv.GetEnv()
	cfg := controllerTestEnv.GetConfig()
	crClient := controllerTestEnv.GetClient()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	g.Expect(err).ToNot(HaveOccurred())

	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).To(BeNil())

	probeConfigPath := filepath.Join(testdataPath, "config", "prober-config.yaml")
	validateIfFileExists(probeConfigPath, g)
	proberConfig, err := proberpackage.LoadConfig(probeConfigPath, scheme)
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

	return crClient, testEnv, clusterReconciler, mgr
}

func teardownEnv(g *WithT, testEnv *envtest.Environment, cancelFn context.CancelFunc) {
	cancelFn()
	err := testEnv.Stop()
	g.Expect(err).NotTo(HaveOccurred())
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
		run   func(ctx context.Context, t *testing.T, envTest *envtest.Environment, crClient client.Client, reconciler *ClusterReconciler, mgr manager.Manager, cancelFn context.CancelFunc)
	}{
		{"calling reconciler after shutting down API server", testReconciliationAfterAPIServerIsDown},
	}
	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		crClient, testEnv, reconciler, mgr := setupProberEnv(ctx, g)
		t.Run(test.title, func(t *testing.T) {
			test.run(ctx, t, testEnv, crClient, reconciler, mgr, cancelFn)
		})
		teardownEnv(g, testEnv, cancelFn)
	}
}

func testReconciliationAfterAPIServerIsDown(ctx context.Context, t *testing.T, testEnv *envtest.Environment, _ client.Client, reconciler *ClusterReconciler, _ manager.Manager, cancelFn context.CancelFunc) {
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

// testProberSharedEnvTest creates an envTest just once and that is then shared by all the subtests. Shared envTest is destroyed once all subtests have run.
func testProberSharedEnvTest(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	crClient, testEnv, reconciler, _ := setupProberEnv(ctx, g)
	defer teardownEnv(g, testEnv, cancelFn)

	tests := []struct {
		title string
		run   func(g *WithT, crClient client.Client, reconciler *ClusterReconciler)
	}{
		{"changing hibernation spec", testChangingHibernationSpec},
		{"changing hibernation status", testChangingHibernationStatus},
		{"invalid shoot in cluster spec", testInvalidShootInClusterSpec},
		{"deletion time stamp check", testProberShouldBeRemovedIfDeletionTimeStampIsSet},
		{"no prober if shoot creation is not successful", testShootCreationNotComplete},
		{"no prober if shoot control plane is migrating", testShootIsMigrating},
		{"no prober if shoot control plane is restoring after migrate", testShootRestoringIsNotComplete},
		{"start prober if last operation is restore and successfully", testLastOperationIsRestoreAndSuccessful},
		{"start prober if last operation is reconciliation of shoot", testLastOperationIsShootReconciliation},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(g, crClient, reconciler)
		})
		deleteAllClusters(g, crClient)
	}
}

func deleteAllClusters(g *WithT, crClient client.Client) {
	err := crClient.DeleteAllOf(context.Background(), &gardenerv1alpha1.Cluster{})
	g.Expect(err).To(BeNil())
}

func createClusterAndCheckProber(g *WithT, crClient client.Client, reconciler *ClusterReconciler,
	cluster *gardenerv1alpha1.Cluster, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	err := crClient.Create(context.Background(), cluster)
	g.Expect(err).To(BeNil())
	time.Sleep(2 * time.Second) // giving some time for controller to take action
	if checkProber != nil {
		checkProber(g, reconciler, cluster)
	}
}

func updateClusterAndCheckProber(g *WithT, crClient client.Client, reconciler *ClusterReconciler,
	cluster *gardenerv1alpha1.Cluster, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	err := crClient.Update(context.Background(), cluster)
	g.Expect(err).To(BeNil())
	if checkProber != nil {
		checkProber(g, reconciler, cluster)
	}
}

func deleteClusterAndCheckIfProberRemoved(g *WithT, crClient client.Client, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster) {
	err := crClient.Delete(context.Background(), cluster)
	g.Expect(err).To(BeNil())
	proberShouldNotBePresent(g, reconciler, cluster)
}

func updateShootHibernationSpecAndCheckProber(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, isHibernationEnabled *bool,
	reconciler *ClusterReconciler, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	shoot.Spec.Hibernation.Enabled = isHibernationEnabled
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, checkProber)
}

func testChangingHibernationSpec(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	updateShootHibernationSpecAndCheckProber(g, crClient, cluster, shoot, pointer.Bool(true), reconciler, proberShouldNotBePresent)
	updateShootHibernationSpecAndCheckProber(g, crClient, cluster, shoot, pointer.Bool(false), reconciler, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func updateShootHibernationStatus(g *WithT, crClient client.Client, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster,
	shoot *gardencorev1beta1.Shoot, IsHibernated bool, checkProber func(g *WithT, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster)) {
	shoot.Status.IsHibernated = IsHibernated
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, checkProber)
}

func testChangingHibernationStatus(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	updateShootHibernationStatus(g, crClient, reconciler, cluster, shoot, true, nil) // checkProber is nil because in practice it is not possible that prober is present and cluster is hibernated.
	updateShootHibernationStatus(g, crClient, reconciler, cluster, shoot, false, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testInvalidShootInClusterSpec(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, _ := createClusterResource()
	cluster.Spec.Shoot.Object = nil
	cluster.Spec.Shoot.Raw = []byte(`{"apiVersion": 8}`)
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
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
	g.Expect(err).To(BeNil())
}

func testProberShouldBeRemovedIfDeletionTimeStampIsSet(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
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

func testShootCreationNotComplete(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateProcessing)
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)

	setShootLastOperationStatusAndCheckProber(g, crClient, reconciler, cluster, shoot, gardencorev1beta1.LastOperationStatePending)
	setShootLastOperationStatusAndCheckProber(g, crClient, reconciler, cluster, shoot, gardencorev1beta1.LastOperationStateFailed)
	setShootLastOperationStatusAndCheckProber(g, crClient, reconciler, cluster, shoot, gardencorev1beta1.LastOperationStateError)
	setShootLastOperationStatusAndCheckProber(g, crClient, reconciler, cluster, shoot, gardencorev1beta1.LastOperationStateAborted)

	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, gardencorev1beta1.LastOperationStateSucceeded)
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func setShootLastOperationStatusAndCheckProber(g *WithT, crClient client.Client, reconciler *ClusterReconciler, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, lastOpState gardencorev1beta1.LastOperationState) {
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, lastOpState)
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
}

func testShootIsMigrating(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testShootRestoringIsNotComplete(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	// cluster migration starts
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
	// cluster migrate done, restore in progress
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, gardencorev1beta1.LastOperationStateProcessing)
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testLastOperationIsRestoreAndSuccessful(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, gardencorev1beta1.LastOperationStateSucceeded)
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testLastOperationIsShootReconciliation(g *WithT, crClient client.Client, reconciler *ClusterReconciler) {
	cluster, shoot := createClusterResource()
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeReconcile, "")
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
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

func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	localSchemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		gardenerv1alpha1.AddToScheme,
	)
	utilruntime.Must(localSchemeBuilder.AddToScheme(scheme))
	return scheme
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
