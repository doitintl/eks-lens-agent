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
	Run(ctx context.Context) error
}

type scanner struct {
	log          *logrus.Entry
	client       *kubernetes.Clientset
	uploader     firehose.Uploader
	nodeInformer NodesInformer
	deletedPods  []*usage.PodInfo
}

func New(log *logrus.Entry, client *kubernetes.Clientset, uploader firehose.Uploader, informer NodesInformer) Scanner {
	return &scanner{
		log:          log,
		client:       client,
		uploader:     uploader,
		nodeInformer: informer,
		deletedPods:  make([]*usage.PodInfo, 0),
	}
}

func (s *scanner) DeletePod(obj interface{}) {
	pod := obj.(*v1.Pod)
	s.log.WithFields(logrus.Fields{
		"namespace": pod.Namespace,
		"name":      pod.Name,
	}).Debug("pod deleted")
	// skip "Failed" pods (e.g. DaemonSet pods on Fargate)
	if pod.Status.Phase == v1.PodFailed {
		s.log.WithFields(logrus.Fields{
			"namespace": pod.Namespace,
			"name":      pod.Name,
			"reason":    pod.Status.Reason,
		}).Debug("skipped failed pod ")
		return
	}
	// get the node info from the cache
	node, ok := s.nodeInformer.GetNode(pod.Spec.NodeName)
	if !ok {
		s.log.Warnf("getting node %s from cache", pod.Spec.NodeName)
	}
	// convert PodInfo to usage record
	now := time.Now()
	beginTime := now.Add(-syncPeriod)
	record := usage.GetPodInfo(s.log, pod, beginTime, now, node)
	// keep the record till the next sync period
	s.deletedPods = append(s.deletedPods, record)
}

func (s *scanner) Run(ctx context.Context) error {
	// Create a new PodInfo shared informer
	podInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list running pods only
				options.FieldSelector = "status.phase=Running"
				return s.client.CoreV1().Pods("").List(context.Background(), options) //nolint:wrapcheck
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return s.client.CoreV1().Pods("").Watch(context.Background(), options) //nolint:wrapcheck
			},
			DisableChunking: true,
		},
		&v1.Pod{},
		syncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	// on delete upload PodInfo record with entTime	(now)
	_, err := podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{DeleteFunc: s.DeletePod})
	if err != nil {
		return errors.Wrap(err, "adding pod informer event handler")
	}

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
				node, ok := s.nodeInformer.GetNode(pod.Spec.NodeName)
				if !ok {
					s.log.Warnf("getting node %s from cache", pod.Spec.NodeName)
				}
				record := usage.GetPodInfo(s.log, pod, beginTime, now, node)
				records = append(records, record)
			}
			// add deleted pods and clear the list if any
			if len(s.deletedPods) > 0 {
				s.log.WithField("count", len(s.deletedPods)).Debug("adding deleted pods to the pod records")
				records = append(records, s.deletedPods...)
				s.deletedPods = make([]*usage.PodInfo, 0)
			}
			// upload the records to EKS Lens
			s.log.WithField("count", len(records)).Debug("uploading pod records to EKS Lens")
			err = s.uploader.Upload(ctx, records)
			if err != nil {
				s.log.WithError(err).Error("uploading pods records to EKS Lens")
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
}
