// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package prober

import (
	"context"
	"errors"
	"github.com/gardener/dependency-watchdog/internal/mock/controller-runtime/client"
	"github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	mockdiscovery "github.com/gardener/dependency-watchdog/internal/mock/client-go/discovery"
	mockinterface "github.com/gardener/dependency-watchdog/internal/mock/client-go/kubernetes"
	mockcoordinationv1 "github.com/gardener/dependency-watchdog/internal/mock/client-go/kubernetes/coordinationv1"
	mockcorev1 "github.com/gardener/dependency-watchdog/internal/mock/client-go/kubernetes/corev1"
	mockprober "github.com/gardener/dependency-watchdog/internal/mock/prober"
	mockscaler "github.com/gardener/dependency-watchdog/internal/mock/prober/scaler"
)

var (
	errFoo                   = errors.New("unknown error")
	proberTestLogger         = logr.Discard()
	testProbeTimeout         = metav1.Duration{Duration: 3 * time.Millisecond}
	testProbeInterval        = metav1.Duration{Duration: 3 * time.Millisecond}
	expiredLeaseRenewTime    = metav1.NewMicroTime(time.Now().Add(-(2 * time.Minute)))
	nonExpiredLeaseRenewTime = metav1.NewMicroTime(time.Now().Add(-time.Second))
)

type probeTestMocks struct {
	scaler             *mockscaler.MockScaler
	shootClientCreator *mockprober.MockShootClientCreator
	seedClient         *client.MockClient
	kubernetes         *mockinterface.MockInterface
	discovery          *mockdiscovery.MockDiscoveryInterface
	coreV1             *mockcorev1.MockCoreV1Interface
	coordinationV1     *mockcoordinationv1.MockCoordinationV1Interface
	node               *mockcorev1.MockNodeInterface
	lease              *mockcoordinationv1.MockLeaseInterface
}

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
	testCases := []probeTestCase{
		{name: "Unknown error is returned by api server", discoveryError: errFoo},
		{name: "Forbidden request error is returned by api server", discoveryError: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden"))},
		{name: "Unauthorized request error is returned by api server", discoveryError: apierrors.NewUnauthorized("unauthorized")},
		{name: "Throttling error is returned by api server", discoveryError: apierrors.NewTooManyRequests("Too many requests", 10)},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			mocks := createAndInitializeMocks(t, entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
		})
	}
}

func TestSuccessfulProbesShouldRunScaleUp(t *testing.T) {
	leaseList := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime})
	nodeList := createNodes(len(leaseList.Items))

	testCases := []probeTestCase{
		{name: "Scale Up Succeeds", leaseList: leaseList, nodeList: nodeList, minScaleUpCount: 1, maxScaleUpCount: math.MaxInt8},
		{name: "Scale Up Fails", leaseList: leaseList, nodeList: nodeList, scaleUpError: errors.New("scale Up failed"), minScaleUpCount: 1, maxScaleUpCount: math.MaxInt8},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			mocks := createAndInitializeMocks(t, entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
		})
	}
}

func TestLeaseProbeShouldNotConsiderUnrelatedLeases(t *testing.T) {
	leaseList1 := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime})
	leaseList2 := createNodeLeases([]metav1.MicroTime{expiredLeaseRenewTime, nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime})

	testCases := []probeTestCase{
		{name: "Scale Up Succeeds", leaseList: leaseList1, nodeList: createNodes(1), minScaleUpCount: 1, maxScaleUpCount: math.MaxInt8},
		{name: "Scale Down Succeeds", leaseList: leaseList2, nodeList: createNodes(1), minScaleDownCount: 1, maxScaleDownCount: math.MaxInt8},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			mocks := createAndInitializeMocks(t, entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
		})
	}
}

func TestLeaseProbeFailureShouldRunScaleDown(t *testing.T) {
	leaseList := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime, expiredLeaseRenewTime})
	nodeList := createNodes(len(leaseList.Items))

	testCases := []probeTestCase{
		{name: "Scale Down Succeeds", leaseList: leaseList, nodeList: nodeList, minScaleDownCount: 1, maxScaleDownCount: math.MaxInt8},
		{name: "Scale Down Fails", leaseList: leaseList, nodeList: nodeList, scaleDownError: errors.New("scale Down failed"), minScaleDownCount: 1, maxScaleDownCount: math.MaxInt8},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			mocks := createAndInitializeMocks(t, entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: time.Minute}, 0.2)
			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
		})
	}
}

func TestLeaseProbeListCallFailureShouldSkipScaling(t *testing.T) {
	leaseList := createNodeLeases([]metav1.MicroTime{nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime, nonExpiredLeaseRenewTime})
	nodeList := createNodes(len(leaseList.Items))

	testCases := []probeTestCase{
		{name: "Forbidden request error is returned by lease list call", nodeList: nodeList, leaseList: leaseList, leaseListError: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden"))},
		{name: "Unauthorized request error is returned by lease list call", nodeList: nodeList, leaseList: leaseList, leaseListError: apierrors.NewUnauthorized("unauthorized")},
		{name: "Throttling error is returned by lease list call", nodeList: nodeList, leaseListError: apierrors.NewTooManyRequests("Too many requests", 10)},
		{name: "Throttling error is returned by node list call", nodeListError: apierrors.NewTooManyRequests("Too many requests", 10)},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			mocks := createAndInitializeMocks(t, entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeInterval.Duration, config, mocks)
		})
	}
}

