package usage

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	fargateType = "fargate"
)

type Allocation struct {
	// CPU fraction of total CPU
	CPU float64 `json:"cpu"`
	// GPU fraction of total GPU
	GPU float64 `json:"gpu,omitempty"`
	// memory fraction of total memory
	Memory float64 `json:"memory"`
	// storage fraction of total storage
	Storage float64 `json:"storage,omitempty"`
	// ephemeral storage fraction of total ephemeral storage
	StorageEphemeral float64 `json:"storage_ephemeral,omitempty"`
}

type Allocations struct {
	Requests Allocation `json:"requests"`
	Limits   Allocation `json:"limits"`
}

type Ask struct {
	CPU              int64 `json:"cpu,omitempty"`
	Memory           int64 `json:"memory,omitempty"`
	Storage          int64 `json:"storage,omitempty"`
	StorageEphemeral int64 `json:"storage_ephemeral,omitempty"`
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
	StorageEphemeral int64 `json:"storage_ephemeral,omitempty"`
}

type NodeInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Cluster      string `json:"cluster"`
	Nodegroup    string `json:"nodegroup,omitempty"`
	InstanceType string `json:"type,omitempty"`
	// ComputeType: fargate or ec2
	ComputeType string `json:"compute_type,omitempty"`
	// CapacityType: SPOT or ON_DEMAND
	CapacityType   string    `json:"capacity_type,omitempty"`
	Region         string    `json:"region"`
	Zone           string    `json:"zone"`
	Arch           string    `json:"arch"`
	OS             string    `json:"os"`
	OSImage        string    `json:"os_image"`
	KernelVersion  string    `json:"kernel"`
	KubeletVersion string    `json:"kubelet"`
	Runtime        string    `json:"runtime"`
	Allocatable    Capacity  `json:"allocatable"`
	Capacity       Capacity  `json:"capacity"`
	Created        time.Time `json:"created"`
}

