// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"context"
	"fmt"
	"strconv"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	"github.com/gardener/dependency-watchdog/internal/util"
	"github.com/go-logr/logr"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	resLogger := logger.WithValues("resNamespace", namespace, "kind", resourceInfo.ref.Kind, "apiVersion", resourceInfo.ref.APIVersion, "name", resourceInfo.ref.Name, "level", resourceInfo.level)
	return &resScaler{
		client:       client,
		scaler:       scaler,
		logger:       resLogger,
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
	// sleep for initial delay
	if err = util.SleepWithContext(ctx, r.resourceInfo.initialDelay); err != nil {
		r.logger.Error(err, "Looks like the context has been cancelled. exiting scaling operation")
		return err
	}

	if resourceAnnot, err = util.GetResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref); err != nil {
		if apierrors.IsNotFound(err) && r.resourceInfo.optional {
			r.logger.Info("Resource not found. Ignoring this resource as its existence is marked as optional")
			return nil
		}
		r.logger.Error(err, "Error trying to get annotations for resource")
		return err
	}

	if ignoreScaling(resourceAnnot) {
		r.logger.Info("Scaling ignored due to explicit instruction via an annotation", "resource", r.resourceInfo.ref.Name, "annotation", ignoreScalingAnnotationKey)
		// check if the meltdown protection annotation is set to true, if so then remove it to ensure that the resource is now manually managed by operator.
		if hasMeltdownProtectionEnabled(resourceAnnot) {
			r.logger.Info("Found leftover meltdown protection annotation, to be removed as scaling is ignored for resource and is not managed by DWD anymore")
			if err = r.removeMeltdownAnnotations(ctx); err != nil {
				r.logger.Error(err, "Failed to remove meltdown protection annotations after ignoring scaling was enabled")
				return err
			}
		}
		return nil
	}

	_, scaleSubRes, err := util.GetScaleResource(ctx, r.client, r.scaler, r.logger, r.resourceInfo.ref, r.resourceInfo.timeout)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Error(err, "Resource does not have a scale subresource. Skipping scaling of dependent resources. Invalid config file")
		}
		return err
	}
	if r.resourceInfo.operation.shouldScaleReplicas(scaleSubRes.Spec.Replicas) {
		if err := r.updateResourceAndScale(ctx, scaleSubRes, resourceAnnot); err != nil {
			return err
		}
	} else {
		if r.resourceInfo.operation == scaleUp {
			r.logger.Info("Skipping scale-up for resource as current spec replicas > 0")
			// It is possible that the meltdown protection annotation was not removed during the last scale up operation.
			// In that case, check if the annotation is present and remove the meltdown protection annotation.
			if hasMeltdownProtectionEnabled(resourceAnnot) {
				r.logger.Info("Removing leftover meltdown protection annotation from last scale up operation")
				if err = r.removeMeltdownAnnotations(ctx); err != nil {
					r.logger.Error(err, "Failed to remove meltdown protection annotations after ignoring scaling was enabled")
					return err
				}
			}
		} else {
			r.logger.Info("Skipping scale-down for resource as current spec replicas == 0")
		}
	}

	return r.waitTillMinTargetReplicasReached(ctx)
}

func (r *resScaler) waitTillMinTargetReplicasReached(ctx context.Context) error {
	var minTargetReplicas int32
	if r.resourceInfo.operation == scaleUp {
		minTargetReplicas = 1
	}
	r.logger.Info("Waiting for resource to reach minimum target replicas", "minTargetReplicas", minTargetReplicas)
	opDesc := fmt.Sprintf("wait for resource to reach minimum required target replicas %d", minTargetReplicas)
	resMinTargetReached := util.RetryUntilPredicate(ctx, r.logger, opDesc, func() bool {
		readyReplicas, err := util.GetResourceReadyReplicas(ctx, r.client, r.namespace, r.resourceInfo.ref)
		if err != nil {
			return false
		}
		if r.resourceInfo.operation.minTargetReplicasReached(readyReplicas) {
			r.logger.Info("Resource has reached desired replicas", "minTargetReplicas", minTargetReplicas)
			return true
		}
		return false
	}, *r.opts.resourceCheckTimeout, *r.opts.resourceCheckInterval)
	if !resMinTargetReached {
		return fmt.Errorf("timed out waiting for {namespace: %s, resource: %s} to reach minTargetReplicas %d", r.namespace, r.resourceInfo.ref.Name, minTargetReplicas)
	}
	return nil
}

