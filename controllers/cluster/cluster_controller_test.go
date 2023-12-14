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

package cluster

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

func setupProberEnv(ctx context.Context, g *WithT) (client.Client, *envtest.Environment, *Reconciler, manager.Manager) {
	scheme := buildScheme()
	crdDirectoryPaths := []string{testdataPath}

	controllerTestEnv, err := testutil.CreateControllerTestEnv(scheme, crdDirectoryPaths, nil)
	g.Expect(err).ToNot(HaveOccurred())

	testEnv := controllerTestEnv.GetEnv()
	cfg := controllerTestEnv.GetConfig()
	crClient := controllerTestEnv.GetClient()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
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
		ProbeConfig:             proberConfig,
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
	cluster, _, err := testutil.CreateClusterResource(1, false)
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
		{"changing hibernation spec", testChangingHibernationSpec},
		{"changing hibernation status", testChangingHibernationStatus},
		{"invalid shoot in cluster spec", testInvalidShootInClusterSpec},
		{"deletion time stamp check", testProberShouldBeRemovedIfDeletionTimeStampIsSet},
		{"no prober if shoot creation is not successful", testShootCreationNotComplete},
		{"no prober if shoot control plane is migrating", testShootIsMigrating},
		{"no prober if shoot control plane is restoring after migrate", testShootRestoringIsNotComplete},
		{"start prober if last operation is restore and successfully", testLastOperationIsRestoreAndSuccessful},
		{"start prober if last operation is reconciliation of shoot", testLastOperationIsShootReconciliation},
		{"no prober if shoot has no workers", testShootHasNoWorkers},
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
	g.Expect(err).ToNot(HaveOccurred())
}

func createClusterAndCheckProber(g *WithT, crClient client.Client, reconciler *Reconciler,
	cluster *gardenerv1alpha1.Cluster, checkProber func(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster)) {
	err := crClient.Create(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
	time.Sleep(2 * time.Second) // giving some time for controller to take action
	if checkProber != nil {
		checkProber(g, reconciler, cluster)
	}
}

func updateClusterAndCheckProber(g *WithT, crClient client.Client, reconciler *Reconciler,
	cluster *gardenerv1alpha1.Cluster, checkProber func(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster)) {
	err := crClient.Update(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
	if checkProber != nil {
		checkProber(g, reconciler, cluster)
	}
}

func deleteClusterAndCheckIfProberRemoved(g *WithT, crClient client.Client, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster) {
	err := crClient.Delete(context.Background(), cluster)
	g.Expect(err).ToNot(HaveOccurred())
	proberShouldNotBePresent(g, reconciler, cluster)
}

func updateShootHibernationSpecAndCheckProber(g *WithT, crClient client.Client, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, isHibernationEnabled *bool,
	reconciler *Reconciler, checkProber func(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster)) {
	shoot.Spec.Hibernation.Enabled = isHibernationEnabled
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, checkProber)
}

func testChangingHibernationSpec(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	updateShootHibernationSpecAndCheckProber(g, crClient, cluster, shoot, pointer.Bool(true), reconciler, proberShouldNotBePresent)
	updateShootHibernationSpecAndCheckProber(g, crClient, cluster, shoot, pointer.Bool(false), reconciler, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func updateShootHibernationStatus(g *WithT, crClient client.Client, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster,
	shoot *gardencorev1beta1.Shoot, IsHibernated bool, checkProber func(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster)) {
	shoot.Status.IsHibernated = IsHibernated
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: shoot,
	}
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, checkProber)
}

func testChangingHibernationStatus(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	updateShootHibernationStatus(g, crClient, reconciler, cluster, shoot, true, nil) // checkProber is nil because in practice it is not possible that prober is present and cluster is hibernated.
	updateShootHibernationStatus(g, crClient, reconciler, cluster, shoot, false, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testInvalidShootInClusterSpec(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, _, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
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
	g.Expect(err).ToNot(HaveOccurred())
}

func testProberShouldBeRemovedIfDeletionTimeStampIsSet(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
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

func testShootCreationNotComplete(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
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

func setShootLastOperationStatusAndCheckProber(g *WithT, crClient client.Client, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster, shoot *gardencorev1beta1.Shoot, lastOpState gardencorev1beta1.LastOperationState) {
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeCreate, lastOpState)
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
}

func testShootIsMigrating(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testShootRestoringIsNotComplete(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	// cluster migration starts
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeMigrate, "")
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
	// cluster migrate done, restore in progress
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, gardencorev1beta1.LastOperationStateProcessing)
	updateClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testLastOperationIsRestoreAndSuccessful(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeRestore, gardencorev1beta1.LastOperationStateSucceeded)
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testLastOperationIsShootReconciliation(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, shoot, err := testutil.CreateClusterResource(1, false)
	g.Expect(err).ToNot(HaveOccurred())
	setShootLastOperationStatus(cluster, shoot, gardencorev1beta1.LastOperationTypeReconcile, "")
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldBePresent)
	deleteClusterAndCheckIfProberRemoved(g, crClient, reconciler, cluster)
}

func testShootHasNoWorkers(g *WithT, crClient client.Client, reconciler *Reconciler) {
	cluster, _, err := testutil.CreateClusterResource(0, false)
	g.Expect(err).ToNot(HaveOccurred())
	createClusterAndCheckProber(g, crClient, reconciler, cluster, proberShouldNotBePresent)
}

func proberShouldBePresent(g *WithT, reconciler *Reconciler, cluster *gardenerv1alpha1.Cluster) {
	g.Eventually(func() int { return len(reconciler.ProberMgr.GetAllProbers()) }, 10*time.Second, 1*time.Second).Should(Equal(1))
	prober, ok := reconciler.ProberMgr.GetProber(cluster.ObjectMeta.Name)
	g.Expect(ok).To(BeTrue())
	g.Expect(prober.IsClosed()).To(BeFalse())
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
