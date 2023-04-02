package controller

import (
	"context"
	"testing"
	"time"

	"github.com/doitintl/eks-lens-agent/internal/usage"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodesInformerLoad(t *testing.T) {
	type testCase struct {
		name          string
		initialNodes  []string
		addNodes      []string
		removeNodes   []string
		expectedNodes []string
	}

	testCases := []testCase{
		{
			name:          "Test initial 4 nodes",
			initialNodes:  []string{"node1", "node2", "node3", "node4"},
			addNodes:      []string{},
			removeNodes:   []string{},
			expectedNodes: []string{"node1", "node2", "node3", "node4"},
		},
		{
			name:          "Test adding nodes",
			initialNodes:  []string{"node1", "node2"},
			addNodes:      []string{"node3", "node4"},
			removeNodes:   []string{},
			expectedNodes: []string{"node1", "node2", "node3", "node4"},
		},
		{
			name:          "Test removing nodes",
			initialNodes:  []string{"node1", "node2", "node3", "node4"},
			addNodes:      []string{},
			removeNodes:   []string{"node2", "node4"},
			expectedNodes: []string{"node1", "node3"},
		},
		{
			name:          "Test adding and removing nodes",
			initialNodes:  []string{"node1", "node2"},
			addNodes:      []string{"node3", "node4"},
			removeNodes:   []string{"node1", "node3"},
			expectedNodes: []string{"node2", "node4"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fake clientset
			clientset := fake.NewSimpleClientset()

			// create fake nodes in the cluster
			for _, node := range tc.initialNodes {
				clientset.CoreV1().Nodes().Create(context.Background(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: node}}, metav1.CreateOptions{})
			}
			// add nodes to the cluster
			for _, node := range tc.addNodes {
				clientset.CoreV1().Nodes().Create(context.Background(), &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: node}}, metav1.CreateOptions{})
			}
			// remove nodes from the cluster
			for _, node := range tc.removeNodes {
				clientset.CoreV1().Nodes().Delete(context.Background(), node, metav1.DeleteOptions{})
			}

			// Initialize the NodesMap
			nodesInformer := &NodesMap{
				data: make(map[string]usage.NodeInfo),
			}

			// Load the nodes using the fake clientset
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
			defer cancel()
			loaded := nodesInformer.Load(ctx, "test-cluster", clientset)

			// Check if the nodes are loaded
			select {
			case <-loaded:
				// Nodes are loaded
			case <-ctx.Done():
				// Loading didn't finish in time
				t.Fatal("Loading nodes didn't finish in time")
			}

			// Get the nodes from the NodesMap
			actualNodes := make([]string, 0, len(nodesInformer.data))
			for _, node := range nodesInformer.data {
				actualNodes = append(actualNodes, node.Name)
			}

			// assert that actual nodes are the same as expected nodes
			assert.ElementsMatch(t, tc.expectedNodes, actualNodes)
		})
	}
}
