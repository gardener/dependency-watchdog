package controllers

import (
	"context"
	internalutils "github.com/gardener/dependency-watchdog/internal/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"path/filepath"
	"testing"
	"time"

	weederpackage "github.com/gardener/dependency-watchdog/internal/weeder"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	maxConcurrentReconcilesWeeder = 1
	epName                        = "etcd-main"
	namespace                     = "default"
	healthyPod                    = "pod-h"
	crashingPod                   = "pod-c"
	testPodName                   = "test-pod"
)

var (
	ctxCommonTests, ctxCommonTestsCancelFn = context.WithCancel(context.Background())
	correctLabels                          = map[string]string{
		"gardener.cloud/role": "controlplane",
		"role":                "NotEtcd",
	}

	inCorrectLabels = map[string]string{
		"incorrect-labels": "true",
	}

	createEp = func(t *testing.T, g *WithT, ctx context.Context, reconciler *EndpointReconciler) {
		e := newEndpoint(epName, namespace)
		g.Expect(reconciler.Client.Create(ctx, e)).To(BeNil())
		t.Log("New endpoint created")
	}
	startMgr = func(t *testing.T, ctx context.Context, g *WithT, scheme *runtime.Scheme, cfg *rest.Config, reconciler *EndpointReconciler) {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme,
		})
		err = reconciler.SetupWithManager(mgr)
		g.Expect(err).To(BeNil())
		t.Log("Started manager for test")
		err = mgr.Start(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	}
	stopMgr = func(t *testing.T, cancelFn context.CancelFunc) {
		cancelFn()
		t.Log("Stopping Manager")
	}
)

func setupWeederEnv(t *testing.T, g *WithT, ctx context.Context, apiServerFlags map[string]string, withManager bool) (client.Client, *envtest.Environment, *EndpointReconciler, *runtime.Scheme, *rest.Config) {
	t.Log("setting up the test Env for Weeder")
	scheme := buildScheme()
	testEnv := &envtest.Environment{
		Scheme:                scheme,
		ErrorIfCRDPathMissing: false,
	}

	kubeAPIServer := testEnv.ControlPlane.GetAPIServer()
	args := kubeAPIServer.Configure()
	for k, v := range apiServerFlags {
		args.Set(k, v)
	}

	cfg, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg).NotTo(BeNil())

	crClient, err := client.New(cfg, client.Options{Scheme: scheme})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(crClient).NotTo(BeNil())

	clientSet, err := internalutils.CreateClientSetFromRestConfig(cfg)
	g.Expect(err).NotTo(HaveOccurred())

	weederConfigPath := filepath.Join(testdataPath, "config", "weeder-config.yaml")
	validateIfFileExists(weederConfigPath, g)
	weederConfig, err := weederpackage.LoadConfig(weederConfigPath)
	g.Expect(err).To(BeNil())

	epReconciler := &EndpointReconciler{
		Client:                  crClient,
		WeederConfig:            weederConfig,
		SeedClient:              clientSet,
		WeederMgr:               weederpackage.NewManager(),
		MaxConcurrentReconciles: maxConcurrentReconcilesWeeder,
	}

	if withManager {
		go startMgr(t, ctx, g, scheme, cfg, epReconciler)
	}

	return crClient, testEnv, epReconciler, scheme, cfg
}

func TestEndpointsControllerSuite(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"tests with common environment", testWeederCommonEnvTest},
		{"tests with dedicated environment for each test", testWeederDedicatedEnvTest},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
}

