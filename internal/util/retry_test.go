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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

var (
	list        []string
	numAttempts = 3
	backoff     = 10 * time.Millisecond
	interval    = 2 * time.Millisecond
	timeout     = 10 * time.Millisecond
)

func TestNoErrorIfTaskEventuallySucceeds(t *testing.T) {
	g := NewWithT(t)
	result := Retry(context.Background(), "", passEventually(), numAttempts, backoff, AlwaysRetry)
	g.Expect(result.Err).Should(BeNil())
	g.Expect(result.Value).Should(Equal("appendPass"))
	g.Expect(len(list)).Should(Equal(3))
	g.Expect(list[0:2]).Should(ConsistOf("appendFail", "appendFail"))
	g.Expect(list[2]).To(Equal("appendPass"))
	emptyList()
}

func TestErrorIfExceedsAttempts(t *testing.T) {
	g := NewWithT(t)
	result := Retry(context.Background(), "", appendFail, numAttempts, backoff, AlwaysRetry)
	g.Expect(len(list)).Should(Equal(numAttempts))
	g.Expect(result.Err.Error()).Should(Equal("appendFail"))
	g.Expect(result.Value).Should(Equal("appendFail"))
	emptyList()
}

func TestCanRetryReturnsFalse(t *testing.T) {
	g := NewWithT(t)
	var hasRunOnce bool
	runOnceFn := func(error) bool {
		if !hasRunOnce {
			hasRunOnce = true
			return true
		}
		return false
	}
	result := Retry(context.Background(), "", passEventually(), numAttempts, backoff, runOnceFn)
	g.Expect(len(list)).Should(Equal(2))
	g.Expect(list[0:2]).Should(ConsistOf("appendFail", "appendFail"))
	g.Expect(result.Err.Error()).Should(Equal("appendFail"))
	g.Expect(result.Value).Should(Equal(""))
	emptyList()
}

func TestContextCancelledBeforeTaskIsRun(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	var result RetryResult[string]
	var wg sync.WaitGroup
	wg.Add(1)
	cancelFn()
	go func() {
		defer wg.Done()
		result = Retry(ctx, "", appendPass, numAttempts, backoff, AlwaysRetry)
		g.Expect(result.Err).Should(Equal(ctx.Err()))
		g.Expect(result.Value).Should(Equal(""))
		g.Expect(len(list)).Should(BeNumerically("<=", numAttempts))
	}()
	wg.Wait()
	emptyList()
}

func TestContextCancelledBeforeBackoffBegins(t *testing.T) {
	g := NewWithT(t)
	var result RetryResult[string]
	var wg sync.WaitGroup
	list := make([]string, 0, 1)
	ctx, cancelFn := context.WithCancel(context.Background())
	wg.Add(1)
	go func() {
		defer wg.Done()
		result = Retry(ctx, "", func() (string, error) {
			list = append(list, "appendFail")
			cancelFn()
			return "", fmt.Errorf("appendFail")
		}, numAttempts, backoff, AlwaysRetry)

		g.Expect(result.Err).Should(Equal(context.Canceled))
		g.Expect(result.Value).Should(Equal(""))
		g.Expect(len(list)).Should(Equal(1))
	}()
	wg.Wait()
	emptyList()
}

func TestRetryUntilPredicateForContextCancelled(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		result := RetryUntilPredicate(ctx, "", func() bool { return false }, timeout, interval)
		g.Expect(result).Should(BeFalse())
	}()
	cancelFn()
	wg.Wait()
}

func TestRetryUntilPredicateWithBackgroundContext(t *testing.T) {
	counter := 0
	table := []struct {
		predicateFn    func() bool
		expectedResult bool
	}{
		{func() bool { return false }, false},
		{func() bool { return true }, true},
		{func() bool {
			counter++
			return counter%2 == 0
		}, true},
	}
	for _, entry := range table {
		g := NewWithT(t)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := RetryUntilPredicate(context.Background(), "", entry.predicateFn, timeout, interval)
			g.Expect(result).Should(Equal(entry.expectedResult))
		}()
		wg.Wait()
	}

}

func TestRetryOnError(t *testing.T) {
	g := NewWithT(t)
	counter := 0
	fn := func() error {
		counter++
		if counter < 3 {
			return errors.New("counter is less than 3. Returning an error")
		}
		return nil
	}
	RetryOnError(context.Background(), "", fn, 10*time.Millisecond)
	g.Expect(counter).To(Equal(3))
}

func TestRetryOnErrorWhenContextIsCancelled(t *testing.T) {
	g := NewWithT(t)
	ctx, cancelFn := context.WithCancel(context.Background())
	counter := 0
	fn := func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			counter++
		}
	}
	go RetryOnError(context.Background(), "", fn, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond) //forcing counter to be incremented
	cancelFn()
	g.Expect(counter).To(BeNumerically(">", 0))
	g.Expect(ctx.Err()).ToNot(BeNil())
}

func appendFail() (string, error) {
	list = append(list, "appendFail")
	return "appendFail", fmt.Errorf("appendFail")
}

func appendPass() (string, error) {
	list = append(list, "appendPass")
	return "appendPass", nil
}

func passEventually() func() (string, error) {
	var runCounter = 0
	return func() (string, error) {
		runCounter++
		if runCounter%3 == 0 {
			return appendPass()
		}
		return appendFail()
	}
}

func emptyList() {
	list = nil
}
