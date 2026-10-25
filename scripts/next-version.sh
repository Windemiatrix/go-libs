#!/usr/bin/env bash
# Computes the next release tag of the osxphotos module from the PR titles
# since the last tag, and writes the release notes of a tag from the same
# titles. The title of a PR becomes the title of the squash commit on main
# (scripts/check-pr-title.sh), so the history says what changed.
#
# Run:    scripts/next-version.sh [--major]      (scripts/release.sh calls it)
#         scripts/next-version.sh --notes <tag>  ("make release" calls it in CI)
#         from anywhere inside the repository.
# Input:  --major        the next major version; the only way from 0.x to 1.0.0
#         --notes <tag>  the release notes of an existing tag osxphotos/vX.Y.Z
# Output: without --notes, stdout has the next tag and nothing else, for example
#         osxphotos/v0.2.0; stderr lists every title with its class, the
#         warnings and the tag. With --notes, stdout has the notes in markdown.
# Exit:   0 done; 1 nothing to release, or a check failed; 2 wrong arguments.
#
# Version rule, by the titles since the last tag:
#
#   type(scope)!: ...   minor before 1.0.0, major from 1.0.0
#   feat(scope): ...    minor
#   anything else       patch, a title outside the rule too
#
# Only commits that touch osxphotos/ count: the repository may later hold other
# modules with their own tags. There is no BREAKING CHANGE footer: the body of
# a squash commit is empty. From major version 2 on, Go requires the module
# path in osxphotos/go.mod to end with /vN: a version that does not fit the
# path is refused.
#
# A title is checked against the PR title rule once the " (#N)" that GitHub
# appends is cut off. The length limit is not checked: it applies to the PR
# title, and the squash commit is longer by the suffix.
#
# Titles are untrusted data: they are never executed, and every byte outside
# printable ASCII is printed as "?".
#
# bash 3.2 compatible: the script also runs on macOS.
set -euo pipefail

# In the C locale a bracket range and a tr class are plain byte ranges.
export LC_ALL=C

module_dir="osxphotos"
module_go_mod="$module_dir/go.mod"
tag_prefix="osxphotos/v"
tag_glob="osxphotos/v[0-9]*"
tag_re='^osxphotos/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
# The rule of scripts/check-pr-title.sh with the type and the "!" captured:
# change both together.
title_re='^(feat|fix|docs|style|refactor|test|chore)\([a-z0-9._/-]+\)(!?): [ -~]+$'
pr_number_suffix_re='^(.*) \(#[0-9]+\)$'

# Arguments.
mode=version
force_major=0
notes_tag=""

# The titles collected from the history: markdown list items by section, and
# the listing and warnings shown on stderr.
commit_count=0
breaking_notes=""
feature_notes=""
fix_notes=""
docs_notes=""
other_notes=""
listing=""
warnings=""

# print_section puts a blank line between sections.
first_section=1

die() {
  echo "error: $1" >&2
  exit 1
}

usage() {
  echo "usage: scripts/next-version.sh [--major] | --notes <tag>" >&2
  exit 2
}

# esc <text>: the text with every byte outside printable ASCII shown as "?".
esc() {
  printf '%s' "$1" | tr -c '[:print:]' '?'
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --major)
        force_major=1
        ;;
      --notes)
        (($# >= 2)) || usage
        mode=notes
        notes_tag="$2"
        shift
        ;;
      *)
        usage
        ;;
    esac
    shift
  done
  if [[ "$mode" == notes ]] && ((force_major)); then
    usage
  fi
}

go_to_repository_root() {
  local root
  root="$(git rev-parse --show-toplevel)" || die "not inside a git repository"
  cd "$root"
}

# title_class <title>: prints breaking, feat, fix, docs, other or invalid.
title_class() {
  local title="$1"
  if [[ "$title" =~ $pr_number_suffix_re ]]; then
    title="${BASH_REMATCH[1]}"
  fi
  if ! [[ "$title" =~ $title_re ]]; then
    echo invalid
    return
  fi

  local commit_type="${BASH_REMATCH[1]}" breaking_mark="${BASH_REMATCH[2]}"
  if [[ -n "$breaking_mark" ]]; then
    echo breaking
    return
  fi
  case "$commit_type" in
    feat | fix | docs) echo "$commit_type" ;;
    *) echo other ;;
  esac
}

# listing_label <class>: breaking, feat or patch, the label shown next to the
# title on stderr.
listing_label() {
  case "$1" in
    breaking | feat) echo "$1" ;;
    *) echo patch ;;
  esac
}

add_title() {
  local title="$1" class shown
  class="$(title_class "$title")"
  shown="$(esc "$title")"

  commit_count=$((commit_count + 1))
  case "$class" in
    breaking) breaking_notes+="- $shown"$'\n' ;;
    feat) feature_notes+="- $shown"$'\n' ;;
    fix) fix_notes+="- $shown"$'\n' ;;
    docs) docs_notes+="- $shown"$'\n' ;;
    *) other_notes+="- $shown"$'\n' ;;
  esac

  listing+="  $(listing_label "$class")  $shown"$'\n'
  if [[ "$class" == invalid ]]; then
    warnings+="warning: title outside the PR title rule, counted as patch: $shown"$'\n'
  fi
}

