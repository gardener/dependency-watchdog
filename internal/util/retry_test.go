package util

import (
	"context"
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
	go func() {
		defer wg.Done()
		result = Retry(ctx, "", appendPass, numAttempts, backoff, AlwaysRetry)
		g.Expect(result.Err).Should(Equal(ctx.Err()))
		g.Expect(result.Value).Should(Equal(""))
		g.Expect(len(list)).Should(BeNumerically("<=", numAttempts))
	}()
	cancelFn()
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

// var _ = Describe("Retry", Label("retry"), func() {
// 	var list []string
// 	var appendPass func() (string, error)
// 	var appendFail func() (string, error)
// 	var passEventually func() (string, error)
// 	var numAttempts int
// 	var backoff time.Duration
// 	var ctx context.Context

// 	BeforeEach(func() {
// 		ctx = context.Background()
// 		numAttempts = 3
// 		backoff = 10 * time.Millisecond
// 		appendFail = func() (string, error) {
// 			list = append(list, "appendFail")
// 			return "appendFail", fmt.Errorf("appendFail")
// 		}
// 		appendPass = func() (string, error) {
// 			list = append(list, "appendPass")
// 			return "appendPass", nil
// 		}
// 		runCounter := 0
// 		passEventually = func() (string, error) {
// 			runCounter++
// 			if runCounter%3 == 0 {
// 				return appendPass()
// 			}
// 			return appendFail()
// 		}
// 	})

// 	It("should not return error if task function eventually succeeds", func() {
// 		result := util.Retry(ctx, "", passEventually, numAttempts, backoff, util.AlwaysRetry)
// 		g.g.Expect(result.Err).Should(BeNil())
// 		Expect(result.Value).Should(Equal("appendPass"))
// 		Expect(len(list)).Should(Equal(3))
// 		Expect(list[0:2]).Should(ConsistOf("appendFail", "appendFail"))
// 		Expect(list[2]).To(Equal("appendPass"))
// 	})

// 	It("should return error if it exceeds number of attempts", func() {
// 		result := util.Retry(ctx, "", appendFail, numAttempts, backoff, util.AlwaysRetry)
// 		Expect(len(list)).Should(Equal(numAttempts))
// 		Expect(result.Err.Error()).Should(Equal("appendFail"))
// 		Expect(result.Value).Should(Equal("appendFail"))
// 	})

// 	It("should stop if canRetry returns false", func() {
// 		var hasRunOnce bool
// 		runOnceFn := func(error) bool {
// 			if !hasRunOnce {
// 				hasRunOnce = true
// 				return true
// 			}
// 			return false
// 		}
// 		result := util.Retry(ctx, "", passEventually, numAttempts, backoff, runOnceFn)
// 		Expect(len(list)).Should(Equal(2))
// 		Expect(list[0:2]).Should(ConsistOf("appendFail", "appendFail"))
// 		Expect(result.Err.Error()).Should(Equal("appendFail"))
// 		Expect(result.Value).Should(Equal(""))
// 	})

// 	Describe("Cancel Context", func() {
// 		It("should stop if context is cancelled before task is run", func() {
// 			ctx, cancelFn := context.WithCancel(ctx)
// 			var result util.RetryResult[string]
// 			var wg sync.WaitGroup
// 			wg.Add(1)
// 			go func() {
// 				defer wg.Done()
// 				defer GinkgoRecover()
// 				result = util.Retry(ctx, "", appendPass, numAttempts, backoff, util.AlwaysRetry)
// 				Expect(result.Err).Should(Equal(ctx.Err()))
// 				Expect(result.Value).Should(Equal(""))
// 				Expect(len(list)).Should(BeNumerically("<=", numAttempts))
// 			}()
// 			cancelFn()
// 			wg.Wait()
// 		})

// 		It("should stop if context is cancelled before backoff period begins", func() {
// 			var result util.RetryResult[string]
// 			var wg sync.WaitGroup
// 			list := make([]string, 0, 1)
// 			ctx, cancelFn := context.WithCancel(ctx)
// 			wg.Add(1)
// 			go func() {
// 				defer wg.Done()
// 				defer GinkgoRecover()
// 				result = util.Retry(ctx, "", func() (string, error) {
// 					list = append(list, "appendFail")
// 					cancelFn()
// 					return "", fmt.Errorf("appendFail")
// 				}, numAttempts, backoff, util.AlwaysRetry)

// 				Expect(result.Err).Should(Equal(context.Canceled))
// 				Expect(result.Value).Should(Equal(""))
// 				Expect(len(list)).Should(Equal(1))
// 			}()
// 			wg.Wait()
// 		})
// 	})

// 	AfterEach(func() {
// 		list = nil
// 	})

// })
