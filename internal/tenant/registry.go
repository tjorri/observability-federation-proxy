package tenant

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/tjorri/observability-federation-proxy/internal/cluster"
	"github.com/tjorri/observability-federation-proxy/internal/config"
)

// Registry manages tenant watchers for multiple clusters.
type Registry struct {
	watchers map[string]*Watcher
	mu       sync.RWMutex
}

// NewRegistry creates a new tenant registry from cluster configuration.
func NewRegistry(ctx context.Context, clusterRegistry *cluster.Registry, configs []config.ClusterConfig) (*Registry, error) {
	r := &Registry{
		watchers: make(map[string]*Watcher),
	}

	for _, cfg := range configs {
		// Get the cluster from registry
		c, ok := clusterRegistry.Get(cfg.Name)
		if !ok {
			log.Warn().Str("cluster", cfg.Name).Msg("cluster not found in registry, skipping tenant watcher")
			continue
		}

		// Create watcher
		watcher, err := NewWatcher(WatcherConfig{
			ClusterName:     cfg.Name,
			Client:          c.Client,
			IncludePatterns: cfg.Tenants.IncludePatterns,
			ExcludePatterns: cfg.Tenants.ExcludePatterns,
			RefreshInterval: cfg.Tenants.RefreshInterval,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create tenant watcher for cluster %s: %w", cfg.Name, err)
		}

		r.watchers[cfg.Name] = watcher
		log.Info().
			Str("cluster", cfg.Name).
			Int("include_patterns", len(cfg.Tenants.IncludePatterns)).
			Int("exclude_patterns", len(cfg.Tenants.ExcludePatterns)).
			Msg("created tenant watcher")
	}

	return r, nil
}

// Start starts all tenant watchers.
func (r *Registry) Start(ctx context.Context) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, watcher := range r.watchers {
		log.Info().Str("cluster", name).Msg("starting tenant watcher")
		watcher.StartAsync(ctx)
	}
}

// Stop stops all tenant watchers.
func (r *Registry) Stop() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, watcher := range r.watchers {
		log.Info().Str("cluster", name).Msg("stopping tenant watcher")
		watcher.Stop()
	}
}

// Get returns the tenant watcher for a cluster.
func (r *Registry) Get(clusterName string) (*Watcher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.watchers[clusterName]
	return w, ok
}

// Tenants returns the list of tenants for a cluster.
func (r *Registry) Tenants(clusterName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w, ok := r.watchers[clusterName]
	if !ok {
		return nil
	}
	return w.Tenants()
}

// BuildOrgIDHeader builds the X-Scope-OrgID header for a cluster.
func (r *Registry) BuildOrgIDHeader(clusterName string, maxLength int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w, ok := r.watchers[clusterName]
	if !ok {
		return ""
	}
	return w.BuildOrgIDHeader(maxLength)
}

// List returns all cluster names with tenant watchers.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.watchers))
	for name := range r.watchers {
		names = append(names, name)
	}
	return names
}

// TenantCounts returns tenant counts for all clusters.
func (r *Registry) TenantCounts() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int, len(r.watchers))
	for name, watcher := range r.watchers {
		counts[name] = watcher.TenantCount()
	}
	return counts
}
