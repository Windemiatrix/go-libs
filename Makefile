GOLANGCI_LINT_VERSION ?= v2.15.0
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.9.0
SHFMT_VERSION ?= v3.14.1
SVU_VERSION ?= v3.4.1

# Not "?=": the environment must not lower it.
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
	go -C osxphotos vet -tags difflocal ./...

.PHONY: cover
cover: ## Tests with the race detector and the coverage threshold
	go -C osxphotos test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	@pct=$$(go -C osxphotos tool cover -func=coverage.out | tail -n 1 | tr -dc '0-9.'); \
	echo "coverage: $$pct% (minimum $(COVERAGE_MIN)%)"; \
	test "$${pct%.*}" -ge $(COVERAGE_MIN)

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

# Only a run of golden-regen.yml: an artifact named "golden" of another
# workflow must not end up in testdata/golden. The old files go first, so that
# none survives for a library the new run lacks. gh must not be older than
# 2.63.1 (a fix for writing outside --dir in "gh run download").
.PHONY: golden-fetch
golden-fetch: ## Download the reference data of the latest successful golden-regen run
	@run=$$(gh run list --repo Windemiatrix/go-libs --workflow golden-regen.yml --event workflow_dispatch --status success --limit 1 --json databaseId --jq '.[0].databaseId'); \
	test -n "$$run" || { echo "golden-fetch: no successful golden-regen run" >&2; exit 1; }; \
	echo "golden-fetch: run $$run"; \
	rm -f testdata/golden/*/query.json testdata/golden/*/deleted.json; \
	gh run download "$$run" --repo Windemiatrix/go-libs --name golden --dir testdata/golden

# The library is the owner's data. TMPDIR puts the temporary files of osxphotos
# (photosdb.py:170) and the database copy of the test under the trap. TZ and LC_ALL are
# those of scripts/golden-build.sh: osxphotos writes a missing date in the local zone
# (photos_datetime.py:19-21). The variables are read from the environment, not
# substituted into the shell text, so a path with a quote stays a path. The
# warnings of osxphotos name the files and UUIDs of the library, so its stderr
# stays in the trapped directory too, and osxphotos runs there: it writes
# osxphotos_crash.log into the current directory.
.PHONY: diff-local
diff-local: ## Compare a library with osxphotos (absolute LIBRARY, OSXPHOTOS, optional REPORT)
	$(if $(and $(LIBRARY),$(OSXPHOTOS)),,$(error diff-local: set LIBRARY=<absolute path of the .photoslibrary> and OSXPHOTOS=<path of osxphotos 0.77.2>))
	@umask 077; set -e; tmp=$$(mktemp -d); export TMPDIR="$$tmp"; trap 'rm -rf "$$tmp"' EXIT; trap 'exit 1' INT TERM HUP; \
	failed() { echo "diff-local: osxphotos failed; run it by hand to see why (its messages hold data of the library)" >&2; exit 1; }; \
	(cd "$$tmp" && TZ=UTC LC_ALL=C "$$OSXPHOTOS" query --library "$$LIBRARY" --json --mute >query.json 2>>osxphotos.log) || failed; \
	(cd "$$tmp" && TZ=UTC LC_ALL=C "$$OSXPHOTOS" query --library "$$LIBRARY" --json --mute --deleted-only >deleted.json 2>>osxphotos.log) || failed; \
	go -C osxphotos test -tags difflocal -count=1 -v -run '^TestDiffLocal$$' ./photosdb \
		-args -library "$$LIBRARY" -json "$$tmp/query.json" -deleted "$$tmp/deleted.json" -report "$$REPORT"

##@ Pull requests

.PHONY: pr-title
pr-title: ## PR_TITLE follows the title convention (make pr-title PR_TITLE='...')
	bash scripts/check-pr-title.sh

##@ Release

# BUMP=major gives 1.0.0: from 0.x the only way there.
BUMP ?= next

.PHONY: next-version
next-version: ## The version that the next merge to main releases (svu, .svu.yml; needs a tag)
	@go run github.com/caarlos0/svu/v3@$(SVU_VERSION) $(BUMP)
