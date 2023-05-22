package controller

import (
	"context"
	"sync"
	"time"

	"github.com/doitintl/eks-lens-agent/internal/usage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	delayDelete         = 1 * time.Minute
	nodeCacheSyncPeriod = 5 * time.Minute
)

var (
	// ErrCacheSync is returned when the cache fails to sync
	ErrCacheSync = errors.New("failed to sync cache")
)

type NodesInformer interface {
	Load(ctx context.Context, log *logrus.Entry, cluster string, clientset kubernetes.Interface) (chan bool, error)
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
//
//nolint:funlen
func (n *NodesMap) Load(ctx context.Context, log *logrus.Entry, cluster string, clientset kubernetes.Interface) (chan bool, error) {
	// Create a new Node informer
	nodeInformer := cache.NewSharedInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (object runtime.Object, err error) {
				return clientset.CoreV1().Nodes().List(context.Background(), options) //nolint:wrapcheck
			},
			WatchFunc: func(options metav1.ListOptions) (retWc watch.Interface, err error) {
				return clientset.CoreV1().Nodes().Watch(context.Background(), options) //nolint:wrapcheck
			},
		},
		&v1.Node{},
		nodeCacheSyncPeriod,
	)

	// create stopper channel
	stopper := make(chan struct{})

	// Start the Node informer
	go nodeInformer.Run(stopper)

	// Wait for the Node informer to sync
	log.Debug("waiting for node informer to sync")
	if !cache.WaitForCacheSync(make(chan struct{}), nodeInformer.HasSynced) {
		return nil, ErrCacheSync
	}

	// Process Node add and delete events
	_, err := nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			nodeInfo := usage.NodeInfoFromNode(cluster, node)
			n.mu.Lock()
			defer n.mu.Unlock()
			log.WithField("node", node.Name).Debug("adding node to map")
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
			// non-blocking delete from the map after 5 minutes
			go func() {
				time.Sleep(delayDelete)
				n.mu.Lock()
				defer n.mu.Unlock()
				log.WithField("node", node.Name).Debug("deleting node from map after delay")
				delete(n.data, node.Name)
			}()
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to add event handler to node informer")
	}

	// Create a channel to signal when the map is loaded
	loaded := make(chan bool)

	// Update the Node map periodically
	previousResourceVersion := "0" // the resource version of the nodes at the last sync
	go func() {
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

			// clear the nodes map
			log.Debug("refreshing nodes map with latest nodes")
			n.data = make(map[string]usage.NodeInfo)

			// update the nodes map
			for _, obj := range nodeInformer.GetStore().List() {
				node := obj.(*v1.Node)
				log.WithField("node", node.Name).Debug("adding node to map")
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

		// refresh the nodes map periodically
		ticker := time.NewTicker(nodeCacheSyncPeriod)
		defer ticker.Stop()
		for {
			// loop until the context is cancelled
			select {
			case <-ctx.Done():
				// context is cancelled, close the stopper channel to stop the informer
				close(stopper)
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	return loaded, nil
}
