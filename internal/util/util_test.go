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

package util

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	. "github.com/onsi/gomega"
)

func TestSleepWithContextShouldStopIfDeadlineExceeded(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancelFn()
	err := SleepWithContext(ctx, 10*time.Millisecond)
	g.Expect(err).ShouldNot(BeNil())
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
	g.Expect(err).Should(BeNil())
}

func TestReadAndUnmarshallNonExistingFile(t *testing.T) {
	g := NewWithT(t)
	_, err := ReadAndUnmarshall[papi.Config]("file-that-does-not-exists.yaml")
	g.Expect(err).ToNot(BeNil())
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
	g.Expect(err).To(BeNil())
	g.Expect(c.Name).To(Equal("zeus"))
	g.Expect(c.Version).To(Equal("v1.0"))
	expectedData := map[string]string{"level": "god-like", "type": "warrior"}
	for k, v := range c.Data {
		g.Expect(expectedData).To(HaveKeyWithValue(k, v))
	}
}