func testWeederCommonEnvTest(t *testing.T) {
	g := NewWithT(t)

	_, testEnv, reconciler, scheme, config := setupWeederEnv(t, g, ctxCommonTests, nil, false)
	defer teardownEnv(t, g, testEnv, ctxCommonTestsCancelFn)

	tests := []struct {
		title string
		run   func(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config)
	}{
		{"Single Crashlooping pod , single healthy pod with matching labels expect only Crashlooping pod to be deleted", testOnlyCLBFpodDeletion},
		{"Single healthy pod, turned to CrashLoopBackoff , should be deleted", testPodTurningCLBFDeletion},
		{"Single CrashLooping pod with non-matching labels present, shouldn't be deleted", testCLBFPodWithWrongLabelsDeletion},
		{"Single healthy pod with matching labels turning to CrashLoopBackoff after watchDuration, shouldn't be deleted", testPodTurningCLBFAfterWatchDuration},
		{"Single CrashLooping pod with matching label shouldn't be deleted when endpoint is not Ready", testNoCLBFPodDeletionWhenEndpointNotReady},
		{"No pod termination happens when main context is cancelled", testNoCLBFPodDeletionOnContextCancellation},
	}

	createEp(t, g, ctxCommonTests, reconciler)
	for _, test := range tests {
		mgrCtx, mgrCancelFn := context.WithCancel(ctxCommonTests)
		t.Run(test.title, func(t *testing.T) {
			test.run(t, g, mgrCtx, mgrCancelFn, reconciler, scheme, config)
		})
		deleteAllPods(ctxCommonTests, g, reconciler.Client)
	}
}

func testWeederDedicatedEnvTest(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		title          string
		run            func(t *testing.T, g *WithT, ctx context.Context, reconciler *EndpointReconciler)
		apiServerFlags map[string]string
	}{
		{"single Crashlooping pod should be deleted even when watch on pods times-out in the middle", testPodWatchEndsAbruptlyBeforeSpecifiedWatchDuration, map[string]string{"min-request-timeout": "5"}},
	}
	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		_, testEnv, reconciler, _, _ := setupWeederEnv(t, g, ctx, test.apiServerFlags, true)
		t.Run(test.title, func(t *testing.T) {
			test.run(t, g, ctx, reconciler)
		})
		teardownEnv(t, g, testEnv, cancelFn)
	}
}

// tests
// case 1: single pod in CLBF deleted, single healthy pod , other healthy pod remained
// case 2: single pod healthy first, turned to CLBF gets deleted
// case 3: CLBF pod not having req labels is not deleted
// case 4: pod turned CLBF after the watch duration is not deleted
// case 5: deletion of CLBF pod shouldn't happen if endpoint is not ready (means the serving pod is not present/not ready)
// case 6: cancelling the context should mean no deletion of CLBF pod happens
// case 7: watch cancelled by API server, should lead to create of new watch (#dedicated env test)
func testOnlyCLBFpodDeletion(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config) {
	pC := newPod(crashingPod, "node-0", correctLabels)
	pH := newPod(healthyPod, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pH)
	g.Expect(err).To(BeNil())
	turnPodToHealthy(g, ctx, reconciler.Client, pH)

	err = reconciler.Client.Create(ctx, pC)
	g.Expect(err).To(BeNil())
	turnPodToCrashLoop(g, ctx, reconciler.Client, pC)

	pl, err := reconciler.SeedClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	g.Expect(err).To(BeNil())
	g.Expect(len(pl.Items)).Should(Equal(2))

	t.Log("2 pods are present, 1 CrashLooping , 1 Healthy")

	go startMgr(t, ctx, g, scheme, config, reconciler)
	defer stopMgr(t, cancelFn)

	t.Log("waiting for controller to act")

	// wait for endpoint controller to take action
	time.Sleep(5 * time.Second)

	t.Log("validating expectations")
	resultpC := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crashingPod}, &resultpC)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CrashLooping pod should've been deleted")

	resultpH := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: healthyPod}, &resultpH)
	g.Expect(err).To(BeNil())
	g.Expect(resultpH.DeletionTimestamp).To(BeNil(), "Healthy pod shouldn't be deleted")
}

func testPodTurningCLBFDeletion(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config) {
	pH := newPod(testPodName, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pH)
	g.Expect(err).To(BeNil())
	turnPodToHealthy(g, ctx, reconciler.Client, pH)

	pl, err := reconciler.SeedClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(len(pl.Items)).Should(Equal(1))

	t.Log("a healthy pod present")

	go startMgr(t, ctx, g, scheme, config, reconciler)
	defer stopMgr(t, cancelFn)

	t.Log("turning pod to CrashLooping")
	turnPodToCrashLoop(g, ctx, reconciler.Client, pH)

	t.Log("waiting for controller to act")

	// wait for endpoint controller to take action
	time.Sleep(5 * time.Second)

	t.Log("validating expectations")
	resultpC := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: testPodName}, &resultpC)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CrashLooping pod should be deleted")
}

