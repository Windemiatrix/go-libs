#!/usr/bin/env bash
# Self-test for scripts/next-version.sh: builds throwaway git repositories with
# commits that touch osxphotos/ or not, and checks the computed version, the
# refusals, the escaping of hostile titles and the release notes. Run by the
# repo-checks job and by "make check-repo".
set -euo pipefail

# Byte semantics for the patterns that look for raw bytes in the output.
export LC_ALL=C

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/next-version.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
repo="$work/repo"

# The throwaway repositories must not read the configuration of the person who
# runs the test (commit signing would ask for a passphrase).
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
export HOME="$work/home"
mkdir -p "$HOME"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com

failures=0
counter=0
out="" err="" rc=0

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  failures=$((failures + 1))
}

g() {
  git -C "$repo" -c commit.gpgsign=false -c tag.gpgsign=false "$@"
}

set_module() {
  printf 'module %s\n\ngo 1.26.0\n' "$1" >"$repo/osxphotos/go.mod"
}

# commit <title> [path]: commits a change to a file, by default in osxphotos/.
commit() {
  local title="$1" path="${2:-osxphotos/file.txt}"
  counter=$((counter + 1))
  mkdir -p "$(dirname "$repo/$path")"
  echo "$counter" >>"$repo/$path"
  g add -A
  g commit -q -m "$title"
}

new_repo() {
  rm -rf "$repo"
  mkdir -p "$repo/osxphotos"
  g init -q
  set_module github.com/Windemiatrix/go-libs/osxphotos
  commit "chore(repo): seed"
}

# run <args>: runs next-version.sh in the repository, sets out, err and rc.
run() {
  rc=0
  out="$(cd "$repo" && bash "$script" "$@" 2>"$work/err")" || rc=$?
  err="$(cat "$work/err")"
}

# expect_tag <case> <tag> [args]: the script prints exactly the tag.
expect_tag() {
  local name="$1" want="$2"
  shift 2
  run "$@"
  if [[ "$rc" != 0 || "$out" != "$want" ]]; then
    fail "$name: want $want, got rc=$rc out='$out' err='$err'"
  fi
}

# expect_refusal <case> [args]: non-zero exit and nothing on stdout.
expect_refusal() {
  local name="$1"
  shift
  run "$@"
  if [[ "$rc" == 0 || -n "$out" ]]; then
    fail "$name: want a refusal, got rc=$rc out='$out'"
  fi
}

# expect_out <case> <want> [args]: exit 0 and exactly this stdout.
expect_out() {
  local name="$1" want="$2"
  shift 2
  run "$@"
  if [[ "$rc" != 0 || "$out" != "$want" ]]; then
    fail "$name: got rc=$rc, out:"$'\n'"$out"$'\n'"want:"$'\n'"$want"
  fi
}

# Without a tag the base is 0.0.0.
new_repo
commit "feat(photosdb): read albums (#1)"
expect_tag "first release, feat" osxphotos/v0.1.0

new_repo
commit "fix(photosdb): handle a locked database (#2)"
expect_tag "first release, fix" osxphotos/v0.0.1

# The classes and their priority, from a tag.
new_repo
git -C "$repo" tag osxphotos/v0.3.1
expect_refusal "no commits since the tag"
commit "feat(ci)!: break the workflow" scripts/other.sh
expect_refusal "only commits outside osxphotos/ are ignored"
commit "docs(readme): describe the schema (#3)"
expect_tag "docs is patch" osxphotos/v0.3.2
commit "chore(deps): bump a module (#4)"
expect_tag "chore is patch" osxphotos/v0.3.2
commit "fix(sqlread): retry (#5)"
expect_tag "fix is patch" osxphotos/v0.3.2
commit "feat(query): add filters (#6)"
expect_tag "feat is minor" osxphotos/v0.4.0
commit "feat(photosdb)!: rename the options (#7)"
expect_tag "breaking is minor before 1.0.0" osxphotos/v0.4.0
expect_tag "--major gives 1.0.0" osxphotos/v1.0.0 --major
expect_refusal "--major with --notes" --major --notes osxphotos/v0.3.1

# A breaking change outranks a feature, a feature outranks a fix (after 1.0.0).
new_repo
git -C "$repo" tag osxphotos/v1.2.3
commit "fix(a): b (#8)"
expect_tag "fix after 1.0.0" osxphotos/v1.2.4
commit "feat(a): c (#9)"
expect_tag "feat after 1.0.0" osxphotos/v1.3.0
commit "feat(a)!: d (#10)"
expect_refusal "major 2 needs /v2 in go.mod"
if [[ "$err" != *"/v2"* ]]; then
  fail "the refusal must name the /v2 suffix, got: $err"
