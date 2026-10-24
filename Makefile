GOLANGCI_LINT_VERSION ?= v2.15.0
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.9.0

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: mod vet cover lint cross-build check-repo ## Run every check (before a PR)

.PHONY: mod
mod: ## go.mod is tidy and the modules match go.sum
	go -C osxphotos mod tidy -diff
	go -C osxphotos mod verify

.PHONY: vet
vet: ## go vet
	go -C osxphotos vet ./...

.PHONY: cover
cover: ## Tests with the race detector and the coverage threshold
	go -C osxphotos test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	cd osxphotos && bash ../scripts/check-coverage.sh coverage.out

.PHONY: cross-build
cross-build: ## cgo build for amd64 (macOS)
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

.PHONY: vuln
vuln: ## Known vulnerabilities in dependencies (not in "make test")
	go -C osxphotos run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

##@ Pull requests

.PHONY: pr-title
pr-title: ## PR_TITLE follows the title convention (PR_TITLE='...' make pr-title)
	bash scripts/check-pr-title.sh

##@ Release

.PHONY: release
release: ## GitHub Release for TAG with grouped notes, then the Go proxy warm-up (needs GH_TOKEN)
	@notes="$$(mktemp "$${RUNNER_TEMP:-$${TMPDIR:-/tmp}}/osxphotos-notes.XXXXXX")" && \
	bash scripts/next-version.sh --notes "$$TAG" >"$$notes" && \
	gh release create "$$TAG" --verify-tag --title "$$TAG" --notes-file "$$notes"; \
	status=$$?; rm -f "$$notes"; exit $$status
	GOPROXY=https://proxy.golang.org go list -m "github.com/Windemiatrix/go-libs/osxphotos@$${TAG#osxphotos/}"
