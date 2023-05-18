package usage

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPodInfo(t *testing.T) {
	type args struct {
		log       *logrus.Entry
		pod       *v1.Pod
		beginTime time.Time
		endTime   time.Time
		node      *NodeInfo
	}
	tests := []struct {
		name string
		args args
		want *PodInfo
	}{
		{
			name: "full record test",
			args: args{
				log: logrus.NewEntry(logrus.New()),
				pod: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels: map[string]string{
							"app":        "my-app",
							"version":    "v1",
							"hash":       "abc123",
							"extra-hash": "def456",
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name: "container1",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:     resource.MustParse("100m"),
										v1.ResourceMemory:  resource.MustParse("256Mi"),
										v1.ResourceStorage: resource.MustParse("100Gi"),
									},
									Limits: v1.ResourceList{
										v1.ResourceCPU:     resource.MustParse("200m"),
										v1.ResourceMemory:  resource.MustParse("512Mi"),
										v1.ResourceStorage: resource.MustParse("200Gi"),
									},
								},
							},
							{
								Name: "container2",
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:              resource.MustParse("200m"),
										v1.ResourceMemory:           resource.MustParse("512Mi"),
										"nvidia.com/gpu":            resource.MustParse("1"),
										v1.ResourceEphemeralStorage: resource.MustParse("4Gi"),
									},
									Limits: v1.ResourceList{
										v1.ResourceCPU:              resource.MustParse("1000m"),
										v1.ResourceMemory:           resource.MustParse("1Gi"),
										"nvidia.com/gpu":            resource.MustParse("1"),
										v1.ResourceEphemeralStorage: resource.MustParse("8Gi"),
									},
								},
							},
						},
					},
					Status: v1.PodStatus{
						QOSClass:  v1.PodQOSBurstable,
						StartTime: &metav1.Time{Time: time.Date(2020, 1, 2, 0, 2, 0, 0, time.UTC)},
					},
				},
				node: &NodeInfo{
					Name: "test-node",
					Allocatable: Capacity{
						CPU:              1000,            // 1 core
						Memory:           2 * (1 << 30),   // 2Gi
						GPU:              1,               // 1 GPU
						Storage:          300 * (1 << 30), // 300Gi
						StorageEphemeral: 10 * (1 << 30),  // 10Gi
					},
				},
				beginTime: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
				endTime:   time.Date(2020, 1, 2, 0, 10, 0, 0, time.UTC),
			},
			want: &PodInfo{
				Name:      "test-pod",
				Namespace: "default",
				Labels: map[string]string{
					"app":     "my-app",
					"version": "v1",
					"hash":    "abc123",
				},
				QosClass:  "Burstable",
				StartTime: time.Date(2020, 1, 1, 0, 2, 0, 0, time.UTC),
				BeginTime: time.Date(2020, 1, 2, 0, 2, 0, 0, time.UTC),
				EndTime:   time.Date(2020, 1, 2, 0, 10, 0, 0, time.UTC),
				Resources: Resources{
					Requests: Ask{
						CPU:              300,             // 300m
						Memory:           768 * (1 << 20), // 768Mi
						Storage:          100 * (1 << 30), // 100Gi
						GPU:              1,               // 1 GPU
						StorageEphemeral: 4 * (1 << 30),   // 4Gi
					},
					Limits: Ask{
						CPU:              1200,             // 1200m
						Memory:           1536 * (1 << 20), // 1536Mi
						Storage:          200 * (1 << 30),  // 200Gi
						GPU:              1,                // 1 GPU
						StorageEphemeral: 8 * (1 << 30),    // 8Gi
					},
				},
				Allocations: Allocations{
					Requests: Allocation{
						CPU:              float64(300) / 1000 * 100,
						Memory:           float64(768) / (2 * (1 << 10)) * 100,
						GPU:              100,
						Storage:          float64(100) / 300 * 100,
						StorageEphemeral: float64(4) / 10 * 100,
					},
					Limits: Allocation{
						CPU:              float64(1200) / 1000 * 100,
						Memory:           float64(1536) / (2 * (1 << 10)) * 100,
						GPU:              100,
						Storage:          float64(200) / 300 * 100,
						StorageEphemeral: float64(8) / 10 * 100,
					},
				},
				Node: NodeInfo{
					Name: "test-node",
					Allocatable: Capacity{
						CPU:              1000,            // 1 core
						Memory:           2 * (1 << 30),   // 2Gi
						GPU:              1,               // 1 GPU
						Storage:          300 * (1 << 30), // 300Gi
						StorageEphemeral: 10 * (1 << 30),  // 10Gi
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPodInfo(tt.args.log, tt.args.pod, tt.args.beginTime, tt.args.endTime, tt.args.node)

			if got.Name != tt.want.Name {
				t.Errorf("GetPodInfo().Name = %v, want %v", got.Name, tt.want.Name)
			}

			if got.Namespace != tt.want.Namespace {
				t.Errorf("GetPodInfo().Namespace = %v, want %v", got.Namespace, tt.want.Namespace)
			}

			if got.QosClass != tt.want.QosClass {
				t.Errorf("GetPodInfo().QosClass = %v, want %v", got.QosClass, tt.want.QosClass)
			}

			if !reflect.DeepEqual(got.Allocations, tt.want.Allocations) {
				t.Errorf("GetPodInfo().Allocations = %v, want %v", got.Allocations, tt.want.Allocations)
			}

			if !reflect.DeepEqual(got.Resources, tt.want.Resources) {
				t.Errorf("GetPodInfo().Resources = %v, want %v", got.Resources, tt.want.Resources)
			}

			if got.BeginTime != tt.want.BeginTime {
				t.Errorf("GetPodInfo().BegibTime = %v, want %v", got.BeginTime, tt.want.BeginTime)
			}

			if got.EndTime != tt.want.EndTime {
				t.Errorf("GetPodInfo().EndTime = %v, want %v", got.EndTime, tt.want.EndTime)
			}

			if !reflect.DeepEqual(got.Labels, tt.want.Labels) {
				t.Errorf("GetPodInfo().Labels = %v, want %v", got.Labels, tt.want.Labels)
			}

			if !reflect.DeepEqual(got.Node, tt.want.Node) {
				t.Errorf("GetPodInfo().Node = %v, want %v", got.Node, tt.want.Node)
			}
		})
	}
}

