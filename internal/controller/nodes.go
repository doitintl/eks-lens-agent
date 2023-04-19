package controller

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/doitintl/eks-lens-agent/internal/usage"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NodesInformer interface {
	Load(ctx context.Context, cluster string, clientset kubernetes.Interface) chan bool
	GetNode(nodeName string) (*usage.NodeInfo, bool)
}

type NodesMap struct {
	mu   sync.RWMutex
	data map[string]usage.NodeInfo
}

func NewNodesInformer() NodesInformer {
	return &NodesMap{
		data: make(map[string]usage.NodeInfo),
	}
}

func (n *NodesMap) GetNode(nodeName string) (*usage.NodeInfo, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	nodeInfo, ok := n.data[nodeName]
	return &nodeInfo, ok
}

// Load loads the NodesMap with the current nodes in the cluster return channel to signal when the map is loaded
func (n *NodesMap) Load(ctx context.Context, cluster string, clientset kubernetes.Interface) chan bool {
	// Create a new Node informer
	nodeInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (object runtime.Object, err error) {
				return clientset.CoreV1().Nodes().List(context.Background(), options) //nolint:wrapcheck
			},
			WatchFunc: func(options metav1.ListOptions) (retWc watch.Interface, err error) {
				return clientset.CoreV1().Nodes().Watch(context.Background(), options) //nolint:wrapcheck
			},
		},
		&v1.Node{},
		0, // resyncPeriod
		cache.Indexers{},
	)

	// create stopper channel
	stopper := make(chan struct{})
	defer close(stopper)

	// Start the Node informer
	go nodeInformer.Run(stopper)

	// Wait for the Node informer to sync
	if !cache.WaitForCacheSync(make(chan struct{}), nodeInformer.HasSynced) {
		log.Panicf("Error syncing node informer cache")
	}

	// Process Node add and delete events
	_, err := nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			nodeInfo := usage.NodeInfoFromNode(cluster, node)
			n.mu.Lock()
			defer n.mu.Unlock()
			n.data[node.Name] = nodeInfo
		},
		DeleteFunc: func(obj interface{}) {
			node, ok := obj.(*v1.Node)
			if !ok {
				// The object is of an unexpected type
				return
			}
			n.mu.Lock()
			defer n.mu.Unlock()
			delete(n.data, node.Name)
		},
	})
	if err != nil {
		log.Panicf("Error adding event handler to node informer: %v", err)
	}

	// Create a channel to signal when the map is loaded
	loaded := make(chan bool)

	// Update the Node map periodically
	previousResourceVersion := "0" // the resource version of the nodes at the last sync
	go func() {
		ticker := time.NewTicker(syncPeriod)
		// abort if the context is cancelled
		defer ticker.Stop()

		// refresh the nodes map and send to the loaded channel (if not already sent)
		refresh := func() {
			n.mu.Lock()
			defer n.mu.Unlock()
			// Get the latest resource version of the nodes
			lastSyncResourceVersion := nodeInformer.LastSyncResourceVersion()
			// if not different from the current resource version of the nodes, skip
			if lastSyncResourceVersion == previousResourceVersion {
				return
			}

			// If the nodes have been updated, update the nodes map
			for _, obj := range nodeInformer.GetStore().List() {
				node := obj.(*v1.Node)
				n.data[node.Name] = usage.NodeInfoFromNode(cluster, node)
			}

			// Update the previous resource version
			previousResourceVersion = lastSyncResourceVersion

			// non-blocking send to the loaded channel
			select {
			case loaded <- true:
				// channel is empty, send
			default:
				// channel is full, skip
			}
		}

		// refresh the nodes map once before starting the ticker
		refresh()

		// loop until the context is cancelled
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}()

	return loaded
}
