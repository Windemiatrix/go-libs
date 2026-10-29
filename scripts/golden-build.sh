#!/usr/bin/env bash
# The reference data is built in one place, so that golden-regen.yml (the
# pinned osxphotos) and upstream-check.yml (the latest one) build it alike.
#
# Input:  the output directory as the only argument; the osxphotos on PATH is
#         the version. Runs from the repository root.
# Output: <dir>/<library>/query.json and deleted.json with only the keys of
#         testdata/golden/fields.txt; <dir>/<library>/templates.json, the
#         rendering of every template of testdata/golden/templates.txt by
#         "osxphotos query --field"; <dir>/parse.json, what the grammar of
#         osxphotos says about every line of testdata/golden/parse.txt.
# Exit:   0 built; non-zero a command failed, a template field is not allowed
#         or a file holds a path of the machine.
set -euo pipefail

# osxphotos writes a missing date in the local zone (photos_datetime.py:19-21);
# C keeps the glob of reference/*.sql in the byte order of testlib.applyDir.
export TZ=UTC LC_ALL=C

out="$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# This runs before anything of osxphotos or Python does: {function:<url>::f}
# downloads and runs foreign Python on the runner (phototemplate.py:1473-1479),
# {exiftool} starts a program (859-866), {detected_text} calls Vision. Only
# file:line is printed, so the log never repeats a template.
forbidden='\{function|\|function:|\{exiftool|\{detected_text|\{osxphotos_cmd_line'
hits=$(grep -nE "$forbidden" testdata/golden/templates.txt testdata/golden/parse.txt) || [ $? -eq 1 ]
if [ -n "$hits" ]; then
  echo "error: a template field that runs code is not allowed at:" >&2
  cut -d: -f1,2 <<<"$hits" >&2
  exit 1
fi

# The arrays are built once: the retry below walks the same templates.
ids=()
lines=()
args=()
while IFS= read -r line || [ -n "$line" ]; do
  if [[ -z $line || $line == '#'* ]]; then
    continue
  fi
  id=$(printf 't%03d' $((${#ids[@]} + 1)))
  ids+=("$id")
  lines+=("$line")
  args+=(--field "$id" "$line")
done <testdata/golden/templates.txt

# One call renders every template, so a refusal (an unmatched field) does not
# say which one failed. osxphotos is silent here: its message holds the
# template text, and only the id may reach the log.
first_failing_template() {
  for i in "${!ids[@]}"; do
    if ! osxphotos query --library "$1/" --json --mute --field uuid "{uuid}" --field "${ids[i]}" "${lines[i]}" >/dev/null 2>&1; then
      echo "error: template ${ids[i]} fails in osxphotos" >&2
      return
    fi
  done
  echo "error: osxphotos fails on the library, not on a template" >&2
}

# $f and $k are jq variables.
# shellcheck disable=SC2016
listed_fields='($f | split("\n") | map(select(test("^[a-z]")))) as $k
  | map(with_entries(select(.key | IN($k[]))))'

for lib in testdata/libraries/*/; do
  [ -f "${lib}database/Photos.sqlite" ] || continue
  name=$(basename "$lib")
  # osxphotos reads a copy: SQLite leaves -wal and -shm next to a database it
  # opens, and the tests check which libraries of testdata have a -wal.
  copy="$work/$name"
  mkdir -p "$copy"
  cp -R "${lib}database" "$copy/"
  # reference/*.sql is part of the library: apply it to the copy, as
  # testlib.OpenPristine does. The triggers of Photos.sqlite call Core Data
  # functions; the stand-ins are those of testlib.
  if compgen -G "${lib}reference/*.sql" >/dev/null; then
    python3 - "$copy/database/Photos.sqlite" "${lib}"reference/*.sql <<'EOF'
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
for name in ("NSCoreDataTriggerUpdateAffectedObjectValue",
             "NSCoreDataDATriggerUpdatedAffectedObjectValue",
             "NSCoreDataDATriggerInsertUpdatedAffectedObjectValue"):
    con.create_function(name, -1, lambda *args: None)
for path in sys.argv[2:]:
    with open(path) as f:
        con.executescript(f.read())
con.execute("PRAGMA wal_checkpoint(TRUNCATE)")
con.close()
EOF
  fi
  mkdir -p "$out/$name"
  osxphotos query --library "$copy/" --json --mute |
    jq --rawfile f testdata/golden/fields.txt "$listed_fields" >"$out/$name/query.json"
  osxphotos query --library "$copy/" --json --mute --deleted-only |
    jq --rawfile f testdata/golden/fields.txt "$listed_fields" >"$out/$name/deleted.json"
  if ! osxphotos query --library "$copy/" --json --mute --field uuid "{uuid}" "${args[@]}" >"$out/$name/templates.json" 2>/dev/null; then
    first_failing_template "$copy"
    exit 1
  fi
done

# The grammar of osxphotos accepts or rejects a line without running it; the
# Go parser is tested against this answer.
python3 - testdata/golden/parse.txt "$out/parse.json" <<'EOF'
import json, sys
from osxphotos.phototemplate import PhotoTemplateParser

with open(sys.argv[1], encoding="utf-8") as f:
    lines = f.read().split("\n")[:-1]
parser = PhotoTemplateParser()
out = {}
for i, line in enumerate(lines, 1):
    try:
        out[f"p{i:03d}"] = {"accept": True, "fields": parser.fields(line)}
    except Exception:
        out[f"p{i:03d}"] = {"accept": False}
with open(sys.argv[2], "w", encoding="utf-8") as f:
    json.dump(out, f, indent=1, sort_keys=True)
    f.write("\n")
EOF

# A listed field may hold a path of the machine; -l prints the names of the
# files, never the data.
if grep -rlF -e "$work" -e "$HOME" -e "$PWD" "$out"; then
  echo "error: the files above contain a path of the machine" >&2
  exit 1
fi
