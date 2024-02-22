// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"os"
	"time"

	"sigs.k8s.io/yaml"
)

// SleepWithContext sleeps until sleepFor duration has expired or the context has been cancelled.
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

// ReadAndUnmarshall reads file and Unmarshall the contents in a generic type
func ReadAndUnmarshall[T any](filename string) (*T, error) {
	configBytes, err := os.ReadFile(filename)
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

// EqualOrBeforeNow returns false if the argument passed is after the current time.
func EqualOrBeforeNow(expiryTime time.Time) bool {
	return !expiryTime.After(time.Now())
}

// GetValOrDefault assigns the default value if the pointer is nil
func GetValOrDefault[T any](val *T, defaultVal T) *T {
	if val == nil {
		return &defaultVal
	}
	return val
}
