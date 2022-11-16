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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	config                              *papi.Config
	ctrl                                *gomock.Controller
	mds                                 *mockscaler.MockScaler
	msc                                 *mockprober.MockShootClientCreator
	clientBuilder                       *fake.ClientBuilder
	fakeClient                          client.WithWatch
	mki                                 *mockinterface.MockInterface
	mdi                                 *mockdiscovery.MockDiscoveryInterface
	errNotIgnorable                     = errors.New("not Ignorable error")
	internalProbeFailureBackoffDuration = metav1.Duration{Duration: time.Millisecond}
	pLogger                             = logr.Discard()
	defaultProbeTimeout                 = metav1.Duration{Duration: 40 * time.Second}
)

type probeStatusEntry struct {
	name                              string
	err                               error
	expectedInternalProbeSuccessCount int
	expectedInternalProbeErrorCount   int
	expectedExternalProbeSuccessCount int
	expectedExternalProbeErrorCount   int
}

func TestInternalProbeErrorCount(t *testing.T) {
	table := []probeStatusEntry{
		{"Success Count is less than Threshold", nil, 1, 0, 0, 0},
		{"Unignorable error is returned by pingKubeApiServer", errNotIgnorable, 0, 1, 0, 0},
		{"Forbidden request error is returned by pingKubeApiServer", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), 0, 0, 0, 0},
		{"Unauthorized request error is returned by pingKubeApiServer", apierrors.NewUnauthorized("unauthorized"), 0, 0, 0, 0},
		{"Throttling error is returned by pingKubeApiServer", apierrors.NewTooManyRequests("Too many requests", 10), 0, 0, 0, 0},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(2, 1, metav1.Duration{Duration: 2 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, 0.2)

			msc.EXPECT().CreateClient(gomock.Any(), pLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(1)
			mki.EXPECT().Discovery().Return(mdi).Times(1)
			mdi.EXPECT().ServerVersion().Return(nil, probeStatusEntry.err).Times(1)

			runProberAndCheckStatus(t, time.Millisecond, probeStatusEntry)
		})
	}
}

func TestHealthyProbesShouldRunScaleUp(t *testing.T) {
	table := []probeStatusEntry{
		{"Scale Up Succeeds", nil, 1, 0, 1, 0},
		{"Scale Up Fails", errors.New("scale Up failed"), 1, 0, 1, 0},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 1, metav1.Duration{Duration: 2 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, 0.2)

			msc.EXPECT().CreateClient(gomock.Any(), pLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(2)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(2)
			mdi.EXPECT().ServerVersion().Return(nil, nil).Times(2)
			mds.EXPECT().ScaleUp(gomock.Any()).Return(probeStatusEntry.err).Times(1)

			runProberAndCheckStatus(t, time.Millisecond, probeStatusEntry)
		})
	}
}

func TestExternalProbeFailingShouldRunScaleDown(t *testing.T) {
	table := []probeStatusEntry{
		{"Scale Down Succeeds", nil, 1, 0, 0, 2},
		{"Scale Down Fails", errors.New("scale Down failed"), 1, 0, 0, 2},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, 0.2)
			runCounter := 0

			msc.EXPECT().CreateClient(gomock.Any(), pLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(4)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(4)
			mdi.EXPECT().ServerVersion().DoAndReturn(func() (*version.Info, error) {
				runCounter++
				if runCounter%2 == 1 {
					return nil, nil
				}
				return nil, errNotIgnorable
			}).Times(4)
			mds.EXPECT().ScaleDown(gomock.Any()).Return(probeStatusEntry.err).Times(1)

			runProberAndCheckStatus(t, 8*time.Millisecond, probeStatusEntry)
		})
	}
}

func TestUnchangedExternalErrorCountForIgnorableErrors(t *testing.T) {
	table := []probeStatusEntry{
		{"Forbidden request error is returned by pingKubeApiServer", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), 1, 0, 0, 0},
		{"Unauthorized request error is returned by pingKubeApiServer", apierrors.NewUnauthorized("unauthorized"), 1, 0, 0, 0},
		{"Throttling error is returned by pingKubeApiServer", apierrors.NewTooManyRequests("Too many requests", 10), 1, 0, 0, 0},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, 0.2)
			runCounter := 0

			msc.EXPECT().CreateClient(gomock.Any(), pLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).MinTimes(2).MaxTimes(4)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(4).MinTimes(2).MaxTimes(4)
			mdi.EXPECT().ServerVersion().DoAndReturn(func() (*version.Info, error) {
				runCounter++
				if runCounter%2 == 1 {
					return nil, nil
				}
				return nil, probeStatusEntry.err
			}).MinTimes(2).MaxTimes(4)

			runProberAndCheckStatus(t, 8*time.Millisecond, probeStatusEntry)
		})
	}
}

