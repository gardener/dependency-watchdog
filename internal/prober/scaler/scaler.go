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

// replicasCheckPredicate checks if scaling should be done for the current number of replicas
type replicasCheckPredicate func(currentReplicas int32) bool

// scalingCompletePredicate checks if scaling of the resource is complete based on the current and target replica count
type scalingCompletePredicate func(currentReplicas, targetReplicas int32) bool

// operation denotes either a scale up or scale down operation.
type operationType uint8

const (
	// scaleUp represents a scale-up operation for a kubernetes resource.
	scaleUp operationType = iota // scale-up
	// scaleDown represents a scale-up operation for a kubernetes resource.
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
		fn1 replicasCheckPredicate
		fn2 scalingCompletePredicate
	)

	if opType == scaleUp {
		fn1 = scaleUpReplicasPredicate
		fn2 = scaleUpCompletePredicate

	} else {
		fn1 = scaleDownReplicasPredicate
		fn2 = scaleDownCompletePredicate
	}
	return operation{
		opType:                       opType,
		shouldScaleReplicasPredicate: fn1,
		scalingCompletePredicate:     fn2,
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