fi
set_module github.com/Windemiatrix/go-libs/osxphotos/v2
commit "feat(osxphotos)!: move to v2 (#11)"
expect_tag "breaking after 1.0.0 with /v2" osxphotos/v2.0.0

# A /vN suffix that does not match the version is refused too.
new_repo
git -C "$repo" tag osxphotos/v1.2.3
set_module github.com/Windemiatrix/go-libs/osxphotos/v2
commit "fix(a): b (#12)"
expect_refusal "a /v2 suffix with version 1.x"

# Tags of other schemes are ignored.
new_repo
git -C "$repo" tag osxphotos/v0.1.0
commit "fix(a): b (#13)"
git -C "$repo" tag v9.9.9
commit "fix(a): c (#14)"
expect_tag "tag v9.9.9 is not a module tag" osxphotos/v0.1.1

# Titles outside the rule count as patch and are printed escaped.
new_repo
git -C "$repo" tag osxphotos/v0.5.0
commit "Add something"
commit "Feat(x)!: wrong case"
commit "feat!: no scope"
commit "feat(x): $(printf 'add \xd0\xbf\xd0\xbb\xd0\xb0\xd0\xbd')"
commit "fix(x): esc$(printf '\033')[31m end"
commit "fix(x): bidi $(printf '\xe2\x80\xae') end"
expect_tag "titles outside the rule are patch" osxphotos/v0.5.1
case "$err" in
  *"warning: title outside the PR title rule"*) ;;
  *) fail "no warning for titles outside the rule: $err" ;;
esac
case "$err" in
  *$'\033'*) fail "a raw ESC reached stderr" ;;
esac
case "$err" in
  *$'\xe2'*) fail "a raw non-ASCII byte reached stderr" ;;
esac
case "$err" in
  *'esc?[31m end'*) ;;
  *) fail "ESC is not shown as ?: $err" ;;
esac
case "$err" in
  *'bidi ??? end'*) ;;
  *) fail "the three bytes of the bidi mark are not shown as ???: $err" ;;
esac

# Usage errors.
expect_refusal "unknown flag" --bogus
expect_refusal "--notes without a tag" --notes
expect_refusal "no flags but a stray argument" osxphotos/v0.1.0

# Release notes: sections in a fixed order, oldest first, only osxphotos/.
new_repo
commit "feat(photosdb): read albums (#1)"
git -C "$repo" tag osxphotos/v0.1.0
commit "docs(readme): describe it (#2)"
commit "fix(sqlread): retry (#3)"
commit "feat(config)!: rename a key (#4)"
commit "chore(deps): bump a module (#5)"
commit "feat(ci): outside the module (#6)" scripts/other.sh
commit "Add something odd"
commit "feat(query): add filters (#7)"
git -C "$repo" tag osxphotos/v0.2.0

want_second='## Breaking changes

- feat(config)!: rename a key (#4)

## Features

- feat(query): add filters (#7)

## Bug fixes

- fix(sqlread): retry (#3)

## Documentation

- docs(readme): describe it (#2)

## Other

- chore(deps): bump a module (#5)
- Add something odd'
expect_out "notes of the second release" "$want_second" --notes osxphotos/v0.2.0

# The first release: no earlier tag, the whole history of the module.
want_first='## Features

- feat(photosdb): read albums (#1)

## Other

- chore(repo): seed'
expect_out "notes of the first release" "$want_first" --notes osxphotos/v0.1.0

# On the runner the release tag is checked out (HEAD is the tagged commit): the
# base of the notes is still the previous tag, and the next version has
# nothing to release.
git -C "$repo" checkout -q osxphotos/v0.2.0
expect_out "notes with the tag checked out" "$want_second" --notes osxphotos/v0.2.0
expect_refusal "no version for a tagged HEAD"

# A tag that touches nothing in the module.
new_repo
git -C "$repo" tag osxphotos/v0.1.0
commit "feat(ci): outside the module (#8)" scripts/other.sh
git -C "$repo" tag osxphotos/v0.1.1
expect_out "notes without module changes" "No changes in osxphotos/." --notes osxphotos/v0.1.1

# Bad tag arguments.
expect_refusal "a tag that does not exist" --notes osxphotos/v7.7.7
expect_refusal "a tag of another scheme" --notes v0.1.1
expect_refusal "an incomplete version" --notes osxphotos/v0.1

if ((failures > 0)); then
  echo "$failures next-version case(s) failed" >&2
  exit 1
fi
echo "next-version: all cases passed"
