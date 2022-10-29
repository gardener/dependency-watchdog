package scaler

import (
	"fmt"
	"time"

	papi "github.com/gardener/dependency-watchdog/api/prober"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

const (
	defaultTimeout       = 10 * time.Second
	defaultInitialDelay  = 10 * time.Millisecond
	deploymentKind       = "Deployment"
	deploymentAPIVersion = "apps/v1"
)

var (
	mcmObjectRef = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "machine-controller-manager", APIVersion: "apps/v1"}
	kcmObjectRef = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "kube-controller-manager", APIVersion: "apps/v1"}
	caObjectRef  = autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "cluster-autoscaler", APIVersion: "apps/v1"}
)

func createTestDeploymentDependentResourceInfo(name string, scaleUpLevel, scaleDownLevel int, timeout *time.Duration, shouldExist bool) papi.DependentResourceInfo {
	if timeout == nil {
		timeout = pointer.Duration(defaultTimeout)
	}
	return papi.DependentResourceInfo{
		Ref:         &autoscalingv1.CrossVersionObjectReference{Name: name, Kind: deploymentKind, APIVersion: deploymentAPIVersion},
		ShouldExist: &shouldExist,
		ScaleUpInfo: &papi.ScaleInfo{
			Level:        scaleUpLevel,
			InitialDelay: &metav1.Duration{Duration: defaultInitialDelay},
			Timeout:      &metav1.Duration{Duration: *timeout},
		},
		ScaleDownInfo: &papi.ScaleInfo{
			Level:        scaleDownLevel,
			InitialDelay: &metav1.Duration{Duration: defaultInitialDelay},
			Timeout:      &metav1.Duration{Duration: *timeout},
		},
	}
}

func createTestScalableResourceInfos(numResInfosByLevel map[int]int) []scalableResourceInfo {
	var resInfos []scalableResourceInfo
	for k, v := range numResInfosByLevel {
		for i := 0; i < v; i++ {
			resInfos = append(resInfos, scalableResourceInfo{
				ref:   &autoscalingv1.CrossVersionObjectReference{Name: fmt.Sprintf("resource-%d%d", k, i)},
				level: k,
			})
		}
	}
	return resInfos
}
