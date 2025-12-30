# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial implementation of Observability Federation Proxy
- Multi-cluster support with EKS and kubeconfig authentication
- Loki API proxy with multi-tenant query support
- Mimir API proxy with tenant federation support
- Dynamic tenant discovery from Kubernetes namespaces
- Automatic `X-Scope-OrgID` header injection for multi-tenant queries
- Prometheus metrics endpoint
- Health and readiness endpoints
- Bearer token authentication (optional)
- Helm chart for Kubernetes deployment
- End-to-end test suite with kind cluster
