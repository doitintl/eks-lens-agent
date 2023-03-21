package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/doitintl/eks-lens-agent/internal/aws/firehose"
	"github.com/doitintl/eks-lens-agent/internal/config"
	"github.com/doitintl/eks-lens-agent/internal/usage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	version      = "dev"
	buildDate    = "unknown"
	gitCommit    = "dirty"
	gitBranch    = "master"
	errEmptyPath = errors.New("empty path")
)

func runController(ctx context.Context, cluster string, log *logrus.Entry, clientset *kubernetes.Clientset, uploader firehose.Uploader) error {
	// get all pods in all namespaces
	log.Debug("listing pods")
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "listing pods")
	}
	// listing nodes
	log.Debug("listing nodes")
	nodesList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "listing nodes")
	}
	nodes := usage.NodeListToMap(cluster, nodesList)
	// set end time to now
	endTime := time.Now()
	// set begin time to 60 minutes ago
	beginTime := endTime.Add(-60 * time.Minute)
	// allocate slice for pod usage records
	records := make([]*usage.Pod, 0, len(pods.Items))
	// collect pod info for all pods
	for _, pod := range pods.Items {
		record := &usage.Pod{}
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
		// get node by pod's node name
		node, ok := nodes[pod.Spec.NodeName]
		if !ok {
			log.Warnf("node %q not found", pod.Spec.NodeName)
		} else {
			record.Node = node
		}
		// append to records
		records = append(records, record)
	}
	// send collected records to EKS Lens
	log.Debug("uploading records")
	err = uploader.Upload(ctx, records)
	if err != nil {
		return errors.Wrap(err, "uploading records")
	}
	return nil
}

func run(ctx context.Context, cluster string, log *logrus.Entry, cfg config.Config) error {
	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	log.Infof("eks-lens agent started")

	restconfig, err := retrieveKubeConfig(log, cfg)
	if err != nil {
		return errors.Wrap(err, "retrieving kube config")
	}

	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return errors.Wrap(err, "initializing kubernetes client")
	}

	uploader, err := firehose.NewUploader(ctx, cfg.StreamName)
	if err != nil {
		return errors.Wrap(err, "initializing firehose uploader")
	}

	err = runController(ctx, cluster, log, clientset, uploader)
	if err != nil {
		return errors.Wrap(err, "running controller")
	}

	log.Infof("eks-lens agent stopped")
	return nil
}

func mainCmd(c *cli.Context) error {
	ctx := signals.SetupSignalHandler()
	logger := logrus.New()
	log := logger.WithField("version", version)
	cfg := config.Get()

	if err := run(ctx, c.String("cluster-name"), log, cfg); err != nil {
		log.Fatalf("eks-lens agent failed: %v", err)
	}

	return nil
}

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "bool",
				Value: true,
				Usage: "boolean app flag",
			},
			&cli.StringFlag{
				Name:  "custer-name",
				Usage: "EKS cluster name",
			},
		},
		Name:    "eks-lens-agent",
		Usage:   "eks-lens-agent is a data collection agent for EKS Lens",
		Action:  mainCmd,
		Version: version,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("eks-lens-agent %s\n", version)
		fmt.Printf("  Build date: %s\n", buildDate)
		fmt.Printf("  Git commit: %s\n", gitCommit)
		fmt.Printf("  Git branch: %s\n", gitBranch)
		fmt.Printf("  Built with: %s\n", runtime.Version())
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}
}

func kubeConfigFromPath(kubepath string) (*rest.Config, error) {
	if kubepath == "" {
		return nil, errEmptyPath
	}

	data, err := os.ReadFile(kubepath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading kubeconfig at %s", kubepath)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, errors.Wrapf(err, "building rest config from kubeconfig at %s", kubepath)
	}

	return cfg, nil
}

func retrieveKubeConfig(log logrus.FieldLogger, cfg config.Config) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromPath(cfg.Kubeconfig)
	if err != nil && !errors.Is(err, errEmptyPath) {
		return nil, errors.Wrap(err, "retrieving kube config from path")
	}

	if kubeconfig != nil {
		log.Debug("using kube config from env variables")
		return kubeconfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving in cluster kube config")
	}
	log.Debug("using in cluster kube config")
	return inClusterConfig, nil
}
