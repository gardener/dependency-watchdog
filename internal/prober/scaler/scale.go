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
	"strconv"

	"github.com/go-logr/logr"

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
	// ignoreScalingAnnotationKey is the key for an annotation if present on a resource will suspend any scaling action for that resource.
	ignoreScalingAnnotationKey = "dependency-watchdog.gardener.cloud/ignore-scaling"
	// replicasAnnotationKey is the key for an annotation whose value captures the current spec.replicas prior to scale down for that resource.
	// This is used when DWD attempts to restore the state of the resource it scale down.
	replicasAnnotationKey = "dependency-watchdog.gardener.cloud/replicas"
	// defaultScaleUpReplicas is the default value of number of replicas for a scale-up operation by a probe when the external probe transitions from failed to success.
	defaultScaleUpReplicas int32 = 1
	// defaultScaleDownReplicas is the default value of number of replicas for a scale-down operation by a probe when the external probe transitions from success to failed.
	defaultScaleDownReplicas int32 = 0
)

type resourceScaler interface {
	scale(ctx context.Context) error
}

type resScaler struct {
	client       client.Client
	scaler       scalev1.ScaleInterface
	logger       logr.Logger
	namespace    string
	resourceInfo scalableResourceInfo
	opts         *scalerOptions
}

func newResourceScaler(client client.Client, scaler scalev1.ScaleInterface, logger logr.Logger, opts *scalerOptions, namespace string, resourceInfo scalableResourceInfo) resourceScaler {
	return &resScaler{
		client:       client,
		scaler:       scaler,
		logger:       logger,
		namespace:    namespace,
		resourceInfo: resourceInfo,
		opts:         opts,
	}
}

func (r *resScaler) scale(ctx context.Context) error {
	var (
		err           error
		resourceAnnot map[string]string
	)
	r.logger.V(4).Info("Attempting to scale resource", "resourceInfo", r.resourceInfo)
	// sleep for initial delay
	if err = util.SleepWithContext(ctx, r.resourceInfo.initialDelay); err != nil {
		r.logger.Error(err, "Looks like the context has been cancelled. exiting scaling operation", "resourceInfo", r.resourceInfo)
		return err
	}

	if resourceAnnot, err = util.GetResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref); err != nil {
		if apierrors.IsNotFound(err) && !r.resourceInfo.shouldExist {
			r.logger.V(4).Info("Resource not found. Ignoring this resource as its existence is marked as optional", "namespace", r.namespace, "resource", r.resourceInfo.ref)
			return nil
		}
		r.logger.Error(err, "Error trying to get annotations for resource", "namespace", r.namespace, "resource", r.resourceInfo.ref)
		return err
	}

	if ignoreScaling(resourceAnnot) {
		r.logger.V(4).Info("Scaling ignored due to explicit instruction via annotation", "namespace", r.namespace, "name", r.resourceInfo.ref.Name, "annotation", ignoreScalingAnnotationKey)
		return nil
	}

	gr, scaleSubRes, err := util.GetScaleResource(ctx, r.client, r.scaler, r.logger, r.resourceInfo.ref, r.resourceInfo.timeout)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Error(err, "Resource does not have a scale subresource. Scaling of this resource is not possible and scaling of all downstream resources will be skipped. Please check and correct DWD configuration", "namespace", r.namespace, "resource", r.resourceInfo.ref)
		}
		return err
	}

	if r.resourceInfo.operation.shouldScaleReplicas(scaleSubRes.Spec.Replicas) {
		if err := r.updateResourceAndScale(ctx, gr, scaleSubRes, resourceAnnot); err != nil {
			return err
		}
	} else {
		r.logger.V(4).Info("Skipping scaling for resource. This can happen if the current spec.Replicas > 0 for scaleUp or current spec.Replicas == 0 for scaleDown", "namespace", r.namespace, "name", r.resourceInfo.ref.Name, "operation", r.resourceInfo.operation)
	}

	return r.waitTillMinTargetReplicasReached(ctx)
}

