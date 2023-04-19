package usage

import (
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type Ask struct {
	CPU              int64 `json:"cpu,omitempty"`
	Memory           int64 `json:"memory,omitempty"`
	Storage          int64 `json:"storage,omitempty"`
	StorageEphemeral int64 `json:"storageEphemeral,omitempty"`
	GPU              int64 `json:"gpu,omitempty"`
}

type Resources struct {
	Requests Ask `json:"requests,omitempty"`
	Limits   Ask `json:"limits,omitempty"`
}

type Capacity struct {
	// CPU millicores
	CPU int64 `json:"cpu"`
	// GPU int64 `json:"gpu"`
	GPU int64 `json:"gpu,omitempty"`
	// memory Kibibytes
	Memory int64 `json:"memory"`
	// maximum number of Pods (depends on number of ENI
	Pods int64 `json:"pods,omitempty"`
	// local storage in Kibibytes
	Storage int64 `json:"storage,omitempty"`
	// ephemeral storage in Kibibytes
	StorageEphemeral int64 `json:"storageEphemeral,omitempty"`
}

type NodeInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Cluster      string `json:"cluster"`
	Nodegroup    string `json:"nodegroup,omitempty"`
	InstanceType string `json:"type,omitempty"`
	// ComputeType: fargate or ec2
	ComputeType string `json:"computeType,omitempty"`
	// CapacityType: SPOT or ON_DEMAND
	CapacityType   string    `json:"capacityType,omitempty"`
	Region         string    `json:"region"`
	Zone           string    `json:"zone"`
	Arch           string    `json:"arch"`
	OS             string    `json:"os"`
	OSImage        string    `json:"osImage"`
	KernelVersion  string    `json:"kernel"`
	KubeletVersion string    `json:"kubelet"`
	Runtime        string    `json:"runtime"`
	Allocatable    Capacity  `json:"allocatable"`
	Capacity       Capacity  `json:"capacity"`
	Created        time.Time `json:"created"`
}

type PodInfo struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Node      NodeInfo          `json:"node"`
	QosClass  string            `json:"qosClass"`
	StartTime time.Time         `json:"startTime"`
	BeginTime time.Time         `json:"beginTime"`
	EndTime   time.Time         `json:"endTime"`
	Resources Resources         `json:"resources,omitempty"`
}

func NodeInfoFromNode(cluster string, node *v1.Node) NodeInfo {
	// get compute type from node label, default to ec2
	computeType := node.GetLabels()["eks.amazonaws.com/compute-type"]
	if computeType == "" {
		computeType = "ec2"
	}
	// get capacity type from node label, default to on-demand
	capacityType := node.GetLabels()["eks.amazonaws.com/capacityType"]
	if capacityType == "" {
		capacityType = "ON_DEMAND"
	}
	// get instance ID from node provider ID
	// EC2: aws:///us-west-2a/i-0f9f9f9f9f9f9f9f9
	// Fargate: aws:///us-west-2d/999999999-55555555555555555555/fargate-ip-192-168-164-24.us-west-2.compute.internal
	id := node.Spec.ProviderID
	if id != "" {
		id = id[strings.LastIndex(id, "/")+1:]
	}

	result := NodeInfo{
		ID:             id,
		Name:           node.GetName(),
		Cluster:        cluster,
		Nodegroup:      node.GetLabels()["eks.amazonaws.com/nodegroup"],
		InstanceType:   node.GetLabels()["node.kubernetes.io/instance-type"],
		ComputeType:    computeType,
		CapacityType:   capacityType,
		Region:         node.GetLabels()["topology.kubernetes.io/region"],
		Zone:           node.GetLabels()["topology.kubernetes.io/zone"],
		Arch:           node.Status.NodeInfo.Architecture,
		OS:             node.Status.NodeInfo.OperatingSystem,
		OSImage:        node.Status.NodeInfo.OSImage,
		KernelVersion:  node.Status.NodeInfo.KernelVersion,
		KubeletVersion: node.Status.NodeInfo.KubeletVersion,
		Runtime:        node.Status.NodeInfo.ContainerRuntimeVersion,
		Allocatable: Capacity{
			GPU:              node.Status.Allocatable.Name("nvidia.com/gpu", resource.DecimalSI).Value(),
			CPU:              node.Status.Allocatable.Cpu().MilliValue(),
			Memory:           node.Status.Allocatable.Memory().Value(),
			Pods:             node.Status.Allocatable.Pods().Value(),
			Storage:          node.Status.Allocatable.Storage().Value(),
			StorageEphemeral: node.Status.Allocatable.StorageEphemeral().Value(),
		},
		Capacity: Capacity{
			GPU:              node.Status.Capacity.Name("nvidia.com/gpu", resource.DecimalSI).Value(),
			CPU:              node.Status.Capacity.Cpu().MilliValue(),
			Memory:           node.Status.Capacity.Memory().Value(),
			Pods:             node.Status.Capacity.Pods().Value(),
			Storage:          node.Status.Capacity.Storage().Value(),
			StorageEphemeral: node.Status.Capacity.StorageEphemeral().Value(),
		},
		Created: node.GetCreationTimestamp().Time,
	}
	return result
}

func NewPodInfo(pod *v1.Pod, beginTime, endTime time.Time, node *NodeInfo) *PodInfo {
	record := &PodInfo{}
	record.Name = pod.GetName()
	record.Namespace = pod.GetNamespace()
	// calculate pod's requested CPU and memory for all containers
	for _, container := range pod.Spec.Containers {
		record.Resources.Requests.CPU += container.Resources.Requests.Cpu().MilliValue()
		record.Resources.Requests.Memory += container.Resources.Requests.Memory().Value()
		record.Resources.Limits.CPU += container.Resources.Limits.Cpu().MilliValue()
		record.Resources.Limits.Memory += container.Resources.Limits.Memory().Value()
	}
	// copy pod labels, skip ending with "-hash"
	record.Labels = make(map[string]string)
	for k, v := range pod.GetLabels() {
		if !strings.HasSuffix(k, "-hash") {
			record.Labels[k] = v
		}
	}
	// copy pod QoS class
	record.QosClass = string(pod.Status.QOSClass)
	// set pod measured time
	record.BeginTime = beginTime
	record.EndTime = endTime
	// copy pod start time
	record.StartTime = pod.Status.StartTime.Time
	// update pod begin time to the earliest pod start time
	if record.StartTime.Before(beginTime) {
		record.BeginTime = record.StartTime
	}
	if node != nil {
		record.Node = *node
	}
	return record
}
