package util_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/dependency-watchdog/internal/util"
)

var _ = Describe("Util", func() {
	Describe("ReplicaMismatch", func() {
		It("checks scaleUpReplicaMismatch", func() {
			Expect(util.ScaleUpReplicasMismatch(1, 2)).To(BeTrue())
			Expect(util.ScaleUpReplicasMismatch(2, 1)).To(BeFalse())
			Expect(util.ScaleUpReplicasMismatch(1, 1)).To(BeFalse())
		})

		It("checks scaleDownReplicaMismatch", func() {
			Expect(util.ScaleDownReplicasMismatch(1, 2)).To(BeFalse())
			Expect(util.ScaleDownReplicasMismatch(2, 1)).To(BeTrue())
			Expect(util.ScaleDownReplicasMismatch(1, 1)).To(BeFalse())
		})
	})

	Describe("SleepWithContext", func() {
		It("should stop if the context deadline is exceeded", func() {
			ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Millisecond)
			defer cancelFn()
			err := util.SleepWithContext(ctx, 10*time.Millisecond)
			Expect(err).ShouldNot(BeNil())
			Expect(err).Should(Equal(context.DeadlineExceeded))
		})

		It("should stop if the context is cancelled", func() {
			ctx, cancelFn := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			var err error
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				err = util.SleepWithContext(ctx, 10*time.Millisecond)
				Expect(err).Should(Equal(context.Canceled))
			}()
			cancelFn()
			wg.Wait()
		})

		It("should not return error if context is not cancelled or timed out", func() {
			ctx := context.Background()
			err := util.SleepWithContext(ctx, time.Microsecond)
			Expect(err).Should(BeNil())
		})
	})
})
