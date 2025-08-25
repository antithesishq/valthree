SHELL := bash
.DELETE_ON_ERROR:
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-print-directory

.PHONY: help
help: ## Describe useful make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-30s %s\n", $$1, $$2}'

.PHONY: all
all: ## Build, test, and lint (default)
	$(MAKE) test
	$(MAKE) lint

.PHONY: test
test: build ## Run tests
	go test -vet=off -race ./...

.PHONY: build
build: ## Build all packages
	go build ./...
	go build -o valthree .

.PHONY: lint
lint: ## Lint Go
	test -z "$$(go run cmd/gofmt -s -l . | tee /dev/stderr)"
	go vet ./...
	go tool staticcheck ./...
	test $$(git grep -o 'minio/minio\:[A-Za-z0-9.-]*' | cut -d: -f 3 | sort -u | wc -l) -eq 1 || (echo "Mismatched MinIO container versions" && exit 1)

.PHONY: lintfix
lintfix: ## Automatically fix some lint errors
	go run cmd/gofmt -s -w .

.PHONY: upgrade
upgrade: ## Upgrade dependencies
	go get -u -t ./... && go mod tidy -v

.PHONY: run
run: ## Build containers and run Docker Compose
	docker build -f Dockerfile.valthree -t valthree:latest .
	docker build -f Dockerfile.config -t valthree-config:latest .
	docker compose up
