GOLANGCI_LINT_VERSION ?= v2.15.0
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.9.0
SHFMT_VERSION ?= v3.14.1

# The minimum total test coverage, in percent. Not "?=": the environment must
# not lower it.
COVERAGE_MIN := 80

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: mod vet cover lint lint-sh cross-build check-repo ## Run every check (before a PR)

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
	go -C osxphotos tool cover -func=coverage.out | awk '/^total:/ { found = 1; sub(/%/, "", $$3); print; exit ($$3 + 0 < $(COVERAGE_MIN)) } END { if (!found) exit 1 }'

.PHONY: cross-build
cross-build: ## cgo build for amd64 (macOS)
	GOARCH=amd64 CC="clang -arch x86_64" go -C osxphotos build ./...

.PHONY: lint
lint: ## Run golangci-lint
	go -C osxphotos run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

.PHONY: lint-sh
lint-sh: ## shellcheck and the shfmt format check of scripts/ (needs shellcheck installed)
	shellcheck scripts/*.sh
	go run mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION) -d -i 2 -ci scripts

.PHONY: fmt
fmt: ## Format the code and the scripts
	gofmt -s -w osxphotos
	go run mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION) -w -i 2 -ci scripts

##@ Checks

.PHONY: check-repo
check-repo: ## Repository hygiene checks and actionlint
	bash scripts/check-repo.sh
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

.PHONY: vuln
vuln: ## Known vulnerabilities in dependencies (not in "make test")
	go -C osxphotos run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

##@ Reference data

# RUN is read from the environment of the recipe ($$RUN), not substituted into
# the text of a command. The run must be a successful golden-regen started by
# hand, otherwise an artifact named "golden" of another workflow could end up
# in testdata/golden. gh must not be older than 2.63.1 (a fix for writing
# outside --dir in "gh run download"). The JSON files of an earlier run are
# removed first, so that none of them survives in a library the new run lacks.
GOLDEN_RUN_IS_MANUAL_SUCCESS := .workflowName == "golden-regen" and .event == "workflow_dispatch" \
    and .status == "completed" and .conclusion == "success"

.PHONY: golden-fetch
golden-fetch: ## Download the reference data of a golden-regen run (make golden-fetch RUN=<run id>)
	@case "$$RUN" in ''|*[!0-9]*) echo "usage: make golden-fetch RUN=<numeric run id>" >&2; exit 2;; esac
	@info="$$(gh run view "$$RUN" --repo Windemiatrix/go-libs --json workflowName,event,status,conclusion,headBranch,headSha \
		--jq 'select($(GOLDEN_RUN_IS_MANUAL_SUCCESS)) | "headBranch: \(.headBranch)\nheadSha: \(.headSha)"')" \
		|| { echo "golden-fetch: cannot verify run $$RUN (see the gh message above)" >&2; exit 2; }; \
	[ -n "$$info" ] || { echo "golden-fetch: run $$RUN is not a successful golden-regen run started by workflow_dispatch" >&2; exit 2; }; \
	echo "$$info"
	rm -f testdata/golden/*/query.json testdata/golden/*/deleted.json
	gh run download "$$RUN" --repo Windemiatrix/go-libs --name golden --dir testdata/golden

##@ Pull requests

.PHONY: pr-title
pr-title: ## PR_TITLE follows the title convention (PR_TITLE='...' make pr-title)
	bash scripts/check-pr-title.sh

##@ Release

.PHONY: release
release: ## GitHub Release for TAG with grouped notes, then the Go proxy warm-up (needs GH_TOKEN)
	@notes="$$(bash scripts/next-version.sh --notes "$$TAG")" && \
	gh release create "$$TAG" --verify-tag --title "$$TAG" --notes "$$notes"
	GOPROXY=https://proxy.golang.org go list -m "github.com/Windemiatrix/go-libs/osxphotos@$${TAG#osxphotos/}"
