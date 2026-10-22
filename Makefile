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

# Every command is one target. CI (.github/workflows) runs these targets, one
# per step, so a failed step names the command. The composite targets below
# only list other targets; "make test" is every check on one machine.

##@ Composite

.PHONY: test
test: mod cover lint cross-build check-repo ## Run every check (before a PR)

.PHONY: mod
mod: tidy mod-verify ## go.mod is tidy and the modules are verified

.PHONY: unit
unit: vet build race ## Vet, build, tests with the race detector

.PHONY: cover
cover: vet build coverage coverage-check ## Vet, build, tests with coverage and the threshold

.PHONY: check-repo
check-repo: repo-hygiene actionlint ## Repository checks and actionlint

##@ Development

.PHONY: tidy
tidy: ## go.mod and go.sum are tidy
	go -C osxphotos mod tidy -diff

.PHONY: mod-verify
mod-verify: ## The downloaded modules match go.sum
	go -C osxphotos mod verify

.PHONY: vet
vet: ## go vet
	go -C osxphotos vet ./...

.PHONY: build
build: ## go build
	go -C osxphotos build ./...

.PHONY: race
race: ## Tests with the race detector
	go -C osxphotos test -race -shuffle=on -count=1 ./...

.PHONY: coverage
coverage: ## Tests with the race detector and a coverage profile (osxphotos/coverage.out)
	go -C osxphotos test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: coverage-check
coverage-check: ## Coverage threshold on osxphotos/coverage.out (run "make coverage" first)
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

.PHONY: repo-hygiene
repo-hygiene: ## Repository hygiene checks (scripts/check-repo.sh)
	bash scripts/check-repo.sh

.PHONY: actionlint
actionlint: ## Lint the GitHub workflows
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# Not part of "make test": a new vulnerability in a dependency must not turn an
# unrelated change red. CI runs it on a schedule, the release job blocks on it.
.PHONY: vuln
vuln: ## Check dependencies for known vulnerabilities
	go -C osxphotos run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# CI only: the title comes from the pull request event (PR_TITLE).
.PHONY: pr-title
pr-title: ## Check PR_TITLE against the title convention
	bash scripts/check-pr-title.sh
