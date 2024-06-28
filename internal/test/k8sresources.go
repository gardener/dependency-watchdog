package test

import (
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"time"
)

type NodeSpec struct {
	Name        string
	Annotations map[string]string
	Labels      map[string]string
	Conditions  []corev1.NodeCondition
}

type NodeLeaseSpec struct {
	Name          string
	IsExpired     bool
	IsOwnerRefSet bool
}

type MachineSpec struct {
	Name          string
	Labels        map[string]string
	CurrentStatus v1alpha1.CurrentStatus
	Namespace     string
}

// GenerateDeployment generates a deployment object with the given parameters.
func GenerateDeployment(name, namespace, imageName string, replicas int32, annotations map[string]string) *appsv1.Deployment {
	labels := map[string]string{"app": name}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Volumes:        nil,
					InitContainers: nil,
					Containers:     []corev1.Container{{Name: name, Image: imageName, Ports: []corev1.ContainerPort{{ContainerPort: 80}}}},
				},
			},
		},
	}
}

func GenerateNodeLeases(leaseSpecs []NodeLeaseSpec) []*coordinationv1.Lease {
	var leases []*coordinationv1.Lease
	for _, leaseSpec := range leaseSpecs {
		leases = append(leases, createNodeLease(leaseSpec.Name, leaseSpec.IsExpired))
	}
	return leases
}

func GenerateNodes(nodeSpecs []NodeSpec) []*corev1.Node {
	var nodes []*corev1.Node
	for _, nodeSpec := range nodeSpecs {
		nodes = append(nodes, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        nodeSpec.Name,
				Annotations: nodeSpec.Annotations,
				Labels:      nodeSpec.Labels,
			},
			Status: corev1.NodeStatus{
				Conditions: nodeSpec.Conditions,
			},
		})
	}
	return nodes
}

func GenerateMachines(machineSpecs []MachineSpec, namespace string) []*v1alpha1.Machine {
	var machines []*v1alpha1.Machine
	for _, machineSpec := range machineSpecs {
		machines = append(machines, &v1alpha1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      machineSpec.Name,
				Namespace: namespace,
				Labels:    machineSpec.Labels,
			},
			Status: v1alpha1.MachineStatus{
				CurrentStatus: machineSpec.CurrentStatus,
			},
		})
	}
	return machines
}

func createNodeLease(name string, isExpired bool) *coordinationv1.Lease {
	lease := coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "kube-node-lease",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Node",
					Name:       name,
				},
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &name,
			LeaseDurationSeconds: pointer.Int32(40),
		},
	}
	if isExpired {
		renewTime := metav1.NewMicroTime(time.Now().Add(-time.Minute))
		lease.Spec.RenewTime = &renewTime
	} else {
		renewTime := metav1.NewMicroTime(time.Now().Add(-10 * time.Second))
		lease.Spec.RenewTime = &renewTime
	}
	return &lease
}