type PodInfo struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Node        NodeInfo          `json:"node"`
	QosClass    string            `json:"qos_class"`
	StartTime   time.Time         `json:"start_time"`
	BeginTime   time.Time         `json:"begin_time"`
	EndTime     time.Time         `json:"end_time"`
	Resources   Resources         `json:"resources,omitempty"`
	Allocations Allocations       `json:"allocations,omitempty"`
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

	// get nodegroup from node label, fargate nodegroup is empty
	nodegroup := node.GetLabels()["eks.amazonaws.com/nodegroup"]
	if nodegroup == "" {
		// assume fargate and get fargate profile name from node label
		nodegroup = node.GetLabels()["eks.amazonaws.com/fargate-profile"]
		if nodegroup == "" {
			nodegroup = fargateType
		}
	}

	// get region from node label
	region := node.GetLabels()["topology.kubernetes.io/region"]

	// get zone from node label
	zone := node.GetLabels()["topology.kubernetes.io/zone"]

	// get instance type from node label
	instanceType := node.GetLabels()["beta.kubernetes.io/instance-type"]
	if instanceType == "" {
		instanceType = node.GetLabels()["node.kubernetes.io/instance-type"]
		// if empty, assume fargate and build instance type based on pattern "fargate-vCPU-memoryGB" where memory is rounded to GiB
		if instanceType == "" {
			// get memory in GiB
			memory := float64(node.Status.Capacity.Memory().ScaledValue(resource.Giga))
			// construct instance type based on pattern "fargate-vCPU-memoryGB"
			instanceType = fmt.Sprintf("fargate-%dvCPU-%dGB", node.Status.Capacity.Cpu().Value(), int(memory))
		}
	}

	result := NodeInfo{
		ID:             id,
		Name:           node.GetName(),
		Cluster:        cluster,
		Nodegroup:      nodegroup,
		InstanceType:   instanceType,
		ComputeType:    computeType,
		CapacityType:   capacityType,
		Region:         region,
		Zone:           zone,
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

func GetPodInfo(log *logrus.Entry, pod *v1.Pod, beginTime, endTime time.Time, node *NodeInfo) *PodInfo {
	record := &PodInfo{}
	record.Name = pod.GetName()
	record.Namespace = pod.GetNamespace()
	// calculate pod's requested CPU and memory for all containers
	for _, container := range pod.Spec.Containers {
		record.Resources.Requests.CPU += container.Resources.Requests.Cpu().MilliValue()
		record.Resources.Requests.Memory += container.Resources.Requests.Memory().Value()
		record.Resources.Requests.GPU += container.Resources.Requests.Name("nvidia.com/gpu", resource.DecimalSI).Value()
		record.Resources.Requests.Storage += container.Resources.Requests.Storage().Value()
		record.Resources.Requests.StorageEphemeral += container.Resources.Requests.StorageEphemeral().Value()

		record.Resources.Limits.CPU += container.Resources.Limits.Cpu().MilliValue()
		record.Resources.Limits.Memory += container.Resources.Limits.Memory().Value()
		record.Resources.Limits.GPU += container.Resources.Limits.Name("nvidia.com/gpu", resource.DecimalSI).Value()
		record.Resources.Limits.Storage += container.Resources.Limits.Storage().Value()
		record.Resources.Limits.StorageEphemeral += container.Resources.Limits.StorageEphemeral().Value()
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
	// copy pod start time if available
	if pod.Status.StartTime != nil {
		record.StartTime = pod.Status.StartTime.Time
		// update pod begin time to the earliest pod start time
		if record.StartTime.After(beginTime) {
			record.BeginTime = record.StartTime
		}
	}
	if node != nil {
		// patch fargate node info from pod annotations, if needed
		err := patchFargateNodeInfo(pod, node)
		if err != nil {
			log.WithError(err).WithField("node", node.Name).Warn("failed to patch fargate node info")
		}
		record.Node = *node
		// calculate pod's allocation requests as a percentage of node's allocatable resources
		if node.Allocatable.CPU > 0 {
			record.Allocations.Requests.CPU = float64(record.Resources.Requests.CPU) / float64(node.Allocatable.CPU) * 100 //nolint:gomnd
			record.Allocations.Limits.CPU = float64(record.Resources.Limits.CPU) / float64(node.Allocatable.CPU) * 100     //nolint:gomnd
		}
		if node.Allocatable.Memory > 0 {
			record.Allocations.Requests.Memory = float64(record.Resources.Requests.Memory) / float64(node.Allocatable.Memory) * 100 //nolint:gomnd
			record.Allocations.Limits.Memory = float64(record.Resources.Limits.Memory) / float64(node.Allocatable.Memory) * 100     //nolint:gomnd
		}
		if node.Allocatable.GPU > 0 {
			record.Allocations.Requests.GPU = float64(record.Resources.Requests.GPU) / float64(node.Allocatable.GPU) * 100 //nolint:gomnd
			record.Allocations.Limits.GPU = float64(record.Resources.Limits.GPU) / float64(node.Allocatable.GPU) * 100     //nolint:gomnd
		}
		if node.Allocatable.Storage > 0 {
			record.Allocations.Requests.Storage = float64(record.Resources.Requests.Storage) / float64(node.Allocatable.Storage) * 100 //nolint:gomnd
			record.Allocations.Limits.Storage = float64(record.Resources.Limits.Storage) / float64(node.Allocatable.Storage) * 100     //nolint:gomnd
		}
		if node.Allocatable.StorageEphemeral > 0 {
			record.Allocations.Requests.StorageEphemeral = float64(record.Resources.Requests.StorageEphemeral) / float64(node.Allocatable.StorageEphemeral) * 100 //nolint:gomnd
			record.Allocations.Limits.StorageEphemeral = float64(record.Resources.Limits.StorageEphemeral) / float64(node.Allocatable.StorageEphemeral) * 100     //nolint:gomnd
		}
	}
	return record
}

func patchFargateNodeInfo(pod *v1.Pod, node *NodeInfo) error {
	if node.ComputeType != fargateType {
		return nil
	}
	// get CPU and memory from pod annotation "CapacityProvisioned": "0.25vCPU 0.5GB",
	// and patch node allocatable CPU and memory
	if capacityProvisioned, ok := pod.Annotations["CapacityProvisioned"]; ok {
		cpu, memory, err := parseCapacityProvisioned(capacityProvisioned)
		if err != nil {
			return errors.Wrap(err, "failed to parse capacity provisioned")
		}
		node.Allocatable.CPU = cpu
		// 256MB is reserved for Kubernetes components on Fargate, so we need to subtract it from allocatable memory
		node.Allocatable.Memory = memory - 256*int64(math.Pow10(6)) //nolint:gomnd
		// update node instance type: "fargate-{CapacityProvisioned}", e.g. "fargate-0.25vCPU-0.5GB"
		node.InstanceType = fmt.Sprintf("%s-%s", fargateType, strings.ReplaceAll(capacityProvisioned, " ", "-"))
	}
	// get Fargate profile name from pod label "eks.amazonaws.com/fargate-profile"
	// and patch node nodegroup name
	if fargateProfileName, ok := pod.Labels["eks.amazonaws.com/fargate-profile"]; ok {
		node.Nodegroup = fargateProfileName
	}
	return nil
}

func parseCapacityProvisioned(capacityProvisioned string) (int64, int64, error) {
	// split capacity provisioned string by space
	capacity := strings.Split(capacityProvisioned, " ")
	if len(capacity) != 2 { //nolint:gomnd
		return 0, 0, errors.Errorf("invalid capacity provisioned string: %s", capacityProvisioned)
	}
	// parse CPU
	// remove "vCPU" suffix
	capacity[0] = strings.TrimSuffix(capacity[0], "vCPU")
	// convert CPU value to float and if it is less than 1 multiplies it by 1000 add "m" suffix
	if cpu, err := strconv.ParseFloat(capacity[0], 64); err == nil && cpu < 1 {
		capacity[0] = strconv.Itoa(int(cpu * 1000)) //nolint:gomnd
		capacity[0] += "m"
	}

	cpu, err := resource.ParseQuantity(capacity[0])
	if err != nil {
		return 0, 0, errors.Wrap(err, "invalid CPU capacity provisioned")
	}
	// parse memory
	// remove "B" suffix (GB, MB, KB)
	capacity[1] = strings.TrimSuffix(capacity[1], "B")
	memory, err := resource.ParseQuantity(capacity[1])
	if err != nil {
		return 0, 0, errors.Wrap(err, "invalid memory capacity provisioned")
	}
	return cpu.MilliValue(), memory.Value(), nil
}
