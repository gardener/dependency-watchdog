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
	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machineutils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	testProbeTimeout     = metav1.Duration{Duration: 100 * time.Millisecond}
	testProbeInterval    = metav1.Duration{Duration: 100 * time.Millisecond}
	testSeedClientScheme = initializeTestScheme()
)

func initializeTestScheme() *runtime.Scheme {
	seedClientScheme := *scheme.Scheme
	_ = v1alpha1.AddToScheme(&seedClientScheme)
	return &seedClientScheme
}

func TestAPIServerProbeFailure(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		discoveryErr  error
		shouldBackOff bool
	}{
		{name: "Forbidden request error is returned by api server", discoveryErr: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), shouldBackOff: false},
		{name: "Unauthorized request error is returned by api server", discoveryErr: apierrors.NewUnauthorized("unauthorized"), shouldBackOff: false},
		{name: "Throttling error is returned by api server", discoveryErr: apierrors.NewTooManyRequests("Too many requests", 10), shouldBackOff: true},
	}

	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			scc := shootfakes.NewFakeShootClientBuilder(k8sfakes.NewFakeDiscoveryClient(entry.discoveryErr), k8sfakes.NewFakeClientBuilder().Build()).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(context.Background(), nil, test.DefaultNamespace, config, nil, nil, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			g.Expect(p.IsInBackOff()).To(Equal(entry.shouldBackOff))
			assertError(g, err, entry.discoveryErr, perrors.ErrProbeAPIServer)
		})
	}
}

func TestDiscoveryClientCreationFailed(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                       string
		discoveryClientCreationErr error
		shouldBackOff              bool
	}{
		{name: "Forbidden request error is returned while creating discovery client", discoveryClientCreationErr: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), shouldBackOff: false},
		{name: "Unauthorized request error is returned while creating discovery client", discoveryClientCreationErr: apierrors.NewUnauthorized("unauthorized"), shouldBackOff: false},
		{name: "Throttling error is returned while creating discovery client", discoveryClientCreationErr: apierrors.NewTooManyRequests("Too many requests", 10), shouldBackOff: true},
	}
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			scc := shootfakes.NewFakeShootClientBuilder(nil, nil).WithDiscoveryClientCreationError(entry.discoveryClientCreationErr).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(context.Background(), nil, test.DefaultNamespace, config, nil, nil, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			g.Expect(p.IsInBackOff()).To(Equal(entry.shouldBackOff))
			assertError(g, err, entry.discoveryClientCreationErr, perrors.ErrProbeAPIServer)
		})
	}
}

func TestClientCreationFailed(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name              string
		clientCreationErr error
		shouldBackOff     bool
	}{
		{name: "Forbidden request error is returned while creating client", clientCreationErr: apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("forbidden")), shouldBackOff: false},
		{name: "Unauthorized request error is returned while creating client", clientCreationErr: apierrors.NewUnauthorized("unauthorized"), shouldBackOff: false},
		{name: "Throttling error is returned while creating client", clientCreationErr: apierrors.NewTooManyRequests("Too many requests", 10), shouldBackOff: true},
	}

	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, nil).WithClientCreationError(entry.clientCreationErr).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(context.Background(), nil, test.DefaultNamespace, config, nil, nil, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			g.Expect(p.IsInBackOff()).To(Equal(entry.shouldBackOff))
			assertError(g, err, entry.clientCreationErr, perrors.ErrSetupProbeClient)
		})
	}
}

func TestNoScalingIfErrorInListingNodes(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                      string
		nodeListErr               *apierrors.StatusError
		areLeasesExpired          bool
		initialDeploymentReplicas int32
		shouldBackOff             bool
	}{
		{name: "no scale up should happen if error in listing nodes", nodeListErr: apierrors.NewInternalError(errors.New("test internal error")), areLeasesExpired: false, initialDeploymentReplicas: 0, shouldBackOff: false},
		{name: "no scale down should happen if error in listing nodes", nodeListErr: apierrors.NewTooManyRequests("Too many requests", 10), areLeasesExpired: true, initialDeploymentReplicas: 1, shouldBackOff: true},
	}
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{{Name: test.Node1Name, IsExpired: entry.areLeasesExpired}, {Name: test.Node2Name, IsExpired: entry.areLeasesExpired}})
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)

			shootClient := initializeShootClientBuilder(nodes, leases).RecordErrorForObjectsWithGVK("List", "", corev1.SchemeGroupVersion.WithKind("Nodes"), entry.nodeListErr).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			g.Expect(p.IsInBackOff()).To(Equal(entry.shouldBackOff))
			assertError(g, err, entry.nodeListErr, perrors.ErrProbeNodeLease)
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.initialDeploymentReplicas)
		})
	}
}

