package scaler

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/gardener/dependency-watchdog/internal/util"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	scalev1 "k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ignoreScalingAnnotationKey = "dependency-watchdog.gardener.cloud/ignore-scaling"
	replicasAnnotationKey      = "dependency-watchdog.gardener.cloud/replicas"
	// defaultScaleUpReplicas is the default value of number of replicas for a scale-up operation by a probe when the external probe transitions from failed to success.
	defaultScaleUpReplicas int32 = 1
	// defaultScaleDownReplicas is the default value of number of replicas for a scale-down operation by a probe when the external probe transitions from success to failed.
	defaultScaleDownReplicas int32 = 0
)

type resourceScaler interface {
	scale(ctx context.Context) error
}

type resScaler struct {
	client              client.Client
	scaler              scalev1.ScaleInterface
	namespace           string
	resourceInfo        scalableResourceInfo
	waitOnResourceInfos []scalableResourceInfo
	opts                *scalerOptions
}

func newResourceScaler(client client.Client, scaler scalev1.ScaleInterface, opts *scalerOptions, namespace string, resourceInfo scalableResourceInfo, waitOnResourceInfos []scalableResourceInfo) resourceScaler {
	return &resScaler{
		client:              client,
		scaler:              scaler,
		namespace:           namespace,
		resourceInfo:        resourceInfo,
		waitOnResourceInfos: waitOnResourceInfos,
		opts:                opts,
	}
}

func (r *resScaler) scale(ctx context.Context) error {
	var (
		err            error
		resourceAnnot  map[string]string
		targetReplicas int32
	)

	logger.V(4).Info("Attempting to scale resource", "resourceInfo", r.resourceInfo)
	// sleep for initial delay
	if err = util.SleepWithContext(ctx, r.resourceInfo.initialDelay); err != nil {
		logger.Error(err, "looks like the context has been cancelled. exiting scaling operation", "resourceInfo", r.resourceInfo)
		return err
	}
	if resourceAnnot, err = util.GetResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref); err != nil {
		return err
	}
	if targetReplicas, err = r.determineTargetReplicas(r.resourceInfo.ref.Name, resourceAnnot); err != nil {
		return err
	}
	gr, scaleSubRes, err := util.GetScaleResource(ctx, r.client, r.scaler, logger, r.resourceInfo.ref, r.resourceInfo.timeout)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "resource does not have a scale subresource. skipping scaling of this resource.", "namespace", r.namespace, "resource", r.resourceInfo.ref)
		}
		return err
	}

	if r.shouldScale(ctx, resourceAnnot, scaleSubRes.Spec.Replicas, targetReplicas) {
		if _, err = r.updateResourceAndScale(ctx, gr, scaleSubRes, targetReplicas); err == nil {
			logger.V(4).Info("Resource has been scaled", "namespace", r.namespace, "resource", r.resourceInfo.ref)
		}
	}
	return err
}

func (r *resScaler) shouldScale(ctx context.Context, resourceAnnot map[string]string, currentReplicas, targetReplicas int32) bool {
	if ignoreScaling(resourceAnnot) {
		logger.V(4).Info("scaling ignored due to explicit instruction via annotation", "namespace", r.namespace, "name", r.resourceInfo.ref.Name, "annotation", ignoreScalingAnnotationKey)
		return false
	}

	// check the current replicas to decide if scaling is needed
	if r.resourceInfo.operation.replicasCheckPredicate(currentReplicas) {
		logger.V(4).Info("Spec replicas matches the target replicas. scaling for this resource is skipped", "namespace", r.namespace, "name", r.resourceInfo.ref.Name, "currentReplicas", currentReplicas, "targetReplicas", targetReplicas)
		return false
	}

	// check if all resources this resource should wait on have been scaled, if not then we cannot scale this resource.
	// Check for currently available replicas and not the desired replicas on the upstream resource dependencies.
	if len(r.waitOnResourceInfos) > 0 {
		areUpstreamResourcesScaled := r.waitUntilUpstreamResourcesAreScaled(ctx)
		if !areUpstreamResourcesScaled {
			logger.V(4).Info("skipping scaling of resource. waiting for upstream resources are not scaled first", "namespace", r.namespace, "resource", r.resourceInfo.ref)
		}
		return areUpstreamResourcesScaled
	}
	return true
}

