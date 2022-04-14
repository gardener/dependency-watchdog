package prober_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProber(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prober Suite")
}
