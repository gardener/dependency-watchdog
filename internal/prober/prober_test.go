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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	ctrl                                 *gomock.Controller
	mockScaler                           *mockscaler.MockScaler
	mockShootClientCreator               *mockprober.MockShootClientCreator
	clientBuilder                        *fake.ClientBuilder
	fakeClient                           client.WithWatch
	mockKubernetesInterface              *mockinterface.MockInterface
	mockDiscoveryInterface               *mockdiscovery.MockDiscoveryInterface
	mockCoordinationV1Interface          *mockcoordinationv1.MockCoordinationV1Interface
	mockLeaseInterface                   *mockcoordinationv1.MockLeaseInterface
	notIgnorableError                    = errors.New("not Ignorable error")
	apiServerProbeFailureBackoffDuration = metav1.Duration{Duration: time.Millisecond}
	proberTestLogger                     = logr.Discard()
	defaultProbeTimeout                  = metav1.Duration{Duration: 40 * time.Second}
	nodeLease1FilePath                   = filepath.Join(testdataPath, "node_lease_1.yaml")
	nodeLease2FilePath                   = filepath.Join(testdataPath, "node_lease_2.yaml")
)

type probeSimulatorEntry struct {
	name                               string
	leaseList                          *coordinationv1.LeaseList
	mockShootClientCreatorError        error
	mockDiscoveryError                 error
	mockLeaseListError                 error
	mockScaleUpError                   error
	mockScaleDownError                 error
	expectedAPIServerProbeSuccessCount int
	expectedAPIServerProbeErrorCount   int
	expectedLeaseProbeSuccessCount     int
	expectedLeaseProbeErrorCount       int
}

func TestAPIServerProbeErrorCount(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-time.Second))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Success Count is incremented", leaseList, nil, nil, nil, nil, nil, 1, 0, 1, 0},
		{"Unignorable error is returned by pingKubeApiServer", leaseList, nil, notIgnorableError, nil, nil, nil, 0, 1, 0, 0},
		{"Forbidden request error is returned by pingKubeApiServer", leaseList, nil, apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), nil, nil, nil, 0, 0, 0, 0},
		{"Unauthorized request error is returned by pingKubeApiServer", leaseList, nil, apierrors.NewUnauthorized("unauthorized"), nil, nil, nil, 0, 0, 0, 0},
		{"Throttling error is returned by pingKubeApiServer", leaseList, nil, apierrors.NewTooManyRequests("Too many requests", 10), nil, nil, nil, 0, 0, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(1, 1, metav1.Duration{Duration: 4 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			runProberAndCheckStatus(t, 12*time.Millisecond, config, entry)
		})
	}
}

func TestHealthyProbesShouldRunScaleUp(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-time.Second))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Scale Up Succeeds", leaseList, nil, nil, nil, nil, nil, 1, 0, 1, 0},
		{"Scale Up Fails", leaseList, nil, nil, nil, errors.New("scale Up failed"), nil, 1, 0, 1, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(1, 1, metav1.Duration{Duration: 4 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			runProberAndCheckStatus(t, 12*time.Millisecond, config, entry)
		})
	}
}

func TestLeaseProbeFailingShouldRunScaleDown(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-(2 * time.Minute)))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Scale Down Succeeds", leaseList, nil, nil, nil, nil, nil, 1, 0, 0, 2},
		{"Scale Down Fails", leaseList, nil, nil, nil, nil, errors.New("scale Down failed"), 1, 0, 0, 2},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: time.Minute}, 0.2)
			runProberAndCheckStatus(t, 20*time.Millisecond, config, entry)
		})
	}
}

