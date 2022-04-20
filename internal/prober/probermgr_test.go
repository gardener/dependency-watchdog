package prober_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/dependency-watchdog/internal/prober"
)

var _ = Describe("Probermgr", func() {
	var mgr prober.Manager

	BeforeEach(func() {
		mgr = prober.NewManager()
		Expect(mgr).ShouldNot(BeNil())
	})

	AfterEach(func() {
		for _, p := range mgr.GetAllProbers() {
			mgr.Unregister(p.GetNamespace())
		}
	})

	It("register a new prober and check if it exists and is not closed", func() {
		By("registering a prober")
		const namespace = "bingo"
		p := prober.NewProber(namespace, &prober.Config{Name: "bingo"}, nil, nil)
		Expect(p).ShouldNot(BeNil())
		Expect(p.GetNamespace()).Should(Equal(namespace))
		Expect(mgr.Register(p)).To(BeTrue())

		By("checking if it got registered with correct key")
		foundProber, ok := mgr.GetProber(namespace)
		Expect(ok).Should(BeTrue())
		Expect(foundProber).ShouldNot(BeNil())
		Expect(foundProber.GetNamespace()).Should(Equal(p.GetNamespace()))
		Expect(foundProber.IsClosed()).Should(BeFalse())
	})

	It("unregister an existing prober and check its removed from the manager and is also closed", func() {
		By("registering the prober")
		const namespace = "bingo"
		p := prober.NewProber(namespace, &prober.Config{Name: "bingo"}, nil, nil)
		Expect(mgr.Register(p)).To(BeTrue())
		By("unregistering the prober")
		mgr.Unregister(namespace)
		By("checking if it got de-registered and the context got cancelled")
		_, ok := mgr.GetProber(namespace)
		Expect(ok).Should(BeFalse())
		Eventually(p.IsClosed()).Should(BeTrue())
	})

	It("unregister a non-existing prober and check if this does not fail", func() {
		Expect(mgr.Unregister("bazingo")).To(BeFalse())
	})

	It("prober registration with the same key should not overwrite existing prober registration", func() {
		By("registering the prober")
		const namespace = "bingo"
		p1 := prober.NewProber(namespace, &prober.Config{Name: "bingo"}, nil, nil)
		Expect(mgr.Register(p1)).To(BeTrue())
		By("attempting to register another prober with the same namespace but with a different Config.Name")
		p2 := prober.NewProber(namespace, &prober.Config{Name: "zingo"}, nil, nil)
		Expect(mgr.Register(p2)).To(BeFalse())
		By("validate if the old prober is still registered and is not overwritten")
		foundProber, ok := mgr.GetProber(namespace)
		Expect(ok).Should(BeTrue())
		Expect(foundProber.GetConfig().Name).ShouldNot(Equal(p2.GetConfig().Name))
		Expect(foundProber.GetConfig().Name).Should(Equal(p1.GetConfig().Name))
	})

})
