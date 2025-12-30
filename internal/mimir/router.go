package mimir

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

// Router handles Mimir/Prometheus API requests and routes them to the appropriate cluster.
type Router struct {
	clients        map[string]ProxyClient
	tenantRegistry *tenant.Registry
	maxOrgIDLength int
}

// RouterConfig holds configuration for creating a Mimir router.
type RouterConfig struct {
	Clients        map[string]ProxyClient
	TenantRegistry *tenant.Registry
	MaxOrgIDLength int
}

// NewRouter creates a new Mimir router.
func NewRouter(cfg RouterConfig) *Router {
	return &Router{
		clients:        cfg.Clients,
		tenantRegistry: cfg.TenantRegistry,
		maxOrgIDLength: cfg.MaxOrgIDLength,
	}
}

// RegisterRoutes registers Mimir routes on the given ServeMux.
// The pathPrefix should be "/clusters/{cluster}/mimir".
func (r *Router) RegisterRoutes(mux *http.ServeMux, pathPrefix string) {
	// Query endpoints (Prometheus-compatible)
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

	// Metadata endpoint (Mimir/Prometheus specific)
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/metadata", pathPrefix), r.handleMetadata)

	// Query exemplars (Prometheus 2.x feature)
	mux.HandleFunc(fmt.Sprintf("GET %s/api/v1/query_exemplars", pathPrefix), r.handleQueryExemplars)
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/query_exemplars", pathPrefix), r.handleQueryExemplars)

	// Remote read endpoint
	mux.HandleFunc(fmt.Sprintf("POST %s/api/v1/read", pathPrefix), r.handleRemoteRead)

	// Catch-all for other Mimir endpoints
	mux.HandleFunc(fmt.Sprintf("GET %s/", pathPrefix), r.handleGenericProxy)
	mux.HandleFunc(fmt.Sprintf("POST %s/", pathPrefix), r.handleGenericProxy)
}

// handleQuery handles /api/v1/query requests (instant query).
// Required parameter: query (PromQL)
// Optional: time, timeout
func (r *Router) handleQuery(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
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
		Str("time", req.Form.Get("time")).
		Msg("mimir query request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/query")
}

// handleQueryRange handles /api/v1/query_range requests (range query).
// Required parameters: query, start, end
// Optional: step, timeout
func (r *Router) handleQueryRange(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
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
		Msg("mimir query_range request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/query_range")
}

// handleLabels handles /api/v1/labels requests.
// Returns all label names.
// Optional: start, end, match[]
func (r *Router) handleLabels(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Msg("mimir labels request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/labels")
}

// handleLabelValues handles /api/v1/label/{name}/values requests.
// Returns all values for a given label.
// Optional: start, end, match[]
func (r *Router) handleLabelValues(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")
	labelName := req.PathValue("name")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
		return
	}

	if labelName == "" {
		r.writeError(w, http.StatusBadRequest, "missing label name")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("label", labelName).
		Msg("mimir label values request")

	r.proxyRequest(w, req, clusterName, client, fmt.Sprintf("/api/v1/label/%s/values", labelName))
}

// handleSeries handles /api/v1/series requests.
// Required: match[] (one or more)
// Optional: start, end
func (r *Router) handleSeries(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
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
		Msg("mimir series request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/series")
}

// handleMetadata handles /api/v1/metadata requests.
// Returns metric metadata.
// Optional: limit, metric
func (r *Router) handleMetadata(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("metric", req.URL.Query().Get("metric")).
		Msg("mimir metadata request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/metadata")
}

// handleQueryExemplars handles /api/v1/query_exemplars requests.
// Required: query
// Optional: start, end
func (r *Router) handleQueryExemplars(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
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
		Msg("mimir query_exemplars request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/query_exemplars")
}

// handleRemoteRead handles /api/v1/read requests (Prometheus remote read protocol).
func (r *Router) handleRemoteRead(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
		return
	}

	log.Debug().
		Str("cluster", clusterName).
		Msg("mimir remote read request")

	r.proxyRequest(w, req, clusterName, client, "/api/v1/read")
}

// handleGenericProxy handles any other Mimir API requests.
func (r *Router) handleGenericProxy(w http.ResponseWriter, req *http.Request) {
	clusterName := req.PathValue("cluster")

	client, ok := r.clients[clusterName]
	if !ok {
		r.writeError(w, http.StatusNotFound, "cluster not found or mimir not configured")
		return
	}

	// Extract the path after /clusters/{cluster}/mimir
	pathPrefix := fmt.Sprintf("/clusters/%s/mimir", clusterName)
	path := strings.TrimPrefix(req.URL.Path, pathPrefix)
	if path == "" {
		path = "/"
	}

	log.Debug().
		Str("cluster", clusterName).
		Str("path", path).
		Msg("mimir generic proxy request")

	r.proxyRequest(w, req, clusterName, client, path)
}

// proxyRequest proxies a request to the Mimir backend.
func (r *Router) proxyRequest(w http.ResponseWriter, req *http.Request, clusterName string, client ProxyClient, path string) {
	// Build path prefix for stripping
	pathPrefix := fmt.Sprintf("/clusters/%s/mimir", clusterName)

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

// PrometheusResponse represents a standard Prometheus/Mimir API response.
type PrometheusResponse struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType string      `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}