func (r *resScaler) updateResourceAndScale(ctx context.Context, scaleSubRes *autoscalingv1.Scale, annot map[string]string) error {
	childCtx, cancelFn := context.WithTimeout(ctx, r.resourceInfo.timeout)
	defer cancelFn()

	// update the annotation capturing the current spec.replicas as the annotation value if the operation is scale down.
	// This allows restoration of the resource to the same replica count when a subsequent scale up operation is triggered.
	if r.resourceInfo.operation == scaleDown {
		patchBytes := []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"%s\":\"%s\", \"%s\":\"%s\"}}}", replicasAnnotationKey, strconv.Itoa(int(scaleSubRes.Spec.Replicas)), papi.MeltdownProtectionActive, ""))
		err := util.PatchResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref, patchBytes)
		if err != nil {
			r.logger.Error(err, "Failed to update annotation to capture the current replicas before scaling it down")
			return err
		}
	}

	targetReplicas, err := r.determineTargetReplicas(annot)
	if err != nil {
		return err
	}

	// need the updated scale subresource
	gr, scaleSubRes, err := util.GetScaleResource(ctx, r.client, r.scaler, r.logger, r.resourceInfo.ref, r.resourceInfo.timeout)
	if err != nil {
		return err
	}

	scaleSubRes.Spec.Replicas = targetReplicas
	r.logger.Info("Scaling kubernetes resource", "operation", r.resourceInfo.operation, "targetReplicas", targetReplicas)
	if _, err = r.scaler.Update(childCtx, *gr, scaleSubRes, metav1.UpdateOptions{}); err != nil {
		return err
	}
	if r.resourceInfo.operation == scaleUp {
		if err := r.removeMeltdownAnnotations(ctx); err != nil {
			r.logger.Error(err, "Failed to remove meltdown protection annotations after scaling up")
			return err
		}
	}
	return nil
}

// remove meltdown protection annotation to ensure that the resource is not ignore anymore during shoot reconciliation
func (r *resScaler) removeMeltdownAnnotations(ctx context.Context) error {
	patchBytes := []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"%s\":null}}}", papi.MeltdownProtectionActive))
	if err := util.PatchResourceAnnotations(ctx, r.client, r.namespace, r.resourceInfo.ref, patchBytes); err != nil {
		r.logger.Error(err, "Failed to remove annotations after scaling operation")
		return err
	}
	r.logger.Info("Removed annotations", "annotationKeys", []string{replicasAnnotationKey, papi.MeltdownProtectionActive})
	return nil
}

func (r *resScaler) determineTargetReplicas(annotations map[string]string) (int32, error) {
	if r.resourceInfo.operation == scaleDown {
		return defaultScaleDownReplicas, nil
	}
	if replicasStr, ok := annotations[replicasAnnotationKey]; ok {
		replicas, err := strconv.Atoi(replicasStr) // #nosec G109 -- replicas will not exceed MaxInt32
		if err != nil {
			return 0, fmt.Errorf("unexpected and invalid replicasStr set as value for annotation: %s for resource, Err: %w", replicasAnnotationKey, err)
		}
		return int32(replicas), nil // #nosec G109 G115 -- number of replicas will not exceed MaxInt32
	}
	r.logger.Info("Replicas annotation not found, falling back to default scale-up replicas", "operation", r.resourceInfo.operation, "annotationKey", replicasAnnotationKey, "default-replicas", defaultScaleUpReplicas)
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

func hasMeltdownProtectionEnabled(annotations map[string]string) bool {
	if _, ok := annotations[papi.MeltdownProtectionActive]; ok {
		return true
	}
	return false
}