func TestParseCapacityProvisioned(t *testing.T) {
	tests := []struct {
		input                 string
		expectedCPUMilliValue int64
		expectedMemoryValue   int64
		expectedError         error
	}{
		{
			input:                 "0.25 200MB",
			expectedCPUMilliValue: 250,
			expectedMemoryValue:   200 * int64(math.Pow10(6)), // 200MB
			expectedError:         nil,
		},
		{
			input:                 "100m 200Mi",
			expectedCPUMilliValue: 100,
			expectedMemoryValue:   200 * (1 << 20), // 200Mi
			expectedError:         nil,
		},
		{
			input:                 "2vCPU 400MB",
			expectedCPUMilliValue: 2000,
			expectedMemoryValue:   400 * int64(math.Pow10(6)), // 400MB
			expectedError:         nil,
		},
		{
			input:                 "0.25vCPU 0.5GB",
			expectedCPUMilliValue: 250,
			expectedMemoryValue:   5 * int64(math.Pow10(8)), // 0.5GB
			expectedError:         nil,
		},
		{
			input:                 "1 1Gi",
			expectedCPUMilliValue: 1000,
			expectedMemoryValue:   1 << 30, // 1Gi
			expectedError:         nil,
		},
		{
			input:                 "500m",
			expectedCPUMilliValue: 0,
			expectedMemoryValue:   0,
			expectedError:         errors.Errorf("invalid capacity provisioned string: 500m"),
		},
		{
			input:                 "1.5 2Gi",
			expectedCPUMilliValue: 1500,
			expectedMemoryValue:   2 * (1 << 30), // 2Gi
			expectedError:         nil,
		},
	}

	for _, test := range tests {
		cpu, memory, err := parseCapacityProvisioned(test.input)

		if err != nil && test.expectedError == nil {
			t.Errorf("Unexpected error. Input: %s, Error: %v", test.input, err)
		} else if err == nil && test.expectedError != nil {
			t.Errorf("Expected error not returned. Input: %s, Expected Error: %v", test.input, test.expectedError)
		} else if err != nil && test.expectedError != nil && err.Error() != test.expectedError.Error() {
			t.Errorf("Error mismatch. Input: %s, Expected Error: %v, Got Error: %v", test.input, test.expectedError, err)
		}

		if cpu != test.expectedCPUMilliValue {
			t.Errorf("CPU value mismatch. Input: %s, Expected: %d, Got: %d", test.input, test.expectedCPUMilliValue, cpu)
		}

		if memory != test.expectedMemoryValue {
			t.Errorf("Memory value mismatch. Input: %s, Expected: %d, Got: %d", test.input, test.expectedMemoryValue, memory)
		}
	}
}

