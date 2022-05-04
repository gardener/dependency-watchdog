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

	//"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	k8sClient          client.Client
	testEnv            *envtest.Environment
	cluster            gardenerv1alpha1.Cluster
	shoot              gardencorev1beta1.Shoot
	reconciler         *ClusterReconciler
	ctx, cancelFn      = context.WithCancel(context.Background())
	enableHibernation  = true
	disableHibernation = false
)

const (
	testdataPath = "testdata"
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

func beforeAll(g *WithT) {
	log.Println("running before all, setting up the test Env")
	scheme := buildScheme()
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("testdata", "crd")},
		ErrorIfCRDPathMissing: false,
		Scheme:                scheme,
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).NotTo(BeNil())
	log.Println("testEnvStarted")

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	g.Expect(err).ToNot(HaveOccurred())

	scalesGetter, err := util.CreateScalesGetter(cfg)
	g.Expect(err).To(BeNil())

	log.Println("creating prober config")
	probeConfigPath := filepath.Join(testdataPath, "config.yaml")
	validateIfFileExists(probeConfigPath, g)
	proberConfig, err := proberpackage.LoadConfig(probeConfigPath)
	g.Expect(err).To(BeNil())

	log.Println("creating cluster reconciler")
	reconciler = &ClusterReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		ScaleGetter: scalesGetter,
		ProberMgr:   proberpackage.NewManager(),
		ProbeConfig: proberConfig,
	}
	err = reconciler.SetupWithManager(mgr)
	g.Expect(err).To(BeNil())

	log.Println("starting Manager")
	go func() {
		err = mgr.Start(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	}()
	log.Println("manager started")

	cluster = *createClusterResource()
	log.Println("cluster resource created")
}

func afterAll(g *WithT) {
	log.Println("running afterAll, destroying the test Env")
	cancelFn()
	err := testEnv.Stop()
	g.Expect(err).NotTo(HaveOccurred())
}

func TestClusterReconciler(t *testing.T) {
	g := NewWithT(t)
	beforeAll(g)
	defer afterAll(g)
	g.Expect(len(reconciler.ProberMgr.GetAllProbers())).To(Equal(0))

	// creating cluster should create prober
	err := k8sClient.Create(ctx, &cluster)
	g.Expect(err).To(BeNil())
	proberShouldBePresent(g)

	// enabling hibernation should remove prober
	shoot.Spec.Hibernation.Enabled = &enableHibernation
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: &shoot,
	}
	err = k8sClient.Update(ctx, &cluster)
	g.Expect(err).To(BeNil())
	proberShouldNotBePresent(g)

	// disabling hibernation should add the prober
	shoot.Spec.Hibernation.Enabled = &disableHibernation
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: &shoot,
	}
	err = k8sClient.Update(ctx, &cluster)
	g.Expect(err).To(BeNil())
	proberShouldBePresent(g)

	// if shoot is hibernated then prober should be removed
	shoot.Status.IsHibernated = true
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: &shoot,
	}
	err = k8sClient.Update(ctx, &cluster)
	g.Expect(err).To(BeNil())
	proberShouldNotBePresent(g)

	// if shoot wakes up from hibernation prober should be added
	shoot.Status.IsHibernated = false
	cluster.Spec.Shoot = runtime.RawExtension{
		Object: &shoot,
	}
	err = k8sClient.Update(ctx, &cluster)
	g.Expect(err).To(BeNil())
	proberShouldBePresent(g)

	// deleting cluster should remove the prober
	err = k8sClient.Delete(ctx, &cluster)
	g.Expect(err).To(BeNil())
	proberShouldNotBePresent(g)

	// if shoot is not obtained from cluster then requeue it and don't add or remove any probers
	//cluster.Spec.Shoot.Object = nil
	newCluster := createClusterResource()
	newCluster.Spec.Shoot.Raw = []byte(`{"key":"value"}`)
	err = k8sClient.Create(ctx, newCluster)
	g.Expect(err).To(BeNil())
	proberShouldNotBePresent(g)

	// err = k8sClient.Delete(ctx, &cluster)
	// g.Expect(err).To(BeNil())
}

func proberShouldBePresent(g *WithT) {
	g.Eventually(func() int { return len(reconciler.ProberMgr.GetAllProbers()) }, 10*time.Second, 1*time.Second).Should(Equal(1))
	prober, ok := reconciler.ProberMgr.GetProber(cluster.ObjectMeta.Name)
	g.Expect(ok).To(BeTrue())
	g.Expect(prober.IsClosed()).To(BeFalse())
}

func proberShouldNotBePresent(g *WithT) {
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

func createClusterResource() *gardenerv1alpha1.Cluster {
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
	shoot = gardencorev1beta1.Shoot{
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
		},
	}
	return &gardenerv1alpha1.Cluster{
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
}
