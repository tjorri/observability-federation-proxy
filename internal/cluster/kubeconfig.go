package cluster

import (
	"encoding/base64"
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tjorri/observability-federation-proxy/internal/config"
)

// kubeconfigRESTConfig wraps rest.Config to implement RESTConfig interface.
type kubeconfigRESTConfig struct {
	*rest.Config
}

func (c *kubeconfigRESTConfig) Host() string {
	return c.Config.Host
}

func (r *Registry) createKubeconfigCluster(cfg config.ClusterConfig) (*Cluster, error) {
	if cfg.Kubeconfig == nil {
		return nil, fmt.Errorf("kubeconfig config is required for kubeconfig cluster type")
	}

	var restCfg *rest.Config
	var err error

	if cfg.Kubeconfig.Data != "" {
		// Load from inline base64-encoded kubeconfig
		data, err := base64.StdEncoding.DecodeString(cfg.Kubeconfig.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode kubeconfig data: %w", err)
		}
		restCfg, err = clientcmd.RESTConfigFromKubeConfig(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse kubeconfig data: %w", err)
		}
	} else if cfg.Kubeconfig.Path != "" {
		// Load from file path
		data, err := os.ReadFile(cfg.Kubeconfig.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read kubeconfig file: %w", err)
		}
		restCfg, err = clientcmd.RESTConfigFromKubeConfig(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse kubeconfig file: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either kubeconfig.path or kubeconfig.data is required")
	}

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Cluster{
		Name:       cfg.Name,
		Config:     cfg,
		Client:     client,
		restConfig: &kubeconfigRESTConfig{restCfg},
	}, nil
}
