---
name: osxphotos-update
description: Use when the osxphotos module has to follow a new upstream release of osxphotos (RhetTbull/osxphotos), or to check whether there is one. Fetches the releases, diffs the ported upstream modules between the pinned tag and the new one, ports fixes and new behavior test first, and updates UPSTREAM.md, tools/golden/requirements.txt and the reference data.
---

The `osxphotos` module is a port of osxphotos, not a library of its own: upstream sets its scope. Whatever upstream fixes or adds in a ported module is ported, whether or not a known consumer uses it. The port follows releases (tags) only, never the upstream `main` branch.

Paths below are relative to the repository root. `UP` is a clone of upstream: `.cache/osxphotos` (ignored by git), or the path of a clone the maintainer already keeps. Its work tree always stays on the pinned tag, so that the `file:line` references in the code and in `UPSTREAM.md` match what is read.

## Steps

Run every command on its own.

1. **Pinned version.** Read the block in `osxphotos/UPSTREAM.md` (`Upstream version`, `Upstream tag`, `Tag commit`). `OLD` is that tag.

2. **Upstream clone.** No clone yet: `git clone --filter=blob:none https://github.com/RhetTbull/osxphotos .cache/osxphotos`, then `git -C .cache/osxphotos switch --detach OLD`.

3. **New releases.**
   - `git -C UP fetch --tags origin`
   - `git -C UP log --tags --simplify-by-decoration --format=%D -10`
   - Or ask the maintainer to run `gh workflow run upstream-check.yml`: it compares the latest osxphotos on PyPI with the pinned one and, when it is newer, runs the tests of the module on reference data built by it; the run summary and, on a difference, the issue `osxphotos <version>: the port differs from upstream` give the failed tests and the counts by key.
   - Nothing newer than `OLD`: report "nothing to update" and stop. Otherwise `NEW` is the latest tag; the releases in between are ported together with it.

4. **What changed in the ported code.** The files are the "Correspondence" table of `osxphotos/UPSTREAM.md` (paths inside the `osxphotos/` package of upstream), plus `osxphotos/_constants.py`, `tests/` and the dependency files of upstream.
   - `git -C UP diff --stat OLD NEW -- <files>`
   - `git -C UP log --oneline OLD..NEW -- <files>`: the commit titles carry the PR numbers and tell the purpose.
   - The full diff of each file: `git -C UP diff OLD NEW -- <file>`.
   - New test libraries: `git -C UP diff --stat OLD NEW -- 'tests/*.photoslibrary/database/Photos.sqlite'`.

5. **Triage.** A table in the PR description: commit, upstream file, what it changes, decision, Go package. Decisions:
   - **port**: a fix or new behavior in a ported module (the default for anything about the data the port reads);
   - **new schema**: new model numbers in `_constants.py` or a new test library: an entry in `internal/schema`, the database in `testdata/libraries/<name>/database/`, a row in `testdata/libraries/SOURCES.md`;
   - **not ported**: a module the port does not cover yet (command line; templates, export and PhotoKit until their packages exist): one line why, nothing else;
   - **already differs**: a change at a place listed in "Intentional differences" of `UPSTREAM.md`: reread the difference, update the reference or drop the difference.
   Ask the maintainer about unclear cases before writing code.

6. **Branch and code.** Branch `chore/upstream-<NEW>` from an up-to-date `main`; a large update goes in several PRs, one per package. Porting rules (`AGENTS.md`):
   - test first: the test reproduces the behavior of `NEW` and fails on the old code;
   - every ported rule cites `file:line` in `NEW`;
   - every upstream reference in the code and in `UPSTREAM.md` that points into a changed file is checked against `NEW` and fixed;
   - additions beyond osxphotos stay marked `// extension: not in osxphotos`.

7. **Version.** In one commit:
   - `osxphotos/UPSTREAM.md`: the three lines of the block (version, tag, `git -C UP rev-parse NEW`) and the references in "Intentional differences";
   - `tools/golden/requirements.txt`: `osxphotos==<version of NEW>` (otherwise `make check-repo` fails);
   - `git -C UP switch --detach NEW`, so the clone is on the new tag.

8. **Checks.** `make test`. A red `TestUpstreamRecord` or `check-repo` means the version differs between the two files.

9. **Reference data.** osxphotos `NEW` builds the reference data itself; it is never edited by hand:
   - push the branch and ask the maintainer to run `gh workflow run golden-regen.yml --ref chore/upstream-<NEW>`;
   - after the run: `make golden-fetch` and `git diff --stat testdata/golden`;
   - every change of the reference data is explained by a row of the triage table; an unexplained one is a porting error or a new divergence in `testdata/golden/divergences.json` with its reason and upstream line;
   - `make test` again.

10. **PR.** The title follows the change: `feat(osxphotos): port osxphotos <NEW>` with new behavior, otherwise `fix(osxphotos): …` or `chore(upstream): update osxphotos to <NEW>`; `!` only when the public API of the module changes. The merge releases a new version of the module.

## Do not

- Port commits of the upstream `main` branch that are not in a release.
- Change files in the upstream clone: only `fetch` and `switch --detach <tag>`.
- Edit `testdata/golden/*/query.json` or `deleted.json` by hand.
- Leave out a ported change because no known consumer uses it.
- Push, run workflows or create tags without the maintainer's confirmation (`AGENTS.md`, "Boundaries").

## Report

`OLD` → `NEW`; how many commits touched the ported files and how they were triaged (port / new schema / not ported / already differs); the branch, the PR and what waits for the maintainer (the `golden-regen` run).
