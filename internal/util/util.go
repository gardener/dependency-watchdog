package util

import (
	"context"
	"errors"
	"github.com/gardener/dependency-watchdog/api/weeder"
	"github.com/goccy/go-yaml"
	"github.com/onsi/gomega"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"os"
	"testing"
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

// ScaleUpReplicasMismatch scales down if current number of replicas is less than target replicas
func ScaleUpReplicasMismatch(replicas, targetReplicas int32) bool {
	return targetReplicas > replicas
}

// ScaleDownReplicasMismatch scales down if current number of replicas is more than target replicas
func ScaleDownReplicasMismatch(replicas, targetReplicas int32) bool {
	return replicas > targetReplicas
}

// ValidateIfFileExists validates the existence of a file
func ValidateIfFileExists(file string, t *testing.T) {
	g := gomega.NewWithT(t)
	var err error
	if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s does not exist. This should not have happened. Check testdata directory.\n", file)
	}
	g.Expect(err).ToNot(gomega.HaveOccurred(), "File at path %v should exist")
}

// ReadAndUnmarshall reads file and Unmarshall the contents in a generic type
func ReadAndUnmarshall[T any](filename string) (*T, error) {
	configBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	t := new(T)
	err = yaml.Unmarshal(configBytes, t)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// IsPodDeleted returns true if a pod is deleted; false otherwise.
func IsPodDeleted(pod *v1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

// ShouldDeletePod checks if the pod is in CrashloopBackoff and decides to delete the pod if its is
// not already deleted.
func ShouldDeletePod(pod *v1.Pod) bool {
	return !IsPodDeleted(pod) && IsPodInCrashloopBackoff(pod.Status)
}

// IsPodInCrashloopBackoff checks if the pod is in CrashloopBackoff from its status fields.
func IsPodInCrashloopBackoff(status v1.PodStatus) bool {
	for _, containerStatus := range status.ContainerStatuses {
		if isContainerInCrashLoopBackOff(containerStatus.State) {
			return true
		}
	}
	return false
}

func isContainerInCrashLoopBackOff(containerState v1.ContainerState) bool {
	if containerState.Waiting != nil {
		return containerState.Waiting.Reason == weeder.CrashLoopBackOff
	}
	return false
}
