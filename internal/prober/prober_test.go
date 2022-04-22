package prober

import (
	"errors"
	"testing"
	"time"

	mockprober "github.com/gardener/dependency-watchdog/internal/mock/prober"
	mockinterface "github.com/gardener/dependency-watchdog/internal/mock/prober/k8s/client"
	mockdiscovery "github.com/gardener/dependency-watchdog/internal/mock/prober/k8s/discovery"
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	version "k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	config          *Config
	ctrl            *gomock.Controller
	mds             *mockprober.MockDeploymentScaler
	msc             *mockprober.MockShootClientCreator
	clientBuilder   *fake.ClientBuilder
	fakeClient      client.WithWatch
	mki             *mockinterface.MockInterface
	mdi             *mockdiscovery.MockDiscoveryInterface
	notIgnorableErr = errors.New("Not Ignorable error")
)

type entry struct {
	name                              string
	err                               error
	expectedInternalProbeSuccessCount int
	expectedInternalProbeErrorCount   int
	expectedExternalProbeSuccessCount int
	expectedExternalProbeErrorCount   int
}

func TestInternalProbeErrorCount(t *testing.T) {
	table := []entry{
		{"Success Count is less than Threshold", nil, 1, 0, 0, 0},
		{"Unignorable error is returned by doProbe", notIgnorableErr, 0, 1, 0, 0},
		{"Not found error is returned by doProbe", apierrors.NewNotFound(schema.GroupResource{}, "test"), 0, 0, 0, 0},
		{"Forbidden request error is returned by doProbe", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), 0, 0, 0, 0},
		{"Unauthorized request error is returned by doProbe", apierrors.NewUnauthorized("unauthorized"), 0, 0, 0, 0},
		{"Throttling error is returned by doProbe", apierrors.NewTooManyRequests("Too many requests", 10), 0, 0, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(2, 1, 2*time.Millisecond, 0.2)

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(1)
			mki.EXPECT().Discovery().Return(mdi).Times(1)
			mdi.EXPECT().ServerVersion().Return(nil, entry.err).Times(1)

			runProberAndCheckStatus(t, time.Millisecond, entry)
		})
	}
}

func TestHealthyProbesShouldRunScaleUp(t *testing.T) {
	table := []entry{
		{"Scale Up Succeeds", nil, 1, 0, 1, 0},
		{"Scale Up Fails", errors.New("Scale Up failed"), 1, 0, 1, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 1, 2*time.Millisecond, 0.2)

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(2)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(2)
			mdi.EXPECT().ServerVersion().Return(nil, nil).Times(2)
			mds.EXPECT().ScaleUp(gomock.Any()).Return(entry.err).Times(1)

			runProberAndCheckStatus(t, time.Millisecond, entry)
		})
	}
}

func TestExternalProbeFailingShouldRunScaleDown(t *testing.T) {
	table := []entry{
		{"Scale Down Succeeds", nil, 2, 0, 0, 2},
		{"Scale Down Fails", errors.New("Scale Down failed"), 2, 0, 0, 2},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 2, 5*time.Millisecond, 0.2)
			runCounter := 0

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(4)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(4)
			mdi.EXPECT().ServerVersion().DoAndReturn(func() (*version.Info, error) {
				runCounter++
				if runCounter%2 == 1 {
					return nil, nil
				}
				return nil, notIgnorableErr
			}).Times(4)
			mds.EXPECT().ScaleDown(gomock.Any()).Return(entry.err).Times(1)

			runProberAndCheckStatus(t, 8*time.Millisecond, entry)
		})
	}
}

func TestUnchangedExternalErrorCountForIgnorableErrors(t *testing.T) {
	table := []entry{
		{"Not found error is returned by doProbe", apierrors.NewNotFound(schema.GroupResource{}, "test"), 2, 0, 0, 0},
		{"Forbidden request error is returned by doProbe", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), 2, 0, 0, 0},
		{"Unauthorized request error is returned by doProbe", apierrors.NewUnauthorized("unauthorized"), 2, 0, 0, 0},
		{"Throttling error is returned by doProbe", apierrors.NewTooManyRequests("Too many requests", 10), 2, 0, 0, 0},
	}

	for _, entry := range table {
		t.Run(entry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 2, 5*time.Millisecond, 0.2)
			runCounter := 0

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).MinTimes(2).MaxTimes(4)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(4).MinTimes(2).MaxTimes(4)
			mdi.EXPECT().ServerVersion().DoAndReturn(func() (*version.Info, error) {
				runCounter++
				if runCounter%2 == 1 {
					return nil, nil
				}
				return nil, entry.err
			}).MinTimes(2).MaxTimes(4)

			runProberAndCheckStatus(t, 8*time.Millisecond, entry)
		})
	}
}

func runProberAndCheckStatus(t *testing.T, duration time.Duration, entry entry) {
	g := NewWithT(t)
	p := NewProber("default", config, fakeClient, mds, msc)
	g.Expect(p.IsClosed()).To(BeFalse())

	runProber(p, duration)

	g.Expect(p.IsClosed()).To(BeTrue())
	checkProbeStatus(t, p.internalProbeStatus, entry.expectedInternalProbeSuccessCount, entry.expectedInternalProbeErrorCount)
	checkProbeStatus(t, p.externalProbeStatus, entry.expectedExternalProbeSuccessCount, entry.expectedExternalProbeErrorCount)
}

func runProber(p *Prober, d time.Duration) {
	exitAfter := time.NewTimer(d)
	go p.Run()
	for {
		select {
		case <-exitAfter.C:
			p.Close()
			return
		case <-p.stopC:
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
	mds = mockprober.NewMockDeploymentScaler(ctrl)
	msc = mockprober.NewMockShootClientCreator(ctrl)
	clientBuilder = fake.NewClientBuilder()
	fakeClient = clientBuilder.Build()
	mki = mockinterface.NewMockInterface(ctrl)
	mdi = mockdiscovery.NewMockDiscoveryInterface(ctrl)
}

func createConfig(successThreshold int, failureThreshold int, probeInterval time.Duration, backoffJitterFactor float64) *Config {
	return &Config{
		SuccessThreshold:    &successThreshold,
		FailureThreshold:    &failureThreshold,
		ProbeInterval:       &probeInterval,
		BackoffJitterFactor: &backoffJitterFactor,
	}
}
