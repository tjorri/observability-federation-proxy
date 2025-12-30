package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestLoad_Defaults(t *testing.T) {
	viper.Reset()
	setDefaults()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Proxy.ListenAddress != ":8080" {
		t.Errorf("expected listen address :8080, got %s", cfg.Proxy.ListenAddress)
	}
	if cfg.Proxy.QueryTimeout != 30*time.Second {
		t.Errorf("expected query timeout 30s, got %v", cfg.Proxy.QueryTimeout)
	}
	if cfg.Proxy.MaxTenantHeaderLength != 8192 {
		t.Errorf("expected max tenant header length 8192, got %d", cfg.Proxy.MaxTenantHeaderLength)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Logging.Format)
	}
}

func TestLoad_FromFile(t *testing.T) {
	configContent := `
proxy:
  listenAddress: ":9090"
  queryTimeout: 60s
  maxTenantHeaderLength: 4096
logging:
  level: debug
  format: text
clusters:
  - name: test-cluster
    type: eks
    eks:
      clusterName: my-cluster
      region: us-east-1
    loki:
      namespace: observability
      service: loki-gateway
      port: 80
    tenants:
      refreshInterval: 120s
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	viper.Reset()
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	setDefaults()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Proxy.ListenAddress != ":9090" {
		t.Errorf("expected listen address :9090, got %s", cfg.Proxy.ListenAddress)
	}
	if cfg.Proxy.QueryTimeout != 60*time.Second {
		t.Errorf("expected query timeout 60s, got %v", cfg.Proxy.QueryTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}
	if len(cfg.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(cfg.Clusters))
	}
	if cfg.Clusters[0].Name != "test-cluster" {
		t.Errorf("expected cluster name test-cluster, got %s", cfg.Clusters[0].Name)
	}
	if cfg.Clusters[0].EKS.ClusterName != "my-cluster" {
		t.Errorf("expected EKS cluster name my-cluster, got %s", cfg.Clusters[0].EKS.ClusterName)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with no clusters",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
			},
			wantErr: false,
		},
		{
			name: "missing listen address",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ""},
			},
			wantErr: true,
			errMsg:  "proxy.listenAddress is required",
		},
		{
			name: "missing cluster name",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{Type: "eks"},
				},
			},
			wantErr: true,
			errMsg:  "clusters[0].name is required",
		},
		{
			name: "missing cluster type",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{Name: "test"},
				},
			},
			wantErr: true,
			errMsg:  "clusters[0].type is required",
		},
		{
			name: "invalid cluster type",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{Name: "test", Type: "invalid"},
				},
			},
			wantErr: true,
			errMsg:  "clusters[0].type must be 'eks' or 'kubeconfig'",
		},
		{
			name: "eks type without eks config",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{Name: "test", Type: "eks", Loki: &ServiceConfig{}},
				},
			},
			wantErr: true,
			errMsg:  "clusters[0].eks is required when type is 'eks'",
		},
		{
			name: "kubeconfig type without kubeconfig config",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{Name: "test", Type: "kubeconfig", Loki: &ServiceConfig{}},
				},
			},
			wantErr: true,
			errMsg:  "clusters[0].kubeconfig is required when type is 'kubeconfig'",
		},
		{
			name: "cluster without loki or mimir",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{Name: "test", Type: "eks", EKS: &EKSConfig{}},
				},
			},
			wantErr: true,
			errMsg:  "clusters[0] must have at least one of loki or mimir configured",
		},
		{
			name: "valid eks cluster",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{
						Name: "test",
						Type: "eks",
						EKS:  &EKSConfig{ClusterName: "cluster", Region: "us-east-1"},
						Loki: &ServiceConfig{Namespace: "obs", Service: "loki", Port: 80},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid kubeconfig cluster",
			config: Config{
				Proxy: ProxyConfig{ListenAddress: ":8080"},
				Clusters: []ClusterConfig{
					{
						Name:       "test",
						Type:       "kubeconfig",
						Kubeconfig: &KubeconfigConfig{Path: "/path/to/kubeconfig"},
						Mimir:      &ServiceConfig{Namespace: "obs", Service: "mimir", Port: 80},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
