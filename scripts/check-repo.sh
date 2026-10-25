#!/usr/bin/env bash
# Rules that neither the compiler nor the linter enforce. Only files tracked by
# git are examined, so build output and local junk never matter. Runs from the
# repository root.
#
# Exit:0 every check passed; 1 at least one failed. Every check runs, and
#         each failure is printed as "FAIL: ..." on stderr.
set -euo pipefail

module_dir="osxphotos"

# The Go language version in go.mod must not be newer than this minor version.
# Raise it deliberately: a newer version forces every user of the library to
# upgrade Go.
max_go_minor=27

cgo_allowed_dir="$module_dir/photokit/"

spike_build_tag_re='^//go:build.*[[:space:](&|]spike([[:space:])&|]|$)|^// \+build.*[[:space:]]spike'
cgo_import_re='^import "C"$|^[[:space:]]+"C"$'
action_use_re='uses: *[^ ]+@'
pinned_to_full_sha_re='uses: *[^ @]+@[0-9a-f]{40}( |$)'
go_version_re='^1\.([0-9]+)(\.[0-9]+)?$'

failed=0

fail() {
  echo "FAIL: $1" >&2
  failed=1
}

# pkg.go.dev and the module zip only see the module directory.
check_module_license() {
  if ! cmp LICENSE "$module_dir/LICENSE"; then
    fail "$module_dir/LICENSE must be an exact copy of LICENSE"
  fi
}

# UPSTREAM.md and tools/golden/requirements.txt must name the same osxphotos
# version: the reference data is generated with the version the module claims
# to follow.
check_upstream_version() {
  local upstream_version golden_version
  upstream_version="$(sed -n 's/^Upstream version: \([0-9][0-9.]*\)[[:space:]]*$/\1/p' "$module_dir/UPSTREAM.md")"
  golden_version="$(sed -n 's/^osxphotos==\([0-9][0-9.]*\)[[:space:]]*$/\1/p' tools/golden/requirements.txt)"
  if [[ -z "$upstream_version" || "$upstream_version" != "$golden_version" ]]; then
    fail "version in $module_dir/UPSTREAM.md ('$upstream_version') must equal the one in tools/golden/requirements.txt ('$golden_version')"
  fi
}

# Spike code lives only in the unpublished spike branches and never reaches main.
check_no_spike_code() {
  if git ls-files | grep -E '(^|/)internal/spike/'; then
    fail "the internal/spike/ directory must not be committed"
  fi
  if git grep -nE "$spike_build_tag_re" -- '*.go'; then
    fail "files with the 'spike' build tag must not be committed"
  fi
}

check_no_dependency_overrides() {
  if grep -nE '^[[:space:]]*replace([[:space:](]|$)' "$module_dir/go.mod"; then
    fail "$module_dir/go.mod must not contain replace directives"
  fi
  if git ls-files | grep -E '(^|/)go\.work(\.sum)?$'; then
    fail "go.work must not be committed"
  fi
}

check_go_version() {
  local go_version go_minor=""
  go_version="$(awk '$1 == "go" { print $2; exit }' "$module_dir/go.mod")"
  if [[ "$go_version" =~ $go_version_re ]]; then
    go_minor="${BASH_REMATCH[1]}"
  fi
  if [[ -z "$go_minor" ]] || ((go_minor > max_go_minor)); then
    fail "$module_dir/go.mod has 'go $go_version', expected 1.$max_go_minor or older"
  fi
}

check_cgo_only_where_allowed() {
  if git grep -lE "$cgo_import_re" -- '*.go' ":!$cgo_allowed_dir"; then
    fail "the files above import \"C\" outside $cgo_allowed_dir"
  fi
}

check_actions_pinned() {
  if git grep -nE "$action_use_re" -- '.github/workflows' | grep -vE "$pinned_to_full_sha_re"; then
    fail "workflow actions must be pinned to a full 40-character commit SHA"
  fi
}

# No job writes through git, and the release is created through the API, so
# no checkout needs to keep the git credentials.
check_checkouts_drop_credentials() {
  local workflow checkouts dropped
  while IFS= read -r workflow; do
    [[ -n "$workflow" ]] || continue
    checkouts="$(grep -cE '^ *(- )?uses: *actions/checkout@' "$workflow" || true)"
    dropped="$(grep -cE '^ *persist-credentials: false *$' "$workflow" || true)"
    if [[ "$checkouts" != "$dropped" ]]; then
      fail "$workflow: every actions/checkout needs 'persist-credentials: false' ($checkouts checkouts, $dropped with it)"
    fi
  done < <(git ls-files '.github/workflows/*.yml' '.github/workflows/*.yaml')
}

run_self_tests() {
  bash scripts/check-pr-title-test.sh || failed=1
}

check_module_license
check_upstream_version
check_no_spike_code
check_no_dependency_overrides
check_go_version
check_cgo_only_where_allowed
check_actions_pinned
check_checkouts_drop_credentials
run_self_tests

if ((failed)); then
  exit 1
fi
echo "repo checks passed"
