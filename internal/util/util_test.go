package util

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestScaleUpReplicaMismatch(t *testing.T) {
	g := NewWithT(t)
	g.Expect(ScaleUpReplicasMismatch(1, 2)).To(BeTrue())
	g.Expect(ScaleUpReplicasMismatch(2, 1)).To(BeFalse())
	g.Expect(ScaleUpReplicasMismatch(1, 1)).To(BeFalse())
}

func TestScaleDownReplicaMismatch(t *testing.T) {
	g := NewWithT(t)
	g.Expect(ScaleDownReplicasMismatch(1, 2)).To(BeFalse())
	g.Expect(ScaleDownReplicasMismatch(2, 1)).To(BeTrue())
	g.Expect(ScaleDownReplicasMismatch(1, 1)).To(BeFalse())
}

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
