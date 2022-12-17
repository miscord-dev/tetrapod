package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ControlPlane struct {
	APIEndpoint string `json:"apiEndpoint"`
	RootCACert  string `json:"rootCACert"`
	Token       string `json:"-"`
	Namespace   string `json:"namespace"`

	Selector metav1.LabelSelector `json:"selector"`
	SizeBit  int                  `json:"sizeBit"`
}

type Config struct {
	ClusterName string `json:"clusterName"`
	NodeName    string `json:"nodeName"`

	ControlPlane     ControlPlane `json:"controlPlane"`
	NetworkNamespace string       `json:"networkNamespace"`
}

var (
	configPath string

	defaultConfig = Config{
		ClusterName: os.Getenv("KUBE_CLUSTER_NAME"),
		NodeName:    os.Getenv("KUBE_NODE_NAME"),
		ControlPlane: ControlPlane{
			APIEndpoint: os.Getenv("CONTROLPLANE_API_ENDPOINT"),
			RootCACert:  os.Getenv("CONTROLPLANE_API_ROOT_CA_CERT"),
			Token:       os.Getenv("CONTROLPLANE_API_TOKEN"),
			Namespace:   os.Getenv("CONTROLPLANE_NAMESPACE"),
		},
		NetworkNamespace: loadEnvWithDefault("TOXFU_NETNS", "toxfu0"),
	}
)

func init() {
	flag.StringVar(&configPath, "config", "", "Path to toxfu.json")
}

func New() (*Config, error) {
	flag.Parse()

	if configPath == "" {
		return &defaultConfig, nil
	}

	fp, err := os.Open(configPath)

	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer fp.Close()

	cfg := defaultConfig
	if err := json.NewDecoder(fp).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	return &cfg, nil
}
