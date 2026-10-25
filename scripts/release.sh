#!/usr/bin/env bash
# Creates and pushes the release tag of the osxphotos module. Pushing the tag
# starts the "release" workflow, which checks the commit, publishes the GitHub
# Release with the notes and warms up the Go proxy.
#
# Run:    scripts/release.sh [--major]
#         by the maintainer, on a local machine, from anywhere inside the
#         repository; AI agents do not run it (AGENTS.md).
# Input:  --major  the next major version; the only way from 0.x to 1.0.0.
#         Otherwise scripts/next-version.sh computes the version from the PR
#         titles since the last tag.
# Exit:   0 the tag is pushed; 1 a check failed or the release was cancelled;
#         2 wrong arguments; the status of git or gh when one of them fails.
#
# Steps, in order; the first failed check stops the script:
#   1. fetch origin/main and the tags;
#   2. check that the current branch is main, HEAD equals origin/main and the
#      working tree is clean;
#   3. compute the next version and show the class of every title
#      (next-version.sh also checks the /vN suffix of the module path);
#   4. check that the tag exists neither locally nor in origin;
#   5. check that the "ci" workflow succeeded on this commit (event push, so
#      the aggregate job "ci-ok" is green);
#   6. ask the maintainer to confirm, then create the signed tag and push it.
#
# A pushed tag is never moved: the Go proxy caches the version for good.
# Titles are untrusted data: next-version.sh prints them escaped, and nothing
# here evaluates them.
#
# bash 3.2 compatible: the script also runs on macOS.
set -euo pipefail
export LC_ALL=C

# Named explicitly: otherwise gh picks the repository from the git remotes,
# and one of them may be a fork.
repo_slug=Windemiatrix/go-libs

major_flag=""
# The commit that passed the checks; the tag goes on it even if HEAD moves
# while the script waits for the confirmation.
sha=""
tag=""

die() {
  echo "error: $1" >&2
  exit 1
}

usage() {
  echo "usage: scripts/release.sh [--major]" >&2
  exit 2
}

parse_args() {
  if (($# == 0)); then
    return
  fi
  if (($# == 1)) && [[ "$1" == --major ]]; then
    major_flag="--major"
    return
  fi
  usage
}

# The repository that holds this script, whatever the current directory.
go_to_repository_root() {
  local here root
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  root="$(git -C "$here" rev-parse --show-toplevel)" || die "not inside a git repository"
  cd "$root"
}

fetch_main_and_tags() {
  echo "==> fetching origin/main and tags"
  git fetch origin main --tags
}

require_clean_main() {
  local branch
  branch="$(git symbolic-ref --short -q HEAD || true)"
  if [[ "$branch" != main ]]; then
    die "the current branch is '${branch:-detached HEAD}', releases are made from main"
  fi
  sha="$(git rev-parse HEAD)"
  # In full: a local tag or branch named origin/main would otherwise win.
  if [[ "$sha" != "$(git rev-parse refs/remotes/origin/main)" ]]; then
    die "HEAD is not origin/main: pull or push first"
  fi
  if [[ -n "$(git status --porcelain)" ]]; then
    die "the working tree is not clean"
  fi
}

compute_tag() {
  echo "==> computing the next version"
  tag="$(bash scripts/next-version.sh ${major_flag:+"$major_flag"})" || die "no version was computed"
}

require_new_tag() {
  if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
    die "tag $tag already exists locally"
  fi
  if [[ -n "$(git ls-remote --tags origin "refs/tags/$tag")" ]]; then
    die "tag $tag already exists in origin"
  fi
}

require_green_ci() {
  local runs
  echo "==> checking CI on $sha"
  runs="$(gh run list --repo "$repo_slug" --workflow ci.yml --commit "$sha" --event push --status success --limit 1 --json databaseId --jq length)"
  if [[ "$runs" != 1 ]]; then
    die "no successful push run of ci.yml on $sha"
  fi
}

confirm() {
  local answer
  echo
  echo "Tag:    $tag"
  echo "Commit: $sha"
  if ! [[ -t 0 ]]; then
    die "no terminal to ask for confirmation"
  fi
  read -r -p "Create and push the signed tag $tag? [y/N] " answer
  if [[ "$answer" != y && "$answer" != Y ]]; then
    die "cancelled"
  fi
}

create_and_push_tag() {
  git tag -s "$tag" -m "$tag" "$sha"
  git push origin "refs/tags/$tag"
  echo "pushed $tag: the release workflow publishes the GitHub Release"
}

parse_args "$@"
go_to_repository_root
fetch_main_and_tags
require_clean_main
compute_tag
require_new_tag
require_green_ci
confirm
create_and_push_tag
