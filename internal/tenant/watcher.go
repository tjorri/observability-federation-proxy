package tenant

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// Watcher watches Kubernetes namespaces and maintains a cached list of tenants.
type Watcher struct {
	clusterName     string
	client          kubernetes.Interface
	includePatterns []*regexp.Regexp
	excludePatterns []*regexp.Regexp
	refreshInterval time.Duration

	informerFactory informers.SharedInformerFactory
	namespaceLister corev1listers.NamespaceLister
	hasSynced       cache.InformerSynced

	tenants []string
	mu      sync.RWMutex

	stopCh chan struct{}
}

// WatcherConfig holds configuration for creating a tenant watcher.
type WatcherConfig struct {
	ClusterName     string
	Client          kubernetes.Interface
	IncludePatterns []string
	ExcludePatterns []string
	RefreshInterval time.Duration
}

// NewWatcher creates a new tenant watcher.
func NewWatcher(cfg WatcherConfig) (*Watcher, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}
	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("cluster name is required")
	}

	refreshInterval := cfg.RefreshInterval
	if refreshInterval == 0 {
		refreshInterval = 60 * time.Second
	}

	// Compile include patterns
	includePatterns := make([]*regexp.Regexp, 0, len(cfg.IncludePatterns))
	for _, pattern := range cfg.IncludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid include pattern %q: %w", pattern, err)
		}
		includePatterns = append(includePatterns, re)
	}

	// Compile exclude patterns
	excludePatterns := make([]*regexp.Regexp, 0, len(cfg.ExcludePatterns))
	for _, pattern := range cfg.ExcludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		excludePatterns = append(excludePatterns, re)
	}

	// Create informer factory with resync period
	informerFactory := informers.NewSharedInformerFactory(cfg.Client, refreshInterval)
	namespaceInformer := informerFactory.Core().V1().Namespaces()

	w := &Watcher{
		clusterName:     cfg.ClusterName,
		client:          cfg.Client,
		includePatterns: includePatterns,
		excludePatterns: excludePatterns,
		refreshInterval: refreshInterval,
		informerFactory: informerFactory,
		namespaceLister: namespaceInformer.Lister(),
		hasSynced:       namespaceInformer.Informer().HasSynced,
		tenants:         []string{},
		stopCh:          make(chan struct{}),
	}

	// Add event handlers
	_, _ = namespaceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			w.onNamespaceChange()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			// Only refresh if namespace name changed (unlikely but possible via replace)
			oldNs, ok := oldObj.(*corev1.Namespace)
			if !ok {
				return
			}
			newNs, ok := newObj.(*corev1.Namespace)
			if !ok {
				return
			}
			if oldNs.Name != newNs.Name {
				w.onNamespaceChange()
			}
		},
		DeleteFunc: func(_ interface{}) {
			w.onNamespaceChange()
		},
	})

	return w, nil
}

// Start starts the watcher and blocks until the context is cancelled or Stop is called.
func (w *Watcher) Start(ctx context.Context) error {
	log.Info().
		Str("cluster", w.clusterName).
		Dur("refresh_interval", w.refreshInterval).
		Int("include_patterns", len(w.includePatterns)).
		Int("exclude_patterns", len(w.excludePatterns)).
		Msg("starting tenant watcher")

	// Start informer factory
	w.informerFactory.Start(w.stopCh)

	// Wait for cache sync
	if !cache.WaitForCacheSync(ctx.Done(), w.hasSynced) {
		return fmt.Errorf("failed to sync namespace cache")
	}

	log.Info().Str("cluster", w.clusterName).Msg("tenant watcher cache synced")

	// Initial refresh
	w.refreshTenants()

	// Wait for stop signal
	select {
	case <-ctx.Done():
		w.Stop()
		return ctx.Err()
	case <-w.stopCh:
		return nil
	}
}

// StartAsync starts the watcher in the background and returns immediately.
func (w *Watcher) StartAsync(ctx context.Context) {
	go func() {
		if err := w.Start(ctx); err != nil && err != context.Canceled {
			log.Error().Err(err).Str("cluster", w.clusterName).Msg("tenant watcher error")
		}
	}()
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	select {
	case <-w.stopCh:
		// Already stopped
	default:
		close(w.stopCh)
	}
}

// Tenants returns the current list of tenants.
func (w *Watcher) Tenants() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	// Return a copy to prevent mutation
	result := make([]string, len(w.tenants))
	copy(result, w.tenants)
	return result
}

// TenantCount returns the number of tenants.
func (w *Watcher) TenantCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.tenants)
}

// HasSynced returns true if the watcher has synced its cache.
func (w *Watcher) HasSynced() bool {
	return w.hasSynced()
}

func (w *Watcher) onNamespaceChange() {
	w.refreshTenants()
}

func (w *Watcher) refreshTenants() {
	namespaces, err := w.namespaceLister.List(labels.Everything())
	if err != nil {
		log.Error().Err(err).Str("cluster", w.clusterName).Msg("failed to list namespaces")
		return
	}

	tenants := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		if w.shouldInclude(ns.Name) {
			tenants = append(tenants, ns.Name)
		}
	}

	// Sort for consistent ordering
	sort.Strings(tenants)

	w.mu.Lock()
	oldCount := len(w.tenants)
	w.tenants = tenants
	w.mu.Unlock()

	if oldCount != len(tenants) {
		log.Info().
			Str("cluster", w.clusterName).
			Int("tenant_count", len(tenants)).
			Msg("tenant list updated")
	}
}

func (w *Watcher) shouldInclude(name string) bool {
	// If include patterns are specified, namespace must match at least one
	if len(w.includePatterns) > 0 {
		matched := false
		for _, re := range w.includePatterns {
			if re.MatchString(name) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check exclude patterns - if any match, exclude the namespace
	for _, re := range w.excludePatterns {
		if re.MatchString(name) {
			return false
		}
	}

	return true
}

// BuildOrgIDHeader builds the X-Scope-OrgID header value from tenants.
// If maxLength is exceeded, it truncates and logs a warning.
func (w *Watcher) BuildOrgIDHeader(maxLength int) string {
	tenants := w.Tenants()
	if len(tenants) == 0 {
		return ""
	}

	header := strings.Join(tenants, "|")

	if maxLength > 0 && len(header) > maxLength {
		// Truncate to fit within maxLength
		truncated := w.truncateOrgIDHeader(tenants, maxLength)
		log.Warn().
			Str("cluster", w.clusterName).
			Int("total_tenants", len(tenants)).
			Int("header_length", len(header)).
			Int("max_length", maxLength).
			Int("truncated_length", len(truncated)).
			Msg("X-Scope-OrgID header truncated due to length limit")
		return truncated
	}

	return header
}

func (w *Watcher) truncateOrgIDHeader(tenants []string, maxLength int) string {
	if len(tenants) == 0 {
		return ""
	}

	var result strings.Builder
	for i, tenant := range tenants {
		if i > 0 {
			// Check if adding separator and next tenant would exceed limit
			if result.Len()+1+len(tenant) > maxLength {
				break
			}
			result.WriteByte('|')
		} else {
			// First tenant - check if it alone exceeds limit
			if len(tenant) > maxLength {
				return tenant[:maxLength]
			}
		}
		result.WriteString(tenant)
	}
	return result.String()
}

// ListNamespaces lists all namespaces from the cluster (for initial sync or debugging).
func (w *Watcher) ListNamespaces(ctx context.Context) ([]string, error) {
	namespaces, err := w.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	names := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		names = append(names, ns.Name)
	}
	sort.Strings(names)
	return names, nil
}
