#!/usr/bin/env bash
# Computes the next version of the osxphotos module from PR titles, and builds
# the release notes from the same titles.
#
# The title of a PR becomes the title of the squash commit on main (see
# scripts/check-pr-title.sh), so the history since the last tag says what
# changed. Only commits that touch osxphotos/ count: the repository can later
# hold other modules with their own tags.
#
# Usage, from anywhere inside the repository:
#   scripts/next-version.sh [--major]
#       stdout: the next tag, for example osxphotos/v0.2.0 (nothing else).
#       stderr: the titles since the last tag with their class, warnings, the
#       tag. Exit status 1 when there is nothing to release.
#   scripts/next-version.sh --notes <tag>
#       stdout: release notes in markdown for an existing tag: the titles
#       between the previous osxphotos/v* tag and <tag>, in sections.
#
# Version rule (the same classes as the notes sections):
#   type(scope)!: ...   before 1.0.0 -> minor, after 1.0.0 -> major
#   feat(scope): ...    -> minor
#   anything else, including a title outside the rule -> patch
# There is no BREAKING CHANGE footer: the body of a squash commit is empty.
# 1.0.0 is never derived from commits: only --major (scripts/release.sh --major)
# moves 0.x to 1.0.0. From major 2 the module path in go.mod must end with /vN
# (and must not end with it before that); the script checks it.
#
# A title is in the rule when it matches the PR title rule once the suffix
# " (#N)" that GitHub appends is cut off. The length limit is not checked: it
# applies to the PR title, the squash commit is longer by the suffix. Titles
# are untrusted data: they are never executed and are printed with every byte
# outside printable ASCII shown as "?".
#
# Plain bash 3.2 syntax: the script also runs on macOS.
set -euo pipefail

# C locale: a bracket range and a tr class are plain byte ranges.
export LC_ALL=C

module_dir="osxphotos"
tag_prefix="osxphotos/v"
tag_glob="osxphotos/v[0-9]*"
tag_re='^osxphotos/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
# Same expression as scripts/check-pr-title.sh.
rule_re='^(feat|fix|docs|style|refactor|test|chore)\(([a-z0-9._/-]+)\)(!?): [ -~]+$'
pr_suffix_re='^(.*) \(#[0-9]+\)$'

die() {
  echo "error: $*" >&2
  exit 1
}

usage() {
  echo "usage: $0 [--major] | --notes <tag>" >&2
  exit 2
}

# esc <text>: prints the text with every byte outside printable ASCII as "?".
esc() {
  printf '%s' "$1" | tr -c '[:print:]' '?'
}

# classify <title>: sets class to breaking, feat, fix, docs, other or invalid.
class=""
classify() {
  local t="$1"
  if [[ "$t" =~ $pr_suffix_re ]]; then
    t="${BASH_REMATCH[1]}"
  fi
  class=invalid
  if [[ "$t" =~ $rule_re ]]; then
    if [[ -n "${BASH_REMATCH[3]}" ]]; then
      class=breaking
    else
      case "${BASH_REMATCH[1]}" in
        feat) class=feat ;;
        fix) class=fix ;;
        docs) class=docs ;;
        *) class=other ;;
      esac
    fi
  fi
}

# collect <revision range>: classifies the titles of the commits in the range
# that touch the module, oldest first. Fills the globals below.
n_commits=0
has_breaking=0
has_feat=0
sec_breaking=""
sec_feat=""
sec_fix=""
sec_docs=""
sec_other=""
listing=""
warnings=""
collect() {
  local t et label
  while IFS= read -r t; do
    n_commits=$((n_commits + 1))
    classify "$t"
    et="$(esc "$t")"
    case "$class" in
      breaking)
        has_breaking=1
        sec_breaking+="- $et"$'\n'
        label=breaking
        ;;
      feat)
        has_feat=1
        sec_feat+="- $et"$'\n'
        label=feat
        ;;
      fix)
        sec_fix+="- $et"$'\n'
        label=patch
        ;;
      docs)
        sec_docs+="- $et"$'\n'
        label=patch
        ;;
      *)
        sec_other+="- $et"$'\n'
        label=patch
        ;;
    esac
    listing+="  $label  $et"$'\n'
    if [[ "$class" == invalid ]]; then
      warnings+="warning: title outside the PR title rule, counted as patch: $et"$'\n'
    fi
  done < <(git log --reverse --format=%s "$@" -- "$module_dir/")
}

