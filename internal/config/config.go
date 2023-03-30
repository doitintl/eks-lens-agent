package config

import (
	"github.com/urfave/cli/v2"
)

type Config struct {
	// KubeConfigPath is the path to the kubeconfig file
	KubeConfigPath string `json:"kubeconfig"`
	// ClusterName is the name of the EKS cluster
	ClusterName string `json:"cluster-name"`
	// Amazon Kinesis Data Stream name
	StreamName string `json:"stream-name"`
}

func LoadConfig(c *cli.Context) Config {
	var cfg Config
	cfg.KubeConfigPath = c.String("kubeconfig")
	cfg.ClusterName = c.String("cluster-name")
	cfg.StreamName = c.String("stream-name")
	return cfg
}
