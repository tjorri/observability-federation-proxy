.PHONY: build test lint run clean docker-build docker-push e2e e2e-setup e2e-run e2e-teardown

BINARY_NAME=observability-federation-proxy
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

## Development

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/proxy

test:
	go test -race -cover ./...

test-verbose:
	go test -race -cover -v ./...

lint:
	golangci-lint run ./...

run:
	go run ./cmd/proxy

clean:
	rm -rf bin/ dist/

tidy:
	go mod tidy

fmt:
	go fmt ./...

## Docker (for local development)

docker-build:
	docker build -f Dockerfile.dev -t $(BINARY_NAME):$(VERSION) .

docker-run: docker-build
	docker run -p 8080:8080 -v $(PWD)/config.yaml:/etc/proxy/config.yaml $(BINARY_NAME):$(VERSION) --config /etc/proxy/config.yaml

## Release (uses GoReleaser - typically run by CI)

# Default values for local testing (overridden by GitHub Actions)
export GITHUB_REPOSITORY_OWNER ?= $(shell git remote get-url origin 2>/dev/null | sed -n 's/.*github.com[:/]\([^/]*\)\/.*/\1/p' || echo "local")
export GITHUB_REPOSITORY ?= $(shell git remote get-url origin 2>/dev/null | sed -n 's/.*github.com[:/]\([^/]*\/[^.]*\).*/\1/p' || echo "local/observability-federation-proxy")

release-snapshot:
	goreleaser release --snapshot --clean

release-check:
	goreleaser check

# E2E test targets
E2E_CLUSTER_NAME ?= ofp-e2e
E2E_DIR := $(shell pwd)/e2e

e2e: e2e-setup e2e-run e2e-teardown

e2e-setup:
	@echo "==> Setting up e2e infrastructure"
	CLUSTER_NAME=$(E2E_CLUSTER_NAME) $(E2E_DIR)/scripts/setup.sh

e2e-teardown:
	@echo "==> Tearing down e2e infrastructure"
	CLUSTER_NAME=$(E2E_CLUSTER_NAME) $(E2E_DIR)/scripts/teardown.sh

e2e-run:
	@echo "==> Running e2e tests"
	KUBECONFIG=$(E2E_DIR)/kubeconfig \
	E2E_DIR=$(E2E_DIR) \
	go test -v -tags=e2e -timeout=15m ./e2e/...

e2e-logs:
	@echo "==> Showing e2e cluster logs"
	KUBECONFIG=$(E2E_DIR)/kubeconfig kubectl logs -n observability -l app=loki --tail=100 || true
	KUBECONFIG=$(E2E_DIR)/kubeconfig kubectl logs -n observability -l app=mimir --tail=100 || true
