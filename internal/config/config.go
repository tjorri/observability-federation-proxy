package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Proxy    ProxyConfig    `mapstructure:"proxy"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Clusters []ClusterConfig `mapstructure:"clusters"`
}

type ProxyConfig struct {
	ListenAddress         string        `mapstructure:"listenAddress"`
	QueryTimeout          time.Duration `mapstructure:"queryTimeout"`
	MaxTenantHeaderLength int           `mapstructure:"maxTenantHeaderLength"`
	MetricsEnabled        bool          `mapstructure:"metricsEnabled"`
}

type AuthConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	BearerTokens []string `mapstructure:"bearerTokens"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type ClusterConfig struct {
	Name       string           `mapstructure:"name"`
	Type       string           `mapstructure:"type"`
	EKS        *EKSConfig       `mapstructure:"eks,omitempty"`
	Kubeconfig *KubeconfigConfig `mapstructure:"kubeconfig,omitempty"`
	Loki       *ServiceConfig   `mapstructure:"loki,omitempty"`
	Mimir      *ServiceConfig   `mapstructure:"mimir,omitempty"`
	Tenants    TenantsConfig    `mapstructure:"tenants"`
}

type EKSConfig struct {
	ClusterName string          `mapstructure:"clusterName"`
	Region      string          `mapstructure:"region"`
	AssumeRole  *AssumeRoleConfig `mapstructure:"assumeRole,omitempty"`
}

type AssumeRoleConfig struct {
	RoleARN     string `mapstructure:"roleArn"`
	ExternalID  string `mapstructure:"externalId,omitempty"`
	SessionName string `mapstructure:"sessionName,omitempty"`
}

type KubeconfigConfig struct {
	Path string `mapstructure:"path"`
	Data string `mapstructure:"data"`
}

type ServiceConfig struct {
	Namespace  string `mapstructure:"namespace"`
	Service    string `mapstructure:"service"`
	Port       int    `mapstructure:"port"`
	PathPrefix string `mapstructure:"pathPrefix"`
}

type TenantsConfig struct {
	IncludePatterns []string      `mapstructure:"includePatterns"`
	ExcludePatterns []string      `mapstructure:"excludePatterns"`
	RefreshInterval time.Duration `mapstructure:"refreshInterval"`
}

func Load() (*Config, error) {
	cfg := &Config{}

	setDefaults()

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Load bearer tokens from environment variable if set
	// This allows secure token configuration in Kubernetes via Secrets
	if envTokens := os.Getenv("AUTH_BEARER_TOKENS"); envTokens != "" {
		tokens := strings.Split(envTokens, ",")
		// Trim whitespace from tokens
		for i, t := range tokens {
			tokens[i] = strings.TrimSpace(t)
		}
		cfg.Auth.BearerTokens = tokens
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func setDefaults() {
	viper.SetDefault("proxy.listenAddress", ":8080")
	viper.SetDefault("proxy.queryTimeout", "30s")
	viper.SetDefault("proxy.maxTenantHeaderLength", 8192)
	viper.SetDefault("proxy.metricsEnabled", true)
	viper.SetDefault("auth.enabled", false)
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
}

func (c *Config) Validate() error {
	if c.Proxy.ListenAddress == "" {
		return fmt.Errorf("proxy.listenAddress is required")
	}

	for i, cluster := range c.Clusters {
		if cluster.Name == "" {
			return fmt.Errorf("clusters[%d].name is required", i)
		}
		if cluster.Type == "" {
			return fmt.Errorf("clusters[%d].type is required", i)
		}
		if cluster.Type != "eks" && cluster.Type != "kubeconfig" {
			return fmt.Errorf("clusters[%d].type must be 'eks' or 'kubeconfig'", i)
		}
		if cluster.Type == "eks" && cluster.EKS == nil {
			return fmt.Errorf("clusters[%d].eks is required when type is 'eks'", i)
		}
		if cluster.Type == "kubeconfig" && cluster.Kubeconfig == nil {
			return fmt.Errorf("clusters[%d].kubeconfig is required when type is 'kubeconfig'", i)
		}
		if cluster.Loki == nil && cluster.Mimir == nil {
			return fmt.Errorf("clusters[%d] must have at least one of loki or mimir configured", i)
		}
	}

	return nil
}
