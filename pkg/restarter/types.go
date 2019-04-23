package restarter

import (
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type ServiceDependants struct {
	Service    string      `json:"service"`
	Namespace  string      `json:"namespace"`
	Dependants []Dependant `json:"dependants"`
}

type Dependant struct {
	Labels []string `json:"labels"`
	Name   string   `json:"name"`
}

type Controller struct {
	clientset        *kubernetes.Clientset
	podLister        listerv1.PodLister
	endpointInformer cache.SharedIndexInformer
	workqueue        workqueue.RateLimitingInterface
	hasSynced        cache.InformerSynced
	stopCh           <-chan struct{}
}