func TestNoScalingIfErrorInListingMachines(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                      string
		machineListErr            *apierrors.StatusError
		areLeasesExpired          bool
		initialDeploymentReplicas int32
		shouldBackOff             bool
	}{
		{name: "no scale up should happen if error in listing machines", machineListErr: apierrors.NewInternalError(errors.New("test internal error")), areLeasesExpired: false, initialDeploymentReplicas: 0, shouldBackOff: false},
		{name: "no scale down should happen if error in listing machines", machineListErr: apierrors.NewTooManyRequests("Too many requests", 10), areLeasesExpired: true, initialDeploymentReplicas: 1, shouldBackOff: true},
	}
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{{Name: test.Node1Name, IsExpired: entry.areLeasesExpired}, {Name: test.Node2Name, IsExpired: entry.areLeasesExpired}})
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)

			shootClient := initializeShootClientBuilder(nodes, leases).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).RecordErrorForObjectsWithGVK("List", test.DefaultNamespace, corev1.SchemeGroupVersion.WithKind("Machines"), entry.machineListErr).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			g.Expect(p.IsInBackOff()).To(Equal(entry.shouldBackOff))
			assertError(g, err, entry.machineListErr, perrors.ErrProbeNodeLease)
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.initialDeploymentReplicas)
		})
	}
}

func TestLeaseProbeShouldNotConsiderNodesNotManagedByMCM(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name, Annotations: map[string]string{machineutils.NotManagedByMCM: "true"}}, {Name: test.Node2Name, Annotations: map[string]string{machineutils.NotManagedByMCM: "true"}}, {Name: test.Node3Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine3Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node3Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)
	leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{{Name: test.Node1Name, IsExpired: true}, {Name: test.Node2Name, IsExpired: true}, {Name: test.Node3Name, IsExpired: true}})
	scaleTargetDeployments := generateScaleTargetDeployments(1)

	g := NewWithT(t)

	ctx := context.Background()
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	shootClient := initializeShootClientBuilder(nodes, leases).Build()
	seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
	scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
	scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
	config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

	p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, scc, logr.Discard())
	g.Expect(p.IsClosed()).To(BeFalse())

	g.Expect(runProber(p, testProbeTimeout.Duration)).To(BeNil())
	g.Expect(p.IsClosed()).To(BeTrue())
	assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), 1)
}

func TestLeaseProbeShouldNotConsiderFailedOrTerminatingMachines(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}, {Name: test.Node3Name}, {Name: test.Node4Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineFailed}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineTerminating}},
		{Name: test.Machine3Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node3Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine4Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node4Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                       string
		isLeaseExpired             map[string]bool
		initialDeploymentReplicas  int32
		expectedDeploymentReplicas int32
	}{
		{name: "scale up decision should not consider failed machines", isLeaseExpired: map[string]bool{test.Node1Name: true, test.Node2Name: true, test.Node3Name: false, test.Node4Name: true}, initialDeploymentReplicas: 0, expectedDeploymentReplicas: 1},
		{name: "scale down decision should not consider terminating machines", isLeaseExpired: map[string]bool{test.Node1Name: false, test.Node2Name: false, test.Node3Name: true, test.Node4Name: true}, initialDeploymentReplicas: 1, expectedDeploymentReplicas: 0},
	}

	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{
				{Name: test.Node1Name, IsExpired: entry.isLeaseExpired[test.Node1Name]},
				{Name: test.Node2Name, IsExpired: entry.isLeaseExpired[test.Node2Name]},
				{Name: test.Node3Name, IsExpired: entry.isLeaseExpired[test.Node3Name]},
				{Name: test.Node4Name, IsExpired: entry.isLeaseExpired[test.Node4Name]},
			})
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)
			shootClient := initializeShootClientBuilder(nodes, leases).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			g.Expect(runProber(p, testProbeTimeout.Duration)).To(BeNil())
			g.Expect(p.IsClosed()).To(BeTrue())
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.expectedDeploymentReplicas)
		})
	}
}

