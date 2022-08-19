package controllers

import (
	"context"
	internalutils "github.com/gardener/dependency-watchdog/internal/util"
	v1 "k8s.io/api/core/v1"
	"log"
	"path/filepath"
	"testing"
	"time"

	weederpackage "github.com/gardener/dependency-watchdog/internal/weeder"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	maxConcurrentReconcilesWeeder = 1
)

func setupWeederEnv(g *WithT, ctx context.Context) (client.Client, *envtest.Environment, *EndpointReconciler, manager.Manager) {
	log.Println("setting up the test Env for Weeder")
	scheme := buildScheme()
	testEnv := &envtest.Environment{
		ErrorIfCRDPathMissing: false,
		Scheme:                scheme,
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).NotTo(BeNil())

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k8sClient).NotTo(BeNil())

	clientSet, err := internalutils.CreateClientSetFromRestConfig(cfg)
	g.Expect(err).NotTo(HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	g.Expect(err).ToNot(HaveOccurred())

	weederConfigPath := filepath.Join(testdataPath, "weeder-config.yaml")
	validateIfFileExists(weederConfigPath, g)
	weederConfig, err := weederpackage.LoadConfig(weederConfigPath)
	g.Expect(err).To(BeNil())

	epReconciler := &EndpointReconciler{
		Client:                  mgr.GetClient(),
		WeederConfig:            weederConfig,
		SeedClient:              clientSet,
		WeederMgr:               weederpackage.NewManager(),
		MaxConcurrentReconciles: maxConcurrentReconcilesWeeder,
	}
	err = epReconciler.SetupWithManager(mgr)
	g.Expect(err).To(BeNil())

	//go func() {
	//	err = mgr.Start(ctx)
	//	g.Expect(err).ToNot(HaveOccurred())
	//}()

	return k8sClient, testEnv, epReconciler, mgr
}

func TestEndpointControllerSuite(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	_, testEnv, reconciler, mgr := setupWeederEnv(g, ctx)
	defer teardownEnv(g, testEnv, cancelFn)
	tests := []struct {
		title string
		run   func(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, mgr manager.Manager, reconciler *EndpointReconciler)
	}{
		{"single CrashloopBackOff pod , single healthy pod expect only CrashloopBackoff pod to be deleted", testOnlyCLBFpodDeletion},
	}
	for _, test := range tests {
		mgrCtx, mgrCancelFn := context.WithCancel(ctx)
		t.Run("endpoints controller test", func(t *testing.T) {
			test.run(t, g, mgrCtx, mgrCancelFn, mgr, reconciler)
		})
		deleteAllPods(g, reconciler.Client)
	}
}

// tests
// case 1: single pod in CLBF deleted, single healthy pod , other healthy pod remained
// case 2: single pod healthy first, turned to CLBF gets deleted
// case 3: CLBF pod not having req labels is not deleted
// case 4: pod turned CLBF after the watch duration is not deleted
// case 5: deletion of CLBF pod shouldn't happen if endpoint is not ready (means the serving pod is not present/not ready)
// case 6: cancelling the context should mean no deletion of CLBF pod happens

// two ways:
// 1. Create an update event for endpoint by yourself so that reconciler starts a weeder, no need to create a pod backing the svc
// 2. without event need, we can simply call the `Run` fn to start weeder, this will involve not setting up manager, but we won't be able to check case 3 as
// its done by predicates.
func testOnlyCLBFpodDeletion(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, mgr manager.Manager, reconciler *EndpointReconciler) {
	// stop the manager after the test
	defer cancelFn()

	const (
		healthyPod  = "pod-h"
		crashingPod = "pod-c"
	)

	pC := newPodInCrashloop(crashingPod, map[string]string{
		"garden.sapcloud.io/role": "controlplane",
		"role":                    "NotEtcd",
	})
	pH := newPodHealthy(healthyPod, map[string]string{
		"garden.sapcloud.io/role": "controlplane",
		"role":                    "NotEtcd",
	})

	err := reconciler.Client.Create(ctx, pH)
	g.Expect(err).ToNot(HaveOccurred())
	err = reconciler.Client.Create(ctx, pC)
	g.Expect(err).ToNot(HaveOccurred())

	pl := &v1.PodList{}
	err = reconciler.Client.List(ctx, pl)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(len(pl.Items)).Should(Equal(2))

	go func() {
		err = mgr.Start(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	}()

	// wait for endpoint controller to take action
	time.Sleep(2 * time.Second)

	err = reconciler.Client.List(ctx, pl)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(len(pl.Items)).Should(Equal(1))
	g.Expect(pl.Items[0].Name).Should(Equal(healthyPod), "Crashloop backoff pod should have been deleted")
}

func deleteAllPods(g *WithT, k8sClient client.Client) {
	err := k8sClient.DeleteAllOf(context.Background(), &v1.Pod{})
	g.Expect(err).To(BeNil())
}

func newPod(name, host string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: v1.PodSpec{
			NodeName: host,
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	return pod
}

func newPodInCrashloop(name string, labels map[string]string) *v1.Pod {
	p := newPod(name, "node-0")
	p.Labels = labels
	p.Namespace = metav1.NamespaceDefault
	p.Status.ContainerStatuses = []v1.ContainerStatus{
		{
			Name: "Container-0",
			State: v1.ContainerState{
				Waiting: &v1.ContainerStateWaiting{
					Reason:  "CrashLoopBackOff",
					Message: "Container is in CrashLoopBackOff.",
				},
			},
		},
	}
	return p
}

func newPodHealthy(name string, labels map[string]string) *v1.Pod {
	p := newPod(name, "node-0")
	p.Labels = labels
	p.Namespace = metav1.NamespaceDefault
	p.Status.ContainerStatuses = []v1.ContainerStatus{
		{
			Name: "Container-0",
		},
	}
	return p
}

func makePodUnhealthy(p *v1.Pod) *v1.Pod {
	p.Status.ContainerStatuses = []v1.ContainerStatus{
		{
			Name: "Container-0",
			State: v1.ContainerState{
				Waiting: &v1.ContainerStateWaiting{
					Reason:  "CrashLoopBackOff",
					Message: "Container is in CrashLoopBackOff.",
				},
			},
		},
	}
	return p
}
