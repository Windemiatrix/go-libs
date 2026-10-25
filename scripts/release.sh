#!/usr/bin/env bash
# Creates and pushes the release tag of the osxphotos module. Run by the
# maintainer on a local machine, from anywhere inside the repository:
#
#   scripts/release.sh [--major]
#
# The version is computed by scripts/next-version.sh from the PR titles since
# the last tag; --major is the only way to move 0.x to 1.0.0. The script
# checks, in this order, and stops at the first failure:
#   1. origin/main and the tags are fetched;
#   2. the current branch is main, HEAD equals origin/main, the tree is clean;
#   3. the next version is computed and shown with the class of every title
#      (next-version.sh also checks that the module path in go.mod ends with
#      /vN exactly when the major version is 2 or more);
#   4. the tag exists neither locally nor in origin;
#   5. the "ci" workflow succeeded on this commit (event push, so the
#      aggregate job "ci-ok" is green);
#   6. the maintainer confirms.
# Then it creates a signed tag and pushes it. The workflow "release" runs on
# the tag push: it checks the commit, publishes the GitHub Release with the
# notes and warms up the Go proxy. A pushed tag is never moved: the Go proxy
# caches the version for good.
#
# AI agents do not run this script (see AGENTS.md). Titles are untrusted data;
# next-version.sh prints them escaped, nothing here is evaluated.
#
# Plain bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail
export LC_ALL=C

die() {
  echo "error: $*" >&2
  exit 1
}

major_flag=""
case "$#:${1-}" in
  0:) ;;
  1:--major) major_flag="--major" ;;
  *)
    echo "usage: $0 [--major]" >&2
    exit 2
    ;;
esac

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(git -C "$here" rev-parse --show-toplevel)" || die "not inside a git repository"
cd "$root"

echo "==> fetching origin/main and tags"
git fetch origin main --tags

branch="$(git symbolic-ref --short -q HEAD || true)"
[[ "$branch" == main ]] || die "the current branch is '${branch:-detached HEAD}', releases are made from main"
sha="$(git rev-parse HEAD)"
[[ "$sha" == "$(git rev-parse origin/main)" ]] || die "HEAD is not origin/main: pull or push first"
[[ -z "$(git status --porcelain)" ]] || die "the working tree is not clean"

echo "==> computing the next version"
tag="$(bash "$here/next-version.sh" ${major_flag:+"$major_flag"})" || die "no version was computed"

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  die "tag $tag already exists locally"
fi
if [[ -n "$(git ls-remote --tags origin "refs/tags/$tag")" ]]; then
  die "tag $tag already exists in origin"
fi

echo "==> checking CI on $sha"
runs="$(gh run list --workflow ci.yml --commit "$sha" --event push --status success --json databaseId --jq 'length')"
[[ "$runs" =~ ^[0-9]+$ && "$runs" -gt 0 ]] || die "no successful push run of ci.yml on $sha"

echo
echo "Tag:    $tag"
echo "Commit: $sha"
[[ -t 0 ]] || die "no terminal to ask for confirmation"
read -r -p "Create and push the signed tag $tag? [y/N] " answer
[[ "$answer" == y || "$answer" == Y ]] || die "cancelled"

git tag -s "$tag" -m "$tag"
git push origin "refs/tags/$tag"
echo "pushed $tag: the release workflow publishes the GitHub Release"