func (r *resScaler) waitUntilUpstreamResourcesAreScaled(ctx context.Context) bool {
	var wg sync.WaitGroup
	wg.Add(len(r.waitOnResourceInfos))
	resultC := make(chan bool, len(r.waitOnResourceInfos))
	for _, resInfo := range r.waitOnResourceInfos {
		resInfo := resInfo
		go func() {
			defer wg.Done()
			// get the target replicas for the upstream resource info and pass it to resourceMatchDesiredReplicas
			operation := fmt.Sprintf("wait for upstream resource: %s in namespace %s to reach target replicas", resInfo.ref.Name, r.namespace)
			resultC <- util.RetryUntilPredicate(ctx, operation, func() bool { return r.resourceMatchDesiredReplicas(ctx, resInfo) }, *r.opts.dependentResourceCheckTimeout, *r.opts.dependentResourceCheckInterval)
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

func (r *resScaler) resourceMatchDesiredReplicas(ctx context.Context, resourceInfo scalableResourceInfo) bool {
	resAnnot, readyReplicas, err := util.GetAnnotationsAndReadyReplicasForResource(ctx, r.client, r.namespace, resourceInfo.ref)
	if err != nil {
		if apierrors.IsNotFound(err) && !resourceInfo.shouldExist {
			logger.V(4).Info("upstream resource not found. Ignoring this resource as its existence is marked as optional", "namespace", r.namespace, "resource", resourceInfo.ref)
			return true
		}
		logger.Error(err, "error trying to get annotations and ready replicas for resource", "namespace", r.namespace, "resource", resourceInfo.ref)
		return false
	}

	if ignoreScaling(resAnnot) {
		logger.V(5).Info("Ignoring upstream resource due to explicit instruction via annotation", "namespace", r.namespace, "name", resourceInfo.ref.Name, "annotation", ignoreScalingAnnotationKey)
		return true
	}

	targetReplicas, err := r.determineTargetReplicas(resourceInfo.ref.Name, resAnnot)
	if err != nil {
		logger.Error(err, "(resourceMatchDesiredReplicas) error trying to determine target replicas for resource", "namespace", r.namespace, "resource", resourceInfo.ref)
		return false
	}
	if resourceInfo.operation.scalingCompletePredicate(readyReplicas, targetReplicas) {
		logger.V(4).Info("upstream resource has been scaled to desired replicas", "namespace", r.namespace, "name", resourceInfo.ref.Name, "targetReplicas", targetReplicas)
		return true
	}
	logger.V(5).Info("upstream resource has not yet been scaled to desired replicas", "namespace", r.namespace, "name", resourceInfo.ref.Name, "actualReplicas", readyReplicas, "targetReplicas", targetReplicas)
	return false
}

func (r *resScaler) updateResourceAndScale(ctx context.Context, gr *schema.GroupResource, scaleSubRes *autoscalingv1.Scale, targetReplicas int32) (*autoscalingv1.Scale, error) {
	childCtx, cancelFn := context.WithTimeout(ctx, r.resourceInfo.timeout)
	defer cancelFn()

	// update the annotation capturing the current spec.replicas as the annotation value if the operation is scale down.
	// This allows restoration of the resource to the same replica count when a subsequent scale up operation is triggered.
	if r.resourceInfo.operation.opType == scaleDown {
		patchBytes := []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"%s\":\"%s\"}}}", replicasAnnotationKey, strconv.Itoa(int(scaleSubRes.Spec.Replicas))))
		err := util.PatchResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref, patchBytes)
		if err != nil {
			logger.Error(err, "failed to update annotation to capture the current replicas before scaling it down", "namespace", r.namespace, "objectKey", client.ObjectKeyFromObject(scaleSubRes))
			return nil, err
		}
	}

	scaleSubRes.Spec.Replicas = targetReplicas
	logger.V(5).Info("scaling kubernetes resource", "namespace", r.namespace, "objectKey", client.ObjectKeyFromObject(scaleSubRes), "targetReplicas", targetReplicas)
	return r.scaler.Update(childCtx, *gr, scaleSubRes, metav1.UpdateOptions{})
}

func (r *resScaler) determineTargetReplicas(resourceName string, annotations map[string]string) (int32, error) {
	if r.resourceInfo.operation.opType == scaleDown {
		return defaultScaleDownReplicas, nil
	}
	if replicasStr, ok := annotations[replicasAnnotationKey]; ok {
		replicas, err := strconv.Atoi(replicasStr)
		if err != nil {
			return 0, fmt.Errorf("unexpected and invalid replicasStr set as value for annotation: %s for resource: %v, %w", replicasAnnotationKey, types.NamespacedName{Namespace: r.namespace, Name: resourceName}, err)
		}
		return int32(replicas), nil
	}
	logger.Info("replicasStr annotation not present on resource, falling back to default scale-up replicasStr.", "operation", r.resourceInfo.operation.opType, "namespace", r.namespace, "name", resourceName, "annotationKey", replicasAnnotationKey, "default-replicasStr", defaultScaleUpReplicas)
	return defaultScaleUpReplicas, nil
}

func ignoreScaling(annotations map[string]string) bool {
	if val, ok := annotations[ignoreScalingAnnotationKey]; ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return false
		}
		return b
	}
	return false
}
