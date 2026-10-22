dummy := $(shell touch .env)
include .env
# Tools are built from source by "go run" with the Go toolchain from go.mod and
# verified through the Go checksum database. Bump the versions here only.
GOLANGCI_LINT_VERSION ?= v2.15.0
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.9.0
export

.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# CI runs the same targets (.github/workflows/ci.yml): every check is defined
# here once. "make test" is all of them on one machine.

##@ Development

.PHONY: test
test: mod cover lint cross-build check-repo ## Run every check (before a PR)

.PHONY: mod
mod: ## go.mod is tidy and the downloaded modules are verified
	go -C osxphotos mod tidy -diff
	go -C osxphotos mod verify

.PHONY: vet
vet: ## Vet and build
	go -C osxphotos vet ./...
	go -C osxphotos build ./...

.PHONY: unit
unit: vet ## Vet, build and run all tests with the race detector
	go -C osxphotos test -race -shuffle=on -count=1 ./...

.PHONY: cover
cover: vet ## Vet, build, run tests with coverage and check the threshold
	go -C osxphotos test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	cd osxphotos && bash ../scripts/check-coverage.sh coverage.out

# Catches a broken cgo cross-compilation before a release does (macOS only).
.PHONY: cross-build
cross-build: ## Build for amd64 with cgo (macOS)
	GOARCH=amd64 CC="clang -arch x86_64" go -C osxphotos build ./...

.PHONY: lint
lint: ## Run golangci-lint
	go -C osxphotos run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w osxphotos

##@ Checks

.PHONY: check-repo
check-repo: ## Repository hygiene checks and actionlint
	bash scripts/check-repo.sh
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# Not part of "make test": a new vulnerability in a dependency must not turn an
# unrelated change red. CI runs it on a schedule.
.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities
	go -C osxphotos run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