# collect_titles <revision range>: adds the titles of the commits in the range
# that touch the module, oldest first.
collect_titles() {
  local title
  while IFS= read -r title; do
    add_title "$title"
  done < <(git log --reverse --format=%s "$1" -- "$module_dir/")
}

# The module tag before the commit, or nothing for the first release. The
# search starts at the parent: from the commit itself it would find the tag
# being released.
previous_tag() {
  local commit="$1"
  if git rev-parse -q --verify "$commit^" >/dev/null; then
    git describe --tags --abbrev=0 --match "$tag_glob" "$commit^" 2>/dev/null || true
  fi
}

# print_section <heading> <items>: one markdown section, nothing when there are
# no items.
print_section() {
  local heading="$1" items="$2"
  if [[ -z "$items" ]]; then
    return
  fi
  if ((first_section)); then
    first_section=0
  else
    echo
  fi
  printf '## %s\n\n%s' "$heading" "$items"
}

print_release_notes() {
  local tag="$1" commit base
  if ! [[ "$tag" =~ $tag_re ]]; then
    die "'$(esc "$tag")' is not a tag of the form ${tag_prefix}X.Y.Z"
  fi
  commit="$(git rev-parse -q --verify "refs/tags/$tag^{commit}")" || die "tag $tag does not exist"

  base="$(previous_tag "$commit")"
  if [[ -n "$base" ]]; then
    collect_titles "$base..$commit"
  else
    collect_titles "$commit"
  fi

  if ((commit_count == 0)); then
    echo "No changes in $module_dir/."
    return
  fi
  print_section "Breaking changes" "$breaking_notes"
  print_section "Features" "$feature_notes"
  print_section "Bug fixes" "$fix_notes"
  print_section "Documentation" "$docs_notes"
  print_section "Other" "$other_notes"
}

# next_version <major> <minor> <patch>: the version after this one, by the
# titles collected.
next_version() {
  local major="$1" minor="$2" patch="$3"
  if ((force_major)); then
    echo "$((major + 1)).0.0"
  elif [[ -n "$breaking_notes" ]] && ((major == 0)); then
    # Before 1.0.0 an incompatible change is a minor version (semver 0.x).
    echo "0.$((minor + 1)).0"
  elif [[ -n "$breaking_notes" ]]; then
    echo "$((major + 1)).0.0"
  elif [[ -n "$feature_notes" ]]; then
    echo "$major.$((minor + 1)).0"
  else
    echo "$major.$minor.$((patch + 1))"
  fi
}

module_path() {
  awk '$1 == "module" { gsub(/"/, "", $2); print $2; exit }' "$module_go_mod"
}

# check_module_path <major> <tag>: Go requires the module path to end with /vN
# from major version 2 on, and to have no such suffix before that.
check_module_path() {
  local major="$1" tag="$2" path
  if ! [[ -r "$module_go_mod" ]]; then
    die "$module_go_mod not found"
  fi
  path="$(module_path)"

  if ((major >= 2)) && [[ "$path" != *"/v$major" ]]; then
    die "version $tag needs the module path to end with /v$major in $module_go_mod, it is '$(esc "$path")'"
  fi
  if ((major < 2)) && [[ "$path" =~ /v[0-9]+$ ]]; then
    die "version $tag needs the module path without a /vN suffix in $module_go_mod, it is '$(esc "$path")'"
  fi
}

print_next_tag() {
  local last major=0 minor=0 patch=0 version next_major tag
  last="$(git describe --tags --abbrev=0 --match "$tag_glob" HEAD 2>/dev/null || true)"
  if [[ -n "$last" ]]; then
    if ! [[ "$last" =~ $tag_re ]]; then
      die "the latest tag '$(esc "$last")' is not of the form ${tag_prefix}X.Y.Z"
    fi
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    patch="${BASH_REMATCH[3]}"
    collect_titles "$last..HEAD"
  else
    collect_titles HEAD
  fi

  if ((commit_count == 0)); then
    die "no commits touching $module_dir/ since ${last:-the beginning}: nothing to release"
  fi

  version="$(next_version "$major" "$minor" "$patch")"
  next_major="${version%%.*}"
  tag="$tag_prefix$version"
  {
    echo "since ${last:-the beginning}: $commit_count commit(s) touching $module_dir/"
    printf '%s' "$listing"
    printf '%s' "$warnings"
    echo "next: $tag"
  } >&2

  check_module_path "$next_major" "$tag"
  echo "$tag"
}

parse_args "$@"
go_to_repository_root
if [[ "$mode" == notes ]]; then
  print_release_notes "$notes_tag"
else
  print_next_tag
fi
