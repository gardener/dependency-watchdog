package util

import (
	"context"
	"fmt"
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

func GetKubeConfigFromSecret(ctx context.Context, namespace, secretName string, client client.Client) ([]byte, error) {
	secretKey := types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}
	secret := corev1.Secret{}
	err := client.Get(ctx, secretKey, &secret)
	if err != nil {
		logger.Error(err, "failed to retrieve secret, will not be able to create shoot client", "namespace", namespace, "secretName", secretName)
		return nil, err
	}
	// Extract the kubeconfig from the secret
	kubeConfig, ok := secret.Data[kubeConfigSecretKey]
	if !ok {
		logger.Error(err, "secret does not have kube-config", "namespace", namespace, "secretName", secretName)
		return nil, fmt.Errorf("expected key: %s in {namespace: %s, secret: %s} is missing", kubeConfigSecretKey, secretName, namespace)
	}
	return kubeConfig, nil
}

func CreateClientFromKubeConfigBytes(kubeConfigBytes []byte) (kubernetes.Interface, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
	if err != nil {
		return nil, err
	}
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

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