func TestLeaseProbeShouldNotConsiderUnhealthyNodes(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{
		{Name: test.Node1Name, Labels: map[string]string{util.WorkerPoolLabel: test.Worker1Name}, Conditions: []corev1.NodeCondition{{Type: test.NodeConditionDiskPressure, Status: corev1.ConditionTrue}}},
		{Name: test.Node2Name, Labels: map[string]string{util.WorkerPoolLabel: test.Worker1Name}, Conditions: []corev1.NodeCondition{{Type: test.NodeConditionMemoryPressure, Status: corev1.ConditionTrue}}},
		{Name: test.Node3Name, Labels: map[string]string{util.WorkerPoolLabel: test.Worker2Name}, Conditions: []corev1.NodeCondition{{Type: test.NodeConditionMemoryPressure, Status: corev1.ConditionTrue}}}, // This node will not be considered as unhealthy as the corresponding worker has DefaultUnhealthyNodeConditions which does not include MemoryPressure.
		{Name: test.Node4Name, Labels: map[string]string{util.WorkerPoolLabel: test.Worker2Name}},
	})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineUnknown}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineUnknown}},
		{Name: test.Machine3Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node3Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine4Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node4Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                       string
		isLeaseExpired             map[string]bool
		initialDeploymentReplicas  int32
		expectedDeploymentReplicas int32
	}{
		{name: "scale up decision should not consider unhealthy nodes", isLeaseExpired: map[string]bool{test.Node1Name: true, test.Node2Name: true, test.Node3Name: false, test.Node4Name: true}, initialDeploymentReplicas: 0, expectedDeploymentReplicas: 1},
		{name: "scale down decision should not consider unhealthy nodes", isLeaseExpired: map[string]bool{test.Node1Name: false, test.Node2Name: false, test.Node3Name: true, test.Node4Name: true}, initialDeploymentReplicas: 1, expectedDeploymentReplicas: 0},
	}

	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{
				{Name: test.Node1Name, IsExpired: entry.isLeaseExpired[test.Node1Name]},
				{Name: test.Node2Name, IsExpired: entry.isLeaseExpired[test.Node2Name]},
				{Name: test.Node3Name, IsExpired: entry.isLeaseExpired[test.Node3Name]},
				{Name: test.Node4Name, IsExpired: entry.isLeaseExpired[test.Node4Name]},
			})
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)
			shootClient := initializeShootClientBuilder(nodes, leases).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, map[string][]string{test.Worker1Name: {test.NodeConditionDiskPressure, test.NodeConditionMemoryPressure}}, scaler, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			g.Expect(runProber(p, testProbeTimeout.Duration)).To(BeNil())
			g.Expect(p.IsClosed()).To(BeTrue())
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.expectedDeploymentReplicas)
		})
	}
}

func TestNoScalingIfErrorInListingLeases(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                      string
		leaseListErr              *apierrors.StatusError
		areLeasesExpired          bool
		initialDeploymentReplicas int32
		shouldBackOff             bool
	}{
		{name: "no scale up should happen if error in listing leases", leaseListErr: apierrors.NewInternalError(errors.New("test internal error")), initialDeploymentReplicas: 0, shouldBackOff: false},
		{name: "no scale down should happen if error in listing leases", leaseListErr: apierrors.NewTooManyRequests("Too many requests", 10), initialDeploymentReplicas: 1, shouldBackOff: true},
	}
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{{Name: test.Node1Name, IsExpired: entry.areLeasesExpired}, {Name: test.Node2Name, IsExpired: entry.areLeasesExpired}})
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)
			shootClient := initializeShootClientBuilder(nodes, leases).RecordErrorForObjectsWithGVK("List", nodeLeaseNamespace, corev1.SchemeGroupVersion.WithKind("Leases"), entry.leaseListErr).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			assertError(g, err, entry.leaseListErr, perrors.ErrProbeNodeLease)
			g.Expect(p.IsInBackOff()).To(Equal(entry.shouldBackOff))
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.initialDeploymentReplicas)
		})
	}
}

func TestNoScalingInSingleNodeClusters(t *testing.T) {
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                      string
		isLeaseExpired            bool
		initialDeploymentReplicas int32
	}{
		{name: "no scale up should happen", isLeaseExpired: false, initialDeploymentReplicas: 0},
		{name: "no scale down should happen", isLeaseExpired: true, initialDeploymentReplicas: 1},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{{Name: test.Node1Name, IsExpired: entry.isLeaseExpired}})
			shootClient := initializeShootClientBuilder(nodes, leases).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
			shootClientCreator := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()

			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)
			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, shootClientCreator, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			g.Expect(err).To(BeNil())
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.initialDeploymentReplicas)
		})
	}
}

func TestLeaseProbeShouldNotConsiderOrphanedLeases(t *testing.T) {
	t.Parallel()
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)

	testCases := []struct {
		name                       string
		isLeaseExpired             map[string]bool
		initialDeploymentReplicas  int32
		expectedDeploymentReplicas int32
	}{
		{name: "scale up decision should not consider orphaned leases", isLeaseExpired: map[string]bool{test.Node1Name: false, test.Node2Name: true, test.Node3Name: true, test.Node4Name: true}, initialDeploymentReplicas: 0, expectedDeploymentReplicas: 1},
		{name: "scale down decision should not consider orphaned leases", isLeaseExpired: map[string]bool{test.Node1Name: true, test.Node2Name: true, test.Node3Name: false, test.Node4Name: false}, initialDeploymentReplicas: 1, expectedDeploymentReplicas: 0},
	}

	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	g := NewWithT(t)
	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{
				{Name: test.Node1Name, IsExpired: entry.isLeaseExpired[test.Node1Name]},
				{Name: test.Node2Name, IsExpired: entry.isLeaseExpired[test.Node2Name]},
				{Name: test.Node3Name, IsExpired: entry.isLeaseExpired[test.Node3Name]},
				{Name: test.Node4Name, IsExpired: entry.isLeaseExpired[test.Node4Name]},
			})
			scaleTargetDeployments := generateScaleTargetDeployments(entry.initialDeploymentReplicas)
			shootClient := initializeShootClientBuilder(nodes, leases).Build()
			seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, nil)
			scc := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, scc, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			g.Expect(runProber(p, testProbeTimeout.Duration)).To(BeNil())
			g.Expect(p.IsClosed()).To(BeTrue())
			assertScale(ctx, g, seedClient, getDeploymentRefs(scaleTargetDeployments), entry.expectedDeploymentReplicas)
		})
	}
}

