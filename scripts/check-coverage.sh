#!/usr/bin/env bash
# Coverage gate: reads a Go coverage profile and checks the threshold.
#
# All packages of the module together must reach OTHER_MIN (80) percent of
# statements. Packages listed in "excluded" are left out. If nothing is left to
# check (a module without statements, or a profile that only has the mode line)
# the gate passes and says so.
#
# Usage, from the module root (the osxphotos directory, where go.mod is):
#   go test -race -covermode=atomic -coverprofile=coverage.out ./...
#   ../scripts/check-coverage.sh coverage.out
#
# Plain POSIX awk and bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail

profile="${1:-}"
if [[ -z "$profile" || ! -r "$profile" ]]; then
  echo "usage: $0 <coverprofile>   (run from the module root)" >&2
  exit 2
fi

module="$(awk '$1 == "module" { print $2; exit }' go.mod)"
if [[ -z "$module" ]]; then
  echo "error: no module line in go.mod (run from the module root)" >&2
  exit 2
fi

other_min="${OTHER_MIN:-80}"

# Packages relative to the module root, matched as path prefixes, whose code
# runs only against real system APIs or is composition-only wiring. Add an
# entry here, with the reason, instead of lowering the threshold. Files that
# use cgo are not compiled on Linux, so the Linux profile only sees the stubs.
excluded=(
  "internal/gen/cmd"  # main package of the synthetic database generator (added later)
)

awk -v module="$module" -v excl="${excluded[*]}" -v omin="$other_min" '
function is_excluded(pkg,    i, n, parts) {
  n = split(excl, parts, " ")
  for (i = 1; i <= n; i++) {
    if (pkg == parts[i] || index(pkg, parts[i] "/") == 1) return 1
  }
  return 0
}

# Profile line: <import path>/<file>.go:<start>,<end> <statements> <count>
NR == 1 && $1 == "mode:" { next }
NF == 3 {
  key = $1
  stmts[key] = $2
  if ($3 + 0 > hits[key] + 0) hits[key] = $3
}

END {
  for (key in stmts) {
    file = key
    sub(/:[^:]*$/, "", file)
    pkg = file
    sub(/\/[^\/]*$/, "", pkg)
    if (index(pkg, module "/") == 1) pkg = substr(pkg, length(module) + 2)
    else if (pkg == module) pkg = "."
    if (is_excluded(pkg)) continue

    otot += stmts[key]
    ocov += (hits[key] + 0 > 0) ? stmts[key] + 0 : 0
  }

  if (otot == 0) {
    print "coverage: nothing to check (every package is excluded or has no statements)"
    exit 0
  }

  pct = 100 * ocov / otot
  verdict = (pct + 0 < omin + 0) ? "FAIL" : "ok"
  printf "%-4s %-45s %5.1f%% (min %s%%)\n", verdict, "all packages (total)", pct, omin
  exit (verdict == "FAIL")
}
' "$profile"