func TestUnchangedLeaseProbeErrorCountForIgnorableErrors(t *testing.T) {
	leaseRenewTime := metav1.NewMicroTime(time.Now().Add(-time.Second))
	nodeLease1 := getLeaseFromFileAndSetRenewTime(t, nodeLease1FilePath, leaseRenewTime)
	nodeLease2 := getLeaseFromFileAndSetRenewTime(t, nodeLease2FilePath, leaseRenewTime)
	leaseList := &coordinationv1.LeaseList{Items: []coordinationv1.Lease{nodeLease1, nodeLease2}}

	table := []probeSimulatorEntry{
		{"Forbidden request error is returned by lease list call", leaseList, nil, nil, apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), nil, nil, 1, 0, 0, 0},
		{"Unauthorized request error is returned by lease list call", leaseList, nil, nil, apierrors.NewUnauthorized("unauthorized"), nil, nil, 1, 0, 0, 0},
		{"Throttling error is returned by lease list call", leaseList, nil, nil, apierrors.NewTooManyRequests("Too many requests", 10), nil, nil, 1, 0, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			createMockInterfaces(t)
			initializeMockInterfaces(entry)
			config := createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			runProberAndCheckStatus(t, 12*time.Millisecond, config, entry)
		})
	}
}

func TestAPIServerProbeShouldNotRunIfClientNotCreated(t *testing.T) {
	err := errors.New("cannot create kubernetes client")
	entry := probeSimulatorEntry{
		name:                               "api server probe should not run if client to access it is not created",
		mockShootClientCreatorError:        err,
		expectedAPIServerProbeSuccessCount: 0,
		expectedAPIServerProbeErrorCount:   0,
		expectedLeaseProbeSuccessCount:     0,
		expectedLeaseProbeErrorCount:       0,
	}
	createMockInterfaces(t)
	config := createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
	initializeMockInterfaces(entry)
	runProberAndCheckStatus(t, 12*time.Millisecond, config, entry)
}

func runProberAndCheckStatus(t *testing.T, duration time.Duration, config *papi.Config, probeStatusEntry probeSimulatorEntry) {
	g := NewWithT(t)
	p := NewProber(context.Background(), "default", config, fakeClient, mockScaler, mockShootClientCreator, proberTestLogger)
	g.Expect(p.IsClosed()).To(BeFalse())

	runProber(p, duration)

	g.Expect(p.IsClosed()).To(BeTrue())
	checkProbeStatus(t, p.apiServerProbeStatus, probeStatusEntry.expectedAPIServerProbeSuccessCount, probeStatusEntry.expectedAPIServerProbeErrorCount)
	checkProbeStatus(t, p.leaseProbeStatus, probeStatusEntry.expectedLeaseProbeSuccessCount, probeStatusEntry.expectedLeaseProbeErrorCount)
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

func checkProbeStatus(t *testing.T, ps probeStatus, successCount int, errCount int) {
	g := NewWithT(t)
	if successCount == 0 {
		g.Expect(ps.successCount).To(Equal(successCount))
	} else {
		g.Expect(ps.successCount).To(BeNumerically(">=", successCount))
	}
	if errCount == 0 {
		g.Expect(ps.errorCount).To(Equal(errCount))
	} else {
		g.Expect(ps.errorCount).To(BeNumerically(">=", errCount))
	}
}

func createMockInterfaces(t *testing.T) {
	ctrl = gomock.NewController(t)
	mockScaler = mockscaler.NewMockScaler(ctrl)
	mockShootClientCreator = mockprober.NewMockShootClientCreator(ctrl)
	clientBuilder = fake.NewClientBuilder()
	fakeClient = clientBuilder.Build()
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
	mockScaler.EXPECT().ScaleUp(gomock.Any()).Return(entry.mockScaleUpError).AnyTimes()
	mockScaler.EXPECT().ScaleDown(gomock.Any()).Return(entry.mockScaleDownError).AnyTimes()
}

func createConfig(successThreshold int, failureThreshold int, probeInterval metav1.Duration, initialDelay metav1.Duration, kcmNodeMonitorGraceDuration metav1.Duration, backoffJitterFactor float64) *papi.Config {
	return &papi.Config{
		SuccessThreshold:                     &successThreshold,
		FailureThreshold:                     &failureThreshold,
		ProbeInterval:                        &probeInterval,
		BackoffJitterFactor:                  &backoffJitterFactor,
		APIServerProbeFailureBackoffDuration: &apiServerProbeFailureBackoffDuration,
		InitialDelay:                         &initialDelay,
		ProbeTimeout:                         &defaultProbeTimeout,
		KCMNodeMonitorGraceDuration:          &kcmNodeMonitorGraceDuration,
		LeaseFailureThresholdFraction:        pointer.Float64(DefaultLeaseFailureThresholdFraction),
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
