package usage

import (
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
)

type Ask struct {
	CPU     int64 `json:"cpu,omitempty"`
	Memory  int64 `json:"memory,omitempty"`
	Storage int64 `json:"storage,omitempty"`
	GPU     int64 `json:"gpu,omitempty"`
}

type Resources struct {
	Requests Ask `json:"requests,omitempty"`
	Limits   Ask `json:"limits,omitempty"`
}

type Capacity struct {
	// CPU millicores
	CPU int64 // json:"cpu"
	// memory Kibibytes
	Memory int64 // json:"memory"
	// maximum number of Pods (depends on number of ENI
	Pods int64 // json:"pods"
	// ephemeral storage in Kibibytes
	Storage int64 // json:"storage"
}

type Node struct {
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

type Pod struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
	Node      Node              `json:"node"`
	QosClass  string            `json:"qosClass"`
	StartTime time.Time         `json:"startTime"`
	BeginTime time.Time         `json:"beginTime"`
	EndTime   time.Time         `json:"endTime"`
	Resources Resources         `json:"resources,omitempty"`
}

func NodeFromK8s(cluster string, node v1.Node) Node {
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

	result := Node{
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
			CPU:     node.Status.Allocatable.Cpu().MilliValue(),
			Memory:  node.Status.Allocatable.Memory().Value(),
			Pods:    node.Status.Allocatable.Pods().Value(),
			Storage: node.Status.Allocatable.StorageEphemeral().Value(),
		},
		Capacity: Capacity{
			CPU:     node.Status.Capacity.Cpu().MilliValue(),
			Memory:  node.Status.Capacity.Memory().Value(),
			Pods:    node.Status.Capacity.Pods().Value(),
			Storage: node.Status.Capacity.StorageEphemeral().Value(),
		},
		Created: node.GetCreationTimestamp().Time,
	}
	return result
}

func NodeListToMap(cluster string, nodes *v1.NodeList) map[string]Node {
	result := make(map[string]Node, len(nodes.Items))
	for _, node := range nodes.Items {
		result[node.GetName()] = NodeFromK8s(cluster, node)
	}
	return result
}
