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

package prober

import (
	"context"
	"errors"
	mockcoordinationv1 "github.com/gardener/dependency-watchdog/internal/mock/client-go/kubernetes/coordinationv1"
	testutil "github.com/gardener/dependency-watchdog/internal/test"
	"github.com/gardener/dependency-watchdog/internal/util"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	"math"
	"path/filepath"
	"testing"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockdiscovery "github.com/gardener/dependency-watchdog/internal/mock/client-go/discovery"
	mockinterface "github.com/gardener/dependency-watchdog/internal/mock/client-go/kubernetes"
	mockprober "github.com/gardener/dependency-watchdog/internal/mock/prober"
	mockscaler "github.com/gardener/dependency-watchdog/internal/mock/prober/scaler"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
)

var (
	ctrl                        *gomock.Controller
	mockScaler                  *mockscaler.MockScaler
	mockShootClientCreator      *mockprober.MockShootClientCreator
	mockKubernetesInterface     *mockinterface.MockInterface
	mockDiscoveryInterface      *mockdiscovery.MockDiscoveryInterface
	mockCoordinationV1Interface *mockcoordinationv1.MockCoordinationV1Interface
	mockLeaseInterface          *mockcoordinationv1.MockLeaseInterface
	unknownError                = errors.New("unknown error")
	proberTestLogger            = logr.Discard()
	testProbeTimeout            = metav1.Duration{Duration: 3 * time.Millisecond}
	testProbeInterval           = metav1.Duration{Duration: 3 * time.Millisecond}
	nodeLease1FilePath          = filepath.Join(testdataPath, "node_lease_1.yaml")
	nodeLease2FilePath          = filepath.Join(testdataPath, "node_lease_2.yaml")
)

type probeSimulatorEntry struct {
	name                        string
	leaseList                   *coordinationv1.LeaseList
	mockShootClientCreatorError error
	mockDiscoveryError          error
	mockLeaseListError          error
	mockScaleUpError            error
	mockScaleDownError          error
	minMockScaleUpCount         int
	maxMockScaleUpCount         int
	minMockScaleDownCount       int
	maxMockScaleDownCount       int
}

func TestAPIServerProbeFailure(t *testing.T) {
	table := []probeSimulatorEntry{
		{"Unknown error is returned by api server", nil, nil, unknownError, nil, nil, nil, 0, 0, 0, 0},
		{"Forbidden request error is returned by api server", nil, nil, apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), nil, nil, nil, 0, 0, 0, 0},
		{"Unauthorized request error is returned by api server", nil, nil, apierrors.NewUnauthorized("unauthorized"), nil, nil, nil, 0, 0, 0, 0},
		{"Throttling error is returned by api server", nil, nil, apierrors.NewTooManyRequests("Too many requests", 10), nil, nil, nil, 0, 0, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeInterval.Duration, config, entry)
		})
	}
}

func TestSuccessfulProbesShouldRunScaleUp(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-time.Second))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Scale Up Succeeds", leaseList, nil, nil, nil, nil, nil, 1, math.MaxInt8, 0, 0},
		{"Scale Up Fails", leaseList, nil, nil, nil, errors.New("scale Up failed"), nil, 1, math.MaxInt8, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeTimeout.Duration, config, entry)
		})
	}
}

func TestLeaseProbeFailureShouldRunScaleDown(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-(2 * time.Minute)))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Scale Down Succeeds", leaseList, nil, nil, nil, nil, nil, 0, 0, 1, math.MaxInt8},
		{"Scale Down Fails", leaseList, nil, nil, nil, nil, errors.New("scale Down failed"), 0, 0, 1, math.MaxInt8},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: time.Minute}, 0.2)
			createAndRunProber(t, testProbeTimeout.Duration, config, entry)
		})
	}
}

func TestLeaseProbeListCallFailureShouldSkipScaling(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-time.Second))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Forbidden request error is returned by lease list call", leaseList, nil, nil, apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), nil, nil, 0, 0, 0, 0},
		{"Unauthorized request error is returned by lease list call", leaseList, nil, nil, apierrors.NewUnauthorized("unauthorized"), nil, nil, 0, 0, 0, 0},
		{"Throttling error is returned by lease list call", leaseList, nil, nil, apierrors.NewTooManyRequests("Too many requests", 10), nil, nil, 0, 0, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			createAndRunProber(t, testProbeTimeout.Duration, config, entry)
		})
	}
}

