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

package prober

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"

	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/gardener/pkg/utils/flow"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scalev1 "k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// operation denotes either a scale up or scale down operation.
type operation uint8

const (
	ignoreScalingAnnotationKey        = "dependency-watchdog.gardener.cloud/ignore-scaling"
	defaultMaxResourceScalingAttempts = 3
	defaultScaleResourceBackoff       = 100 * time.Millisecond
	replicasAnnotationKey             = "dependency-watchdog.gardener.cloud/replicas"

	// scaleUp represents a scale-up operation for a kubernetes resource.
	scaleUp operation = iota // scale-up
	// scaleDown represents a scale-up operation for a kubernetes resource.
	scaleDown // scale-down
)

//go:generate stringer -type=operation -linecomment

// DeploymentScaler is a facade to provide scaling operations for kubernetes deployments.
type DeploymentScaler interface {
	ScaleUp(ctx context.Context) error
	ScaleDown(ctx context.Context) error
}

// NewDeploymentScaler creates an instance of DeploymentScaler.
func NewDeploymentScaler(namespace string, config *papi.Config, client client.Client, scalerGetter scalev1.ScalesGetter, logger logr.Logger, options ...scalerOption) DeploymentScaler {
	logger = logger.WithName("scaler")
	opts := buildScalerOptions(options...)
	ds := deploymentScaler{
		namespace: namespace,
		scaler:    scalerGetter.Scales(namespace),
		client:    client,
		options:   opts,
		l:         logger,
	}
	scaleDownFlow := ds.createResourceScaleFlow(namespace, fmt.Sprintf("scale-down-%s", namespace), createScaleDownResourceInfos(config.DependentResourceInfos), util.ScaleDownReplicasMismatch)
	ds.l.V(5).Info("Created scaleDownFlow", "flowStepInfos", scaleDownFlow.flowStepInfos)
	ds.scaleDownFlow = scaleDownFlow.flow
	scaleUpFlow := ds.createResourceScaleFlow(namespace, fmt.Sprintf("scale-up-%s", namespace), createScaleUpResourceInfos(config.DependentResourceInfos), util.ScaleUpReplicasMismatch)
	ds.l.V(5).Info("Created scaleUpFlow", "flowStepInfos", scaleUpFlow.flowStepInfos)
	ds.scaleUpFlow = scaleUpFlow.flow
	return &ds
}

// scaleableResourceInfo contains a flattened scaleUp or scaleDown resource info for a given resource reference
type scaleableResourceInfo struct {
	ref          *autoscalingv1.CrossVersionObjectReference
	shouldExist  bool
	level        int
	initialDelay time.Duration
	timeout      time.Duration
	operation    operation
	replicas     int32
}

func (r scaleableResourceInfo) String() string {
	return fmt.Sprintf("{Resource ref: %#v, level: %d, initialDelay: %#v, timeout: %#v, operation: %s}",
		*r.ref, r.level, r.initialDelay, r.timeout, r.operation)
}

type mismatchReplicasCheckFn func(replicas, targetReplicas int32) bool

type deploymentScaler struct {
	namespace     string
	scaler        scalev1.ScaleInterface
	client        client.Client
	scaleDownFlow *flow.Flow
	scaleUpFlow   *flow.Flow
	options       *scalerOptions
	l             logr.Logger
}

func (ds *deploymentScaler) ScaleDown(ctx context.Context) error {
	return ds.scaleDownFlow.Run(ctx, flow.Opts{})
}

func (ds *deploymentScaler) ScaleUp(ctx context.Context) error {
	return ds.scaleUpFlow.Run(ctx, flow.Opts{})
}

func isIgnoreScalingAnnotationSet(objMeta metav1.ObjectMeta) bool {
	if val, ok := objMeta.Annotations[ignoreScalingAnnotationKey]; ok {
		return val == "true"
	}
	return false
}

type scaleFlow struct {
	flow          *flow.Flow
	flowStepInfos []scaleStepInfo
}

type scaleStepInfo struct {
	taskID              flow.TaskID
	dependentTaskIDs    flow.TaskIDs
	waitOnResourceInfos []scaleableResourceInfo
}

func (s scaleStepInfo) String() string {
	return fmt.Sprintf("{taskID: %s, dependentTaskIDs: %s, waitOnResourceInfos: %v}", s.taskID, s.dependentTaskIDs, s.waitOnResourceInfos)
}

func newScaleFlow() *scaleFlow {
	return &scaleFlow{
		flowStepInfos: make([]scaleStepInfo, 0, 1),
	}
}

