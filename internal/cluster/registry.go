package cluster

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"

	"github.com/tjorri/observability-federation-proxy/internal/config"
)

// Registry manages Kubernetes clients for multiple clusters.
type Registry struct {
	clusters map[string]*Cluster
	mu       sync.RWMutex
}

// Cluster represents a connected Kubernetes cluster.
type Cluster struct {
	Name       string
	Config     config.ClusterConfig
	Client     kubernetes.Interface
	restConfig RESTConfig
}

// RESTConfig abstracts the REST config for testing.
type RESTConfig interface {
	Host() string
}

// NewRegistry creates a new cluster registry from configuration.
func NewRegistry(ctx context.Context, configs []config.ClusterConfig) (*Registry, error) {
	r := &Registry{
		clusters: make(map[string]*Cluster),
	}

	for _, cfg := range configs {
		cluster, err := r.createCluster(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster %s: %w", cfg.Name, err)
		}
		r.clusters[cfg.Name] = cluster
		log.Info().
			Str("cluster", cfg.Name).
			Str("type", cfg.Type).
			Msg("registered cluster")
	}

	return r, nil
}

// Get returns a cluster by name.
func (r *Registry) Get(name string) (*Cluster, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clusters[name]
	return c, ok
}

// List returns all cluster names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		names = append(names, name)
	}
	return names
}

// HealthCheck checks connectivity to all clusters.
func (r *Registry) HealthCheck(ctx context.Context) map[string]error {
	r.mu.RLock()
	// Copy cluster references while holding the lock
	clusters := make(map[string]*Cluster, len(r.clusters))
	for name, cluster := range r.clusters {
		clusters[name] = cluster
	}
	r.mu.RUnlock()

	results := make(map[string]error)
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	for name, cluster := range clusters {
		wg.Add(1)
		go func(name string, cluster *Cluster) {
			defer wg.Done()
			_, err := cluster.Client.Discovery().ServerVersion()
			resultsMu.Lock()
			results[name] = err
			resultsMu.Unlock()
		}(name, cluster)
	}

	wg.Wait()
	return results
}

func (r *Registry) createCluster(ctx context.Context, cfg config.ClusterConfig) (*Cluster, error) {
	switch cfg.Type {
	case "eks":
		return r.createEKSCluster(ctx, cfg)
	case "kubeconfig":
		return r.createKubeconfigCluster(cfg)
	default:
		return nil, fmt.Errorf("unknown cluster type: %s", cfg.Type)
	}
}
