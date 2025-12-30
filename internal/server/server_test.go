package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tjorri/observability-federation-proxy/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Proxy: config.ProxyConfig{
			ListenAddress:         ":8080",
			QueryTimeout:          30 * time.Second,
			MaxTenantHeaderLength: 8192,
		},
		Clusters: []config.ClusterConfig{
			{
				Name: "test-cluster",
				Type: "eks",
				EKS: &config.EKSConfig{
					ClusterName: "my-cluster",
					Region:      "us-east-1",
				},
				Loki: &config.ServiceConfig{
					Namespace: "observability",
					Service:   "loki-gateway",
					Port:      80,
				},
				Mimir: &config.ServiceConfig{
					Namespace: "observability",
					Service:   "mimir-gateway",
					Port:      80,
				},
			},
			{
				Name: "loki-only-cluster",
				Type: "kubeconfig",
				Kubeconfig: &config.KubeconfigConfig{
					Path: "/path/to/kubeconfig",
				},
				Loki: &config.ServiceConfig{
					Namespace: "monitoring",
					Service:   "loki",
					Port:      3100,
				},
			},
		},
	}
}

func TestHealthz(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestReadyz(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestListClusters(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		Clusters []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			HasLoki  bool   `json:"hasLoki"`
			HasMimir bool   `json:"hasMimir"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(resp.Clusters))
	}

	if resp.Clusters[0].Name != "test-cluster" {
		t.Errorf("expected first cluster name test-cluster, got %s", resp.Clusters[0].Name)
	}
	if !resp.Clusters[0].HasLoki || !resp.Clusters[0].HasMimir {
		t.Errorf("expected test-cluster to have both loki and mimir")
	}

	if resp.Clusters[1].Name != "loki-only-cluster" {
		t.Errorf("expected second cluster name loki-only-cluster, got %s", resp.Clusters[1].Name)
	}
	if !resp.Clusters[1].HasLoki || resp.Clusters[1].HasMimir {
		t.Errorf("expected loki-only-cluster to have loki but not mimir")
	}
}

func TestListTenants_ClusterNotFound(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/nonexistent/tenants", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestListTenants_Success(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/test-cluster/tenants", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		Cluster string   `json:"cluster"`
		Tenants []string `json:"tenants"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Cluster != "test-cluster" {
		t.Errorf("expected cluster test-cluster, got %s", resp.Cluster)
	}
}

func TestLokiProxy_ClusterNotFound(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clusters/nonexistent/loki/api/v1/query", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestLokiProxy_NotConfigured(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			ListenAddress: ":8080",
			QueryTimeout:  30 * time.Second,
		},
		Clusters: []config.ClusterConfig{
			{
				Name: "mimir-only",
				Type: "eks",
				EKS:  &config.EKSConfig{ClusterName: "cluster", Region: "us-east-1"},
				Mimir: &config.ServiceConfig{
					Namespace: "observability",
					Service:   "mimir",
					Port:      80,
				},
			},
		},
	}
	srv := New(cfg, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clusters/mimir-only/loki/api/v1/query?query={job=\"test\"}", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Loki router returns combined error message
	if resp["error"] != "cluster not found or loki not configured" {
		t.Errorf("unexpected error message: %s", resp["error"])
	}
}

func TestMimirProxy_ClusterNotFound(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clusters/nonexistent/mimir/api/v1/query", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestMimirProxy_NotConfigured(t *testing.T) {
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clusters/loki-only-cluster/mimir/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Mimir router returns combined error message
	if resp["error"] != "cluster not found or mimir not configured" {
		t.Errorf("unexpected error message: %s", resp["error"])
	}
}

func TestLokiProxy_NoClient(t *testing.T) {
	// When there's no registry, no Loki clients are created
	// The Loki router returns 404 when the client is not found
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/loki/api/v1/query?query={job=\"test\"}", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "cluster not found or loki not configured" {
		t.Errorf("unexpected error message: %s", resp["error"])
	}
}

func TestMimirProxy_NoClient(t *testing.T) {
	// When there's no registry, no Mimir clients are created
	// The Mimir router returns 404 when the client is not found
	srv := New(testConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "cluster not found or mimir not configured" {
		t.Errorf("unexpected error message: %s", resp["error"])
	}
}