func (sf *scaleFlow) addScaleStepInfo(id flow.TaskID, dependentTaskIDs flow.TaskIDs, waitOnResources []scaleableResourceInfo) {
	sf.flowStepInfos = append(sf.flowStepInfos, scaleStepInfo{
		taskID:              id,
		dependentTaskIDs:    dependentTaskIDs.Copy(),
		waitOnResourceInfos: waitOnResources,
	})
}

func (sf *scaleFlow) setFlow(flow *flow.Flow) {
	sf.flow = flow
}

func (ds *deploymentScaler) createResourceScaleFlow(namespace, flowName string, resourceInfos []scaleableResourceInfo, replicaPredicateFn mismatchReplicasCheckFn) *scaleFlow {
	levels := sortAndGetUniqueLevels(resourceInfos)
	orderedResourceInfos := collectResourceInfosByLevel(resourceInfos)
	g := flow.NewGraph(flowName)
	sf := newScaleFlow()
	var previousLevelResourceInfos []scaleableResourceInfo
	var previousTaskIDs flow.TaskIDs
	for _, level := range levels {
		if resInfos, ok := orderedResourceInfos[level]; ok {
			dependentTaskIDs := previousTaskIDs
			taskID := g.Add(flow.Task{
				Name:         createTaskName(resInfos, level),
				Fn:           ds.createScaleTaskFn(namespace, resInfos, replicaPredicateFn, previousLevelResourceInfos),
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

func createTaskName(resInfos []scaleableResourceInfo, level int) string {
	resNames := make([]string, 0, len(resInfos))
	for _, resInfo := range resInfos {
		resNames = append(resNames, resInfo.ref.Name)
	}
	return fmt.Sprintf("scale:level-%d:%s", level, strings.Join(resNames, "#"))
}

// createScaleTaskFn creates a flow.TaskFn for a slice of DependentResourceInfo. If there are more than one
// DependentResourceInfo passed to this function, it indicates that they all are at the same level indicating that these functions
// should be invoked concurrently. In this case it will construct a flow.Parallel. If there is only one DependentResourceInfo passed
// then it indicates that at a specific level there is only one DependentResourceInfo that needs to be scaled.
func (ds *deploymentScaler) createScaleTaskFn(namespace string, resourceInfos []scaleableResourceInfo, mismatchReplicasCheckFn func(replicas, targetReplicas int32) bool, waitOnResourceInfos []scaleableResourceInfo) flow.TaskFn {
	taskFns := make([]flow.TaskFn, 0, len(resourceInfos))
	for _, resourceInfo := range resourceInfos {
		taskFn := ds.doCreateTaskFn(namespace, resourceInfo, mismatchReplicasCheckFn, waitOnResourceInfos)
		taskFns = append(taskFns, taskFn)
	}
	if len(taskFns) == 1 {
		return taskFns[0]
	}
	return flow.Parallel(taskFns...)
}

func (ds *deploymentScaler) doCreateTaskFn(namespace string, resInfo scaleableResourceInfo, mismatchReplicasCheckFn func(replicas, targetReplicas int32) bool, waitOnResourceInfos []scaleableResourceInfo) flow.TaskFn {
	return func(ctx context.Context) error {
		operation := fmt.Sprintf("scale-resource-%s.%s", namespace, resInfo.ref.Name)
		result := util.Retry(ctx,
			operation,
			func() (interface{}, error) {
				err := ds.scale(ctx, resInfo, mismatchReplicasCheckFn, waitOnResourceInfos)
				return nil, err
			},
			defaultMaxResourceScalingAttempts,
			defaultScaleResourceBackoff,
			util.AlwaysRetry)
		return result.Err
	}
}
func (ds *deploymentScaler) scale(ctx context.Context, resourceInfo scaleableResourceInfo, mismatchReplicas mismatchReplicasCheckFn, waitOnResourceInfos []scaleableResourceInfo) error {
	var err error
	ds.l.V(4).Info("Attempting to scale resource", "resourceInfo", resourceInfo)
	// sleep for initial delay
	err = util.SleepWithContext(ctx, resourceInfo.initialDelay)
	if err != nil {
		ds.l.Error(err, "Looks like the context has been cancelled. exiting scaling operation", "resourceInfo", resourceInfo)
		return err
	}

	gr, scaleRes, err := util.GetScaleResource(ctx, ds.client, ds.scaler, ds.l, resourceInfo.ref, resourceInfo.timeout)
	if err != nil {
		if !resourceInfo.shouldExist && apierrors.IsNotFound(err) {
			ds.l.V(4).Info("Skipping scaling of resource as it is not found", "resourceInfo", resourceInfo)
			return nil
		}
		ds.l.Error(err, "Scaling operation skipped due to error in getting deployment", "resourceInfo", resourceInfo)
		return err
	}

	targetReplicas, err := ds.determineTargetReplicas(scaleRes.ObjectMeta, resourceInfo.operation)
	if err != nil {
		return err
	}
	resourceInfo.replicas = targetReplicas
	if ds.shouldScale(ctx, scaleRes, resourceInfo.replicas, mismatchReplicas, waitOnResourceInfos) {
		if _, err = ds.doScale(ctx, gr, scaleRes, resourceInfo); err == nil {
			ds.l.V(4).Info("Resource has been scaled", "resInfo", resourceInfo)
		}
	}
	return err
}

func (ds *deploymentScaler) determineTargetReplicas(objMeta metav1.ObjectMeta, scaleOp operation) (int32, error) {
	if scaleOp == scaleDown {
		return DefaultScaleDownReplicas, nil
	}
	if replicas, ok := objMeta.Annotations[replicasAnnotationKey]; ok {
		r, err := strconv.Atoi(replicas)
		if err != nil {
			return 0, fmt.Errorf("unexpected and invalid replicas set as value for annotation: %s, %w", replicasAnnotationKey, err)
		}
		return int32(r), nil
	}
	ds.l.V(3).Info("replicas annotation not present on resource, falling back to default scale-up replicas.", "operation", scaleOp, "namespace", objMeta.Namespace, "name", objMeta.Name, "annotationKey", replicasAnnotationKey, "default-replicas", DefaultScaleUpReplicas)
	return DefaultScaleUpReplicas, nil
}

func (ds *deploymentScaler) shouldScale(ctx context.Context, scaleRes *autoscalingv1.Scale, targetReplicas int32, mismatchReplicas mismatchReplicasCheckFn, waitOnResourceInfos []scaleableResourceInfo) bool {
	if isIgnoreScalingAnnotationSet(scaleRes.ObjectMeta) {
		ds.l.V(4).Info("Scaling ignored due to explicit instruction via annotation", "namespace", ds.namespace, "deploymentName", scaleRes.Name, "annotation", ignoreScalingAnnotationKey)
		return false
	}
	// check the current replicas and compare it against the desired replicas
	if !mismatchReplicas(scaleRes.Spec.Replicas, targetReplicas) {
		ds.l.V(4).Info("Spec replicas matches the target replicas. scaling for this resource is skipped", "namespace", ds.namespace, "deploymentName", scaleRes.Name, "deploymentSpecReplicas", scaleRes.Spec.Replicas, "targetReplicas", targetReplicas)
		return false
	}
	// check if all resources this resource should wait on have been scaled, if not then we cannot scale this resource.
	// Check for currently available replicas and not the desired replicas on the upstream resource dependencies.
	if len(waitOnResourceInfos) > 0 {
		areUpstreamResourcesScaled := ds.waitUntilUpstreamResourcesAreScaled(ctx, waitOnResourceInfos, mismatchReplicas)
		if !areUpstreamResourcesScaled {
			ds.l.V(4).Info("Upstream resources are not scaled. scaling for this resource is skipped", "namespace", ds.namespace, "deploymentName", scaleRes.Name)
		}
		return areUpstreamResourcesScaled
	}
	return true
}

func (ds *deploymentScaler) resourceMatchDesiredReplicas(ctx context.Context, resInfo scaleableResourceInfo, mismatchReplicas mismatchReplicasCheckFn) bool {
	_, scaleRes, err := util.GetScaleResource(ctx, ds.client, ds.scaler, ds.l, resInfo.ref, resInfo.timeout)
	if err != nil {
		if apierrors.IsNotFound(err) && !resInfo.shouldExist {
			ds.l.V(4).Info("Upstream resource not found. Ignoring this resource as its existence is marked as optional", "resource", resInfo.ref)
			return true
		}
		ds.l.Error(err, "Error trying to get Deployment for resource", "resource", resInfo.ref)
		return false
	}
	if !isIgnoreScalingAnnotationSet(scaleRes.ObjectMeta) {
		actualReplicas := scaleRes.Status.Replicas
		if !mismatchReplicas(actualReplicas, resInfo.replicas) {
			ds.l.V(4).Info("Upstream resource has been scaled to desired replicas", "namespace", ds.namespace, "name", scaleRes.Name, "resInfo", resInfo, "replicas", actualReplicas)
			return true
		} else {
			ds.l.V(5).Info("Upstream resource has not been scaled to desired replicas", "namespace", ds.namespace, "name", scaleRes.Name, "resInfo", resInfo, "actualReplicas", actualReplicas)
			return false
		}
	} else {
		ds.l.V(5).Info("Ignoring upstream resource due to explicit instruction via annotation", "namespace", ds.namespace, "name", scaleRes.Name, "resInfo", resInfo, "annotation", ignoreScalingAnnotationKey)
	}
	return true
}

func (ds *deploymentScaler) waitUntilUpstreamResourcesAreScaled(ctx context.Context, upstreamResInfos []scaleableResourceInfo, mismatchReplicas mismatchReplicasCheckFn) bool {
	var wg sync.WaitGroup
	wg.Add(len(upstreamResInfos))
	resultC := make(chan bool, len(upstreamResInfos))
	for _, resInfo := range upstreamResInfos {
		resInfo := resInfo
		go func() {
			defer wg.Done()
			operation := fmt.Sprintf("wait for upstream resource: %s in namespace %s to reach replicas %d", resInfo.ref.Name, ds.namespace, resInfo.replicas)
			resultC <- util.RetryUntilPredicate(ctx, operation, func() bool { return ds.resourceMatchDesiredReplicas(ctx, resInfo, mismatchReplicas) }, *ds.options.dependentResourceCheckTimeout, *ds.options.dependentResourceCheckInterval)
		}()
	}
	go func() {
		defer close(resultC)
		wg.Wait()
	}()
	result := true
	for r := range resultC {
		result = result && r
	}
	return result
}

func (ds *deploymentScaler) doScale(ctx context.Context, gr *schema.GroupResource, scaleRes *autoscalingv1.Scale, resourceInfo scaleableResourceInfo) (*autoscalingv1.Scale, error) {
	childCtx, cancelFn := context.WithTimeout(ctx, resourceInfo.timeout)
	defer cancelFn()

	scaleRes.Spec.Replicas = resourceInfo.replicas
	if resourceInfo.operation == scaleUp {
		delete(scaleRes.Annotations, replicasAnnotationKey)
	} else {
		metav1.SetMetaDataAnnotation(&scaleRes.ObjectMeta, replicasAnnotationKey, string(scaleRes.Spec.Replicas))
	}
	ds.l.V(5).Info("updating kubernetes scalable resource", "objectKey", client.ObjectKeyFromObject(scaleRes), "replicas", scaleRes.Spec.Replicas, "annotations", scaleRes.Annotations)
	return ds.scaler.Update(childCtx, *gr, scaleRes, metav1.UpdateOptions{})
}

func collectResourceInfosByLevel(resourceInfos []scaleableResourceInfo) map[int][]scaleableResourceInfo {
	resInfosByLevel := make(map[int][]scaleableResourceInfo)
	for _, resInfo := range resourceInfos {
		level := resInfo.level
		if _, ok := resInfosByLevel[level]; !ok {
			var levelResInfos []scaleableResourceInfo
			levelResInfos = append(levelResInfos, resInfo)
			resInfosByLevel[level] = levelResInfos
		} else {
			resInfosByLevel[level] = append(resInfosByLevel[level], resInfo)
		}
	}
	return resInfosByLevel
}

func sortAndGetUniqueLevels(resourceInfos []scaleableResourceInfo) []int {
	var levels []int
	keys := make(map[int]bool)
	for _, resInfo := range resourceInfos {
		if _, found := keys[resInfo.level]; !found {
			keys[resInfo.level] = true
			levels = append(levels, resInfo.level)
		}
	}
	sort.Ints(levels)
	return levels
}

func createScaleUpResourceInfos(dependentResourceInfos []papi.DependentResourceInfo) []scaleableResourceInfo {
	resourceInfos := make([]scaleableResourceInfo, 0, len(dependentResourceInfos))
	for _, depResInfo := range dependentResourceInfos {
		resInfo := scaleableResourceInfo{
			ref:          depResInfo.Ref,
			shouldExist:  *depResInfo.ShouldExist,
			level:        depResInfo.ScaleUpInfo.Level,
			initialDelay: depResInfo.ScaleUpInfo.InitialDelay.Duration,
			timeout:      depResInfo.ScaleUpInfo.Timeout.Duration,
			operation:    scaleUp,
		}
		resourceInfos = append(resourceInfos, resInfo)
	}
	return resourceInfos
}

func createScaleDownResourceInfos(dependentResourceInfos []papi.DependentResourceInfo) []scaleableResourceInfo {
	resourceInfos := make([]scaleableResourceInfo, 0, len(dependentResourceInfos))
	for _, depResInfo := range dependentResourceInfos {
		resInfo := scaleableResourceInfo{
			ref:          depResInfo.Ref,
			shouldExist:  *depResInfo.ShouldExist,
			level:        depResInfo.ScaleDownInfo.Level,
			initialDelay: depResInfo.ScaleDownInfo.InitialDelay.Duration,
			timeout:      depResInfo.ScaleDownInfo.Timeout.Duration,
			operation:    scaleDown,
		}
		resourceInfos = append(resourceInfos, resInfo)
	}
	return resourceInfos
}
