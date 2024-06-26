// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package prober

import (
	"context"
	"errors"
	perrors "github.com/gardener/dependency-watchdog/internal/prober/errors"
	k8sfakes "github.com/gardener/dependency-watchdog/internal/prober/fakes/k8s"
	scalefakes "github.com/gardener/dependency-watchdog/internal/prober/fakes/scale"
	shootfakes "github.com/gardener/dependency-watchdog/internal/prober/fakes/shoot"
	"github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"

	papi "github.com/gardener/dependency-watchdog/api/prober"
)

var (
	testProbeTimeout  = metav1.Duration{Duration: 100 * time.Millisecond}
	testProbeInterval = metav1.Duration{Duration: 100 * time.Millisecond}
)

//type probeTestMocks struct {
//	scaler             *mockscaler.MockScaler
//	shootClientCreator *mockprober.MockShootClientCreator
//	seedClient         *client.MockClient
//	kubernetes         *mockinterface.MockInterface
//	discovery          *mockdiscovery.MockDiscoveryInterface
//	coreV1             *mockcorev1.MockCoreV1Interface
//	coordinationV1     *mockcoordinationv1.MockCoordinationV1Interface
//	node               *mockcorev1.MockNodeInterface
//	lease              *mockcoordinationv1.MockLeaseInterface
//}

type probeTestCase struct {
	name                    string
	nodeList                *corev1.NodeList
	leaseList               *coordinationv1.LeaseList
	shootClientCreatorError error
	discoveryError          error
	nodeListError           error
	leaseListError          error
	scaleUpError            error
	scaleDownError          error
	minScaleUpCount         int
	maxScaleUpCount         int
	minScaleDownCount       int
	maxScaleDownCount       int
}

func TestAPIServerProbeFailure(t *testing.T) {
	testCases := []struct {
		name           string
		discoveryError error
	}{
		{name: "Forbidden request error is returned by api server", discoveryError: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden"))},
		{name: "Unauthorized request error is returned by api server", discoveryError: apierrors.NewUnauthorized("unauthorized")},
		{name: "Throttling error is returned by api server", discoveryError: apierrors.NewTooManyRequests("Too many requests", 10)},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			scc := shootfakes.NewFakeShootClientCreator(k8sfakes.NewFakeDiscoveryClient(entry.discoveryError), k8sfakes.NewFakeClientBuilder().Build())
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			p := NewProber(context.Background(), nil, test.DefaultNamespace, config, nil, nil, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			assertError(g, err, entry.discoveryError, perrors.ErrProbeAPIServer)
		})
	}
}

func TestSuccessfulProbesShouldRunScaleUp(t *testing.T) {
	g := NewWithT(t)
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}})
	leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{
		{Name: test.Node1Name, IsExpired: false},
		{Name: test.Node2Name, IsExpired: false},
	})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)
	scaleTargetDeployments := generateScaleTargetDeployments(0)

	shootClient := initializeShootClientBuilder(nodes, leases).Build()
	seedClient := initializeSeedClientBuilder(g, machines, scaleTargetDeployments).Build()
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	shootClientCreator := shootfakes.NewFakeShootClientCreator(shootDiscoveryClient, shootClient)

	testCases := []struct {
		name       string
		scaleUpErr error
	}{
		{name: "Scale Up Succeeds"},
		{name: "Scale Up Fails", scaleUpErr: errors.New("scale up failed")},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			ctx := context.Background()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, entry.scaleUpErr, nil)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, shootClientCreator, logr.Discard())
			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			if entry.scaleUpErr != nil {
				assertError(g, err, entry.scaleUpErr, perrors.ErrScaleUp)
			} else {
				g.Expect(err).To(BeNil())
				targetDeploymentRefs := getDeploymentRefs(scaleTargetDeployments)
				assertScale(ctx, g, seedClient, targetDeploymentRefs, 1)
			}
		})
	}
}

func getDeploymentRefs(deployments []*appsv1.Deployment) []client.ObjectKey {
	refs := make([]client.ObjectKey, 0, len(deployments))
	for _, deploy := range deployments {
		refs = append(refs, client.ObjectKeyFromObject(deploy))
	}
	return refs
}

func assertScale(ctx context.Context, g *WithT, client client.Client, targetDeploymentRefs []client.ObjectKey, expectedReplicas int32) {
	for _, deployRef := range targetDeploymentRefs {
		deploy := &appsv1.Deployment{}
		g.Expect(client.Get(ctx, deployRef, deploy)).To(Succeed())
		g.Expect(deploy.Spec.Replicas).ToNot(BeNil())
		g.Expect(*deploy.Spec.Replicas).To(Equal(expectedReplicas))
	}
}

func initializeShootClientBuilder(nodes []*corev1.Node, nodeLeases []*coordinationv1.Lease) *k8sfakes.FakeClientBuilder {
	shootObjects := make([]client.Object, 0, len(nodes)+len(nodeLeases))
	for _, node := range nodes {
		shootObjects = append(shootObjects, node)
	}
	for _, lease := range nodeLeases {
		shootObjects = append(shootObjects, lease)
	}
	return k8sfakes.NewFakeClientBuilder(shootObjects...)
}

