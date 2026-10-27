#!/usr/bin/env bash
# The reference data is built in one place, so that golden-regen.yml (the
# pinned osxphotos) and upstream-check.yml (the latest one) build it alike.
#
# Input:  the output directory as the only argument; the osxphotos on PATH is
#         the version. Runs from the repository root.
# Output: <dir>/<library>/query.json and deleted.json with only the keys of
#         testdata/golden/fields.txt.
# Exit:   0 built; 1 a command failed or a file holds a path of the machine.
set -euo pipefail

# osxphotos writes a missing date in the local zone (photos_datetime.py:19-21).
export TZ=UTC LC_ALL=C

out="$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# $f and $k are jq variables.
# shellcheck disable=SC2016
listed_fields='($f | split("\n") | map(select(test("^[a-z]")))) as $k
  | map(with_entries(select(.key | IN($k[]))))'

for lib in testdata/libraries/*/; do
  [ -f "${lib}database/Photos.sqlite" ] || continue
  name=$(basename "$lib")
  # reference/*.sql is part of the library: apply it to a copy, as
  # testlib.OpenPristine does. The triggers of Photos.sqlite call Core Data
  # functions; the stand-ins are those of testlib.
  if compgen -G "${lib}reference/*.sql" >/dev/null; then
    copy="$work/$name"
    mkdir -p "$copy"
    cp -R "${lib}database" "$copy/"
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
    lib="$copy/"
  fi
  mkdir -p "$out/$name"
  osxphotos query --library "$lib" --json --mute |
    jq --rawfile f testdata/golden/fields.txt "$listed_fields" >"$out/$name/query.json"
  osxphotos query --library "$lib" --json --mute --deleted-only |
    jq --rawfile f testdata/golden/fields.txt "$listed_fields" >"$out/$name/deleted.json"
done

# A listed field may hold a path of the machine; -l prints the names of the
# files, never the data.
if grep -rlF -e "$work" -e "$HOME" -e "$PWD" "$out"; then
  echo "error: the files above contain a path of the machine" >&2
  exit 1
fi
