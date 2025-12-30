package cluster

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/tjorri/observability-federation-proxy/internal/config"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRegistry_GetAndList(t *testing.T) {
	// Create a registry with mock clusters
	r := &Registry{
		clusters: map[string]*Cluster{
			"cluster-a": {
				Name:   "cluster-a",
				Client: fake.NewSimpleClientset(),
			},
			"cluster-b": {
				Name:   "cluster-b",
				Client: fake.NewSimpleClientset(),
			},
		},
	}

	// Test Get - existing cluster
	cluster, ok := r.Get("cluster-a")
	if !ok {
		t.Error("expected to find cluster-a")
	}
	if cluster.Name != "cluster-a" {
		t.Errorf("expected cluster name cluster-a, got %s", cluster.Name)
	}

	// Test Get - non-existing cluster
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent cluster")
	}

	// Test List
	names := r.List()
	if len(names) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(names))
	}

	// Check both names are present
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}
	if !nameSet["cluster-a"] || !nameSet["cluster-b"] {
		t.Error("expected both cluster-a and cluster-b in list")
	}
}

func TestRegistry_HealthCheck(t *testing.T) {
	// Create a registry with fake clients
	r := &Registry{
		clusters: map[string]*Cluster{
			"healthy": {
				Name:   "healthy",
				Client: fake.NewSimpleClientset(),
			},
		},
	}

	results := r.HealthCheck(context.Background())

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Fake client should return nil error (healthy)
	if err, ok := results["healthy"]; !ok {
		t.Error("expected healthy cluster in results")
	} else if err != nil {
		t.Errorf("expected nil error for fake client, got: %v", err)
	}
}

func TestCreateKubeconfigCluster_FromFile(t *testing.T) {
	// Create a minimal kubeconfig file with insecure-skip-tls-verify for testing
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`

	tmpDir := t.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}

	r := &Registry{clusters: make(map[string]*Cluster)}
	cfg := config.ClusterConfig{
		Name: "test-cluster",
		Type: "kubeconfig",
		Kubeconfig: &config.KubeconfigConfig{
			Path: kubeconfigPath,
		},
		Loki: &config.ServiceConfig{
			Namespace: "observability",
			Service:   "loki",
			Port:      80,
		},
	}

	cluster, err := r.createKubeconfigCluster(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Name != "test-cluster" {
		t.Errorf("expected cluster name test-cluster, got %s", cluster.Name)
	}
	if cluster.Client == nil {
		t.Error("expected non-nil client")
	}
	if cluster.restConfig == nil {
		t.Error("expected non-nil restConfig")
	}
	if cluster.restConfig.Host() != "https://localhost:6443" {
		t.Errorf("expected host https://localhost:6443, got %s", cluster.restConfig.Host())
	}
}

func TestCreateKubeconfigCluster_FromData(t *testing.T) {
	// Create kubeconfig and encode as base64
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://localhost:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`
	kubeconfigData := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	r := &Registry{clusters: make(map[string]*Cluster)}
	cfg := config.ClusterConfig{
		Name: "test-cluster",
		Type: "kubeconfig",
		Kubeconfig: &config.KubeconfigConfig{
			Data: kubeconfigData,
		},
		Mimir: &config.ServiceConfig{
			Namespace: "observability",
			Service:   "mimir",
			Port:      80,
		},
	}

	cluster, err := r.createKubeconfigCluster(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cluster.Name != "test-cluster" {
		t.Errorf("expected cluster name test-cluster, got %s", cluster.Name)
	}
	if cluster.Client == nil {
		t.Error("expected non-nil client")
	}
}

func TestCreateKubeconfigCluster_Errors(t *testing.T) {
	r := &Registry{clusters: make(map[string]*Cluster)}

	tests := []struct {
		name    string
		cfg     config.ClusterConfig
		wantErr string
	}{
		{
			name: "nil kubeconfig",
			cfg: config.ClusterConfig{
				Name: "test",
				Type: "kubeconfig",
			},
			wantErr: "kubeconfig config is required",
		},
		{
			name: "empty kubeconfig",
			cfg: config.ClusterConfig{
				Name:       "test",
				Type:       "kubeconfig",
				Kubeconfig: &config.KubeconfigConfig{},
			},
			wantErr: "either kubeconfig.path or kubeconfig.data is required",
		},
		{
			name: "invalid path",
			cfg: config.ClusterConfig{
				Name: "test",
				Type: "kubeconfig",
				Kubeconfig: &config.KubeconfigConfig{
					Path: "/nonexistent/path/kubeconfig",
				},
			},
			wantErr: "failed to read kubeconfig file",
		},
		{
			name: "invalid base64 data",
			cfg: config.ClusterConfig{
				Name: "test",
				Type: "kubeconfig",
				Kubeconfig: &config.KubeconfigConfig{
					Data: "not-valid-base64!!!",
				},
			},
			wantErr: "failed to decode kubeconfig data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.createKubeconfigCluster(tt.cfg)
			if err == nil {
				t.Error("expected error but got nil")
			} else if tt.wantErr != "" && !containsString(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestCreateCluster_UnknownType(t *testing.T) {
	r := &Registry{clusters: make(map[string]*Cluster)}
	cfg := config.ClusterConfig{
		Name: "test",
		Type: "unknown",
	}

	_, err := r.createCluster(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for unknown cluster type")
	}
	if !containsString(err.Error(), "unknown cluster type") {
		t.Errorf("expected error about unknown type, got: %v", err)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
