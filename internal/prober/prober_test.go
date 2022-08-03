package prober

import (
	"context"
	"errors"
	papi "github.com/gardener/dependency-watchdog/api/prober"
	"testing"
	"time"

	mockprober "github.com/gardener/dependency-watchdog/internal/mock/prober"
	mockinterface "github.com/gardener/dependency-watchdog/internal/mock/prober/k8s/client"
	mockdiscovery "github.com/gardener/dependency-watchdog/internal/mock/prober/k8s/discovery"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	config                              *papi.Config
	ctrl                                *gomock.Controller
	mds                                 *mockprober.MockDeploymentScaler
	msc                                 *mockprober.MockShootClientCreator
	clientBuilder                       *fake.ClientBuilder
	fakeClient                          client.WithWatch
	mki                                 *mockinterface.MockInterface
	mdi                                 *mockdiscovery.MockDiscoveryInterface
	notIgnorableErr                     = errors.New("Not Ignorable error")
	internalProbeFailureBackoffDuration = time.Millisecond
	pLogger                             = log.FromContext(context.Background()).WithName("proberLogger")
	defaultProbeTimeout                 = 40 * time.Second
)

type probeStatusEntry struct {
	name                              string
	err                               error
	expectedInternalProbeSuccessCount int
	expectedInternalProbeErrorCount   int
	expectedExternalProbeSuccessCount int
	expectedExternalProbeErrorCount   int
	isInternalProbeBackoffNil         bool
	isExternalProbeBackoffNil         bool
}

func TestInternalProbeErrorCount(t *testing.T) {
	table := []probeStatusEntry{
		{"Success Count is less than Threshold", nil, 1, 0, 0, 0, false, true},
		{"Unignorable error is returned by pingKubeApiServer", notIgnorableErr, 0, 1, 0, 0, false, true},
		{"Forbidden request error is returned by pingKubeApiServer", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), 0, 0, 0, 0, true, true},
		{"Unauthorized request error is returned by pingKubeApiServer", apierrors.NewUnauthorized("unauthorized"), 0, 0, 0, 0, true, true},
		{"Throttling error is returned by pingKubeApiServer", apierrors.NewTooManyRequests("Too many requests", 10), 0, 0, 0, 0, false, true},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(2, 1, 2*time.Millisecond, time.Microsecond, 0.2)

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(1)
			mki.EXPECT().Discovery().Return(mdi).Times(1)
			mdi.EXPECT().ServerVersion().Return(nil, probeStatusEntry.err).Times(1)

			runProberAndCheckStatus(t, time.Millisecond, probeStatusEntry)
		})
	}
}

func TestHealthyProbesShouldRunScaleUp(t *testing.T) {
	table := []probeStatusEntry{
		{"Scale Up Succeeds", nil, 1, 0, 1, 0, false, false},
		{"Scale Up Fails", errors.New("Scale Up failed"), 1, 0, 1, 0, false, false},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 1, 2*time.Millisecond, time.Microsecond, 0.2)

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(2)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(2)
			mdi.EXPECT().ServerVersion().Return(nil, nil).Times(2)
			mds.EXPECT().ScaleUp(gomock.Any()).Return(probeStatusEntry.err).Times(1)

			runProberAndCheckStatus(t, time.Millisecond, probeStatusEntry)
		})
	}
}

func TestExternalProbeFailingShouldRunScaleDown(t *testing.T) {
	table := []probeStatusEntry{
		{"Scale Down Succeeds", nil, 1, 0, 0, 2, false, false},
		{"Scale Down Fails", errors.New("Scale Down failed"), 1, 0, 0, 2, false, false},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 2, 5*time.Millisecond, time.Microsecond, 0.2)
			runCounter := 0

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).Times(4)
			mki.EXPECT().Discovery().Return(mdi).AnyTimes().Times(4)
			mdi.EXPECT().ServerVersion().DoAndReturn(func() (*version.Info, error) {
				runCounter++
				if runCounter%2 == 1 {
					return nil, nil
				}
				return nil, notIgnorableErr
			}).Times(4)
			mds.EXPECT().ScaleDown(gomock.Any()).Return(probeStatusEntry.err).Times(1)

			runProberAndCheckStatus(t, 8*time.Millisecond, probeStatusEntry)
		})
	}
}

func TestUnchangedExternalErrorCountForIgnorableErrors(t *testing.T) {
	table := []probeStatusEntry{
		{"Forbidden request error is returned by pingKubeApiServer", apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), 1, 0, 0, 0, false, true},
		{"Unauthorized request error is returned by pingKubeApiServer", apierrors.NewUnauthorized("unauthorized"), 1, 0, 0, 0, false, true},
		{"Throttling error is returned by pingKubeApiServer", apierrors.NewTooManyRequests("Too many requests", 10), 1, 0, 0, 0, false, false},
	}

	for _, probeStatusEntry := range table {
		t.Run(probeStatusEntry.name, func(t *testing.T) {
			setupProberTest(t)
			config = createConfig(1, 2, 5*time.Millisecond, time.Microsecond, 0.2)
			runCounter := 0

			msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mki, nil).MinTimes(2).MaxTimes(4)
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
		isInternalProbeBackoffNil:         true,
		expectedExternalProbeSuccessCount: 0,
		expectedExternalProbeErrorCount:   0,
		isExternalProbeBackoffNil:         true,
	}
	config = createConfig(1, 2, 5*time.Millisecond, time.Microsecond, 0.2)
	msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, err).Times(2)
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
		isInternalProbeBackoffNil:         false,
		expectedExternalProbeSuccessCount: 0,
		expectedExternalProbeErrorCount:   0,
		isExternalProbeBackoffNil:         true,
	}
	config = createConfig(1, 2, 5*time.Millisecond, time.Microsecond, 0.2)
	msc.EXPECT().CreateClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, string, time.Duration) (kubernetes.Interface, error) {
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
	checkProbeStatus(t, p.internalProbeStatus, probeStatusEntry.expectedInternalProbeSuccessCount, probeStatusEntry.expectedInternalProbeErrorCount, probeStatusEntry.isInternalProbeBackoffNil)
	checkProbeStatus(t, p.externalProbeStatus, probeStatusEntry.expectedExternalProbeSuccessCount, probeStatusEntry.expectedExternalProbeErrorCount, probeStatusEntry.isExternalProbeBackoffNil)
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

func checkProbeStatus(t *testing.T, ps probeStatus, successCount int, errCount int, isBackoffNil bool) {
	g := NewWithT(t)
	g.Expect(ps.errorCount).To(Equal(errCount))
	g.Expect(ps.successCount).To(Equal(successCount))
	g.Expect(ps.backOff == nil).To(Equal(isBackoffNil))
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

func createConfig(successThreshold int, failureThreshold int, probeInterval time.Duration, initialDelay time.Duration, backoffJitterFactor float64) *papi.Config {
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
