// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/gardener/pkg/utils/flow"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	scalev1 "k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultMaxResourceScalingAttempts = 3
)

type flowCreator interface {
	createFlow(name string, namespace string, opType operation) *scaleFlow
}

type creator struct {
	client                 client.Client
	scaler                 scalev1.ScaleInterface
	logger                 logr.Logger
	options                *scalerOptions
	dependentResourceInfos []papi.DependentResourceInfo
}

func newFlowCreator(client client.Client, scaler scalev1.ScaleInterface, logger logr.Logger, options *scalerOptions, dependentResourceInfos []papi.DependentResourceInfo) flowCreator {
	return &creator{
		client:                 client,
		scaler:                 scaler,
		logger:                 logger,
		options:                options,
		dependentResourceInfos: dependentResourceInfos,
	}
}

func (c *creator) createFlow(name string, namespace string, opType operation) *scaleFlow {
	resourceInfos := createScalableResourceInfos(opType, c.dependentResourceInfos)
	levels := sortAndGetUniqueLevels(resourceInfos)
	orderedResourceInfos := collectResourceInfosByLevel(resourceInfos)
	g := flow.NewGraph(name)
	sf := newScaleFlow()
	var previousLevelResourceInfos []scalableResourceInfo
	var previousTaskIDs flow.TaskIDs
	for _, level := range levels {
		if resInfos, ok := orderedResourceInfos[level]; ok {
			dependentTaskIDs := previousTaskIDs
			taskID := g.Add(flow.Task{
				Name:         createTaskName(resInfos, level),
				Fn:           c.createScaleTaskFn(namespace, resInfos),
				Dependencies: dependentTaskIDs,
			})
			sf.addScaleStepInfo(taskID, dependentTaskIDs, previousLevelResourceInfos)
			previousLevelResourceInfos = append(previousLevelResourceInfos, resInfos...)
			if previousTaskIDs == nil {
				previousTaskIDs = flow.NewTaskIDs(taskID)
			} else {
				previousTaskIDs.Insert(taskID)
			}
		}
	}
	sf.setFlow(g.Compile())
	return sf
}

// createScaleTaskFn creates a flow.TaskFn for a slice of DependentResourceInfo. If there are more than one
// DependentResourceInfo passed to this function, it indicates that they all are at the same level indicating that these functions
// should be invoked concurrently. In this case it will construct a flow.Parallel. If there is only one DependentResourceInfo passed
// then it indicates that at a specific level there is only one DependentResourceInfo that needs to be scaled.
func (c *creator) createScaleTaskFn(namespace string, resourceInfos []scalableResourceInfo) flow.TaskFn {
	taskFns := make([]flow.TaskFn, 0, len(resourceInfos))
	for _, resourceInfo := range resourceInfos {
		taskFn := c.doCreateTaskFn(namespace, resourceInfo)
		taskFns = append(taskFns, taskFn)
	}
	if len(taskFns) == 1 {
		return taskFns[0]
	}
	return flow.Parallel(taskFns...)
}

func (c *creator) doCreateTaskFn(namespace string, resInfo scalableResourceInfo) flow.TaskFn {
	return func(ctx context.Context) error {
		var operation string
		if resInfo.operation == scaleUp {
			operation = fmt.Sprintf("scaleUp-resource-%s.%s", namespace, resInfo.ref.Name)
		} else {
			operation = fmt.Sprintf("scaleDown-resource-%s.%s", namespace, resInfo.ref.Name)
		}
		resScaler := newResourceScaler(c.client, c.scaler, c.logger, c.options, namespace, resInfo)
		result := util.Retry(ctx, c.logger,
			operation,
			func() (interface{}, error) {
				err := resScaler.scale(ctx)
				return nil, err
			},
			defaultMaxResourceScalingAttempts,
			*c.options.scaleResourceBackOff,
			util.AlwaysRetry)
		return result.Err
	}
}

type scaleFlow struct {
	flow          *flow.Flow
	flowStepInfos []scaleStepInfo
}

type scaleStepInfo struct {
	taskID           flow.TaskID
	dependentTaskIDs flow.TaskIDs
	waitOnResources  []autoscalingv1.CrossVersionObjectReference
}

func newScaleFlow() *scaleFlow {
	return &scaleFlow{
		flowStepInfos: make([]scaleStepInfo, 0, 1),
	}
}

func (sf *scaleFlow) addScaleStepInfo(id flow.TaskID, dependentTaskIDs flow.TaskIDs, waitOnResourceInfos []scalableResourceInfo) {
	sf.flowStepInfos = append(sf.flowStepInfos, scaleStepInfo{
		taskID:           id,
		dependentTaskIDs: dependentTaskIDs.Copy(),
		waitOnResources:  mapToCrossVersionObjectRef(waitOnResourceInfos),
	})
}

func (sf *scaleFlow) setFlow(flow *flow.Flow) {
	sf.flow = flow
}

func (s scaleStepInfo) String() string {
	return fmt.Sprintf("{taskID: %s, dependentTaskIDs: %s, waitOnResources: %v}", s.taskID, s.dependentTaskIDs, s.waitOnResources)
}
