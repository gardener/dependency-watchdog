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

type AtomicStringList struct {
	lock   sync.RWMutex
	values []string
}

func NewAtomicStringList() *AtomicStringList {
	return &AtomicStringList{}
}

func (a *AtomicStringList) Append(values ...string) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.values = append(a.values, values...)
}

func (a *AtomicStringList) Values() []string {
	a.lock.RLock()
	defer a.lock.RUnlock()

	if a.values == nil {
		return nil
	}

	out := make([]string, len(a.values))
	copy(out, a.values)
	return out
}

var _ = Describe("Retry", func() {
	var ctx context.Context
	var cancelFn context.CancelFunc
	var list *AtomicStringList
	var pass func() (string, error)
	var fail func() (string, error)
	var passEventually func() (string, error)
	//var cancelContextAfterOneRun func() (string, error)
	var retryOnlyOnce func(error) bool
	var numAttempts int
	var backoff time.Duration

	BeforeEach(func() {
		ctx, cancelFn = context.WithCancel(context.Background())
		list = NewAtomicStringList()
		numAttempts = 3
		backoff = 10 * time.Millisecond
		fail = func() (string, error) {
			list.Append("fail")
			return "fail", fmt.Errorf("fail")
		}
		pass = func() (string, error) {
			list.Append("pass")
			return "pass", nil
		}
		i := 0
		j := 0
		passEventually = func() (string, error) {
			i++
			if i%3 == 0 {
				return pass()
			}
			return fail()
		}
		// cancelContextAfterOneRun = func() (string, error) {
		// 	defer cancelFn()
		// 	return fail()
		// }
		retryOnlyOnce = func(err error) bool {
			if j == 0 {
				j = 1
				return true
			}
			return false
		}
	})

	It("should not return error if task function eventually succeeds", func() {
		result := util.Retry(ctx, "", passEventually, numAttempts, backoff, util.AlwaysRetry)
		values := list.Values()
		Expect(result.Err).Should(BeNil())
		Expect(result.Value).Should(Equal("pass"))
		Expect(len(values)).Should(Equal(3))
		Expect(values[0:2]).Should(ConsistOf("fail", "fail"))
		Expect(values[2]).To(Equal("pass"))
	})

	It("should return error if it exceeds number of attempts", func() {
		result := util.Retry(ctx, "", fail, numAttempts, backoff, util.AlwaysRetry)
		values := list.Values()
		Expect(len(values)).Should(Equal(numAttempts))
		Expect(result.Err.Error()).Should(Equal("fail"))
		Expect(result.Value).Should(Equal("fail"))
	})

	It("should stop if canRetry returns false", func() {
		result := util.Retry(ctx, "", passEventually, numAttempts, backoff, retryOnlyOnce)
		values := list.Values()
		Expect(len(values)).Should(Equal(2))
		Expect(values[0:2]).Should(ConsistOf("fail", "fail"))
		Expect(result.Err.Error()).Should(Equal("fail"))
		Expect(result.Value).Should(Equal(""))
	})

	Describe("Cancel Context", func() {
		It("should stop if context is cancelled before task is run", func() {
			var result util.RetryResult[string]
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				result = util.Retry(ctx, "", pass, numAttempts, backoff, util.AlwaysRetry)
				values := list.Values()
				Expect(result.Err).Should(Equal(ctx.Err()))
				Expect(result.Value).Should(Equal(""))
				Expect(len(values)).Should(Equal(0))
			}()
			cancelFn()
			wg.Wait()
		})

		FIt("should stop if context is cancelled before backoff period begins", func() {
			var result util.RetryResult[string]
			var wg sync.WaitGroup
			list := make([]string, 0, 1)
			ctx, cancelFn = context.WithCancel(context.Background())
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				result = util.Retry(ctx, "", func() (string, error) {
					list = append(list, "fail")
					cancelFn()
					return "", fmt.Errorf("fail")
				}, numAttempts, backoff, util.AlwaysRetry)

				Expect(result.Err).Should(Equal(context.Canceled))
				Expect(result.Value).Should(Equal(""))
				Expect(len(list)).Should(Equal(1))
			}()
			cancelFn()
			wg.Wait()
		})
	})

})
