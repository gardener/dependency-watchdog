// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package endpoint

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/config"

	"k8s.io/apimachinery/pkg/util/rand"

	wapi "github.com/gardener/dependency-watchdog/api/weeder"
	testutil "github.com/gardener/dependency-watchdog/internal/test"
	"k8s.io/client-go/kubernetes/scheme"

	internalutils "github.com/gardener/dependency-watchdog/internal/util"
	weederpackage "github.com/gardener/dependency-watchdog/internal/weeder"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	maxConcurrentReconcilesWeeder = 1
	epName                        = "etcd-main"
	healthyPod                    = "pod-h"
	crashingPod                   = "pod-c"
	testPodName                   = "test-pod"
	testdataPath                  = "testdata"
)

var (
	correctLabels = map[string]string{
		"gardener.cloud/role": "controlplane",
		"role":                "NotEtcd",
	}
	inCorrectLabels = map[string]string{
		"incorrect-labels": "true",
	}
)

func setupWeederEnv(ctx context.Context, t *testing.T, kubeApiServerFlags map[string]string) (*envtest.Environment, *Reconciler) {
	s := scheme.Scheme
	g := NewWithT(t)

	controllerTestEnv, err := testutil.CreateDefaultControllerTestEnv(s, kubeApiServerFlags)
	g.Expect(err).ToNot(HaveOccurred())

	testEnv := controllerTestEnv.GetEnv()
	cfg := controllerTestEnv.GetConfig()
	crClient := controllerTestEnv.GetClient()

	clientSet, err := internalutils.CreateClientSetFromRestConfig(cfg)
	g.Expect(err).NotTo(HaveOccurred())

	weederConfigPath := filepath.Join(testdataPath, "weeder-config.yaml")
	testutil.ValidateIfFileExists(weederConfigPath, t)
	weederConfig, err := weederpackage.LoadConfig(weederConfigPath)
	g.Expect(err).ToNot(HaveOccurred())

	epReconciler := &Reconciler{
		Client:                  crClient,
		WeederConfig:            weederConfig,
		SeedClient:              clientSet,
		WeederMgr:               weederpackage.NewManager(),
		MaxConcurrentReconciles: maxConcurrentReconcilesWeeder,
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: s,
		Controller: config.Controller{
			SkipNameValidation: ptr.To(true),
		},
	})

	g.Expect(err).ToNot(HaveOccurred())
	err = epReconciler.SetupWithManager(mgr)
	g.Expect(err).ToNot(HaveOccurred())
	go func() {
		err = mgr.Start(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	}()

	return testEnv, epReconciler
}

func TestEndpointsControllerSuite(t *testing.T) {
	tests := []struct {
		title string
		run   func(t *testing.T)
	}{
		{"tests with shared environment", testWeederSharedEnvTest},
		{"tests with dedicated environment for each test", testWeederDedicatedEnvTest},
	}
	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			test.run(t)
		})
	}
}

func testWeederSharedEnvTest(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	testEnv, reconciler := setupWeederEnv(ctx, t, nil)
	defer testutil.TeardownEnv(g, testEnv, cancelFn)

	tests := []struct {
		name        string
		description string
		run         func(ctx context.Context, cancelFn context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string)
	}{
		{"testOnlyCLBFPodDeletion", "Single Crashlooping pod , single healthy pod with matching labels expect only Crashlooping pod to be deleted", testOnlyCLBFPodDeletion},
		{"testPodTurningCLBFDeletion", "Single healthy pod, turned to CrashLoopBackoff , should be deleted", testPodTurningCLBFDeletion},
		{"testCLBFPodWithWrongLabelsDeletion", "Single CrashLooping pod with non-matching labels present, shouldn't be deleted", testCLBFPodWithWrongLabelsDeletion},
		{"testPodTurningCLBFAfterWatchDuration", "Single healthy pod with matching labels turning to CrashLoopBackoff after watchDuration, shouldn't be deleted", testPodTurningCLBFAfterWatchDuration},
		{"testNoCLBFPodDeletionWhenEndpointNotReady", "Single CrashLooping pod with matching label shouldn't be deleted when endpoint is not Ready", testNoCLBFPodDeletionWhenEndpointNotReady},
	}

	for _, test := range tests {
		childCtx, chileCancelFn := context.WithCancel(ctx)
		testNs := rand.String(4)
		testutil.CreateTestNamespace(childCtx, g, reconciler.Client, testNs)
		t.Run(test.description, func(_ *testing.T) {
			test.run(childCtx, chileCancelFn, g, reconciler, testNs)
		})
		deleteAllPods(childCtx, g, reconciler.Client)
		deleteAllEp(childCtx, g, reconciler.Client)
	}
}

