package test

const (
	Worker1Name                 = "worker-1"
	Worker2Name                 = "worker-2"
	NodeConditionDiskPressure   = "DiskPressure"
	NodeConditionMemoryPressure = "MemoryPressure"
	DefaultNamespace            = "test"
)

// Constants for deployments
const (
	MCMDeploymentName = "machine-controller-manager"
	KCMDeploymentName = "kube-controller-manager"
	CADeploymentName  = "cluster-autoscaler"
	DefaultImage      = "registry.k8s.io/pause:3.5"
)

const (
	Node1Name = "node-1"
	Node2Name = "node-2"
	Node3Name = "node-3"
	Node4Name = "node-4"
)

const (
	Machine1Name = "machine-1"
	Machine2Name = "machine-2"
	Machine3Name = "machine-3"
	Machine4Name = "machine-4"
)
