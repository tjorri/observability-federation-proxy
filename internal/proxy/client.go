package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client proxies HTTP requests through the Kubernetes API service proxy.
// It uses the endpoint: /api/v1/namespaces/{ns}/services/{svc}:{port}/proxy/{path}
type Client struct {
	k8sClient  kubernetes.Interface
	restClient rest.Interface
	namespace  string
	service    string
	port       int
	pathPrefix string
	timeout    time.Duration
}

// ClientConfig holds configuration for creating a proxy client.
type ClientConfig struct {
	K8sClient  kubernetes.Interface
	Namespace  string
	Service    string
	Port       int
	PathPrefix string
	Timeout    time.Duration
}

// NewClient creates a new K8s API proxy client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.K8sClient == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}
	if cfg.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if cfg.Service == "" {
		return nil, fmt.Errorf("service is required")
	}
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("port must be positive")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		k8sClient:  cfg.K8sClient,
		restClient: cfg.K8sClient.CoreV1().RESTClient(),
		namespace:  cfg.Namespace,
		service:    cfg.Service,
		port:       cfg.Port,
		pathPrefix: cfg.PathPrefix,
		timeout:    timeout,
	}, nil
}

// ProxyRequest proxies an HTTP request through the K8s API server.
func (c *Client) ProxyRequest(ctx context.Context, req *Request) (*Response, error) {
	if c.restClient == nil {
		return nil, fmt.Errorf("REST client not initialized")
	}

	// Build the full path including any path prefix
	fullPath := c.pathPrefix + req.Path

	// Build the proxy path (for logging)
	proxyPath := c.buildProxyPath(fullPath)

	log.Debug().
		Str("namespace", c.namespace).
		Str("service", c.service).
		Int("port", c.port).
		Str("method", req.Method).
		Str("path", req.Path).
		Str("path_prefix", c.pathPrefix).
		Str("full_path", fullPath).
		Str("proxy_path", proxyPath).
		Msg("proxying request")

	// Build the full request URI with query parameters
	// The K8s API service proxy URL format is: /api/v1/namespaces/{ns}/services/{service}:{port}/proxy/{path}
	servicePath := fmt.Sprintf("/api/v1/namespaces/%s/services/%s:%d/proxy%s",
		c.namespace, c.service, c.port, fullPath)

	// Add query parameters to the path
	if len(req.Query) > 0 {
		servicePath = servicePath + "?" + req.Query.Encode()
	}

	// Use RequestURI to set the exact URL path without any encoding modifications
	restReq := c.restClient.Verb(req.Method).
		RequestURI(servicePath).
		Timeout(c.timeout)

	// Add headers
	for key, values := range req.Headers {
		for _, value := range values {
			restReq = restReq.SetHeader(key, value)
		}
	}

	// Add body if present
	if req.Body != nil {
		restReq = restReq.Body(req.Body)
	}

	// Execute the request
	result := restReq.Do(ctx)

	// Get raw response
	rawBody, err := result.Raw()
	if err != nil {
		// Try to get status code from error
		statusCode := http.StatusBadGateway
		if result.Error() != nil {
			log.Error().Err(err).Msg("proxy request failed")
		}
		return &Response{
			StatusCode: statusCode,
			Body:       rawBody,
			Headers:    make(http.Header),
		}, err
	}

	// Get status code
	var statusCode int
	result.StatusCode(&statusCode)
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	return &Response{
		StatusCode: statusCode,
		Body:       rawBody,
		Headers:    make(http.Header), // K8s client doesn't expose response headers easily
	}, nil
}

// HTTPOptions contains options for ProxyHTTP.
type HTTPOptions struct {
	// AdditionalHeaders are headers to add to the proxied request.
	AdditionalHeaders http.Header
}

// ProxyHTTP is a convenience method that proxies an http.Request and writes to http.ResponseWriter.
func (c *Client) ProxyHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, pathPrefix string, opts *HTTPOptions) {
	// Strip the path prefix to get the actual API path
	path := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if path == "" {
		path = "/"
	}

	// Read the request body
	var body io.Reader
	if r.Body != nil {
		body = r.Body
		defer func() { _ = r.Body.Close() }()
	}

	// Build headers
	headers := filterHeaders(r.Header)
	if opts != nil && opts.AdditionalHeaders != nil {
		for key, values := range opts.AdditionalHeaders {
			for _, value := range values {
				headers.Add(key, value)
			}
		}
	}

	// Build proxy request
	req := &Request{
		Method:  r.Method,
		Path:    path,
		Query:   r.URL.Query(),
		Headers: headers,
		Body:    body,
	}

	// Execute proxy request
	resp, err := c.ProxyRequest(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("proxy request failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error": "proxy request failed: %s"}`, err.Error())
		return
	}

	// Copy response headers
	for key, values := range resp.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Ensure content type is set
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	// Write response
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		_, _ = w.Write(resp.Body)
	}
}

func (c *Client) buildProxyPath(path string) string {
	return fmt.Sprintf("/api/v1/namespaces/%s/services/%s:%d/proxy%s",
		c.namespace, c.service, c.port, path)
}

// Request represents a request to be proxied.
type Request struct {
	Method  string
	Path    string
	Query   url.Values
	Headers http.Header
	Body    io.Reader
}

// Response represents a response from the proxied service.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// filterHeaders filters out hop-by-hop headers that shouldn't be forwarded.
func filterHeaders(headers http.Header) http.Header {
	filtered := make(http.Header)
	hopByHop := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
		"Accept-Encoding":     true, // Don't forward to avoid gzip responses from backend
	}

	for key, values := range headers {
		if !hopByHop[key] {
			filtered[key] = values
		}
	}

	return filtered
}