func testWeederDedicatedEnvTest(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		name           string
		description    string
		run            func(ctx context.Context, cancelFn context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string)
		apiServerFlags map[string]string
	}{
		{"testPodWatchEndsAbruptlyBeforeSpecifiedWatchDuration", "single Crashlooping pod should be deleted even when watch on pods times-out in the middle", testPodWatchEndsAbruptlyBeforeSpecifiedWatchDuration, map[string]string{"min-request-timeout": "5"}},
		{"testNoCLBFPodDeletionOnContextCancellation", "No pod termination happens when main context is cancelled", testNoCLBFPodDeletionOnContextCancellation, nil},
	}
	for _, test := range tests {
		ctx, cancelFn := context.WithCancel(context.Background())
		testEnv, reconciler := setupWeederEnv(ctx, t, test.apiServerFlags)
		testNs := rand.String(4)
		testutil.CreateTestNamespace(ctx, g, reconciler.Client, testNs)
		t.Run(test.description, func(_ *testing.T) {
			test.run(ctx, cancelFn, g, reconciler, testNs)
		})
		testutil.TeardownEnv(g, testEnv, cancelFn)
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
func testOnlyCLBFPodDeletion(ctx context.Context, _ context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	createEp(ctx, g, reconciler, namespace, true)
	pC := newPod(crashingPod, namespace, "node-0", correctLabels)
	pH := newPod(healthyPod, namespace, "node-1", correctLabels)

	err := reconciler.Client.Create(ctx, pH)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToHealthy(ctx, g, reconciler.Client, pH)

	err = reconciler.Client.Create(ctx, pC)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToCrashLoop(ctx, g, reconciler.Client, pC)

	pl, err := reconciler.SeedClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pl.Items).Should(HaveLen(2))

	// wait for endpoint controller to take action
	time.Sleep(5 * time.Second)

	resultpC := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: crashingPod}, &resultpC)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CrashLooping pod should've been deleted")

	resultpH := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: healthyPod}, &resultpH)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resultpH.DeletionTimestamp).To(BeNil(), "Healthy pod shouldn't be deleted")
}

func testPodTurningCLBFDeletion(ctx context.Context, _ context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	createEp(ctx, g, reconciler, namespace, true)
	pod := newPod(testPodName, namespace, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pod)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToHealthy(ctx, g, reconciler.Client, pod)

	pl, err := reconciler.SeedClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pl.Items).Should(HaveLen(1))

	turnPodToCrashLoop(ctx, g, reconciler.Client, pod)
	// wait for endpoint controller to take action
	time.Sleep(5 * time.Second)

	currentPod := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: testPodName}, &currentPod)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "CrashLooping pod should be deleted")
}

func testCLBFPodWithWrongLabelsDeletion(ctx context.Context, _ context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	createEp(ctx, g, reconciler, namespace, true)
	pod := newPod(crashingPod, namespace, "node-0", inCorrectLabels)
	err := reconciler.Client.Create(ctx, pod)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToCrashLoop(ctx, g, reconciler.Client, pod)

	time.Sleep(5 * time.Second)

	currentPod := v1.Pod{}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(pod), &currentPod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(currentPod.DeletionTimestamp).To(BeNil(), "CrashLoop Pod shouldn't be deleted in this case")
}

func testPodTurningCLBFAfterWatchDuration(ctx context.Context, _ context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	createEp(ctx, g, reconciler, namespace, true)
	pod := newPod(testPodName, namespace, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pod)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToHealthy(ctx, g, reconciler.Client, pod)

	// introducing wait
	time.Sleep(reconciler.WeederConfig.WatchDuration.Duration + 2*time.Second)

	turnPodToCrashLoop(ctx, g, reconciler.Client, pod)
	time.Sleep(5 * time.Second)

	currentPod := v1.Pod{}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(pod), &currentPod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(currentPod.DeletionTimestamp).To(BeNil(), "CrashLoop pod shouldn't be deleted in this case")
}

