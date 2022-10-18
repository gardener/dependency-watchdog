package scaler

import (
	"net/http"

	"k8s.io/client-go/rest"
)

// DisableKeepAlive sets `DisableKeepAlive` to true on the transport that is use by the underline rest client
// Fixes https://github.com/gardener/dependency-watchdog/issues/61
func DisableKeepAlive(config *rest.Config) error {
	transport, err := createTransportWithDisableKeepAlive(config)
	if err != nil {
		return err
	}
	config.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return transport
	})
	return nil
}

func createTransportWithDisableKeepAlive(config *rest.Config) (*http.Transport, error) {
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}
