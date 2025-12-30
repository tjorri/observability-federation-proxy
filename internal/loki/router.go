package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/tjorri/observability-federation-proxy/internal/proxy"
	"github.com/tjorri/observability-federation-proxy/internal/tenant"
)

// ProxyClient defines the interface for proxying HTTP requests.
type ProxyClient interface {
	ProxyHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, pathPrefix string, opts *proxy.ProxyHTTPOptions)
}

// Router handles Loki API requests and routes them to the appropriate cluster.
type Router struct {
	clients        map[string]ProxyClient
	tenantRegistry *tenant.Registry
	maxOrgIDLength int
}

// RouterConfig holds configuration for creating a Loki router.
type RouterConfig struct {
	Clients        map[string]ProxyClient
	TenantRegistry *tenant.Registry
	MaxOrgIDLength int
}

// NewRouter creates a new Loki router.
func NewRouter(cfg RouterConfig) *Router {
	return &Router{
		clients:        cfg.Clients,
		tenantRegistry: cfg.TenantRegistry,
		maxOrgIDLength: cfg.MaxOrgIDLength,
	}
}

// RegisterRoutes registers Loki routes on the given ServeMux.
// The pathPrefix should be "/clusters/{cluster}/loki".
func (r *Router) RegisterRoutes(mux *http.ServeMux, pathPrefix string) {
	// Query endpoints
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/query", pathPrefix), r.handleQuery)
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/query", pathPrefix), r.handleQuery)

	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/query_range", pathPrefix), r.handleQueryRange)
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/query_range", pathPrefix), r.handleQueryRange)

	// Labels endpoints
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/labels", pathPrefix), r.handleLabels)
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/labels", pathPrefix), r.handleLabels)

	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/label/{name}/values", pathPrefix), r.handleLabelValues)

	// Series endpoint
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/series", pathPrefix), r.handleSeries)
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/series", pathPrefix), r.handleSeries)

	// Index stats (optional but useful)
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/index/stats", pathPrefix), r.handleIndexStats)
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/index/stats", pathPrefix), r.handleIndexStats)

	// Tail endpoint for streaming (WebSocket-based, but we can proxy the initial request)
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/tail", pathPrefix), r.handleTail)

	// Catch-all for other Loki endpoints
	mux.HandleFunc(fmt.Sprintf("GET %s/", pathPrefix), r.handleGenericProxy)
	mux.HandleFunc(fmt.Sprintf("POST %s/", pathPrefix), r.handleGenericProxy)
}

// handleQuery handles /api/v1/query requests.
// Required parameter: query (LogQL)
// Optional: time, limit, direction
func (r *Router) handleQuery(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	// Parse query parameter (can be in URL or form body)
	if err := req.ParseForm(); err != nil {
		r.writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	query := req.Form.Get("query")
	if query == "" {
		r.writeError(w, http.StatusBadRequest, "missing required parameter: query")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("query", query).
		Str("time", req.Form.Get("time")).
		Msg("loki query request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/query")
}

// handleQueryRange handles /api/v1/query_range requests.
// Required parameters: query, start, end
// Optional: step, limit, direction
func (r *Router) handleQueryRange(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	if err := req.ParseForm(); err != nil {
		r.writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	query := req.Form.Get("query")
	if query == "" {
		r.writeError(w, http.StatusBadRequest, "missing required parameter: query")
		return
	}

	start := req.Form.Get("start")
	end := req.Form.Get("end")
	if start == "" || end == "" {
		r.writeError(w, http.StatusBadRequest, "missing required parameters: start and end")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("query", query).
		Str("start", start).
		Str("end", end).
		Str("step", req.Form.Get("step")).
		Msg("loki query_range request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/query_range")
}

// handleLabels handles /api/v1/labels requests.
// Returns all label names.
// Optional: start, end
func (r *Router) handleLabels(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Msg("loki labels request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/labels")
}

// handleLabelValues handles /api/v1/label/{name}/values requests.
// Returns all values for a given label.
// Optional: start, end, query
func (r *Router) handleLabelValues(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")
	labelName := req.PathValue("name")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	if labelName == "" {
		r.writeError(w, http.StatusBadRequest, "missing label name")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("label", labelName).
		Msg("loki label values request")

	r.proxyRequest(w, req, clusterName, client, fmt.Sprintf("/api/v1/label/%s/values", labelName))
}

// handleSeries handles /api/v1/series requests.
// Required: match[] (one or more)
// Optional: start, end
func (r *Router) handleSeries(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	if err := req.ParseForm(); err != nil {
		r.writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	matches := req.Form["match[]"]
	if len(matches) == 0 {
		r.writeError(w, http.StatusBadRequest, "missing required parameter: match[]")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Strs("match", matches).
		Msg("loki series request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/series")
}

// handleIndexStats handles /api/v1/index/stats requests.
func (r *Router) handleIndexStats(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Msg("loki index stats request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/index/stats")
}

// handleTail handles /api/v1/tail requests (log streaming).
// Note: Full WebSocket streaming requires additional handling.
func (r *Router) handleTail(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	if err := req.ParseForm(); err != nil {
		r.writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	query := req.Form.Get("query")
	if query == "" {
		r.writeError(w, http.StatusBadRequest, "missing required parameter: query")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("query", query).
		Msg("loki tail request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/tail")
}

// handleGenericProxy handles any other Loki API requests.
func (r *Router) handleGenericProxy(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or loki not configured")
		return
	}

	// Extract the path after /clusters/{cluster}/loki
	pathPrefix := fmt.Sprintf("/clusters/%s/loki", clusterName)
	path := strings.TrimPrefix(req.URL.Path, pathPrefix)
	if path == "" {
		path = "/"
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("path", path).
		Msg("loki generic proxy request")

	r.proxyRequest(w, req, clusterName, client, path)
}

// proxyRequest proxies a request to the Loki backend.
func (r *Router) proxyRequest(w http.ResponseWriter, req *http.Request, clusterName string, client ProxyClient, path string) {
	// Build path prefix for stripping
	pathPrefix := fmt.Sprintf("/clusters/%s/loki", clusterName)

	// Build proxy options with X-Scope-OrgID header
	opts := r.buildProxyOptions(clusterName)

	// Override the path if specified
	if path != "" {
		// Create a modified request with the correct path
		newReq := req.Clone(req.Context())
		newReq.URL.Path = pathPrefix + path
		client.ProxyHTTP(req.Context(), w, newReq, pathPrefix, opts)
	} else {
		client.ProxyHTTP(req.Context(), w, req, pathPrefix, opts)
	}
}

// buildProxyOptions builds proxy options with tenant headers.
func (r *Router) buildProxyOptions(clusterName string) *proxy.ProxyHTTPOptions {
	if r.tenantRegistry == nil {
		return nil
	}

	orgID := r.tenantRegistry.BuildOrgIDHeader(clusterName, r.maxOrgIDLength)
	if orgID == "" {
		return nil
	}

	headers := make(http.Header)
	headers.Set("X-Scope-OrgID", orgID)

	return &proxy.ProxyHTTPOptions{
		AdditionalHeaders: headers,
	}
}

// writeError writes a JSON error response.
func (r *Router) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// LokiResponse represents a standard Loki API response.
type LokiResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}
