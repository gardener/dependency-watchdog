package test

import (
	"fmt"
	"log"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type ControllerTestEnv interface {
	GetClient() client.Client
	GetConfig() *rest.Config
	Delete()
}

type controllerTestEnv struct {
	client     client.Client
	restConfig *rest.Config
	testEnv    *envtest.Environment
}

func CreateControllerTestEnv() (ControllerTestEnv, error) {
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		log.Fatalf("error in starting testEnv: %v", err)
	}
	if cfg == nil {
		log.Fatalf("Got nil config from testEnv")
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create new client %w", err)
	}
	if k8sClient == nil {
		log.Fatalf("Got a nil k8sClient")
	}
	return &controllerTestEnv{
		client:     k8sClient,
		restConfig: cfg,
		testEnv:    testEnv,
	}, nil
}

func (te *controllerTestEnv) GetClient() client.Client {
	return te.client
}

func (te *controllerTestEnv) GetConfig() *rest.Config {
	return te.restConfig
}

func (te *controllerTestEnv) Delete() {
	err := te.testEnv.Stop()
	if err != nil {
		log.Printf("failed to cleanly stop controller test environment %v", err)
		return
	}
}