func TestInternalProbeShouldNotRunIfClientNotCreated(t *testing.T) {
	err := errors.New("cannot create kubernetes client")
	setupProberTest(t)
	entry := probeStatusEntry{
		name:                              "internal probe should not run if client to access it is not created",
		err:                               err,
		expectedInternalProbeSuccessCount: 0,
		expectedInternalProbeErrorCount:   0,
		expectedExternalProbeSuccessCount: 0,
		expectedExternalProbeErrorCount:   0,
	}
	config = createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, 0.2)
	msc.EXPECT().CreateClient(gomock.Any(), pLogger, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, err).Times(2)
	runProberAndCheckStatus(t, 8*time.Millisecond, entry)
}

func TestExternalProbeShouldNotRunIfClientNotCreated(t *testing.T) {
	err := errors.New("cannot create kubernetes client")
	setupProberTest(t)
	counter := 0
	entry := probeStatusEntry{
		name:                              "external probe should not run if client to access it is not created",
		err:                               err,
		expectedInternalProbeSuccessCount: 1,
		expectedInternalProbeErrorCount:   0,
		expectedExternalProbeSuccessCount: 0,
		expectedExternalProbeErrorCount:   0,
	}
	config = createConfig(1, 2, metav1.Duration{Duration: 5 * time.Millisecond}, metav1.Duration{Duration: time.Microsecond}, 0.2)
	msc.EXPECT().CreateClient(gomock.Any(), pLogger, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, logr.Logger, string, string, time.Duration) (kubernetes.Interface, error) {
		counter++
		if counter%2 == 1 {
			return mki, nil
		} else {
			return nil, err
		}
	}).Times(4)
	mki.EXPECT().Discovery().Return(mdi).Times(2)
	mdi.EXPECT().ServerVersion().Return(nil, nil).Times(2)
	runProberAndCheckStatus(t, 8*time.Millisecond, entry)
}

func runProberAndCheckStatus(t *testing.T, duration time.Duration, probeStatusEntry probeStatusEntry) {
	g := NewWithT(t)
	p := NewProber(context.Background(), "default", config, fakeClient, mds, msc, pLogger)
	g.Expect(p.IsClosed()).To(BeFalse())

	runProber(p, duration)

	g.Expect(p.IsClosed()).To(BeTrue())
	checkProbeStatus(t, p.internalProbeStatus, probeStatusEntry.expectedInternalProbeSuccessCount, probeStatusEntry.expectedInternalProbeErrorCount)
	checkProbeStatus(t, p.externalProbeStatus, probeStatusEntry.expectedExternalProbeSuccessCount, probeStatusEntry.expectedExternalProbeErrorCount)
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
	g.Expect(ps.errorCount).To(Equal(errCount))
	g.Expect(ps.successCount).To(Equal(successCount))
}

func setupProberTest(t *testing.T) {
	ctrl = gomock.NewController(t)
	mds = mockscaler.NewMockScaler(ctrl)
	msc = mockprober.NewMockShootClientCreator(ctrl)
	clientBuilder = fake.NewClientBuilder()
	fakeClient = clientBuilder.Build()
	mki = mockinterface.NewMockInterface(ctrl)
	mdi = mockdiscovery.NewMockDiscoveryInterface(ctrl)
}

func createConfig(successThreshold int, failureThreshold int, probeInterval metav1.Duration, initialDelay metav1.Duration, backoffJitterFactor float64) *papi.Config {
	return &papi.Config{
		SuccessThreshold:                    &successThreshold,
		FailureThreshold:                    &failureThreshold,
		ProbeInterval:                       &probeInterval,
		BackoffJitterFactor:                 &backoffJitterFactor,
		InternalProbeFailureBackoffDuration: &internalProbeFailureBackoffDuration,
		InitialDelay:                        &initialDelay,
		ProbeTimeout:                        &defaultProbeTimeout,
	}
}
