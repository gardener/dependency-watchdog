package scaler_test

import (
	"net/http"

	"github.com/gardener/dependency-watchdog/pkg/scaler"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	kubeConfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    name: test
    certificate-authority-date: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURvRENDQW9nQ0NRQ1NWMElyenhoaEdUQU5CZ2txaGtpRzl3MEJBUXNGQURDQmtERUxNQWtHQTFVRUJoTUMKUkVVeEdqQVlCZ05WQkFnTUVVSmhaR1Z1TFZkMWNuUjBaVzFpWlhKbk1SRXdEd1lEVlFRSERBaFhZV3hzWkc5eQpaakVQTUEwR0ExVUVDZ3dHVTBGUUlGTkZNUkV3RHdZRFZRUUxEQWhIWVhKa1pXNWxjakVQTUEwR0ExVUVBd3dHClkyRjBaWE4wTVIwd0d3WUpLb1pJaHZjTkFRa0JGZzVqWVhSbGMzUkFjMkZ3TG1OdmJUQWdGdzB5TWpBNE1ERXcKTmpRNU1qQmFHQTh6TURJeE1USXdNakEyTkRreU1Gb3dnWkF4Q3pBSkJnTlZCQVlUQWtSRk1Sb3dHQVlEVlFRSQpEQkZDWVdSbGJpMVhkWEowZEdWdFltVnlaekVSTUE4R0ExVUVCd3dJVjJGc2JHUnZjbVl4RHpBTkJnTlZCQW9NCkJsTkJVQ0JUUlRFUk1BOEdBMVVFQ3d3SVIyRnlaR1Z1WlhJeER6QU5CZ05WQkFNTUJtTmhkR1Z6ZERFZE1Cc0cKQ1NxR1NJYjNEUUVKQVJZT1kyRjBaWE4wUUhOaGNDNWpiMjB3Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBQTRJQgpEd0F3Z2dFS0FvSUJBUURHaUFTNGJFVlFUMm1LM000TlFVaW1iRnRiRE5JU2NkdDNUWWpUcVk0WGZXd09vRXFnCjdyM2VnSFNoVEhxdGtqa1hqZjJJWW8yL05SL1gyelpmNFJHTWNORTc4RkhMdVQ2QkltNHdVTXdIMU9sWkY0R2cKRjRaT0cvckt3dlNOTndJUThhczRZbmo1d1lHUWpBaVZjRm5SUkNjajFGd2dnalpMQklzeVRNd1laV2EyWHRCbQpOL0lENHc0QkZ4T3NlZ3JhK1hSQWt4dm5SMCtOd0xHbkNjM1hWVGpTWEFzc1oraVdYMFozc1hIRFZQd3U0bTg0Cm9ocTVFbXFoZ21wQ0ZVOEYvSVBmRDFkektDTFZNWVdOQkJqSTBXSFQweHlLZTBlRXd5NFVVN0VNSGdMTU5EeXcKRnllRGt4L1UyRDZYK2RJaFpYOTh6d0NCdktvQlh2Q0ZsTzJaQWdNQkFBRXdEUVlKS29aSWh2Y05BUUVMQlFBRApnZ0VCQUdYM3Vod0xKSGxhNUU2UDBiUlA4QmJiZ25pUDB3VURydUw4RXJxZXZ0SWRZOW5MSFRXV05QcXFSVlQrCjRydFJ5b0ZZSGxzVm9ycE5wQ250RmhKYkJYM05hTGhFQjJlamROVmx3VXlVaUV6WktES09XN1I4YzY3czY0SE8Ka0dUTjJhcFV3TGhoNlZnVkFneHROdmp6dXo1QTRLM0pQV1I4emZYUzJZbVdNcVZIZ1plQi9UeFdyaTRXRkdUSgpmV2FtZVg4YlhXRFE3dWdBMnFlMkw3R1EycWxSaVN0THpsRm1yR1VKaUx0ZEZ0cnFEK0VHdmM4MTlDNUgxOURQClFYV1ZxanVndDJmTEFaMW56VmhsQWowT2lqQ29PbVZWdjl0ZUpCQVhyK1Fjc2RTc21vKzFuZTlpazJLak5oVzQKdk0xQklNajE4dmpaaWR5UDBvZitkODZnT0hFPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
    server: https://kube-apiserver.shoot--test--local.svc
contexts:
- context:
    cluster: shoot--test--local
    user: admin
  name: shoot--test--local
users:
- name: admin
  user:
    username: admin
    password: admin@bingo
`
)

var _ = Describe("Checking KeepAlive setting on rest.config", func() {
	var config *rest.Config
	BeforeEach(func() {
		clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(kubeConfig))
		Expect(err).To(BeNil())
		config, err = clientConfig.ClientConfig()
		Expect(err).To(BeNil())
	})
	It("defaults to KeepAlive enabled", func() {
		roundTripper, err := rest.TransportFor(config)
		Expect(err).To(BeNil())
		transport := roundTripper.(*http.Transport)
		Expect(transport).ToNot(BeNil())
		Expect(transport.DisableKeepAlives).To(BeFalse())
	})
	It("can set KeepAlive to disabled", func() {
		err := scaler.DisableKeepAlive(config)
		Expect(err).To(BeNil())
		roundTripper, err := rest.TransportFor(config)
		Expect(err).To(BeNil())
		t := roundTripper.(*http.Transport)
		Expect(t).ToNot(BeNil())
		Expect(t.DisableKeepAlives).To(BeTrue())
	})
})