func (r *resScaler) waitTillMinTargetReplicasReached(ctx context.Context) error {
	var minTargetReplicas int32
	if r.resourceInfo.operation == scaleUp {
		minTargetReplicas = 1
	}
	opDesc := fmt.Sprintf("wait for resource: %s in namespace %s to reach minimum required target replicas %d", r.resourceInfo.ref.Name, r.namespace, minTargetReplicas)
	resMinTargetReached := util.RetryUntilPredicate(ctx, opDesc, func() bool {
		readyReplicas, err := util.GetResourceReadyReplicas(ctx, r.client, r.namespace, r.resourceInfo.ref)
		if err != nil {
			return false
		}
		if r.resourceInfo.operation.minTargetReplicasReached(readyReplicas) {
			r.logger.V(4).Info("Resource has been scaled to desired replicas", "namespace", r.namespace, "name", r.resourceInfo.ref.Name, "minTargetReplicas", minTargetReplicas)
			return true
		}
		return false
	}, *r.opts.resourceCheckTimeout, *r.opts.resourceCheckInterval)
	if !resMinTargetReached {
		return fmt.Errorf("timed out waiting for {namespace: %s, resource: %s} to reach minTargetReplicas %d", r.namespace, r.resourceInfo.ref.Name, minTargetReplicas)
	}
	return nil
}

func (r *resScaler) updateResourceAndScale(ctx context.Context, gr *schema.GroupResource, scaleSubRes *autoscalingv1.Scale, annot map[string]string) error {
	childCtx, cancelFn := context.WithTimeout(ctx, r.resourceInfo.timeout)
	defer cancelFn()

	// update the annotation capturing the current spec.replicas as the annotation value if the operation is scale down.
	// This allows restoration of the resource to the same replica count when a subsequent scale up operation is triggered.
	if r.resourceInfo.operation == scaleDown {
		patchBytes := []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"%s\":\"%s\"}}}", replicasAnnotationKey, strconv.Itoa(int(scaleSubRes.Spec.Replicas))))
		err := util.PatchResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref, patchBytes)
		if err != nil {
			r.logger.Error(err, "Failed to update annotation to capture the current replicas before scaling it down", "namespace", r.namespace, "objectKey", client.ObjectKeyFromObject(scaleSubRes))
			return err
		}
	}

	targetReplicas, err := r.determineTargetReplicas(r.resourceInfo.ref.Name, annot)
	if err != nil {
		return err
	}

	scaleSubRes.Spec.Replicas = targetReplicas
	r.logger.V(5).Info("Scaling kubernetes resource", "namespace", r.namespace, "objectKey", client.ObjectKeyFromObject(scaleSubRes), "targetReplicas", targetReplicas)
	if _, err = r.scaler.Update(childCtx, *gr, scaleSubRes, metav1.UpdateOptions{}); err != nil {
		return err
	}
	r.logger.V(4).Info("Resource scaling has been triggered successfully, waiting for resource scaling to complete", "namespace", r.namespace, "resource", r.resourceInfo.ref)

	return nil
}

func (r *resScaler) determineTargetReplicas(resourceName string, annotations map[string]string) (int32, error) {
	if r.resourceInfo.operation == scaleDown {
		return defaultScaleDownReplicas, nil
	}
	if replicasStr, ok := annotations[replicasAnnotationKey]; ok {
		replicas, err := strconv.Atoi(replicasStr)
		if err != nil {
			return 0, fmt.Errorf("unexpected and invalid replicasStr set as value for annotation: %s for resource: %v, %w", replicasAnnotationKey, types.NamespacedName{Namespace: r.namespace, Name: resourceName}, err)
		}
		return int32(replicas), nil
	}
	r.logger.Info("Replicas annotation not present on resource, falling back to default scale-up replicas", "operation", r.resourceInfo.operation, "namespace", r.namespace, "name", resourceName, "annotationKey", replicasAnnotationKey, "default-replicas", defaultScaleUpReplicas)
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
