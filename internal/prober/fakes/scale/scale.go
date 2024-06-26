package scale

import (
	"context"
	"github.com/gardener/dependency-watchdog/internal/prober/scaler"
	"github.com/gardener/dependency-watchdog/internal/test"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var defaultScalingTargetNames = []string{test.MCMDeploymentName, test.KCMDeploymentName, test.CADeploymentName}

type fakeScaler struct {
	scaler.Scaler
	client            client.Client
	scalingTargetRefs []client.ObjectKey
	scaleUpErr        error
	scaleDownErr      error
}

func NewFakeScaler(client client.Client, namespace string, scaleUpErr, scaleDownErr error) scaler.Scaler {
	return &fakeScaler{
		client:            client,
		scalingTargetRefs: createScalingTargetRefs(namespace),
		scaleUpErr:        scaleUpErr,
		scaleDownErr:      scaleDownErr,
	}
}

func createScalingTargetRefs(namespace string) []client.ObjectKey {
	var scalingTargetRefs []client.ObjectKey
	for _, scalingTargetName := range defaultScalingTargetNames {
		scalingTargetRefs = append(scalingTargetRefs, client.ObjectKey{
			Namespace: namespace,
			Name:      scalingTargetName,
		})
	}
	return scalingTargetRefs
}

func (f *fakeScaler) ScaleUp(ctx context.Context) error {
	if f.scaleUpErr != nil {
		return f.scaleUpErr
	}
	for _, scalingTargetRef := range f.scalingTargetRefs {
		if err := f.doScale(ctx, scalingTargetRef, 1); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeScaler) ScaleDown(ctx context.Context) error {
	if f.scaleDownErr != nil {
		return f.scaleDownErr
	}
	for _, scalingTargetRef := range f.scalingTargetRefs {
		if err := f.doScale(ctx, scalingTargetRef, 0); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeScaler) doScale(ctx context.Context, ref client.ObjectKey, replicas int32) error {
	deploy := &appsv1.Deployment{}
	if err := f.client.Get(ctx, ref, deploy); err != nil {
		return err
	}
	clone := deploy.DeepCopy()
	clone.Spec.Replicas = &replicas
	if err := f.client.Update(ctx, clone); err != nil {
		return err
	}
	return nil
}
