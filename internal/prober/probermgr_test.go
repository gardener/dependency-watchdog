package prober_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/dependency-watchdog/internal/prober"
)

var _ = Describe("Probermgr", func() {
	var mgr prober.Manager
	var p *prober.Prober
	var namespace string

	BeforeEach(func() {
		namespace = "test"
		mgr = prober.NewManager()
		Expect(mgr).ShouldNot(BeNil())
		p = prober.NewProber(namespace, &prober.Config{}, nil, nil)
		Expect(p).ShouldNot(BeNil())
		Expect(p.Namespace).Should(Equal(namespace))
	})

	It("should register and unregister prober with namespace as the key", func() {
		By("registering the prober")
		mgr.Register(p)

		By("checking if it got registered with correct key")
		regProber, ok := mgr.GetProber(namespace)
		Expect(ok).Should(BeTrue())
		Expect(regProber.Namespace).Should(Equal(namespace))
		Expect(prober.IsClosed(p)).Should(BeFalse())

		By("Deregistering the prober")
		mgr.Unregister(namespace)

		By("checking if it got deregistered and the context got cancelled")
		_, ok = mgr.GetProber(namespace)
		Expect(ok).Should(BeFalse())
		Eventually(prober.IsClosed(p)).Should(BeTrue())
	})

	It("should not overwrite existing prober with a new one", func() {
		By("registering the prober")
		mgr.Register(p)

		By("creating a new prober with same namespace")
		pr := prober.NewProber(namespace, &prober.Config{Name: "test"}, nil, nil)
		Expect(pr).ShouldNot(BeNil())
		Expect(pr.Namespace).Should(Equal(namespace))

		By("registering new prober")
		mgr.Register(pr)

		By("checking that it did not overwrite the existing one")
		regProber, _ := mgr.GetProber(namespace)
		Expect(regProber.Config.Name).ShouldNot(Equal(pr.Config.Name))

	})
})
