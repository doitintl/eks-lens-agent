package controller

import (
	"context"
	"time"

	"github.com/doitintl/eks-lens-agent/internal/aws/firehose"
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
	syncPeriod = 15 * time.Minute
)

type Scanner interface {
	Run(ctx context.Context, log *logrus.Entry, informer NodesInformer) error
}

type scanner struct {
	client   *kubernetes.Clientset
	uploader firehose.Uploader
}

func New(client *kubernetes.Clientset, uploader firehose.Uploader) Scanner {
	return &scanner{
		client:   client,
		uploader: uploader,
	}
}

func (s *scanner) Run(ctx context.Context, log *logrus.Entry, nodeInformer NodesInformer) error {
	// Create a new PodInfo shared informer
	podInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list running pods only
				options.FieldSelector = "status.phase=Running"
				return s.client.CoreV1().Pods("").List(context.Background(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return s.client.CoreV1().Pods("").Watch(context.Background(), options)
			},
			DisableChunking: true,
		},
		&v1.Pod{},
		syncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	// on delete upload PodInfo record with entTime	(now)
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			log.WithFields(logrus.Fields{
				"namespace": pod.Namespace,
				"name":      pod.Name,
			}).Debug("pod deleted")
			// get the node info from the cache
			node, ok := nodeInformer.GetNode(pod.Spec.NodeName)
			if !ok {
				log.Warnf("getting node %s from cache", pod.Spec.NodeName)
			}

			// convert PodInfo to usage record
			endTime := time.Now()
			beginTime := endTime.Add(-60 * time.Minute)
			record := usage.NewPodInfo(*pod, beginTime, endTime, node)
			// upload the record to EKS Lens
			err := s.uploader.UploadOne(ctx, record)
			if err != nil {
				log.WithError(err).Error("uploading pod")
			}
		},
	})

	// start the pod informer
	stopper := make(chan struct{})
	defer close(stopper)
	go podInformer.Run(stopper)

	// wait for the cache to sync
	if !cache.WaitForCacheSync(stopper, podInformer.HasSynced) {
		return errors.New("failed to sync cache")
	}

	// get pod list from the cache every "syncPeriod" minutes
	ticker := time.NewTicker(syncPeriod)
	defer ticker.Stop()
	for {
		upload := func() {
			// get the list of pods from the cache
			pods := podInformer.GetStore().List()
			// convert PodInfo to usage record
			now := time.Now()
			beginTime := now.Add(-syncPeriod)
			records := make([]*usage.PodInfo, 0, len(pods))
			for _, obj := range pods {
				pod := obj.(*v1.Pod)
				// get the node info from the cache
				node, ok := nodeInformer.GetNode(pod.Spec.NodeName)
				if !ok {
					log.Warnf("getting node %s from cache", pod.Spec.NodeName)
				}
				record := usage.NewPodInfo(*pod, beginTime, now, node)
				records = append(records, record)
			}
			// upload the records to EKS Lens
			err := s.uploader.Upload(ctx, records)
			if err != nil {
				log.WithError(err).Error("uploading pods to EKS Lens")
			}
		}

		// upload first time
		upload()

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			upload()
		}
	}

	return nil
}
