# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.4](https://github.com/tjorri/observability-federation-proxy/compare/v0.0.3...v0.0.4) (2025-12-31)


### Features

* configure release-please to update Helm chart version ([e34ff7e](https://github.com/tjorri/observability-federation-proxy/commit/e34ff7e7bacdff928ad3f11d1605286970a5783c))
* **helm:** add helm-docs with pre-commit hook ([7b1a498](https://github.com/tjorri/observability-federation-proxy/commit/7b1a49888b0d19083286a8b5667766a6af4d8ab0))
* **helm:** add secure kubeconfig handling with documentation ([97dc706](https://github.com/tjorri/observability-federation-proxy/commit/97dc70696d1171063ee44dd3e6037e062c40b428))


### Bug Fixes

* allow release-please to create releases (GoReleaser will replace) ([0d4997e](https://github.com/tjorri/observability-federation-proxy/commit/0d4997e550d597ea2ad46823ca53a1b3789f851e))
* **ci:** e2e workflow does not contain permissions ([#6](https://github.com/tjorri/observability-federation-proxy/issues/6)) ([f18b1e4](https://github.com/tjorri/observability-federation-proxy/commit/f18b1e4c951bfa0588749044addfd244ea45b96b))
* **helm:** update default image repository to ghcr.io ([7f94820](https://github.com/tjorri/observability-federation-proxy/commit/7f948206b972c21ad24b0732a5782215f13a0d8e))

## [0.0.3](https://github.com/tjorri/observability-federation-proxy/compare/v0.0.2...v0.0.3) (2025-12-30)


### Bug Fixes

* **ci:** trigger release workflow only on tag push ([0a22030](https://github.com/tjorri/observability-federation-proxy/commit/0a22030695e773df899ef69da5250d9433600ff7))
* let GoReleaser handle GitHub releases instead of release-please ([cbef47e](https://github.com/tjorri/observability-federation-proxy/commit/cbef47e3fcc129f66d20d8c2ba7c026f30d2db3a))
* update Docker image license labels to Apache-2.0 ([1019dfd](https://github.com/tjorri/observability-federation-proxy/commit/1019dfd2a10dd64bc88e24a97d453eced8d706a7))

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
