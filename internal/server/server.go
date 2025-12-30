package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"

	"github.com/tjorri/observability-federation-proxy/internal/cluster"
	"github.com/tjorri/observability-federation-proxy/internal/config"
	"github.com/tjorri/observability-federation-proxy/internal/loki"
	"github.com/tjorri/observability-federation-proxy/internal/metrics"
	"github.com/tjorri/observability-federation-proxy/internal/middleware"
	"github.com/tjorri/observability-federation-proxy/internal/mimir"
	"github.com/tjorri/observability-federation-proxy/internal/proxy"
	"github.com/tjorri/observability-federation-proxy/internal/tenant"
)

// Server is the main HTTP server that handles all proxy requests.
type Server struct {
	config         *config.Config
	registry       *cluster.Registry
	tenantRegistry *tenant.Registry
	lokiClients    map[string]*proxy.Client
	mimirClients   map[string]*proxy.Client
	httpServer     *http.Server
	mux            *http.ServeMux
}

// New creates a new Server with the given configuration and registries.
func New(cfg *config.Config, registry *cluster.Registry, tenantRegistry *tenant.Registry) *Server {
	s := &Server{
		config:         cfg,
		registry:       registry,
		tenantRegistry: tenantRegistry,
		lokiClients:    make(map[string]*proxy.Client),
		mimirClients:   make(map[string]*proxy.Client),
		mux:            http.NewServeMux(),
	}

	// Create proxy clients for each cluster
	if registry != nil {
		s.createProxyClients()
	}

	// Record cluster info metrics
	s.recordClusterMetrics()

	s.registerRoutes()

	// Build handler chain with middleware
	handler := s.buildHandlerChain()

	s.httpServer = &http.Server{
		Addr:         cfg.Proxy.ListenAddress,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: cfg.Proxy.QueryTimeout + 5*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

func (s *Server) buildHandlerChain() http.Handler {
	var handler http.Handler = s.mux

	// Add authentication middleware
	if s.config.Auth.Enabled {
		authMiddleware := middleware.Auth(middleware.AuthConfig{
			Enabled:      true,
			BearerTokens: s.config.Auth.BearerTokens,
			SkipPaths:    []string{"/healthz", "/readyz", "/metrics"},
		})
		handler = authMiddleware(handler)
	}

	// Add standard middleware
	handler = middleware.Chain(handler,
		middleware.Recovery,
		middleware.Logging,
		middleware.Metrics,
	)

	return handler
}

func (s *Server) recordClusterMetrics() {
	for _, c := range s.config.Clusters {
		metrics.RecordClusterInfo(c.Name, c.Type, c.Loki != nil, c.Mimir != nil)
	}
}

func (s *Server) createProxyClients() {
	for _, clusterCfg := range s.config.Clusters {
		c, ok := s.registry.Get(clusterCfg.Name)
		if !ok {
			log.Warn().Str("cluster", clusterCfg.Name).Msg("cluster not found in registry")
			continue
		}

		// Create Loki proxy client if configured
		if clusterCfg.Loki != nil {
			client, err := proxy.NewClient(proxy.ClientConfig{
				K8sClient:  c.Client,
				Namespace:  clusterCfg.Loki.Namespace,
				Service:    clusterCfg.Loki.Service,
				Port:       clusterCfg.Loki.Port,
				PathPrefix: clusterCfg.Loki.PathPrefix,
				Timeout:    s.config.Proxy.QueryTimeout,
			})
			if err != nil {
				log.Error().Err(err).Str("cluster", clusterCfg.Name).Msg("failed to create Loki proxy client")
			} else {
				s.lokiClients[clusterCfg.Name] = client
				log.Info().Str("cluster", clusterCfg.Name).Msg("created Loki proxy client")
			}
		}

		// Create Mimir proxy client if configured
		if clusterCfg.Mimir != nil {
			client, err := proxy.NewClient(proxy.ClientConfig{
				K8sClient:  c.Client,
				Namespace:  clusterCfg.Mimir.Namespace,
				Service:    clusterCfg.Mimir.Service,
				Port:       clusterCfg.Mimir.Port,
				PathPrefix: clusterCfg.Mimir.PathPrefix,
				Timeout:    s.config.Proxy.QueryTimeout,
			})
			if err != nil {
				log.Error().Err(err).Str("cluster", clusterCfg.Name).Msg("failed to create Mimir proxy client")
			} else {
				s.mimirClients[clusterCfg.Name] = client
				log.Info().Str("cluster", clusterCfg.Name).Msg("created Mimir proxy client")
			}
		}
	}
}

func (s *Server) registerRoutes() {
	// Health and readiness endpoints
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	// Metrics endpoint
	if s.config.Proxy.MetricsEnabled {
		s.mux.Handle("GET /metrics", promhttp.Handler())
	}

	// Management endpoints
	s.mux.HandleFunc("GET /api/v1/clusters", s.handleListClusters)
	s.mux.HandleFunc("GET /api/v1/clusters/{cluster}/tenants", s.handleListTenants)

	// Register Loki router
	s.registerLokiRoutes()

	// Register Mimir router
	s.registerMimirRoutes()
}

func (s *Server) registerLokiRoutes() {
	// Convert proxy.Client map to loki.ProxyClient interface map
	lokiProxyClients := make(map[string]loki.ProxyClient)
	for name, client := range s.lokiClients {
		lokiProxyClients[name] = client
	}

	lokiRouter := loki.NewRouter(loki.RouterConfig{
		Clients:        lokiProxyClients,
		TenantRegistry: s.tenantRegistry,
		MaxOrgIDLength: s.config.Proxy.MaxTenantHeaderLength,
	})

	lokiRouter.RegisterRoutes(s.mux, "/clusters/{cluster}/loki")
}

func (s *Server) registerMimirRoutes() {
	// Convert proxy.Client map to mimir.ProxyClient interface map
	mimirProxyClients := make(map[string]mimir.ProxyClient)
	for name, client := range s.mimirClients {
		mimirProxyClients[name] = client
	}

	mimirRouter := mimir.NewRouter(mimir.RouterConfig{
		Clients:        mimirProxyClients,
		TenantRegistry: s.tenantRegistry,
		MaxOrgIDLength: s.config.Proxy.MaxTenantHeaderLength,
	})

	mimirRouter.RegisterRoutes(s.mux, "/clusters/{cluster}/mimir")
}

// Run starts the HTTP server and blocks until shutdown.
func (s *Server) Run() error {
	errChan := make(chan error, 1)
	go func() {
		log.Info().Str("addr", s.config.Proxy.ListenAddress).Msg("starting HTTP server")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return err
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("shutting down server")
	}

	return s.Shutdown()
}

// Shutdown gracefully shuts down the server, stopping all background processes.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop tenant watchers first
	if s.tenantRegistry != nil {
		log.Info().Msg("stopping tenant watchers")
		s.tenantRegistry.Stop()
	}

	// Then shutdown HTTP server
	log.Info().Msg("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}

// Handler returns the HTTP handler for testing purposes.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.registry == nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Check cluster health
	results := s.registry.HealthCheck(r.Context())
	allHealthy := true
	clusterStatus := make(map[string]string)

	for name, err := range results {
		if err != nil {
			allHealthy = false
			clusterStatus[name] = err.Error()
			metrics.RecordClusterHealth(name, false)
		} else {
			clusterStatus[name] = "ok"
			metrics.RecordClusterHealth(name, true)
		}
	}

	// Record tenant counts
	if s.tenantRegistry != nil {
		for cluster, count := range s.tenantRegistry.TenantCounts() {
			metrics.RecordTenantCount(cluster, count)
		}
	}

	if allHealthy {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "ok",
			"clusters": clusterStatus,
		})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "degraded",
			"clusters": clusterStatus,
		})
	}
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	clusters := make([]map[string]interface{}, 0, len(s.config.Clusters))
	for _, c := range s.config.Clusters {
		clusterInfo := map[string]interface{}{
			"name":     c.Name,
			"type":     c.Type,
			"hasLoki":  c.Loki != nil,
			"hasMimir": c.Mimir != nil,
		}

		// Add tenant count if available
		if s.tenantRegistry != nil {
			if watcher, ok := s.tenantRegistry.Get(c.Name); ok {
				clusterInfo["tenantCount"] = watcher.TenantCount()
			}
		}

		clusters = append(clusters, clusterInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"clusters": clusters})
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")

	// Check if cluster exists in registry
	if s.registry != nil {
		if _, ok := s.registry.Get(clusterName); !ok {
			s.writeError(w, http.StatusNotFound, "cluster not found")
			return
		}
	} else {
		// Fall back to config check if no registry
		var found bool
		for _, c := range s.config.Clusters {
			if c.Name == clusterName {
				found = true
				break
			}
		}
		if !found {
			s.writeError(w, http.StatusNotFound, "cluster not found")
			return
		}
	}

	// Get tenants from tenant registry
	var tenants []string
	if s.tenantRegistry != nil {
		tenants = s.tenantRegistry.Tenants(clusterName)
	}
	if tenants == nil {
		tenants = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cluster": clusterName,
		"tenants": tenants,
	})
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// GetLokiClient returns the Loki proxy client for a cluster (for testing).
func (s *Server) GetLokiClient(clusterName string) *proxy.Client {
	return s.lokiClients[clusterName]
}

// GetMimirClient returns the Mimir proxy client for a cluster (for testing).
func (s *Server) GetMimirClient(clusterName string) *proxy.Client {
	return s.mimirClients[clusterName]
}