func initializeSeedClientBuilder(g *WithT, machines []*v1alpha1.Machine, deployments []*appsv1.Deployment) *k8sfakes.FakeClientBuilder {
	seedObjects := make([]client.Object, 0, len(machines)+len(deployments))
	for _, machine := range machines {
		seedObjects = append(seedObjects, machine)
	}
	for _, deploy := range deployments {
		seedObjects = append(seedObjects, deploy)
	}
	seedClientScheme := scheme.Scheme
	g.Expect(v1alpha1.AddToScheme(seedClientScheme)).To(Succeed())
	return k8sfakes.NewFakeClientBuilder(seedObjects...).WithScheme(seedClientScheme)
}

func generateScaleTargetDeployments(replicas int32) []*appsv1.Deployment {
	return []*appsv1.Deployment{
		test.GenerateDeployment(test.KCMDeploymentName, test.DefaultNamespace, test.DefaultImage, replicas, nil),
		test.GenerateDeployment(test.MCMDeploymentName, test.DefaultNamespace, test.DefaultImage, replicas, nil),
		test.GenerateDeployment(test.CADeploymentName, test.DefaultNamespace, test.DefaultImage, replicas, nil),
	}
}

//
//func TestLeaseProbeShouldNotConsiderUnrelatedLeases(t *testing.T) {
//	leaseList1 := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime})
//	leaseList2 := createNodeLeases([]metav1.MicroTime{expiredLeaseRenewTime, nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime})
//
//	testCases := []probeTestCase{
//		{name: "Scale Up Succeeds", leaseList: leaseList1, nodeList: createNodes(1), minScaleUpCount: 1, maxScaleUpCount: math.MaxInt8},
//		{name: "Scale Down Succeeds", leaseList: leaseList2, nodeList: createNodes(1), minScaleDownCount: 1, maxScaleDownCount: math.MaxInt8},
//	}
//
//	for _, entry := range testCases {
//		t.Run(entry.name, func(t *testing.T) {
//			mocks := createAndInitializeMocks(t, entry)
//			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
//			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
//		})
//	}
//}
//
//func TestLeaseProbeFailureShouldRunScaleDown(t *testing.T) {
//	leaseList := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime})
//	nodeList := createNodes(len(leaseList.Items))
//
//	testCases := []probeTestCase{
//		{name: "Scale Down Succeeds", leaseList: leaseList, nodeList: nodeList, minScaleDownCount: 1, maxScaleDownCount: math.MaxInt8},
//		{name: "Scale Down Fails", leaseList: leaseList, nodeList: nodeList, scaleDownError: errors.New("scale Down failed"), minScaleDownCount: 1, maxScaleDownCount: math.MaxInt8},
//	}
//
//	for _, entry := range testCases {
//		t.Run(entry.name, func(t *testing.T) {
//			mocks := createAndInitializeMocks(t, entry)
//			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: time.Minute}, 0.2)
//			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
//		})
//	}
//}
//
//func TestLeaseProbeListCallFailureShouldSkipScaling(t *testing.T) {
//	leaseList := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime})
//	nodeList := createNodes(len(leaseList.Items))
//
//	testCases := []probeTestCase{
//		{name: "Forbidden request error is returned by lease list call", nodeList: nodeList, leaseList: leaseList, leaseListError: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden"))},
//		{name: "Unauthorized request error is returned by lease list call", nodeList: nodeList, leaseList: leaseList, leaseListError: apierrors.NewUnauthorized("unauthorized")},
//		{name: "Throttling error is returned by lease list call", nodeList: nodeList, leaseListError: apierrors.NewTooManyRequests("Too many requests", 10)},
//		{name: "Throttling error is returned by node list call", nodeListError: apierrors.NewTooManyRequests("Too many requests", 10)},
//	}
//
//	for _, entry := range testCases {
//		t.Run(entry.name, func(t *testing.T) {
//			mocks := createAndInitializeMocks(t, entry)
//			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
//			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
//		})
//	}
//}
//
//func TestAPIServerProbeShouldNotRunIfClientNotCreated(t *testing.T) {
//	err := errors.New("cannot create kubernetes client")
//	entry := probeTestCase{
//		name:                    "api server probe should not run if client to access it is not created",
//		shootClientCreatorError: err,
//	}
//	mocks := createAndInitializeMocks(t, entry)
//	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
//	createAndRunProber(t, testProbeInterval.Duration, config, mocks)
//}
//
//func TestScaleUpShouldHappenIfNoOwnedLeasesPresent(t *testing.T) {
//	entry := probeTestCase{
//		name:            "scale up should happen if no owned lease is present",
//		leaseList:       createNodeLeases(nil),
//		nodeList:        createNodes(0),
//		minScaleUpCount: 1,
//		maxScaleUpCount: math.MaxInt8,
//	}
//	mocks := createAndInitializeMocks(t, entry)
//	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
//	createAndRunProber(t, testProbeInterval.Duration, config, mocks)
//}
//
//func createAndRunProber(t *testing.T, duration time.Duration, config *papi.Config, testMocks probeTestMocks) {
//	g := NewWithT(t)
//	workerNodeConditions := map[string][]string{
//		test.Worker1Name: {test.NodeConditionDiskPressure, test.NodeConditionMemoryPressure},
//		test.Worker2Name: util.DefaultUnhealthyNodeConditions,
//	}
//	p := NewProber(context.Background(), testMocks.seedClient, "default", config, workerNodeConditions, testMocks.scaler, testMocks.shootClientCreator, proberTestLogger)
//	g.Expect(p.IsClosed()).To(BeFalse())
//
//	runProber(p, duration)
//	g.Expect(p.IsClosed()).To(BeTrue())
//}

