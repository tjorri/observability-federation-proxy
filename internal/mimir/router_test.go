package mimir

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tjorri/observability-federation-proxy/internal/proxy"
)

// mockProxyClient implements ProxyClient for testing.
type mockProxyClient struct {
	lastPath    string
	lastHeaders http.Header
	response    []byte
	statusCode  int
}

func (m *mockProxyClient) ProxyHTTP(_ context.Context, w http.ResponseWriter, r *http.Request, pathPrefix string, opts *proxy.HTTPOptions) {
	m.lastPath = strings.TrimPrefix(r.URL.Path, pathPrefix)
	if opts != nil {
		m.lastHeaders = opts.AdditionalHeaders
	}

	statusCode := m.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if m.response != nil {
		w.Write(m.response)
	} else {
		w.Write([]byte(`{"status":"success"}`))
	}
}

func TestRouter_Query_MissingCluster(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/nonexistent/mimir/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !strings.Contains(resp["error"], "cluster not found") {
		t.Errorf("expected error about cluster not found, got: %s", resp["error"])
	}
}

func TestRouter_Query_MissingQueryParam(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": &mockProxyClient{},
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !strings.Contains(resp["error"], "missing required parameter: query") {
		t.Errorf("expected error about missing query, got: %s", resp["error"])
	}
}

func TestRouter_QueryRange_MissingParams(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": &mockProxyClient{},
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	tests := []struct {
		name        string
		query       string
		expectedErr string
	}{
		{
			name:        "missing query",
			query:       "start=1&end=2",
			expectedErr: "missing required parameter: query",
		},
		{
			name:        "missing start and end",
			query:       "query=up",
			expectedErr: "missing required parameters: start and end",
		},
		{
			name:        "missing end only",
			query:       "query=up&start=1",
			expectedErr: "missing required parameters: start and end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query_range?"+tt.query, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", w.Code)
			}

			var resp map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if !strings.Contains(resp["error"], tt.expectedErr) {
				t.Errorf("expected error containing %q, got: %s", tt.expectedErr, resp["error"])
			}
		})
	}
}

func TestRouter_LabelValues_ClusterNotFound(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": &mockProxyClient{},
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/nonexistent/mimir/api/v1/label/job/values", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestRouter_Series_MissingMatch(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": &mockProxyClient{},
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/series", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !strings.Contains(resp["error"], "missing required parameter: match[]") {
		t.Errorf("expected error about missing match[], got: %s", resp["error"])
	}
}

func TestRouter_QueryExemplars_MissingQuery(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": &mockProxyClient{},
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query_exemplars", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !strings.Contains(resp["error"], "missing required parameter: query") {
		t.Errorf("expected error about missing query, got: %s", resp["error"])
	}
}

func TestRouter_PostQuery(t *testing.T) {
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": &mockProxyClient{},
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	form := url.Values{}
	form.Set("query", "up")

	req := httptest.NewRequest(http.MethodPost, "/clusters/test-cluster/mimir/api/v1/query", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should pass validation and return success from mock
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_Query_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_QueryRange_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query_range?query=up&start=1609459200&end=1609545600&step=60", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_Labels_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/labels", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_LabelValues_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/label/job/values", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_Series_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/series?match[]=up", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_Metadata_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/metadata", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_QueryExemplars_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/api/v1/query_exemplars?query=up", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_RemoteRead_Success(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodPost, "/clusters/test-cluster/mimir/api/v1/read", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouter_GenericProxy(t *testing.T) {
	mockClient := &mockProxyClient{}
	router := NewRouter(RouterConfig{
		Clients: map[string]ProxyClient{
			"test-cluster": mockClient,
		},
	})

	mux := http.NewServeMux()
	router.RegisterRoutes(mux, "/clusters/{cluster}/mimir")

	req := httptest.NewRequest(http.MethodGet, "/clusters/test-cluster/mimir/some/other/path", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