func TestAPIServerProbeShouldNotRunIfClientNotCreated(t *testing.T) {
	err := errors.New("cannot create kubernetes client")
	entry := probeSimulatorEntry{
		name:                        "api server probe should not run if client to access it is not created",
		mockShootClientCreatorError: err,
		minMockScaleUpCount:         0,
		maxMockScaleUpCount:         0,
		minMockScaleDownCount:       0,
		maxMockScaleDownCount:       0,
	}
	createMockInterfaces(t)
	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
	initializeMockInterfaces(entry)
	createAndRunProber(t, testProbeTimeout.Duration, config, entry)
}

func TestScalingShouldNotHappenIfNoOwnedLeasesPresent(t *testing.T) {
	createMockInterfaces(t)
	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
	entry := probeSimulatorEntry{
		name:                  "lease probe should reset if no owned lease is present",
		leaseList:             &coordinationv1.LeaseList{},
		minMockScaleUpCount:   0,
		maxMockScaleUpCount:   0,
		minMockScaleDownCount: 0,
		maxMockScaleDownCount: 0,
	}
	initializeMockInterfaces(entry)
	p := NewProber(context.Background(), "default", config, mockScaler, mockShootClientCreator, proberTestLogger)
	runProber(p, testProbeTimeout.Duration)
}

func createAndRunProber(t *testing.T, duration time.Duration, config *papi.Config, simulatorEntry probeSimulatorEntry) {
	g := NewWithT(t)
	p := NewProber(context.Background(), "default", config, mockScaler, mockShootClientCreator, proberTestLogger)
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

func createMockInterfaces(t *testing.T) {
	ctrl = gomock.NewController(t)
	mockScaler = mockscaler.NewMockScaler(ctrl)
	mockShootClientCreator = mockprober.NewMockShootClientCreator(ctrl)
	mockKubernetesInterface = mockinterface.NewMockInterface(ctrl)
	mockCoordinationV1Interface = mockcoordinationv1.NewMockCoordinationV1Interface(ctrl)
	mockLeaseInterface = mockcoordinationv1.NewMockLeaseInterface(ctrl)
	mockDiscoveryInterface = mockdiscovery.NewMockDiscoveryInterface(ctrl)
}

func initializeMockInterfaces(entry probeSimulatorEntry) {
	mockShootClientCreator.EXPECT().CreateClient(gomock.Any(), proberTestLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mockKubernetesInterface, entry.mockShootClientCreatorError).AnyTimes()
	mockKubernetesInterface.EXPECT().Discovery().Return(mockDiscoveryInterface).AnyTimes()
	mockKubernetesInterface.EXPECT().CoordinationV1().Return(mockCoordinationV1Interface).AnyTimes()
	mockCoordinationV1Interface.EXPECT().Leases(nodeLeaseNamespace).Return(mockLeaseInterface).AnyTimes()
	mockLeaseInterface.EXPECT().List(gomock.Any(), gomock.Any()).Return(entry.leaseList, entry.mockLeaseListError).AnyTimes()
	mockDiscoveryInterface.EXPECT().ServerVersion().Return(nil, entry.mockDiscoveryError).AnyTimes()
	mockScaler.EXPECT().ScaleUp(gomock.Any()).Return(entry.mockScaleUpError).MaxTimes(entry.maxMockScaleUpCount).MinTimes(entry.minMockScaleUpCount)
	mockScaler.EXPECT().ScaleDown(gomock.Any()).Return(entry.mockScaleDownError).MaxTimes(entry.maxMockScaleDownCount).MinTimes(entry.minMockScaleDownCount)
}

func createConfig(probeInterval metav1.Duration, initialDelay metav1.Duration, kcmNodeMonitorGraceDuration metav1.Duration, backoffJitterFactor float64) *papi.Config {
	return &papi.Config{
		ProbeInterval:                 &probeInterval,
		BackoffJitterFactor:           &backoffJitterFactor,
		InitialDelay:                  &initialDelay,
		ProbeTimeout:                  &testProbeTimeout,
		KCMNodeMonitorGraceDuration:   &kcmNodeMonitorGraceDuration,
		LeaseFailureThresholdFraction: pointer.Float64(DefaultLeaseFailureThresholdFraction),
	}
}

func getLeaseFromFileAndSetRenewTime(t *testing.T, filePath string, renewTime metav1.MicroTime) coordinationv1.Lease {
	g := NewWithT(t)
	testutil.ValidateIfFileExists(filePath, t)
	lease, err := util.ReadAndUnmarshall[coordinationv1.Lease](filePath)
	g.Expect(err).To(BeNil())
	lease.Spec.RenewTime = &renewTime
	return *lease
}