func runProber(p *Prober, d time.Duration) (err error) {
	exitAfter := time.NewTimer(d)
	go p.Run()
	for {
		select {
		case <-exitAfter.C:
			err = p.lastErr
			p.Close()
			return
		case <-p.ctx.Done():
			return
		}
	}
}

func assertError(g *WithT, err error, expectedError error, expectedErrorCode perrors.ErrorCode) {
	g.Expect(err).To(HaveOccurred())
	probeErr := &perrors.ProbeError{}
	if errors.As(err, &probeErr) {
		g.Expect(probeErr.Code).To(Equal(expectedErrorCode))
		g.Expect(probeErr.Cause).To(Equal(expectedError))
	}
}

//
//func createAndInitializeMocks(t *testing.T, testCase probeTestCase) probeTestMocks {
//	ctrl := gomock.NewController(t)
//	mocks := probeTestMocks{
//		scaler:             mockscaler.NewMockScaler(ctrl),
//		shootClientCreator: mockprober.NewMockShootClientCreator(ctrl),
//		seedClient:         client.NewMockClient(ctrl),
//		kubernetes:         mockinterface.NewMockInterface(ctrl),
//		discovery:          mockdiscovery.NewMockDiscoveryInterface(ctrl),
//		coreV1:             mockcorev1.NewMockCoreV1Interface(ctrl),
//		coordinationV1:     mockcoordinationv1.NewMockCoordinationV1Interface(ctrl),
//		node:               mockcorev1.NewMockNodeInterface(ctrl),
//		lease:              mockcoordinationv1.NewMockLeaseInterface(ctrl),
//	}
//	initializeMocks(mocks, testCase)
//	return mocks
//}
//
//func initializeMocks(mocks probeTestMocks, testCase probeTestCase) {
//	mocks.shootClientCreator.EXPECT().CreateClient(gomock.Any(), proberTestLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mocks.kubernetes, testCase.shootClientCreatorError).AnyTimes()
//	mocks.kubernetes.EXPECT().Discovery().Return(mocks.discovery).AnyTimes()
//	mocks.kubernetes.EXPECT().CoreV1().Return(mocks.coreV1).AnyTimes()
//	mocks.kubernetes.EXPECT().CoordinationV1().Return(mocks.coordinationV1).AnyTimes()
//	mocks.coreV1.EXPECT().Nodes().Return(mocks.node).AnyTimes()
//	mocks.coordinationV1.EXPECT().Leases(nodeLeaseNamespace).Return(mocks.lease).AnyTimes()
//	mocks.node.EXPECT().List(gomock.Any(), gomock.Any()).Return(testCase.nodeList, testCase.nodeListError).AnyTimes()
//	mocks.lease.EXPECT().List(gomock.Any(), gomock.Any()).Return(testCase.leaseList, testCase.leaseListError).AnyTimes()
//	mocks.discovery.EXPECT().ServerVersion().Return(nil, testCase.discoveryError).AnyTimes()
//	mocks.scaler.EXPECT().ScaleUp(gomock.Any()).Return(testCase.scaleUpError).MaxTimes(testCase.maxScaleUpCount).MinTimes(testCase.minScaleUpCount)
//	mocks.scaler.EXPECT().ScaleDown(gomock.Any()).Return(testCase.scaleDownError).MaxTimes(testCase.maxScaleDownCount).MinTimes(testCase.minScaleDownCount)
//}

func createConfig(probeInterval metav1.Duration, initialDelay metav1.Duration, kcmNodeMonitorGraceDuration metav1.Duration, backoffJitterFactor float64) *papi.Config {
	return &papi.Config{
		ProbeInterval:               &probeInterval,
		BackoffJitterFactor:         &backoffJitterFactor,
		InitialDelay:                &initialDelay,
		ProbeTimeout:                &testProbeTimeout,
		KCMNodeMonitorGraceDuration: &kcmNodeMonitorGraceDuration,
		NodeLeaseFailureFraction:    pointer.Float64(DefaultNodeLeaseFailureFraction),
	}
}
