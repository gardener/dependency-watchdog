// Copyright 2022 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"context"
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log.WithName("util")

const (
	kubeConfigSecretKey = "kubeconfig"
)

// GetKubeConfigFromSecret extracts kubeconfig from a k8s secret with name secretName in namespace
func GetKubeConfigFromSecret(ctx context.Context, namespace, secretName string, client client.Client) ([]byte, error) {
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}
	secret := corev1.Secret{}
	err := client.Get(ctx, secretKey, &secret)
	if err != nil {
		logger.Error(err, "Failed to retrieve secret, will not be able to create shoot client", "namespace", namespace, "secretName", secretName)
		return nil, err
	}
	// Extract the kubeconfig from the secret
	kubeConfig, ok := secret.Data[kubeConfigSecretKey]
	if !ok {
		logger.Error(err, "Secret does not have kube-config", "namespace", namespace, "secretName", secretName)
		return nil, fmt.Errorf("expected key: %s in {namespace: %s, secret: %s} is missing", kubeConfigSecretKey, secretName, namespace)
	}
	return kubeConfig, nil
}

// CreateClientFromKubeConfigBytes creates a client to connect to the Kube ApiServer using the kubeConfigBytes passed as a parameter
// It will also set a connection timeout and will disable KeepAlive.
func CreateClientFromKubeConfigBytes(kubeConfigBytes []byte, connectionTimeout time.Duration) (kubernetes.Interface, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
	if err != nil {
		return nil, err
	}
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	config.Timeout = connectionTimeout
	transport, err := createTransportWithDisabledKeepAlive(config)
	if err != nil {
		return nil, err
	}
	config.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return transport
	})
	return kubernetes.NewForConfig(config)
}

// Client created for probing the Kube ApiServer needs to have 'KeepAlive` disabled to ensure
// that the broken TCP connections are not kept alive for longer duration resulting in unwanted
// scale down of critical control plane components.
// See https://github.com/gardener/dependency-watchdog/issues/61
func createTransportWithDisabledKeepAlive(config *rest.Config) (*http.Transport, error) {
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return nil, err
	}
	// rest.Config does not have any transport set and therefore leverages
	// http.DefaultTransport provided by golang. To properly initialize the transport
	// one needs to clone the http.DefaultTransport and also set the correct TLS config
	// which can be extracted from the already constructed rest.Config which is passed to this function.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}

// CreateScalesGetter Creates a new ScalesGetter given the config
func CreateScalesGetter(config *rest.Config) (scale.ScalesGetter, error) {
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	discoveryClient := clientSet.Discovery()
	resolver := scale.NewDiscoveryScaleKindResolver(discoveryClient)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	return scale.New(clientSet.RESTClient(), mapper, dynamic.LegacyAPIPathResolverFunc, resolver), nil
}

// GetDeploymentFor Looks-up a k8s deployment with the give name and namespace
func GetDeploymentFor(ctx context.Context, namespace string, name string, client client.Client, timeout *time.Duration) (*appsv1.Deployment, error) {
	childCtx := ctx
	var cancelFn context.CancelFunc
	if timeout != nil {
		childCtx, cancelFn = context.WithTimeout(ctx, *timeout)
	}
	if cancelFn != nil {
		defer cancelFn()
	}
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	deployment := appsv1.Deployment{}
	err := client.Get(childCtx, key, &deployment)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// CreateClientSetFromRestConfig creates a kubernetes.Clientset from rest.Config.
func CreateClientSetFromRestConfig(config *rest.Config) (*kubernetes.Clientset, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
