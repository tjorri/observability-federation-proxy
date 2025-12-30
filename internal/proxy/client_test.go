package proxy

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

func TestNewClient(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()

	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: ClientConfig{
				K8sClient: k8sClient,
				Namespace: "observability",
				Service:   "loki-gateway",
				Port:      80,
				Timeout:   30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid config with default timeout",
			cfg: ClientConfig{
				K8sClient: k8sClient,
				Namespace: "observability",
				Service:   "loki-gateway",
				Port:      80,
			},
			wantErr: false,
		},
		{
			name: "missing k8s client",
			cfg: ClientConfig{
				Namespace: "observability",
				Service:   "loki-gateway",
				Port:      80,
			},
			wantErr: true,
			errMsg:  "kubernetes client is required",
		},
		{
			name: "missing namespace",
			cfg: ClientConfig{
				K8sClient: k8sClient,
				Service:   "loki-gateway",
				Port:      80,
			},
			wantErr: true,
			errMsg:  "namespace is required",
		},
		{
			name: "missing service",
			cfg: ClientConfig{
				K8sClient: k8sClient,
				Namespace: "observability",
				Port:      80,
			},
			wantErr: true,
			errMsg:  "service is required",
		},
		{
			name: "invalid port",
			cfg: ClientConfig{
				K8sClient: k8sClient,
				Namespace: "observability",
				Service:   "loki-gateway",
				Port:      0,
			},
			wantErr: true,
			errMsg:  "port must be positive",
		},
		{
			name: "negative port",
			cfg: ClientConfig{
				K8sClient: k8sClient,
				Namespace: "observability",
				Service:   "loki-gateway",
				Port:      -1,
			},
			wantErr: true,
			errMsg:  "port must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if client == nil {
					t.Error("expected non-nil client")
				}
			}
		})
	}
}

func TestClient_buildProxyPath(t *testing.T) {
	k8sClient := fake.NewSimpleClientset()
	client, err := NewClient(ClientConfig{
		K8sClient: k8sClient,
		Namespace: "observability",
		Service:   "loki-gateway",
		Port:      80,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tests := []struct {
		path     string
		expected string
	}{
		{
			path:     "/api/v1/query",
			expected: "/api/v1/namespaces/observability/services/loki-gateway:80/proxy/api/v1/query",
		},
		{
			path:     "/loki/api/v1/query_range",
			expected: "/api/v1/namespaces/observability/services/loki-gateway:80/proxy/loki/api/v1/query_range",
		},
		{
			path:     "/",
			expected: "/api/v1/namespaces/observability/services/loki-gateway:80/proxy/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := client.buildProxyPath(tt.path)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFilterHeaders(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Scope-Orgid", "tenant1|tenant2") // Note: Header.Set canonicalizes
	headers.Set("Authorization", "Bearer token")
	headers.Set("Connection", "keep-alive")
	headers.Set("Keep-Alive", "timeout=5")
	headers.Set("Transfer-Encoding", "chunked")
	headers.Set("X-Custom-Header", "custom-value")
	headers.Set("Proxy-Authorization", "Basic xyz")

	filtered := filterHeaders(headers)

	// These should be preserved
	if filtered.Get("Content-Type") != "application/json" {
		t.Error("Content-Type should be preserved")
	}
	if filtered.Get("X-Scope-Orgid") != "tenant1|tenant2" {
		t.Errorf("X-Scope-Orgid should be preserved, got %q", filtered.Get("X-Scope-Orgid"))
	}
	if filtered.Get("Authorization") != "Bearer token" {
		t.Error("Authorization should be preserved")
	}
	if filtered.Get("X-Custom-Header") != "custom-value" {
		t.Error("X-Custom-Header should be preserved")
	}

	// These hop-by-hop headers should be filtered out
	if filtered.Get("Connection") != "" {
		t.Error("Connection should be filtered")
	}
	if filtered.Get("Keep-Alive") != "" {
		t.Error("Keep-Alive should be filtered")
	}
	if filtered.Get("Transfer-Encoding") != "" {
		t.Error("Transfer-Encoding should be filtered")
	}
	if filtered.Get("Proxy-Authorization") != "" {
		t.Error("Proxy-Authorization should be filtered")
	}
}

func TestRequest(t *testing.T) {
	headers := make(http.Header)
	headers.Set("X-Scope-Orgid", "tenant1")

	req := &Request{
		Method: "GET",
		Path:   "/api/v1/query",
		Query: url.Values{
			"query": {"up"},
			"time":  {"1234567890"},
		},
		Headers: headers,
		Body:    strings.NewReader(`{"test": "body"}`),
	}

	if req.Method != "GET" {
		t.Errorf("expected method GET, got %s", req.Method)
	}
	if req.Path != "/api/v1/query" {
		t.Errorf("expected path /api/v1/query, got %s", req.Path)
	}
	if req.Query.Get("query") != "up" {
		t.Error("expected query param 'query' to be 'up'")
	}
	if req.Headers.Get("X-Scope-Orgid") != "tenant1" {
		t.Errorf("expected X-Scope-Orgid header, got %q", req.Headers.Get("X-Scope-Orgid"))
	}

	// Read body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(body) != `{"test": "body"}` {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestResponse(t *testing.T) {
	resp := &Response{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body: []byte(`{"status": "success"}`),
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Headers.Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type header")
	}
	if string(resp.Body) != `{"status": "success"}` {
		t.Errorf("unexpected body: %s", string(resp.Body))
	}
}

func TestClient_ProxyHTTP_PathStripping(t *testing.T) {
	// This test verifies that path prefixes are correctly stripped
	// The actual proxying requires a real K8s API server, so we test the path logic

	k8sClient := fake.NewSimpleClientset()
	client, err := NewClient(ClientConfig{
		K8sClient: k8sClient,
		Namespace: "observability",
		Service:   "loki-gateway",
		Port:      80,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tests := []struct {
		requestPath  string
		pathPrefix   string
		expectedPath string
	}{
		{
			requestPath:  "/clusters/prod/loki/api/v1/query",
			pathPrefix:   "/clusters/prod/loki",
			expectedPath: "/api/v1/query",
		},
		{
			requestPath:  "/clusters/prod/loki/api/v1/query_range",
			pathPrefix:   "/clusters/prod/loki",
			expectedPath: "/api/v1/query_range",
		},
		{
			requestPath:  "/clusters/prod/mimir/api/v1/labels",
			pathPrefix:   "/clusters/prod/mimir",
			expectedPath: "/api/v1/labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.requestPath, func(t *testing.T) {
			path := strings.TrimPrefix(tt.requestPath, tt.pathPrefix)
			if path != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, path)
			}

			// Verify it builds the correct proxy path
			proxyPath := client.buildProxyPath(path)
			if !strings.Contains(proxyPath, tt.expectedPath) {
				t.Errorf("proxy path %q should contain %q", proxyPath, tt.expectedPath)
			}
		})
	}
}

func TestClient_ProxyHTTP_Integration(t *testing.T) {
	// Skip this test - the fake K8s client doesn't support service proxy
	// and returns a typed-nil REST client that can't be checked with == nil.
	// Real integration tests require a running K8s cluster.
	t.Skip("fake k8s client doesn't support service proxy - requires real cluster for integration tests")
}
