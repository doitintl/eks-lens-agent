package usage

import (
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPodInfo(t *testing.T) {
	type args struct {
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
			got := NewPodInfo(tt.args.pod, tt.args.beginTime, tt.args.endTime, tt.args.node)

			if got.Name != tt.want.Name {
				t.Errorf("NewPodInfo().Name = %v, want %v", got.Name, tt.want.Name)
			}

			if got.Namespace != tt.want.Namespace {
				t.Errorf("NewPodInfo().Namespace = %v, want %v", got.Namespace, tt.want.Namespace)
			}

			if got.QosClass != tt.want.QosClass {
				t.Errorf("NewPodInfo().QosClass = %v, want %v", got.QosClass, tt.want.QosClass)
			}

			if !reflect.DeepEqual(got.Allocations, tt.want.Allocations) {
				t.Errorf("NewPodInfo().Allocations = %v, want %v", got.Allocations, tt.want.Allocations)
			}

			if !reflect.DeepEqual(got.Resources, tt.want.Resources) {
				t.Errorf("NewPodInfo().Resources = %v, want %v", got.Resources, tt.want.Resources)
			}

			if got.BeginTime != tt.want.BeginTime {
				t.Errorf("NewPodInfo().BegibTime = %v, want %v", got.BeginTime, tt.want.BeginTime)
			}

			if got.EndTime != tt.want.EndTime {
				t.Errorf("NewPodInfo().EndTime = %v, want %v", got.EndTime, tt.want.EndTime)
			}

			if !reflect.DeepEqual(got.Labels, tt.want.Labels) {
				t.Errorf("NewPodInfo().Labels = %v, want %v", got.Labels, tt.want.Labels)
			}

			if !reflect.DeepEqual(got.Node, tt.want.Node) {
				t.Errorf("NewPodInfo().Node = %v, want %v", got.Node, tt.want.Node)
			}
		})
	}
}