func TestSuccessfulProbesShouldRunScaleUp(t *testing.T) {
	t.Parallel()
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
	seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	shootClientCreator := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()

	testCases := []struct {
		name       string
		scaleUpErr error
	}{
		{name: "Scale Up Succeeds"},
		{name: "Scale Up Fails", scaleUpErr: errors.New("scale up failed")},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, entry.scaleUpErr, nil)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, shootClientCreator, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

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

func TestLeaseProbeFailureShouldRunScaleDown(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	nodes := test.GenerateNodes([]test.NodeSpec{{Name: test.Node1Name}, {Name: test.Node2Name}})
	leases := test.GenerateNodeLeases([]test.NodeLeaseSpec{
		{Name: test.Node1Name, IsExpired: true},
		{Name: test.Node2Name, IsExpired: true},
	})
	machines := test.GenerateMachines([]test.MachineSpec{
		{Name: test.Machine1Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node1Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
		{Name: test.Machine2Name, Labels: map[string]string{v1alpha1.NodeLabelKey: test.Node2Name}, CurrentStatus: v1alpha1.CurrentStatus{Phase: v1alpha1.MachineRunning}},
	}, test.DefaultNamespace)
	scaleTargetDeployments := generateScaleTargetDeployments(1)

	shootClient := initializeShootClientBuilder(nodes, leases).Build()
	seedClient := initializeSeedClientBuilder(machines, scaleTargetDeployments).Build()
	shootDiscoveryClient := k8sfakes.NewFakeDiscoveryClient(nil)
	shootClientCreator := shootfakes.NewFakeShootClientBuilder(shootDiscoveryClient, shootClient).Build()

	testCases := []struct {
		name         string
		scaleDownErr error
	}{
		{name: "Scale Down Succeeds"},
		{name: "Scale Down Fails", scaleDownErr: errors.New("scale down failed")},
	}

	for _, entry := range testCases {
		t.Run(entry.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			scaler := scalefakes.NewFakeScaler(seedClient, test.DefaultNamespace, nil, entry.scaleDownErr)
			config := createConfig(testProbeInterval, metav1.Duration{Duration: time.Microsecond}, metav1.Duration{Duration: 40 * time.Second}, 0.2)

			p := NewProber(ctx, seedClient, test.DefaultNamespace, config, nil, scaler, shootClientCreator, logr.Discard())
			g.Expect(p.IsClosed()).To(BeFalse())

			err := runProber(p, testProbeTimeout.Duration)
			g.Expect(p.IsClosed()).To(BeTrue())
			if entry.scaleDownErr != nil {
				assertError(g, err, entry.scaleDownErr, perrors.ErrScaleDown)
			} else {
				g.Expect(err).To(BeNil())
				targetDeploymentRefs := getDeploymentRefs(scaleTargetDeployments)
				assertScale(ctx, g, seedClient, targetDeploymentRefs, 0)
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

func initializeSeedClientBuilder(machines []*v1alpha1.Machine, deployments []*appsv1.Deployment) *k8sfakes.FakeClientBuilder {
	seedObjects := make([]client.Object, 0, len(machines)+len(deployments))
	for _, machine := range machines {
		seedObjects = append(seedObjects, machine)
	}
	for _, deploy := range deployments {
		seedObjects = append(seedObjects, deploy)
	}
	return k8sfakes.NewFakeClientBuilder(seedObjects...).WithScheme(testSeedClientScheme)
}

func generateScaleTargetDeployments(replicas int32) []*appsv1.Deployment {
	return []*appsv1.Deployment{
		test.GenerateDeployment(test.KCMDeploymentName, test.DefaultNamespace, test.DefaultImage, replicas, nil),
		test.GenerateDeployment(test.MCMDeploymentName, test.DefaultNamespace, test.DefaultImage, replicas, nil),
		test.GenerateDeployment(test.CADeploymentName, test.DefaultNamespace, test.DefaultImage, replicas, nil),
	}
}

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
