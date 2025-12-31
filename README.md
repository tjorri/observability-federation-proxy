# Observability Federation Proxy

[![CI](https://github.com/tjorri/observability-federation-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/tjorri/observability-federation-proxy/actions/workflows/ci.yml)
[![E2E](https://github.com/tjorri/observability-federation-proxy/actions/workflows/e2e.yaml/badge.svg)](https://github.com/tjorri/observability-federation-proxy/actions/workflows/e2e.yaml)
[![Release](https://img.shields.io/github/v/release/tjorri/observability-federation-proxy)](https://github.com/tjorri/observability-federation-proxy/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/tjorri/observability-federation-proxy)](https://goreportcard.com/report/github.com/tjorri/observability-federation-proxy)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A lightweight proxy that federates observability queries across multiple Kubernetes clusters. It provides a unified API for querying Loki (logs) and Mimir (metrics) across different clusters while handling multi-tenant authentication automatically.

## Features

- **Multi-Cluster Support**: Query Loki and Mimir instances across multiple Kubernetes clusters through a single endpoint
- **Flexible Authentication**: Supports EKS (IRSA, Pod Identity, IAM roles) and kubeconfig-based cluster access
- **Dynamic Tenant Discovery**: Automatically discovers namespaces matching configurable patterns and injects `X-Scope-OrgID` headers
- **Cross-Tenant Queries**: Supports querying across multiple tenants using pipe-separated tenant IDs
- **Kubernetes API Proxy**: Uses the Kubernetes API server's service proxy to securely access in-cluster services without requiring direct network access
- **Production Ready**: Includes Prometheus metrics, structured logging, health checks, and graceful shutdown
- **Helm Chart**: Ready-to-deploy Helm chart for Kubernetes

## Architecture

```
┌─────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│   Grafana   │────▶│  Federation Proxy    │────▶│  Cluster A      │
│             │     │                      │     │  ├── Loki       │
└─────────────┘     │  • Tenant Discovery  │     │  └── Mimir      │
                    │  • Header Injection  │     └─────────────────┘
                    │  • K8s API Proxy     │     ┌─────────────────┐
                    │                      │────▶│  Cluster B      │
                    └──────────────────────┘     │  ├── Loki       │
                                                 │  └── Mimir      │
                                                 └─────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.21+
- Access to one or more Kubernetes clusters with Loki and/or Mimir deployed
- Loki configured with `auth_enabled: true` and `multi_tenant_queries_enabled: true`
- Mimir configured with `multitenancy_enabled: true` and `tenant_federation.enabled: true`

### Installation

```bash
# Clone the repository
git clone https://github.com/tjorri/observability-federation-proxy.git
cd observability-federation-proxy

# Build
make build

# Run with a config file
./bin/observability-federation-proxy --config config.yaml
```

### Configuration

Create a `config.yaml` file:

```yaml
proxy:
  listenAddress: ":8080"
  queryTimeout: 30s
  maxTenantHeaderLength: 8192
  metricsEnabled: true

auth:
  enabled: false
  # bearerTokens: ["token1", "token2"]  # Or use AUTH_BEARER_TOKENS env var

logging:
  level: info
  format: json

clusters:
  # EKS cluster with IRSA/Pod Identity
  - name: prod-cluster
    type: eks
    eks:
      clusterName: my-eks-cluster
      region: us-west-2
    loki:
      namespace: observability
      service: loki-gateway
      port: 80
      pathPrefix: /loki  # Loki API prefix
    mimir:
      namespace: observability
      service: mimir-gateway
      port: 80
      pathPrefix: /prometheus  # Mimir Prometheus-compatible API prefix
    tenants:
      includePatterns:
        - "^team-.*"
      excludePatterns:
        - "^kube-.*"
      refreshInterval: 60s

  # Non-EKS cluster with kubeconfig
  - name: on-prem-cluster
    type: kubeconfig
    kubeconfig:
      path: /etc/proxy/kubeconfigs/on-prem.yaml
    loki:
      namespace: monitoring
      service: loki
      port: 3100
      pathPrefix: /loki
    tenants:
      includePatterns:
        - ".*"  # All namespaces
      excludePatterns:
        - "^kube-.*"
        - "^default$"
```

## API Endpoints

### Management

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Liveness probe |
| `GET /readyz` | Readiness probe (checks cluster connectivity) |
| `GET /metrics` | Prometheus metrics |
| `GET /api/v1/clusters` | List configured clusters |
| `GET /api/v1/clusters/{cluster}/tenants` | List discovered tenants for a cluster |

### Loki (Logs)

| Endpoint | Description |
|----------|-------------|
| `GET/POST /clusters/{cluster}/loki/api/v1/query` | Instant query |
| `GET/POST /clusters/{cluster}/loki/api/v1/query_range` | Range query |
| `GET/POST /clusters/{cluster}/loki/api/v1/labels` | List labels |
| `GET /clusters/{cluster}/loki/api/v1/label/{name}/values` | Label values |
| `GET/POST /clusters/{cluster}/loki/api/v1/series` | Series query |

### Mimir (Metrics)

| Endpoint | Description |
|----------|-------------|
| `GET/POST /clusters/{cluster}/mimir/api/v1/query` | Instant query |
| `GET/POST /clusters/{cluster}/mimir/api/v1/query_range` | Range query |
| `GET/POST /clusters/{cluster}/mimir/api/v1/labels` | List labels |
| `GET /clusters/{cluster}/mimir/api/v1/label/{name}/values` | Label values |
| `GET/POST /clusters/{cluster}/mimir/api/v1/series` | Series query |

## Deployment

### Helm

The chart is available via the GitHub OCI registry:

```bash
helm install observability-federation-proxy \
  oci://ghcr.io/tjorri/charts/observability-federation-proxy \
  --namespace observability \
  --create-namespace \
  -f values.yaml
```

See the [chart documentation](charts/observability-federation-proxy/README.md) for all available values and configuration options.

### Docker

```bash
docker build -t observability-federation-proxy:latest .
docker run -p 8080:8080 -v $(pwd)/config.yaml:/etc/proxy/config.yaml \
  observability-federation-proxy:latest
```

## Development

### Running Tests

```bash
# Unit tests
make test

# Unit tests with verbose output
make test-verbose

# Helm chart validation
make helm-test    # Run all Helm validations (lint, template, docs)
make helm-lint    # Lint the chart
make helm-template # Validate template rendering
make helm-docs    # Regenerate chart README

# End-to-end tests (requires Docker for kind cluster)
make e2e

# Or run e2e steps individually
make e2e-setup    # Create kind cluster with Loki/Mimir
make e2e-run      # Run e2e tests
make e2e-teardown # Delete kind cluster
```

### Project Structure

```
.
├── cmd/proxy/          # Application entrypoint
├── internal/
│   ├── cluster/        # Kubernetes cluster management (EKS, kubeconfig)
│   ├── config/         # Configuration loading and validation
│   ├── loki/           # Loki API router
│   ├── mimir/          # Mimir API router
│   ├── proxy/          # K8s API service proxy client
│   ├── server/         # HTTP server setup
│   ├── tenant/         # Tenant discovery and registry
│   ├── middleware/     # HTTP middleware (auth, logging, metrics)
│   └── metrics/        # Prometheus metrics
├── charts/             # Helm chart
├── e2e/                # End-to-end tests
│   ├── manifests/      # Loki/Mimir deployment manifests
│   └── scripts/        # Test setup/teardown scripts
└── config.example.yaml # Example configuration
```

## Multi-Tenant Configuration

### Loki

Ensure Loki is configured with multi-tenant query support:

```yaml
auth_enabled: true

querier:
  multi_tenant_queries_enabled: true
```

### Mimir

Ensure Mimir is configured with tenant federation:

```yaml
multitenancy_enabled: true

tenant_federation:
  enabled: true
```

## Metrics

The proxy exposes Prometheus metrics at `/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `http_requests_total` | Counter | Total HTTP requests by method, path, status |
| `http_request_duration_seconds` | Histogram | Request duration by method and path |
| `cluster_info` | Gauge | Cluster configuration info |
| `cluster_healthy` | Gauge | Cluster health status |
| `tenant_count` | Gauge | Number of discovered tenants per cluster |

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
