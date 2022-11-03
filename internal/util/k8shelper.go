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

	"github.com/go-logr/logr"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

// GetScaleResource returns a kubernetes scale subresource.
func GetScaleResource(ctx context.Context, client client.Client, scaler scale.ScaleInterface, logger logr.Logger, resourceRef *autoscalingv1.CrossVersionObjectReference, timeout time.Duration) (*schema.GroupResource, *autoscalingv1.Scale, error) {
	gr, err := getGroupResource(client, logger, resourceRef)
	if err != nil {
		return nil, nil, err
	}
	scaleRes, err := func() (*autoscalingv1.Scale, error) {
		childCtx, cancelFn := context.WithTimeout(ctx, timeout)
		defer cancelFn()
		return scaler.Get(childCtx, gr, resourceRef.Name, metav1.GetOptions{})
	}()
	return &gr, scaleRes, err
}

// getGroupResource returns a schema.GroupResource for the given resourceRef.
func getGroupResource(client client.Client, logger logr.Logger, resourceRef *autoscalingv1.CrossVersionObjectReference) (schema.GroupResource, error) {
	gv, _ := schema.ParseGroupVersion(resourceRef.APIVersion) // Ignoring the error as this validation has already been done when initially validating the Config
	gk := schema.GroupKind{
		Group: gv.Group,
		Kind:  resourceRef.Kind,
	}
	mapping, err := client.RESTMapper().RESTMapping(gk, gv.Version)
	if err != nil {
		logger.Error(err, "Failed to get RESTMapping for resource", "resourceRef", resourceRef)
		return schema.GroupResource{}, err
	}
	return mapping.Resource.GroupResource(), nil
}

// GetResourceAnnotations gets the annotations for a resource identified by resourceRef withing the given namespace.
func GetResourceAnnotations(ctx context.Context, client client.Client, namespace string, resourceRef *autoscalingv1.CrossVersionObjectReference) (map[string]string, error) {
	partialObjMeta := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       resourceRef.Kind,
			APIVersion: resourceRef.APIVersion,
		},
	}
	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceRef.Name}, partialObjMeta)
	if err != nil {
		return nil, fmt.Errorf("error getting annotations for resource %s. Err: %w", resourceRef, err)
	}
	return partialObjMeta.Annotations, nil
}

// PatchResourceAnnotations patches the resource annotation with patchBytes. It uses StrategicMergePatchType strategy so the consumers should only provide changes to the annotations.
func PatchResourceAnnotations(ctx context.Context, cl client.Client, namespace string, resourceRef *autoscalingv1.CrossVersionObjectReference, patchBytes []byte) error {
	partialObjMeta := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			Kind:       resourceRef.Kind,
			APIVersion: resourceRef.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceRef.Name,
			Namespace: namespace,
		},
	}
	return cl.Patch(ctx, partialObjMeta, client.RawPatch(types.StrategicMergePatchType, patchBytes))
}

// GetAnnotationsAndReadyReplicasForResource gets metadata.annotations and spec.replicas for any resource identified via resourceRef withing the given namespace.
// It is an error if there is no spec.replicas or if there is an error fetching the resource.
func GetAnnotationsAndReadyReplicasForResource(ctx context.Context, client client.Client, namespace string, resourceRef *autoscalingv1.CrossVersionObjectReference) (map[string]string, int32, error) {
	resObj := unstructured.Unstructured{}
	groupVersion, err := schema.ParseGroupVersion(resourceRef.APIVersion)
	if err != nil {
		return nil, 0, err
	}
	resObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   groupVersion.Group,
		Version: groupVersion.Version,
		Kind:    resourceRef.Kind,
	})
	err = client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceRef.Name}, &resObj)
	if err != nil {
		return nil, 0, err
	}
	readyReplicas, found, err := unstructured.NestedInt64(resObj.Object, "status", "readyReplicas")
	if !found {
		return resObj.GetAnnotations(), 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return resObj.GetAnnotations(), int32(readyReplicas), nil
}

func GetResourceReadyReplicas(ctx context.Context, cli client.Client, namespace string, resourceRef *autoscalingv1.CrossVersionObjectReference) (int32, error) {
	resObj := unstructured.Unstructured{}

	groupVersion, err := schema.ParseGroupVersion(resourceRef.APIVersion)
	if err != nil {
		return 0, err
	}
	resObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   groupVersion.Group,
		Version: groupVersion.Version,
		Kind:    resourceRef.Kind,
	})
	err = cli.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resourceRef.Name}, &resObj)
	if err != nil {
		return 0, err
	}
	readyReplicas, found, err := unstructured.NestedInt64(resObj.Object, "status", "readyReplicas")
	if !found {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return int32(readyReplicas), nil
}

// CreateClientSetFromRestConfig creates a kubernetes.Clientset from rest.Config.
func CreateClientSetFromRestConfig(config *rest.Config) (*kubernetes.Clientset, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