# section <heading> <lines>: one markdown section, blank line between sections.
first_section=1
section() {
  [[ -n "$2" ]] || return 0
  if ((first_section)); then first_section=0; else echo; fi
  printf '## %s\n\n%s' "$1" "$2"
}

mode=version
force_major=0
notes_tag=""
while (($# > 0)); do
  case "$1" in
    --major) force_major=1 ;;
    --notes)
      mode=notes
      (($# >= 2)) || usage
      notes_tag="$2"
      shift
      ;;
    *) usage ;;
  esac
  shift
done
if [[ "$mode" == notes && "$force_major" == 1 ]]; then
  usage
fi

root="$(git rev-parse --show-toplevel)" || die "not inside a git repository"
cd "$root"

if [[ "$mode" == notes ]]; then
  [[ "$notes_tag" =~ $tag_re ]] || die "'$(esc "$notes_tag")' is not a tag of the form ${tag_prefix}X.Y.Z"
  commit="$(git rev-parse -q --verify "refs/tags/$notes_tag^{commit}")" || die "tag $notes_tag does not exist"
  # On the runner HEAD is the tagged commit: the base is searched from its
  # parent, otherwise describe finds the release tag itself and the range is empty.
  prev=""
  if git rev-parse -q --verify "$commit^" >/dev/null; then
    prev="$(git describe --tags --abbrev=0 --match "$tag_glob" "$commit^" 2>/dev/null || true)"
  fi
  if [[ -n "$prev" ]]; then
    collect "$prev..$commit"
  else
    collect "$commit"
  fi
  if ((n_commits == 0)); then
    echo "No changes in $module_dir/."
    exit 0
  fi
  section "Breaking changes" "$sec_breaking"
  section "Features" "$sec_feat"
  section "Bug fixes" "$sec_fix"
  section "Documentation" "$sec_docs"
  section "Other" "$sec_other"
  exit 0
fi

last="$(git describe --tags --abbrev=0 --match "$tag_glob" HEAD 2>/dev/null || true)"
major=0
minor=0
patch=0
if [[ -n "$last" ]]; then
  [[ "$last" =~ $tag_re ]] || die "the latest tag '$(esc "$last")' is not of the form ${tag_prefix}X.Y.Z"
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  collect "$last..HEAD"
else
  collect HEAD
fi

if ((n_commits == 0)); then
  die "no commits touching $module_dir/ since ${last:-the beginning}: nothing to release"
fi

if ((force_major)); then
  major=$((major + 1))
  minor=0
  patch=0
elif ((has_breaking)); then
  if ((major == 0)); then
    minor=$((minor + 1))
    patch=0
  else
    major=$((major + 1))
    minor=0
    patch=0
  fi
elif ((has_feat)); then
  minor=$((minor + 1))
  patch=0
else
  patch=$((patch + 1))
fi
next="${tag_prefix}${major}.${minor}.${patch}"

{
  echo "since ${last:-the beginning}: $n_commits commit(s) touching $module_dir/"
  printf '%s' "$listing"
  printf '%s' "$warnings"
  echo "next: $next"
} >&2

# The module path suffix must follow the major version (Go module rules).
mod_file="$module_dir/go.mod"
[[ -r "$mod_file" ]] || die "$mod_file not found"
mod_path="$(awk '$1 == "module" { gsub(/"/, "", $2); print $2; exit }' "$mod_file")"
if ((major >= 2)); then
  [[ "$mod_path" == *"/v$major" ]] || die "version $next needs the module path to end with /v$major in $mod_file, it is '$(esc "$mod_path")'"
elif [[ "$mod_path" =~ /v[0-9]+$ ]]; then
  die "version $next needs the module path without a /vN suffix in $mod_file, it is '$(esc "$mod_path")'"
fi

echo "$next"
