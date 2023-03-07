package config

type Config struct {
	// Kubeconfig is the path to the kubeconfig file
	Kubeconfig string
}

var cfg *Config

func Get() Config {
	if cfg != nil {
		return *cfg
	}
	cfg = &Config{}
	return *cfg
}
