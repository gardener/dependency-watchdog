package prober_test

import (
	"errors"
	"github.com/gardener/dependency-watchdog/internal/prober"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("Config", Ordered, Label("config"), func() {
	var (
		testConfigPath string
		err            error
	)

	BeforeAll(func() {
		testConfigPath = "testdata/config.yaml"
		if _, err := os.Stat(testConfigPath); errors.Is(err, os.ErrNotExist) {
			GinkgoWriter.Printf("%s does not exist. This should not have happened. Check testdata directory.\n", testConfigPath)
		}
		Expect(err).ToNot(HaveOccurred())
	})

	Context("valid config should pass all validations", func() {
		It("", func() {
			config, err := prober.ReadAndUnmarshal(testConfigPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).ToNot(BeNil())
		})
	})
})