func testCLBFPodWithWrongLabelsDeletion(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config) {
	pC := newPod(crashingPod, "node-0", inCorrectLabels)
	err := reconciler.Client.Create(ctx, pC)
	g.Expect(err).To(BeNil())
	turnPodToCrashLoop(g, ctx, reconciler.Client, pC)

	t.Log("a CrashLooping pod with non-matching labels present")

	go startMgr(t, ctx, g, scheme, config, reconciler)
	defer stopMgr(t, cancelFn)

	t.Log("waiting for controller to act")
	time.Sleep(5 * time.Second)

	t.Log("validating expectations")
	resultpC := v1.Pod{}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(pC), &resultpC)
	g.Expect(err).To(BeNil())
	g.Expect(resultpC.DeletionTimestamp).To(BeNil(), "CrashLoop Pod shouldn't be deleted in this case")
}

func testPodTurningCLBFAfterWatchDuration(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config) {
	pT := newPod(testPodName, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pT)
	g.Expect(err).To(BeNil())
	turnPodToHealthy(g, ctx, reconciler.Client, pT)

	go startMgr(t, ctx, g, scheme, config, reconciler)
	defer stopMgr(t, cancelFn)

	// introducing wait
	time.Sleep(*reconciler.WeederConfig.WatchDuration + 2*time.Second)

	turnPodToCrashLoop(g, ctx, reconciler.Client, pT)

	t.Log("waiting for controller to act")
	time.Sleep(5 * time.Second)

	t.Log("validating expectations")
	resultpT := v1.Pod{}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(pT), &resultpT)
	g.Expect(err).To(BeNil())
	g.Expect(resultpT.DeletionTimestamp).To(BeNil(), "CrashLoop pod shouldn't be deleted in this case")
}

func testNoCLBFPodDeletionWhenEndpointNotReady(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config) {
	pC := newPod(crashingPod, "node-0", correctLabels)
	err := reconciler.Client.Create(ctx, pC)
	g.Expect(err).To(BeNil())
	turnPodToCrashLoop(g, ctx, reconciler.Client, pC)

	ep := &v1.Endpoints{}
	g.Expect(reconciler.Client.Get(ctx, types.NamespacedName{Name: epName, Namespace: namespace}, ep)).To(Succeed())
	turnEndpointToNotReady(g, ctx, reconciler.Client, ep)

	go startMgr(t, ctx, g, scheme, config, reconciler)
	defer stopMgr(t, cancelFn)

	t.Log("waiting for controller to act")
	time.Sleep(5 * time.Second)

	t.Log("validating expectations")
	resultpC := v1.Pod{}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(pC), &resultpC)
	g.Expect(err).To(BeNil())
	g.Expect(resultpC.DeletionTimestamp).To(BeNil())

	g.Expect(reconciler.Client.Get(ctx, client.ObjectKeyFromObject(ep), ep)).To(Succeed())
	turnEndpointToReady(g, ctx, reconciler.Client, ep)
}

func testNoCLBFPodDeletionOnContextCancellation(t *testing.T, g *WithT, ctx context.Context, cancelFn context.CancelFunc, reconciler *EndpointReconciler, scheme *runtime.Scheme, config *rest.Config) {
	pC := newPod(crashingPod, "node-0", correctLabels)
	err := reconciler.Client.Create(ctx, pC)
	g.Expect(err).To(BeNil())
	turnPodToCrashLoop(g, ctx, reconciler.Client, pC)

	go startMgr(t, ctx, g, scheme, config, reconciler)
	defer stopMgr(t, cancelFn)

	// cancel main context (like SIGKILL signal to the process)
	t.Log("cancelling context")
	ctxCommonTestsCancelFn()

	t.Log("validating expectations")
	resultpC, err := reconciler.SeedClient.CoreV1().Pods(namespace).Get(context.TODO(), crashingPod, metav1.GetOptions{})
	g.Expect(err).To(BeNil())
	g.Expect(resultpC.DeletionTimestamp).To(BeNil())
}

