# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.2](https://github.com/tjorri/observability-federation-proxy/compare/v0.0.1...v0.0.2) (2025-12-30)


### Bug Fixes

* **ci:** trigger release workflow on release published event ([3915f85](https://github.com/tjorri/observability-federation-proxy/commit/3915f85f525fe25e78b14ef1c760cfdb1a5ec954))
* **ci:** use PAT for release-please to trigger CI on PRs ([9516bc1](https://github.com/tjorri/observability-federation-proxy/commit/9516bc142463c1f37f44c06ce3a875f3122c6ac5))

## 0.0.1 (2025-12-30)


### Features

* initial implementation of observability federation proxy ([b2826b4](https://github.com/tjorri/observability-federation-proxy/commit/b2826b43119600e4df92c0deefc7f936f89831e2))

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