func TestPatchFargateNodeInfo(t *testing.T) {
	tests := []struct {
		pod                     *v1.Pod
		node                    *NodeInfo
		expectedNodeAllocatable NodeInfo
		expectedError           error
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"CapacityProvisioned": "0.25vCPU 0.5GB",
					},
					Labels: map[string]string{
						"eks.amazonaws.com/fargate-profile": "fp-1",
					},
				},
			},
			node: &NodeInfo{
				ComputeType:  fargateType,
				InstanceType: "fargate-1.0",
				Allocatable: Capacity{
					CPU:    0,
					Memory: 0,
				},
			},
			expectedNodeAllocatable: NodeInfo{
				ComputeType:  fargateType,
				InstanceType: "fargate-0.25vCPU-0.5GB",
				Nodegroup:    "fp-1",
				Allocatable: Capacity{
					CPU:    250,
					Memory: 244 * int64(math.Pow10(6)), // 0.244GB
				},
			},
			expectedError: nil,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"CapacityProvisioned": "invalid",
					},
				},
			},
			node: &NodeInfo{
				ComputeType: fargateType,
				Allocatable: Capacity{
					CPU:    0,
					Memory: 0,
				},
			},
			expectedNodeAllocatable: NodeInfo{
				ComputeType: fargateType,
				Allocatable: Capacity{
					CPU:    0,
					Memory: 0,
				},
			},
			expectedError: errors.Wrap(errors.New("invalid CPU capacity provisioned"), "failed to parse capacity provisioned"),
		},
		{
			pod: &v1.Pod{},
			node: &NodeInfo{
				ComputeType: "non-fargate",
				Allocatable: Capacity{
					CPU:    0,
					Memory: 0,
				},
			},
			expectedNodeAllocatable: NodeInfo{
				ComputeType: "non-fargate",
				Allocatable: Capacity{
					CPU:    0,
					Memory: 0,
				},
			},
			expectedError: nil,
		},
	}

	for _, test := range tests {
		err := patchFargateNodeInfo(test.pod, test.node)

		if err != nil && test.expectedError == nil {
			t.Errorf("Unexpected error. Pod: %+v, Node: %+v, Error: %v", test.pod, test.node, err)
		} else if err == nil && test.expectedError != nil {
			t.Errorf("Expected error not returned. Pod: %+v, Node: %+v, Expected Error: %v", test.pod, test.node, test.expectedError)
		}

		if test.node.Allocatable.CPU != test.expectedNodeAllocatable.Allocatable.CPU {
			t.Errorf("CPU value mismatch. Pod: %+v, Node: %+v, Expected CPU: %d, Got CPU: %d", test.pod, test.node, test.expectedNodeAllocatable.Allocatable.CPU, test.node.Allocatable.CPU)
		}
		if test.node.Allocatable.Memory != test.expectedNodeAllocatable.Allocatable.Memory {
			t.Errorf("Memory value mismatch. Pod: %+v, Node: %+v, Expected Memory: %d, Got Memory: %d", test.pod, test.node, test.expectedNodeAllocatable.Allocatable.Memory, test.node.Allocatable.Memory)
		}
	}
}
