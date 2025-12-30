//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Health Check Tests
// ============================================================================

func TestHealthz(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/healthz")
	if err != nil {
		t.Fatalf("Failed to call /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", body["status"])
	}
}

func TestReadyz(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/readyz")
	if err != nil {
		t.Fatalf("Failed to call /readyz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", body["status"])
	}

	// Check cluster health
	clusters, ok := body["clusters"].(map[string]interface{})
	if !ok {
		t.Error("Expected clusters field in response")
		return
	}

	if clusters["e2e-cluster"] != "ok" {
		t.Errorf("Expected cluster 'e2e-cluster' to be 'ok', got '%v'", clusters["e2e-cluster"])
	}
}

// ============================================================================
// Cluster Management Tests
// ============================================================================

func TestListClusters(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/api/v1/clusters")
	if err != nil {
		t.Fatalf("Failed to call /api/v1/clusters: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Clusters []struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			HasLoki     bool   `json:"hasLoki"`
			HasMimir    bool   `json:"hasMimir"`
			TenantCount int    `json:"tenantCount"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(body.Clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(body.Clusters))
	}

	cluster := body.Clusters[0]
	if cluster.Name != "e2e-cluster" {
		t.Errorf("Expected cluster name 'e2e-cluster', got '%s'", cluster.Name)
	}
	if cluster.Type != "kubeconfig" {
		t.Errorf("Expected cluster type 'kubeconfig', got '%s'", cluster.Type)
	}
	if !cluster.HasLoki {
		t.Error("Expected cluster to have Loki")
	}
	if !cluster.HasMimir {
		t.Error("Expected cluster to have Mimir")
	}
}

// ============================================================================
// Tenant Discovery Tests
// ============================================================================

func TestListTenants(t *testing.T) {
	// Give tenant discovery some time to complete
	time.Sleep(3 * time.Second)

	resp, err := testClient.Get(proxyBaseURL + "/api/v1/clusters/e2e-cluster/tenants")
	if err != nil {
		t.Fatalf("Failed to call tenants endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Cluster string   `json:"cluster"`
		Tenants []string `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Cluster != "e2e-cluster" {
		t.Errorf("Expected cluster 'e2e-cluster', got '%s'", body.Cluster)
	}

	// Should find tenant-* namespaces (based on include patterns)
	expectedTenants := []string{"tenant-alpha", "tenant-beta", "tenant-gamma"}
	for _, expected := range expectedTenants {
		found := false
		for _, tenant := range body.Tenants {
			if tenant == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected tenant '%s' not found in %v", expected, body.Tenants)
		}
	}

	// kube-system should NOT be in the list (excluded)
	for _, tenant := range body.Tenants {
		if strings.HasPrefix(tenant, "kube-") {
			t.Errorf("Tenant '%s' should be excluded", tenant)
		}
	}
}

func TestListTenants_ClusterNotFound(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/api/v1/clusters/nonexistent/tenants")
	if err != nil {
		t.Fatalf("Failed to call tenants endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Loki Query Tests
// ============================================================================

func TestLokiQuery(t *testing.T) {
	query := url.QueryEscape(`{job="e2e-test"}`)
	resp, err := testClient.Get(fmt.Sprintf("%s/clusters/e2e-cluster/loki/api/v1/query?query=%s", proxyBaseURL, query))
	if err != nil {
		t.Fatalf("Failed to call Loki query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string        `json:"resultType"`
			Result     []interface{} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestLokiQueryRange(t *testing.T) {
	now := time.Now()
	start := now.Add(-1 * time.Hour).Unix()
	end := now.Unix()

	query := url.QueryEscape(`{job="e2e-test"}`)
	urlStr := fmt.Sprintf("%s/clusters/e2e-cluster/loki/api/v1/query_range?query=%s&start=%d&end=%d&step=60",
		proxyBaseURL, query, start, end)

	resp, err := testClient.Get(urlStr)
	if err != nil {
		t.Fatalf("Failed to call Loki query_range: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestLokiLabels(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/clusters/e2e-cluster/loki/api/v1/labels")
	if err != nil {
		t.Fatalf("Failed to call Loki labels: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}

	// Should contain at least "job" label from seeded data
	found := false
	for _, label := range body.Data {
		if label == "job" {
			found = true
			break
		}
	}
	if !found {
		t.Logf("Warning: 'job' label not found in response: %v (data may not be seeded yet)", body.Data)
	}
}

func TestLokiLabelValues(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/clusters/e2e-cluster/loki/api/v1/label/job/values")
	if err != nil {
		t.Fatalf("Failed to call Loki label values: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestLokiSeries(t *testing.T) {
	match := url.QueryEscape(`{job="e2e-test"}`)
	resp, err := testClient.Get(fmt.Sprintf("%s/clusters/e2e-cluster/loki/api/v1/series?match[]=%s", proxyBaseURL, match))
	if err != nil {
		t.Fatalf("Failed to call Loki series: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestLokiQuery_ClusterNotFound(t *testing.T) {
	query := url.QueryEscape(`{job="test"}`)
	resp, err := testClient.Get(fmt.Sprintf("%s/clusters/nonexistent/loki/api/v1/query?query=%s", proxyBaseURL, query))
	if err != nil {
		t.Fatalf("Failed to call Loki query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestLokiQuery_MissingQuery(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/clusters/e2e-cluster/loki/api/v1/query")
	if err != nil {
		t.Fatalf("Failed to call Loki query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Mimir Query Tests
// ============================================================================

func TestMimirQuery(t *testing.T) {
	query := url.QueryEscape(`up`)
	resp, err := testClient.Get(fmt.Sprintf("%s/clusters/e2e-cluster/mimir/api/v1/query?query=%s", proxyBaseURL, query))
	if err != nil {
		t.Fatalf("Failed to call Mimir query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestMimirQueryRange(t *testing.T) {
	now := time.Now()
	start := now.Add(-1 * time.Hour).Unix()
	end := now.Unix()

	query := url.QueryEscape(`up`)
	urlStr := fmt.Sprintf("%s/clusters/e2e-cluster/mimir/api/v1/query_range?query=%s&start=%d&end=%d&step=60",
		proxyBaseURL, query, start, end)

	resp, err := testClient.Get(urlStr)
	if err != nil {
		t.Fatalf("Failed to call Mimir query_range: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestMimirLabels(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/clusters/e2e-cluster/mimir/api/v1/labels")
	if err != nil {
		t.Fatalf("Failed to call Mimir labels: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestMimirLabelValues(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/clusters/e2e-cluster/mimir/api/v1/label/job/values")
	if err != nil {
		t.Fatalf("Failed to call Mimir label values: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestMimirSeries(t *testing.T) {
	match := url.QueryEscape(`{job="e2e-test"}`)
	now := time.Now()
	start := now.Add(-1 * time.Hour).Unix()
	end := now.Unix()

	urlStr := fmt.Sprintf("%s/clusters/e2e-cluster/mimir/api/v1/series?match[]=%s&start=%d&end=%d",
		proxyBaseURL, match, start, end)

	resp, err := testClient.Get(urlStr)
	if err != nil {
		t.Fatalf("Failed to call Mimir series: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", body.Status)
	}
}

func TestMimirQuery_ClusterNotFound(t *testing.T) {
	query := url.QueryEscape(`up`)
	resp, err := testClient.Get(fmt.Sprintf("%s/clusters/nonexistent/mimir/api/v1/query?query=%s", proxyBaseURL, query))
	if err != nil {
		t.Fatalf("Failed to call Mimir query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}
}

func TestMimirQuery_MissingQuery(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/clusters/e2e-cluster/mimir/api/v1/query")
	if err != nil {
		t.Fatalf("Failed to call Mimir query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

// ============================================================================
// Metrics Endpoint Test
// ============================================================================

func TestMetricsEndpoint(t *testing.T) {
	resp, err := testClient.Get(proxyBaseURL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to call /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Check for expected Prometheus metrics
	bodyStr := string(body)
	expectedMetrics := []string{
		"http_requests_total",
		"http_request_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("Expected metric '%s' not found in /metrics response", metric)
		}
	}
}
