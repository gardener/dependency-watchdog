package util

import (
	"context"
	"time"
)

func SleepWithContext(ctx context.Context, sleepFor time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepFor):
			return nil
		}
	}
}

func ScaleUpReplicasMismatch(replicas, targetReplicas int32) bool {
	return targetReplicas > replicas
}

func ScaleDownReplicasMismatch(replicas, targetReplicas int32) bool {
	return replicas > targetReplicas
}
