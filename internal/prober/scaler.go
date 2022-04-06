package prober

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/gardener/gardener/pkg/utils/flow"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scalev1 "k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ignoreScalingAnnotationKey        = "dependency-watchdog.gardener.cloud/ignore-scaling"
	defaultMaxResourceScalingAttempts = 3
	defaultScaleResourceBackoff       = 100 * time.Millisecond
)

type DeploymentScaler interface {
	ScaleUp(ctx context.Context) error
	ScaleDown(ctx context.Context) error
}

func NewDeploymentScaler(namespace string, config *Config, client client.Client, scalerGetter scalev1.ScalesGetter) DeploymentScaler {
	ds := deploymentScaler{
		namespace: namespace,
		scaler:    scalerGetter.Scales(namespace),
		client:    client,
	}
	ds.scaleDownFlow = ds.createResourceScaleFlow(namespace, fmt.Sprintf("scale-down-%s", namespace), config.ScaleDownResourceInfos, util.ScaleDownReplicasMismatch)
	ds.scaleUpFlow = ds.createResourceScaleFlow(namespace, fmt.Sprintf("scale-up-%s", namespace), config.ScaleUpResourceInfos, util.ScaleUpReplicasMismatch)
	return &ds
}

type mismatchReplicasCheckFn func(replicas, targetReplicas int32) bool

type deploymentScaler struct {
	namespace     string
	scaler        scalev1.ScaleInterface
	client        client.Client
	scaleDownFlow *flow.Flow
	scaleUpFlow   *flow.Flow
}

func (ds *deploymentScaler) ScaleDown(ctx context.Context) error {
	return ds.scaleDownFlow.Run(ctx, flow.Opts{})
}

func (ds *deploymentScaler) ScaleUp(ctx context.Context) error {
	return ds.scaleUpFlow.Run(ctx, flow.Opts{})
}

func (ds *deploymentScaler) doScale(ctx context.Context, resourceInfo ResourceInfo, mismatchReplicas mismatchReplicasCheckFn, waitOnResourceInfos []ResourceInfo) error {
	deployment, err := util.GetDeploymentFor(ctx, ds.namespace, resourceInfo, ds.client)
	if err != nil {
		logger.Error(err, "error getting deployment for resource, skipping scaling operation", "namespace", ds.namespace, "resourceInfo", resourceInfo)
		return err
	}
	// sleep for initial delay
	err = util.SleepWithContext(ctx, *resourceInfo.InitialDelay)
	if err != nil {
		logger.Error(err, "looks like the context has been cancelled. exiting scaling operation", "namespace", ds.namespace, "resourceInfo", resourceInfo)
		return err
	}
	if ds.shouldScale(ctx, deployment, *resourceInfo.Replicas, mismatchReplicas, waitOnResourceInfos) {
		util.Retry(ctx, fmt.Sprintf(""), func() (*autoscalingv1.Scale, error) {
			return ds.scaleResource(resourceInfo)
		}, defaultMaxResourceScalingAttempts, defaultScaleResourceBackoff, util.AlwaysRetry)
	}
	return nil
}

func (ds *deploymentScaler) scaleResource(resourceInfo ResourceInfo) (*autoscalingv1.Scale, error) {
	gr, err := ds.getGroupResource(resourceInfo.Ref)
	if err != nil {
		return nil, err
	}
	scale, err := ds.scaler.Get(gr, resourceInfo.Ref.Name)
	if err != nil {
		return nil, err
	}
	scale.Spec.Replicas = *resourceInfo.Replicas
	return ds.scaler.Update(gr, scale)
}

func (ds *deploymentScaler) getGroupResource(resourceRef autoscalingv1.CrossVersionObjectReference) (schema.GroupResource, error) {
	gv, _ := schema.ParseGroupVersion(resourceRef.APIVersion) // Ignoring the error as this validation has already been done when initially validating the Config
	gk := schema.GroupKind{
		Group: gv.Group,
		Kind:  resourceRef.Kind,
	}
	mapping, err := ds.client.RESTMapper().RESTMapping(gk, gv.Version)
	if err != nil {
		logger.Error(err, "failed to get RESTMapping for resource", "resourceRef", resourceRef)
		return schema.GroupResource{}, err
	}
	return mapping.Resource.GroupResource(), nil
}

