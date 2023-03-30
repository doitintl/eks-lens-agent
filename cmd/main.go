package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/doitintl/eks-lens-agent/internal/aws/firehose"
	"github.com/doitintl/eks-lens-agent/internal/config"
	"github.com/doitintl/eks-lens-agent/internal/controller"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
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
	// load nodes
	nodesInformer := controller.NewNodesInformer()
	loaded := nodesInformer.Load(ctx, cluster, clientset)
	// wait for nodes to be loaded
	<-loaded

	// create controller and run it
	scanner := controller.New(clientset, uploader)
	err := scanner.Run(ctx, log, nodesInformer)
	if err != nil {
		return errors.Wrap(err, "running scanner controller")
	}
	return nil
}

func run(ctx context.Context, log *logrus.Entry, cfg config.Config) error {
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

	err = runController(ctx, cfg.ClusterName, log, clientset, uploader)
	if err != nil {
		return errors.Wrap(err, "running controller")
	}

	log.Infof("eks-lens agent stopped")
	return nil
}

func mainCmd(c *cli.Context) error {
	ctx := signals.SetupSignalHandler()
	cfg := config.LoadConfig(c)

	logger := logrus.New()
	log := logger.WithFields(logrus.Fields{
		"cluster": cfg.ClusterName,
		"version": version,
	})

	if err := run(ctx, log, cfg); err != nil {
		log.Fatalf("eks-lens agent failed: %v", err)
	}

	return nil
}

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "custer-name",
				Usage:    "EKS cluster name",
				Required: true,
				EnvVars:  []string{"CLUSTER_NAME"},
			},
			&cli.StringFlag{
				Name:     "kubeconfig",
				Usage:    "Path to kubeconfig file",
				Required: true,
				EnvVars:  []string{"KUBECONFIG"},
			},
			&cli.StringFlag{
				Name:     "stream-name",
				Usage:    "Amazon Kinesis Data Stream name",
				Required: true,
				EnvVars:  []string{"STREAM_NAME"},
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
	kubeconfig, err := kubeConfigFromPath(cfg.KubeConfigPath)
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
