package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/tjorri/observability-federation-proxy/internal/cluster"
	"github.com/tjorri/observability-federation-proxy/internal/config"
	"github.com/tjorri/observability-federation-proxy/internal/server"
	"github.com/tjorri/observability-federation-proxy/internal/tenant"
)

var cfgFile string

func newRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "observability-federation-proxy",
		Short: "Proxy for federating Loki and Mimir queries across Kubernetes clusters",
		Long: `Observability Federation Proxy enables a centralized Grafana instance to query
Loki and Mimir endpoints in remote Kubernetes clusters via the Kubernetes API proxy.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			setupLogging(cfg.Logging)

			log.Info().
				Str("version", version).
				Str("listen_address", cfg.Proxy.ListenAddress).
				Int("cluster_count", len(cfg.Clusters)).
				Msg("starting observability federation proxy")

			ctx := context.Background()

			// Create cluster registry
			var registry *cluster.Registry
			if len(cfg.Clusters) > 0 {
				registry, err = cluster.NewRegistry(ctx, cfg.Clusters)
				if err != nil {
					return fmt.Errorf("failed to create cluster registry: %w", err)
				}
				log.Info().
					Int("cluster_count", len(registry.List())).
					Msg("cluster registry initialized")
			}

			// Create tenant registry
			var tenantRegistry *tenant.Registry
			if registry != nil {
				tenantRegistry, err = tenant.NewRegistry(ctx, registry, cfg.Clusters)
				if err != nil {
					return fmt.Errorf("failed to create tenant registry: %w", err)
				}
				tenantRegistry.Start(ctx)
				log.Info().
					Strs("clusters", tenantRegistry.List()).
					Msg("tenant registry initialized")
			}

			srv := server.New(cfg, registry, tenantRegistry)
			return srv.Run()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")

	cobra.OnInitialize(initConfig)

	return rootCmd
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/observability-federation-proxy/")
	}

	viper.SetEnvPrefix("OFP")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Warn().Err(err).Msg("failed to read config file")
		}
	}
}

func setupLogging(cfg config.LoggingConfig) {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.Format == "text" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

func Execute(version string) error {
	return newRootCmd(version).Execute()
}
