package util_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gardener/dependency-watchdog/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Retry", Label("retry"), func() {
	var list []string
	var appendPass func() (string, error)
	var appendFail func() (string, error)
	var passEventually func() (string, error)
	var numAttempts int
	var backoff time.Duration
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		numAttempts = 3
		backoff = 10 * time.Millisecond
		appendFail = func() (string, error) {
			list = append(list, "appendFail")
			return "appendFail", fmt.Errorf("appendFail")
		}
		appendPass = func() (string, error) {
			list = append(list, "appendPass")
			return "appendPass", nil
		}
		runCounter := 0
		passEventually = func() (string, error) {
			runCounter++
			if runCounter%3 == 0 {
				return appendPass()
			}
			return appendFail()
		}
	})

	It("should not return error if task function eventually succeeds", func() {
		result := util.Retry(ctx, "", passEventually, numAttempts, backoff, util.AlwaysRetry)
		Expect(result.Err).Should(BeNil())
		Expect(result.Value).Should(Equal("appendPass"))
		Expect(len(list)).Should(Equal(3))
		Expect(list[0:2]).Should(ConsistOf("appendFail", "appendFail"))
		Expect(list[2]).To(Equal("appendPass"))
	})

	It("should return error if it exceeds number of attempts", func() {
		result := util.Retry(ctx, "", appendFail, numAttempts, backoff, util.AlwaysRetry)
		Expect(len(list)).Should(Equal(numAttempts))
		Expect(result.Err.Error()).Should(Equal("appendFail"))
		Expect(result.Value).Should(Equal("appendFail"))
	})

	It("should stop if canRetry returns false", func() {
		var hasRunOnce bool
		runOnceFn := func(error) bool {
			if !hasRunOnce {
				hasRunOnce = true
				return true
			}
			return false
		}
		result := util.Retry(ctx, "", passEventually, numAttempts, backoff, runOnceFn)
		Expect(len(list)).Should(Equal(2))
		Expect(list[0:2]).Should(ConsistOf("appendFail", "appendFail"))
		Expect(result.Err.Error()).Should(Equal("appendFail"))
		Expect(result.Value).Should(Equal(""))
	})

	Describe("Cancel Context", func() {
		It("should stop if context is cancelled before task is run", func() {
			ctx, cancelFn := context.WithCancel(ctx)
			var result util.RetryResult[string]
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				result = util.Retry(ctx, "", appendPass, numAttempts, backoff, util.AlwaysRetry)
				Expect(result.Err).Should(Equal(ctx.Err()))
				Expect(result.Value).Should(Equal(""))
				Expect(len(list)).Should(BeNumerically("<=", numAttempts))
			}()
			cancelFn()
			wg.Wait()
		})

		It("should stop if context is cancelled before backoff period begins", func() {
			var result util.RetryResult[string]
			var wg sync.WaitGroup
			list := make([]string, 0, 1)
			ctx, cancelFn := context.WithCancel(ctx)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				result = util.Retry(ctx, "", func() (string, error) {
					list = append(list, "appendFail")
					cancelFn()
					return "", fmt.Errorf("appendFail")
				}, numAttempts, backoff, util.AlwaysRetry)

				Expect(result.Err).Should(Equal(context.Canceled))
				Expect(result.Value).Should(Equal(""))
				Expect(len(list)).Should(Equal(1))
			}()
			wg.Wait()
		})
	})

	AfterEach(func() {
		list = nil
	})

})
