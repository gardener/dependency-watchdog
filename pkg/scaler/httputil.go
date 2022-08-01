package scaler

import (
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"net/http"
)

type transportWrapper struct {
	Transport http.RoundTripper
}

func (rt *transportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	klog.Infof("(transportWrapper)(RoundTrip) request: %v", req.URL.String())
	return rt.Transport.RoundTrip(req)
}

// DisableKeepAlive sets `DisableKeepAlive` to true on the transport that is use by the underline rest client
func DisableKeepAlive(config *rest.Config) error {
	transport, err := createTransportWithDisableKeepAlive(config)
	if err != nil {
		return err
	}
	config.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return &transportWrapper{Transport: transport}
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