func testPodWatchEndsAbruptlyBeforeSpecifiedWatchDuration(t *testing.T, g *WithT, ctx context.Context, reconciler *EndpointReconciler) {
	pH := newPod(testPodName, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pH)
	g.Expect(err).To(BeNil())
	turnPodToHealthy(g, ctx, reconciler.Client, pH)

	t.Log("a healthy pod present")

	// new endpoint creation should trigger watch creation
	createEp(t, g, ctx, reconciler)

	// waiting more than "min-request-timeout"(5sec) so that watch gets cancelled by APIServer
	time.Sleep(10 * time.Second)

	t.Log("turning pod to CrashLooping")
	turnPodToCrashLoop(g, ctx, reconciler.Client, pH)

	t.Log("waiting for controller to act")
	// wait for endpoint controller to take action
	time.Sleep(3 * time.Second)

	resultpC := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: testPodName}, &resultpC)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CrashLooping pod should be deleted due to recreation of watch even after cancellation")
}

func deleteAllPods(ctx context.Context, g *WithT, crClient client.Client) {
	pl := &v1.PodList{}
	select {
	case <-ctx.Done():
		return
	default:
		g.Expect(crClient.List(ctx, pl)).To(Succeed())
		for _, po := range pl.Items {
			g.Expect(crClient.Delete(ctx, &po)).To(Succeed())
		}
	}
}

func newPod(name, host string, labels map[string]string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: pointer.Int64(0),
			Containers: []v1.Container{
				{Name: "test-container", Image: "nginx:latest"},
			},
			NodeName: host,
		},
	}

	return pod
}

func turnPodToCrashLoop(g *WithT, ctx context.Context, crClient client.Client, p *v1.Pod) {
	pClone := p.DeepCopy()
	pClone.Status = v1.PodStatus{
		ContainerStatuses: []v1.ContainerStatus{
			{
				Name: "Container-0",
				State: v1.ContainerState{
					Waiting: &v1.ContainerStateWaiting{
						Reason:  "CrashLoopBackOff",
						Message: "Container is in CrashLoopBackOff.",
					},
				},
			},
		},
	}
	g.Expect(crClient.Status().Patch(ctx, pClone, client.MergeFrom(p))).To(Succeed())
}

func turnPodToHealthy(g *WithT, ctx context.Context, crClient client.Client, p *v1.Pod) {
	pClone := p.DeepCopy()
	pClone.Status = v1.PodStatus{
		ContainerStatuses: []v1.ContainerStatus{
			{
				Name: "Container-0",
			},
		},
	}
	g.Expect(crClient.Status().Patch(ctx, pClone, client.MergeFrom(p))).To(Succeed())
}

func newEndpoint(name, namespace string) *v1.Endpoints {
	e := v1.Endpoints{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Endpoints"},
		ObjectMeta: metav1.ObjectMeta{
			UID:         uuid.NewUUID(),
			Name:        name,
			Namespace:   namespace,
			Annotations: make(map[string]string),
			Labels:      make(map[string]string),
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP:       "10.1.0.52",
						NodeName: pointer.String("node-1"),
					},
				},
				NotReadyAddresses: []v1.EndpointAddress{},
				Ports:             []v1.EndpointPort{},
			},
		},
	}
	return &e
}

func turnEndpointToNotReady(g *WithT, ctx context.Context, client client.Client, ep *v1.Endpoints) {
	epClone := ep.DeepCopy()
	epClone.Subsets[0].Addresses = nil
	epClone.Subsets[0].NotReadyAddresses = []v1.EndpointAddress{
		{
			IP:       "10.1.0.0",
			NodeName: pointer.String("node-1"),
		},
	}
	g.Expect(client.Update(ctx, epClone)).To(Succeed())
}

func turnEndpointToReady(g *WithT, ctx context.Context, client client.Client, ep *v1.Endpoints) {
	epClone := ep.DeepCopy()
	epClone.Subsets[0].Addresses = []v1.EndpointAddress{
		{
			IP:       "10.1.0.0",
			NodeName: pointer.String("node-1"),
		},
	}

	g.Expect(client.Update(ctx, epClone)).To(Succeed())
}