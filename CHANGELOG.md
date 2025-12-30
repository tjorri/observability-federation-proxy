# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.0.0 (2025-12-30)


### Features

* initial implementation of observability federation proxy ([1dcde48](https://github.com/tjorri/observability-federation-proxy/commit/1dcde486af42ed5c5ebc21bc457b46ad153b8428))


### Bug Fixes

* **ci:** migrate golangci-lint config to v2 format ([7ae6a91](https://github.com/tjorri/observability-federation-proxy/commit/7ae6a918b2f1b0a23008a7980e743138352a6ceb))
* **ci:** simplify build job to single compilation without artifacts ([c317229](https://github.com/tjorri/observability-federation-proxy/commit/c317229512a9a78bed71c9ec44b0a4e297d94132))
* **ci:** update golangci-lint-action to v9 for Go 1.25 support ([c7888c0](https://github.com/tjorri/observability-federation-proxy/commit/c7888c0f48842ac33979ceeb84a5e1ffac4a65af))
* **e2e:** add missing test config template ([84574c2](https://github.com/tjorri/observability-federation-proxy/commit/84574c29a5b44b695b2d546c087a97c6bdbb8d2d))
* exclude AddEventHandler from errcheck and fix import order ([2816390](https://github.com/tjorri/observability-federation-proxy/commit/2816390dce4eded70e93fc66bc4ff9f5a932927b))
* relax linter rules for cleaner code style ([ce88b96](https://github.com/tjorri/observability-federation-proxy/commit/ce88b96689b08b35b41f59c5e9aa26976e92c476))
* resolve all golangci-lint v2 issues ([e3685e6](https://github.com/tjorri/observability-federation-proxy/commit/e3685e67175f3bd15d49859ac9965641e7e35179))

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