func testNoCLBFPodDeletionWhenEndpointNotReady(ctx context.Context, _ context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	createEp(ctx, g, reconciler, namespace, false)
	epSlice := &discoveryv1.EndpointSlice{}
	g.Expect(reconciler.Client.Get(ctx, types.NamespacedName{Name: epName, Namespace: namespace}, epSlice)).To(Succeed())
	el := discoveryv1.EndpointSliceList{}
	g.Expect(reconciler.Client.List(ctx, &el)).To(Succeed())

	pod := newPod(crashingPod, namespace, "node-0", correctLabels)
	err := reconciler.Client.Create(ctx, pod)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToCrashLoop(ctx, g, reconciler.Client, pod)
	time.Sleep(5 * time.Second)

	el = discoveryv1.EndpointSliceList{}
	g.Expect(reconciler.Client.List(ctx, &el)).To(Succeed())

	currentPod := v1.Pod{}
	err = reconciler.Client.Get(ctx, client.ObjectKeyFromObject(pod), &currentPod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(currentPod.DeletionTimestamp).To(BeNil())
}

func testNoCLBFPodDeletionOnContextCancellation(ctx context.Context, cancelFn context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	pod := newPod(crashingPod, namespace, "node-0", correctLabels)
	err := reconciler.Client.Create(ctx, pod)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToCrashLoop(ctx, g, reconciler.Client, pod)

	createEp(ctx, g, reconciler, namespace, true)
	// cancel context (like SIGKILL signal to the process)
	cancelFn()

	currentPod, err := reconciler.SeedClient.CoreV1().Pods(namespace).Get(context.TODO(), crashingPod, metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(currentPod.DeletionTimestamp).To(BeNil())
}

func testPodWatchEndsAbruptlyBeforeSpecifiedWatchDuration(ctx context.Context, _ context.CancelFunc, g *WithT, reconciler *Reconciler, namespace string) {
	pod := newPod(testPodName, namespace, "node-0", correctLabels)

	err := reconciler.Client.Create(ctx, pod)
	g.Expect(err).ToNot(HaveOccurred())
	turnPodToHealthy(ctx, g, reconciler.Client, pod)

	// new endpoint creation should trigger watch creation
	createEp(ctx, g, reconciler, namespace, true)

	// waiting more than "min-request-timeout"(5sec) so that watch gets cancelled by APIServer
	time.Sleep(10 * time.Second)

	turnPodToCrashLoop(ctx, g, reconciler.Client, pod)

	// wait for endpoint controller to take action
	time.Sleep(3 * time.Second)

	currentPod := v1.Pod{}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: testPodName}, &currentPod)
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
			g.Expect(client.IgnoreNotFound(crClient.Delete(ctx, &po))).To(Succeed())
		}
	}
}

func deleteAllEp(ctx context.Context, g *WithT, cli client.Client) {
	el := &discoveryv1.EndpointSliceList{}
	select {
	case <-ctx.Done():
		return
	default:
		g.Expect(cli.List(ctx, el)).To(Succeed())
		for _, epSlice := range el.Items {
			g.Expect(client.IgnoreNotFound(cli.Delete(ctx, &epSlice))).To(Succeed())
		}
	}
}

func createEp(ctx context.Context, g *WithT, reconciler *Reconciler, namespace string, ready bool) {
	ep := newEndpoint(epName, namespace)
	if !ready {
		ep.Endpoints[0].Addresses = nil
		ep.Endpoints[0].Addresses = []string{"10.1.0.0"}
		ep.Endpoints[0].Conditions.Ready = ptr.To(false)
		ep.Endpoints[0].NodeName = ptr.To("node-1")
	}
	g.Expect(reconciler.Client.Create(ctx, ep)).To(Succeed())
}

func newPod(name, namespace, host string, labels map[string]string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: ptr.To[int64](0),
			Containers: []v1.Container{
				{Name: "test-container", Image: "nginx:latest"},
			},
			NodeName: host,
		},
	}

	return pod
}

func turnPodToCrashLoop(ctx context.Context, g *WithT, crClient client.Client, p *v1.Pod) {
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

func turnPodToHealthy(ctx context.Context, g *WithT, crClient client.Client, p *v1.Pod) {
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

func newEndpoint(name, namespace string) *discoveryv1.EndpointSlice {
	es := discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			UID:                        uuid.NewUUID(),
			Name:                       name,
			Namespace:                  namespace,
			Annotations:                make(map[string]string),
			Labels:                     map[string]string{wapi.ServiceNameLabel: name},
			DeletionGracePeriodSeconds: ptr.To[int64](0),
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.1.0.52"},
				NodeName:  ptr.To("node-1"),
				Conditions: discoveryv1.EndpointConditions{
					Ready: ptr.To(true),
				},
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports:       []discoveryv1.EndpointPort{},
	}
	return &es
}