func TestAPIServerProbeShouldNotRunIfClientNotCreated(t *testing.T) {
	err := errors.New("cannot create kubernetes client")
	entry := probeTestCase{
		name:                    "api server probe should not run if client to access it is not created",
		shootClientCreatorError: err,
	}
	mocks := createAndInitializeMocks(t, entry)
	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
	createAndRunProber(t, testProbeInterval.Duration, config, mocks)
}

func TestScaleUpShouldHappenIfNoOwnedLeasesPresent(t *testing.T) {
	entry := probeTestCase{
		name:            "scale up should happen if no owned lease is present",
		leaseList:       createNodeLeases(nil),
		nodeList:        createNodes(0),
		minScaleUpCount: 1,
		maxScaleUpCount: math.MaxInt8,
	}
	mocks := createAndInitializeMocks(t, entry)
	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
	createAndRunProber(t, testProbeInterval.Duration, config, mocks)
}

func createAndRunProber(t *testing.T, duration time.Duration, config *papi.Config, testMocks probeTestMocks) {
	g := NewWithT(t)
	workerNodeConditions := map[string][]string{
		test.Worker1Name: {test.NodeConditionDiskPressure, test.NodeConditionMemoryPressure},
		test.Worker2Name: util.DefaultUnhealthyNodeConditions,
	}
	p := NewProber(context.Background(), testMocks.seedClient, "default", config, workerNodeConditions, testMocks.scaler, testMocks.shootClientCreator, proberTestLogger)
	g.Expect(p.IsClosed()).To(BeFalse())

	runProber(p, duration)
	g.Expect(p.IsClosed()).To(BeTrue())
}

func runProber(p *Prober, d time.Duration) {
	exitAfter := time.NewTimer(d)
	go p.Run()
	for {
		select {
		case <-exitAfter.C:
			p.Close()
			return
		case <-p.ctx.Done():
			return
		}
	}
}

func createAndInitializeMocks(t *testing.T, testCase probeTestCase) probeTestMocks {
	ctrl := gomock.NewController(t)
	mocks := probeTestMocks{
		scaler:             mockscaler.NewMockScaler(ctrl),
		shootClientCreator: mockprober.NewMockShootClientCreator(ctrl),
		seedClient:         client.NewMockClient(ctrl),
		kubernetes:         mockinterface.NewMockInterface(ctrl),
		discovery:          mockdiscovery.NewMockDiscoveryInterface(ctrl),
		coreV1:             mockcorev1.NewMockCoreV1Interface(ctrl),
		coordinationV1:     mockcoordinationv1.NewMockCoordinationV1Interface(ctrl),
		node:               mockcorev1.NewMockNodeInterface(ctrl),
		lease:              mockcoordinationv1.NewMockLeaseInterface(ctrl),
	}
	initializeMocks(mocks, testCase)
	return mocks
}

func initializeMocks(mocks probeTestMocks, testCase probeTestCase) {
	mocks.shootClientCreator.EXPECT().CreateClient(gomock.Any(), proberTestLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mocks.kubernetes, testCase.shootClientCreatorError).AnyTimes()
	mocks.kubernetes.EXPECT().Discovery().Return(mocks.discovery).AnyTimes()
	mocks.kubernetes.EXPECT().CoreV1().Return(mocks.coreV1).AnyTimes()
	mocks.kubernetes.EXPECT().CoordinationV1().Return(mocks.coordinationV1).AnyTimes()
	mocks.coreV1.EXPECT().Nodes().Return(mocks.node).AnyTimes()
	mocks.coordinationV1.EXPECT().Leases(nodeLeaseNamespace).Return(mocks.lease).AnyTimes()
	mocks.node.EXPECT().List(gomock.Any(), gomock.Any()).Return(testCase.nodeList, testCase.nodeListError).AnyTimes()
	mocks.lease.EXPECT().List(gomock.Any(), gomock.Any()).Return(testCase.leaseList, testCase.leaseListError).AnyTimes()
	mocks.discovery.EXPECT().ServerVersion().Return(nil, testCase.discoveryError).AnyTimes()
	mocks.scaler.EXPECT().ScaleUp(gomock.Any()).Return(testCase.scaleUpError).MaxTimes(testCase.maxScaleUpCount).MinTimes(testCase.minScaleUpCount)
	mocks.scaler.EXPECT().ScaleDown(gomock.Any()).Return(testCase.scaleDownError).MaxTimes(testCase.maxScaleDownCount).MinTimes(testCase.minScaleDownCount)
}

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

func createNodeLeases(renewTimes []metav1.MicroTime) (leaseList *coordinationv1.LeaseList) {
	var items []coordinationv1.Lease
	for i, renewTime := range renewTimes {
		items = append(items, createNodeLease(name(i), renewTime))
	}
	return &coordinationv1.LeaseList{
		Items: items,
	}
}

func createNodes(count int) (nodeList *corev1.NodeList) {
	var items []corev1.Node
	for i := 0; i < count; i++ {
		items = append(items, createNode(name(i)))
	}
	return &corev1.NodeList{
		Items: items,
	}
}

func name(i int) string {
	return "node-" + strconv.Itoa(i)
}

func createNodeLease(name string, renewTime metav1.MicroTime) coordinationv1.Lease {
	return coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "kube-node-lease",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Node",
					Name:       name,
				},
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: &name,
			RenewTime:      &renewTime,
		},
	}
}

func createNode(name string) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
