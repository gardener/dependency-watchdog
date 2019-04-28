package restarter

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	CrashLoopBackOff = "CrashLoopBackOff"
)

type ServiceDependants struct {
	Service    string            `json:"service"`
	Labels     map[string]string `json:"labels"`
	Namespace  string            `json:"namespace"`
	Dependants []DependantPods   `json:"dependantpods"`
}

type DependantPods struct {
	Name     string                `json:"name,omitempty"`
	Selector *metav1.LabelSelector `json:"selector"`
}

type Controller struct {
	clientset         *kubernetes.Clientset
	endpointInformer  cache.SharedIndexInformer
	endpointLister    listerv1.EndpointsLister
	workqueue         workqueue.RateLimitingInterface
	hasSynced         cache.InformerSynced
	stopCh            <-chan struct{}
	serviceDependants *ServiceDependants
	watchDuration     time.Duration
	cancelFn          map[string]context.CancelFunc
	contextCh         chan contextMessage
}

type contextMessage struct {
	key      string
	cancelFn context.CancelFunc
}
