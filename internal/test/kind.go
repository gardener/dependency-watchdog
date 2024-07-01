// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kind "sigs.k8s.io/kind/pkg/cluster"
)

const (
	defaultKindNodeImage            = "kindest/node:v1.24.7"
	defaultKindClusterName          = "kind-test"
	kindNamePrefix                  = "kind-"
	kubeConfigFileName              = "kubeconfig"
	defaultControlPlaneReadyTimeout = 5 * time.Minute
)

// KindCluster provides a convenient interface to interact with a KIND cluster.
type KindCluster interface {
	// CreateNamespace creates a kubernetes namespace with the give name.
	CreateNamespace(name string) error
	// CreateDeployment creates a kubernetes deployment.
	CreateDeployment(name, namespace, imageName string, replicas int32, annotations map[string]string) error
	// DeleteAllDeployments deletes all kubernetes deployments in a given namespace.
	DeleteAllDeployments(namespace string) error
	// GetRestConfig provides access to *rest.Config.
	GetRestConfig() *rest.Config
	// GetClient provides access to client.Client to connect to the Kube ApiServer of the KIND cluster.
	GetClient() client.Client
	// GetDeployment looks up a kubernetes deployment with a given name and namespace and returns it if it is found else returns an error.
	// The consumer will have to check if the error is NotFoundError and take appropriate action.
	GetDeployment(namespace, name string) (*appsv1.Deployment, error)
	// Delete deletes the KIND cluster.
	Delete() error
}

// KindConfig holds configuration which will be used when creating a KIND cluster.
type KindConfig struct {
	Name                    string
	NodeImage               string
	ControlPlanReadyTimeout *time.Duration
}

type kindCluster struct {
	provider       *kind.Provider
	clusterConfig  KindConfig
	restConfig     *rest.Config
	client         client.Client
	kubeConfigPath string
}

// CreateKindCluster creates a new KIND cluster using the config passed
func CreateKindCluster(config KindConfig) (KindCluster, error) {
	kubeConfigPath, err := createKubeConfigPath()
	if err != nil {
		return nil, err
	}
	clusterConfig := config
	err = fillDefaultConfigValues(&clusterConfig)
	if err != nil {
		return nil, err
	}
	// create the kind cluster
	provider := kind.NewProvider(kind.ProviderWithLogger(newKindLogger()))
	err = doDeleteCluster(provider, clusterConfig.Name, kubeConfigPath)
	if err != nil {
		return nil, err
	}
	kubeConfigBytes, err := doCreateCluster(clusterConfig, provider)
	if err != nil {
		return nil, err
	}
	// store the kubeconfig file at kubeConfigPath, this will be later used to delete the cluster or perform operations on the cluster
	err = os.WriteFile(kubeConfigPath, kubeConfigBytes, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to store the kubeconfig file at %s :%w", kubeConfigPath, err)
	}
	log.Printf("sucessfully written kubeconfig file for KIND cluster %s to path: %s", clusterConfig.Name, kubeConfigPath)

	// create *rest.Config and controllerClient.Client for this kind cluster
	restConfig, err := createRestConfig(clusterConfig.Name, kubeConfigBytes)
	if err != nil {
		return nil, err
	}

	controllerClient, err := createClient(clusterConfig.Name, restConfig)
	if err != nil {
		return nil, err
	}

	return &kindCluster{
		provider:       provider,
		clusterConfig:  clusterConfig,
		restConfig:     restConfig,
		client:         controllerClient,
		kubeConfigPath: kubeConfigPath,
	}, nil

}

func doCreateCluster(clusterConfig KindConfig, provider *kind.Provider) ([]byte, error) {
	err := provider.Create(clusterConfig.Name,
		kind.CreateWithNodeImage(clusterConfig.NodeImage),
		kind.CreateWithRetain(false),
		kind.CreateWithWaitForReady(*clusterConfig.ControlPlanReadyTimeout),
		kind.CreateWithDisplayUsage(false),
		kind.CreateWithDisplaySalutation(false),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create kind cluster %s: %w", clusterConfig.Name, err)
	}
	kubeConfig, err := provider.KubeConfig(clusterConfig.Name, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig for kind cluster %s: %w", clusterConfig.Name, err)
	}
	return []byte(kubeConfig), nil
}

func createKubeConfigPath() (string, error) {
	kindConfigDir, err := os.MkdirTemp("", kindNamePrefix)
	if err != nil {
		return "", err
	}
	return filepath.Join(kindConfigDir, kubeConfigFileName), nil
}

func createRestConfig(clusterName string, kubeConfigBytes []byte) (*rest.Config, error) {
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create config from kubeconfig bytes for kind cluster: %s : %w", clusterName, err)
	}
	return clientConfig.ClientConfig()
}

func createClient(clusterName string, restConfig *rest.Config) (client.Client, error) {
	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create http client for KIND cluster %s : %w", clusterName, err)
	}
	mapper, err := apiutil.NewDynamicRESTMapper(restConfig, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic REST Mapper for KIND cluster %s : %w", clusterName, err)
	}
	return client.New(restConfig, client.Options{
		Mapper: mapper,
	})
}

func (kc *kindCluster) CreateNamespace(name string) error {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return kc.client.Create(context.Background(), namespaceObj)
}

func (kc *kindCluster) CreateDeployment(name, namespace, imageName string, replicas int32, annotations map[string]string) error {
	deployment := GenerateDeployment(name, namespace, imageName, replicas, annotations)
	return kc.client.Create(context.Background(), deployment)
}

func (kc *kindCluster) GetDeployment(namespace, name string) (*appsv1.Deployment, error) {
	key := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	deployment := appsv1.Deployment{}
	err := kc.client.Get(context.Background(), key, &deployment)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

func (kc *kindCluster) DeleteAllDeployments(namespace string) error {
	deployment := &appsv1.Deployment{}
	opts := []client.DeleteAllOfOption{client.InNamespace(namespace)}
	return kc.client.DeleteAllOf(context.Background(), deployment, opts...)
}

func (kc *kindCluster) GetRestConfig() *rest.Config {
	return kc.restConfig
}

func (kc *kindCluster) GetClient() client.Client {
	return kc.client
}

func (kc *kindCluster) Delete() error {
	if kc.provider == nil {
		return fmt.Errorf("kind cluster %s has not been started yet. You must call Create to first create a kind cluster", kc.clusterConfig.Name)
	}
	log.Printf("deleting cluster %s\n", kc.clusterConfig.Name)
	return doDeleteCluster(kc.provider, kc.clusterConfig.Name, kc.kubeConfigPath)
}

func doDeleteCluster(provider *kind.Provider, clusterName string, kubeConfigPath string) error {
	err := provider.Delete(clusterName, kubeConfigPath)
	if err != nil {
		return err
	}
	// cleanup the kubeconfig file
	err = os.RemoveAll(kubeConfigPath)
	if err != nil {
		log.Printf("Failed to remove test kubeconfig file at %s. This should ideally not happen! : %v", kubeConfigPath, err)
	}
	return nil
}

func fillDefaultConfigValues(config *KindConfig) error {
	if strings.TrimSpace(config.Name) == "" {
		config.Name = defaultKindClusterName
	}
	if strings.TrimSpace(config.NodeImage) == "" {
		config.NodeImage = defaultKindNodeImage
	}
	if config.ControlPlanReadyTimeout == nil {
		config.ControlPlanReadyTimeout = pointer.Duration(defaultControlPlaneReadyTimeout)
	}
	return nil
}
