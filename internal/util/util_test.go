// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:build !kind_tests

package util

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"k8s.io/utils/pointer"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	. "github.com/onsi/gomega"
)

func TestSleepWithContextShouldStopIfDeadlineExceeded(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancelFn()
	err := SleepWithContext(ctx, 10*time.Millisecond)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(err).Should(Equal(context.DeadlineExceeded))
}

func TestSleepWithContextShouldStopIfContextCancelled(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var err error
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = SleepWithContext(ctx, 10*time.Millisecond)
		g.Expect(err).Should(Equal(context.Canceled))
	}()
	cancelFn()
	wg.Wait()
}

func TestSleepWithContextForNonCancellableContext(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	err := SleepWithContext(ctx, time.Microsecond)
	g.Expect(err).ShouldNot(HaveOccurred())
}

func TestReadAndUnmarshallNonExistingFile(t *testing.T) {
	g := NewWithT(t)
	_, err := ReadAndUnmarshall[papi.Config]("file-that-does-not-exists.yaml")
	g.Expect(err).To(HaveOccurred())
}

func TestReadAndUnmarshall(t *testing.T) {
	g := NewWithT(t)
	type config struct {
		Name    string
		Version string
		Data    map[string]string
	}
	configPath := filepath.Join("testdata", "test-config.yaml")
	c, err := ReadAndUnmarshall[config](configPath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(c.Name).To(Equal("zeus"))
	g.Expect(c.Version).To(Equal("v1.0"))
	expectedData := map[string]string{"level": "god-like", "type": "warrior"}
	for k, v := range c.Data {
		g.Expect(expectedData).To(HaveKeyWithValue(k, v))
	}
}

func TestEqualOrBeforeNow(t *testing.T) {
	g := NewWithT(t)
	g.Expect(EqualOrBeforeNow(time.Now())).To(BeTrue())
	g.Expect(EqualOrBeforeNow(time.Now().Add(-time.Millisecond))).To(BeTrue())
	g.Expect(EqualOrBeforeNow(time.Now().Add(time.Millisecond))).To(BeFalse())
}

func TestFillDefaultIfNil(t *testing.T) {
	g := NewWithT(t)
	var testInt *int
	testInt = GetValOrDefault[int](testInt, 10)
	g.Expect(*testInt).To(Equal(10))

	testFloat := pointer.Float64(1.0)
	testFloat = GetValOrDefault(testFloat, 2.0)
	g.Expect(*testFloat).To(Equal(1.0))
}

func TestGetSliceOrDefault(t *testing.T) {
	defaultSlice := []string{"bingo"}

	tests := []struct {
		description         string
		inputSlice          []string
		expectedOutputSlice []string
	}{
		{description: "default slice should be returned if input slice is nil", inputSlice: nil, expectedOutputSlice: defaultSlice},
		{description: "default slice should be returned if input slice is empty", inputSlice: []string{}, expectedOutputSlice: defaultSlice},
		{description: "input slice should be returned if it is not nil or empty", inputSlice: []string{"bingo", "tringo"}, expectedOutputSlice: []string{"bingo", "tringo"}},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			g.Expect(GetSliceOrDefault(test.inputSlice, test.expectedOutputSlice)).To(Equal(test.expectedOutputSlice))
		})
	}
}
