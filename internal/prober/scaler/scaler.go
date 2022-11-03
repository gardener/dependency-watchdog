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

package scaler

import (
	"context"
	"fmt"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/gardener/pkg/utils/flow"
	"github.com/go-logr/logr"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	scalev1 "k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// replicasCheckPredicate checks if scaling should be done for the current number of replicas.
// The target replicas for a resource are captured as annotation value. It is however possible that another actor
// HPA or HVPA changes the replicas of the resource (scales it down or scales it up) causing the target replica annotation
// value to differ from the spec.replicas for the resource. Since DWD is not a `horizontal-pod-autoscaler` but its intention
// is only to restore the resource to the last captured replicas when it attempts to scale up the resource which was previously scaled-down to 0 by DWD.
// If the current replicas > 0 but it is different from the last captured replicas as annotation value then it will skip any further scaling as that would
// potentially interfere with HPA/HVPA.
type replicasCheckPredicate func(currentReplicas int32) bool

// scalingCompletePredicate checks if scaling of the resource is complete based on the current and target replica count.
// This is used during the scale up for a resource which was previously scaled down by DWD. If the decision is to scale the resource
// (a decision influenced by `replicasCheckPredicate`), then this predicate checks if the scaling is complete.
type scalingCompletePredicate func(currentReplicas, targetReplicas int32) bool

// operation denotes either a scale up or scale down action initiated by DWD.
type operationType uint8

const (
	// scaleUp represents a scale-up action for a kubernetes resource.
	scaleUp operationType = iota // scale-up
	// scaleDown represents a scale-up action for a kubernetes resource.
	scaleDown // scale-down
)

//go:generate stringer -type=operationType -linecomment

// Scaler is a facade to provide scaling operations for kubernetes scalable resources.
type Scaler interface {
	// ScaleUp restores the replicas of a kubernetes resource prior to scale down.
	ScaleUp(ctx context.Context) error
	// ScaleDown scales down a kubernetes scalable resource to 0.
	ScaleDown(ctx context.Context) error
}

// NewScaler creates an instance of Scaler.
func NewScaler(namespace string, config *papi.Config, client client.Client, scalerGetter scalev1.ScalesGetter, logger logr.Logger, options ...scalerOption) Scaler {
	logger = logger.WithName("scaleFlowRunner")
	opts := buildScalerOptions(options...)

	fc := newFlowCreator(client, scalerGetter.Scales(namespace), logger, opts, config.DependentResourceInfos)
	scaleUpFlow := fc.createFlow(fmt.Sprintf("scale-up-%s", namespace), namespace, scaleUp)
	logger.V(5).Info("Created scaleUpFlow", "flowStepInfos", scaleUpFlow.flowStepInfos)
	scaleDownFlow := fc.createFlow(fmt.Sprintf("scale-down-%s", namespace), namespace, scaleDown)
	logger.V(5).Info("Created scaleDownFlow", "flowStepInfos", scaleDownFlow.flowStepInfos)

	return &scaleFlowRunner{
		namespace:     namespace,
		options:       opts,
		scaleUpFlow:   scaleUpFlow.flow,
		scaleDownFlow: scaleDownFlow.flow,
	}
}

type scaleFlowRunner struct {
	namespace     string
	scaleDownFlow *flow.Flow
	scaleUpFlow   *flow.Flow
	options       *scalerOptions
}

func (ds *scaleFlowRunner) ScaleDown(ctx context.Context) error {
	return ds.scaleDownFlow.Run(ctx, flow.Opts{})
}

func (ds *scaleFlowRunner) ScaleUp(ctx context.Context) error {
	return ds.scaleUpFlow.Run(ctx, flow.Opts{})
}

type operation struct {
	opType                       operationType
	shouldScaleReplicasPredicate replicasCheckPredicate
	scalingCompletePredicate     scalingCompletePredicate
}

func newScaleOperation(opType operationType) operation {
	var (
		replicasCheckFn        replicasCheckPredicate
		scalingCompleteCheckFn scalingCompletePredicate
	)

	if opType == scaleUp {
		replicasCheckFn = scaleUpReplicasPredicate
		scalingCompleteCheckFn = scaleUpCompletePredicate
	} else {
		replicasCheckFn = scaleDownReplicasPredicate
		scalingCompleteCheckFn = scaleDownCompletePredicate
	}

	return operation{
		opType:                       opType,
		shouldScaleReplicasPredicate: replicasCheckFn,
		scalingCompletePredicate:     scalingCompleteCheckFn,
	}
}

// scalableResourceInfo captures scaling configuration for a DependentResourceInfo.
type scalableResourceInfo struct {
	ref          *autoscalingv1.CrossVersionObjectReference
	shouldExist  bool
	level        int
	initialDelay time.Duration
	timeout      time.Duration
	operation    operation
}

func (r scalableResourceInfo) String() string {
	return fmt.Sprintf("{Resource ref: %#v, level: %d, initialDelay: %#v, timeout: %#v, operation: %v}",
		*r.ref, r.level, r.initialDelay, r.timeout, r.operation)
}