func (ds *deploymentScaler) shouldScale(ctx context.Context, deployment *appsv1.Deployment, targetReplicas int32, mismatchReplicas mismatchReplicasCheckFn, waitOnResourceInfos []ResourceInfo) bool {
	if isIgnoreScalingAnnotationSet(deployment) {
		logger.V(4).Info("scaling ignored due to explicit instruction via annotation", "namespace", ds.namespace, "deploymentName", deployment.Name, "annotation", ignoreScalingAnnotationKey)
		return false
	}
	// check the current replicas and compare it against the desired replicas
	deploymentSpecReplicas := *deployment.Spec.Replicas
	if !mismatchReplicas(deploymentSpecReplicas, targetReplicas) {
		logger.V(4).Info("spec replicas matches the target replicas. scaling for this resource is skipped", "namespace", ds.namespace, "deploymentName", deployment.Name, "deploymentSpecReplicas", deploymentSpecReplicas, "targetReplicas", targetReplicas)
		return false
	}
	// check if all resources this resource should wait on have been scaled, if not then we cannot scale this resource.
	// Check for currently available replicas and not the desired replicas on the upstream resource dependencies.
	if waitOnResourceInfos != nil {
		for _, upstreamDependentResource := range waitOnResourceInfos {
			upstreamDeployment, err := util.GetDeploymentFor(ctx, ds.namespace, upstreamDependentResource, ds.client)
			if err != nil {
				logger.Error(err, "failed to get deployment for upstream dependent resource, skipping scaling", "upstreamDependentResource", upstreamDependentResource)
				return false
			}
			actualReplicas := upstreamDeployment.Status.Replicas
			if mismatchReplicas(actualReplicas, *upstreamDependentResource.Replicas) {
				logger.V(4).Info("upstream resource has still not been scaled to the desired replicas, skipping scaling of resource", "namespace", ds.namespace, "deploymentToScale", deployment.Name, "upstreamResourceInfo", upstreamDependentResource, "actualReplicas", actualReplicas)
				return false
			}
		}
	}
	return true
}

func isIgnoreScalingAnnotationSet(deployment *appsv1.Deployment) bool {
	if val, ok := deployment.Annotations[ignoreScalingAnnotationKey]; ok {
		return val == "true"
	}
	return false
}

func (ds *deploymentScaler) createResourceScaleFlow(namespace, flowName string, resourceInfos []ResourceInfo, mismatchReplicasCheckFn func(replicas, targetReplicas int32) bool) *flow.Flow {
	levels := sortAndGetUniqueLevels(resourceInfos)
	orderedResourceInfos := collectResourceInfosByLevel(resourceInfos)
	g := flow.NewGraph(flowName)
	var previousLevelResourceInfos []ResourceInfo
	for _, level := range levels {
		var previousTaskID flow.TaskID
		if resInfos, ok := orderedResourceInfos[level]; ok {
			taskID := g.Add(flow.Task{
				Name:         fmt.Sprintf("scaling dependencies %v at level %d", resInfos, level),
				Fn:           ds.createScaleTaskFn(namespace, resInfos, mismatchReplicasCheckFn, previousLevelResourceInfos),
				Dependencies: flow.NewTaskIDs(previousTaskID),
			})
			copy(previousLevelResourceInfos, resInfos)
			previousTaskID = taskID
		}
	}
	return g.Compile()
}

// createScaleTaskFn creates a flow.TaskFn for a slice of ResourceInfo. If there are more than one
// ResourceInfo passed to this function, it indicates that they all are at the same level indicating that these functions
// should be invoked concurrently. In this case it will construct a flow.Parallel. If there is only one ResourceInfo passed
// then it indicates that at a specific level there is only one ResourceInfo that needs to be scaled.
func (ds *deploymentScaler) createScaleTaskFn(namespace string, resourceInfos []ResourceInfo, mismatchReplicasCheckFn func(replicas, targetReplicas int32) bool, waitOnResourceInfos []ResourceInfo) flow.TaskFn {
	if len(resourceInfos) == 0 {
		logger.V(4).Info("(createScaleTaskFn) [unexpected] resourceInfos. This should never be the case.", "namespace", namespace)
		return nil
	}
	taskFns := make([]flow.TaskFn, len(resourceInfos))
	for _, resourceInfo := range resourceInfos {
		taskFn := flow.TaskFn(func(ctx context.Context) error {
			operation := fmt.Sprintf("scale-resource-%s.%s", namespace, resourceInfo.Ref.Name)
			result := util.Retry(ctx,
				operation,
				func() (interface{}, error) {
					err := ds.doScale(ctx, resourceInfo, mismatchReplicasCheckFn, waitOnResourceInfos)
					return nil, err
				},
				defaultMaxResourceScalingAttempts,
				defaultGetSecretBackoff,
				util.AlwaysRetry)
			logger.V(4).Info("resource has been scaled", "namespace", namespace, "resource", resourceInfo)
			return result.Err
		})
		taskFns = append(taskFns, taskFn)
	}
	if len(resourceInfos) == 1 {
		return taskFns[0]
	}
	return flow.Parallel(taskFns...)
}

func collectResourceInfosByLevel(resourceInfos []ResourceInfo) map[int][]ResourceInfo {
	resInfosByLevel := make(map[int][]ResourceInfo)
	for _, resInfo := range resourceInfos {
		level := resInfo.Level
		if _, ok := resInfosByLevel[level]; !ok {
			var levelResInfos []ResourceInfo
			levelResInfos = append(levelResInfos, resInfo)
			resInfosByLevel[level] = levelResInfos
		} else {
			resInfosByLevel[level] = append(resInfosByLevel[level], resInfo)
		}
	}
	return resInfosByLevel
}

func sortAndGetUniqueLevels(resourceInfos []ResourceInfo) []int {
	var levels []int
	keys := make(map[int]bool)
	for _, resInfo := range resourceInfos {
		if _, found := keys[resInfo.Level]; !found {
			keys[resInfo.Level] = true
			levels = append(levels, resInfo.Level)
		}
	}
	sort.Ints(levels)
	return levels
}
