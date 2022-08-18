package util

import (
	"context"
	"errors"
	"github.com/goccy/go-yaml"
	"github.com/onsi/gomega"
	"io/ioutil"
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
