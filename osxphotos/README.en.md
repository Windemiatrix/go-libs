# osxphotos (Go)

**English** | [Русский](README.md)

`github.com/Windemiatrix/go-libs/osxphotos` is a Go library for reading the catalog of the macOS system Photos library, building paths from templates and working with PhotoKit. It is a port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos) (Rhet Turnbull, MIT) plus extensions on top of it; see [NOTICE](NOTICE).

## Status

**Design phase. There is no code and there are no versions yet.** Anything that depends on macOS or PhotoKit behavior and has not been verified is marked "stage 0" (research before implementation starts).

## Why

osxphotos is a thorough open-source parser of the Photos database, written in Python and geared toward exporting. The [photosync](https://github.com/Windemiatrix/photosync) application needs a Python-free binary, an in-memory catalog, stable identifiers for items and albums, and write access to Photos through PhotoKit. The library knows nothing about synchronization and suits any Go program that needs the Photos catalog.

## Principles

- **Read-only database access:** `Photos.sqlite` is read from a copy (with `-wal` and `-shm`) opened with `mode=ro`.
- **Schemas are data, not branching on a version.** Opening a library checks that every required table and column exists.
- **An error beats a guess:** an unknown schema yields `ErrUnsupportedSchema` carrying the model number.
- **Checked against a reference:** the ported behavior is compared with osxphotos itself.
- **cgo only in `photokit`;** everything else builds without cgo.
- **Dependencies:** `modernc.org/sqlite`, `golang.org/x/text`, `howett.net/plist` (reading the required plists: stage 0).

## Packages

`photosdb` (opening, schema, items, albums, system library path), `photoinfo` and `albuminfo` (plain structs with no reference back to the database), `query` (filters), `template` (path templates), `export` (file names on export and the ` (N)` rule), `photokit` (PhotoKit; without macOS and cgo, a stub with the same signatures). `template` and `export` perform no I/O.

## What is ported from osxphotos

Tag `v0.77.2` is ported.

- Reading items, resources, keywords and descriptions; albums and folders with hierarchy and trash.
- Bursts, Live Photos, RAW+JPEG, hidden and favorite items, the Shared Library, syndication.
- Dates with the item's time zone, the cloud master identifier, the original's fingerprint.
- A template subset: literals, `{field}`, `{field,default}`, the `lower` and `upper` filters; dates, `name`, `original_name`, `uuid`, `title`, `album`, `folder_album`, `media_type`. Other constructs are rejected with `ErrUnsupportedConstruct` and a position. Multi-valued fields behave as in osxphotos.
- The system library path.
- From PhotoKit: authorization status, fetching an item, requesting resource data with a timeout.

Not ported: Photos 7 and older, people and faces (only the `HasPersons` flag is read: whether named people are present), place names (coordinates are read), EXIF, search, export and sidecar files, shared albums, database repair, the command line. Also not ported: export through `PHImageManager`, because it returns the edited version rather than the original.

## What is added on top of osxphotos

Marked in code with `// extension: not in osxphotos` and listed in `UPSTREAM.md` (the file is yet to be created; it will also describe how updates to osxphotos are followed).

- `PhotoInfo` fields: `CloudAssetGUID` (uniqueness and stability: stage 0), `Resources` (sizes of Live and RAW resources without downloading), `DateModifiedRaw`, `BurstAlbumUUIDs`, Shared Library flags.
- `AlbumInfo.CloudGUID`, `FolderInfo.CloudGUID`: whether the column exists and the identifier is stable is **unverified (stage 0)**.
- `template.Registry` (custom fields), `RenderOptions.Sanitize` (the caller defines name sanitizing), the `{cloud_asset_guid}` field.
- `photokit`: import (including Live and RAW pairs), albums and folders, updating and deleting items, `CloudIdentifiers`, authorization with write access. osxphotos has none of this (it uses AppleScript), so there is no reference; behavior is pinned down by tests against real PhotoKit. Importing directly into the Shared Library is **unverified (stage 0)**.

Intentional differences: an unknown model is an error; the database is always copied; `Photos()` applies no implicit filters (the osxphotos selection is described in the documentation of `DB.Photos`); items without an additional-attributes row are counted in `Info.SkippedAssets` instead of vanishing silently; `TZOffset` and `InCloud` distinguish "unknown".

## macOS versions (planned)

Support is planned for the release Photos schemas for macOS 13-27; the schema is chosen by the database model number.

| Schema | Model | macOS | Verification |
|---|---|---|---|
| Photos 8 | 16320-16999 | 13 | osxphotos reference |
| Photos 9 | 17000-17599 | 14.0-14.5 | osxphotos reference |
| Photos 9.6 | 17600-17999 | 14.6+ | DDL fixtures |
| Photos 10 | 18201-18999 | 15 | osxphotos reference |
| Photos 11 | 19063-19319 | 26.0 | DDL fixtures |
| Photos 11.1 | 19320-19999 | 26.1+ | DDL fixtures |
| Photos 12 | 270000000-270999999 | 27 | osxphotos reference |

"osxphotos reference" means osxphotos has a test library with such a model (whether those libraries are usable is checked in stage 0). "DDL fixtures" verify table and column names but not values; such schemas count as unverified until compared on a real library; a comparison on the maintainer's library is a condition of the first release.

Will not be supported (`ErrUnsupportedSchema`): models below 16000, 16000-16319 (a pre-release build of macOS 13), 18000-18200 (the macOS 15 beta) and the gaps between ranges. A schema for a new macOS is never added blindly: without a library of that version it stays unsupported.

## How it is tested

- **Differential tests:** A, osxphotos test libraries; B, a synthetic database built from the DDL of real schemas with a reference from `osxphotos query --json`; C, a local run on the maintainer's library (not published; a condition of the first release). Items are compared by UUID; known divergences are listed with reasons. References are generated only by a separate manual workflow from a pinned osxphotos version.
- **Schemas:** range boundaries and gaps; every column that is read exists in the DDL of its schema.
- **Extensions:** osxphotos gives no reference for fields beyond it, for intentional differences, or for states its test libraries lack. These are checked with SQL migrations with synthetic values (`testdata/libraries/<name>/migrations/`): a test applies them to a temporary copy of a test library. The databases themselves stay unchanged copies of upstream, and the reference tests run on them. Checked:
  - cloud identifiers of items, albums and folders, including `NULL`, an empty string (counts as no identifier) and two items sharing one identifier;
  - the cloud master: present but not in iCloud (`InCloud == false`), no master (`InCloud == nil`);
  - `FingerprintSchema` and an empty fingerprint;
  - `Resources`, `IsMissing`, `HasRAW`;
  - the Shared Library: `SharedLibrary`, `LibraryScopeShareState`, `LibraryScope`;
  - `NULL` in the time zone, file name and creation date; `DateModifiedRaw` apart from `DateModified`;
  - bursts: every pick type, `BurstUnselected()`, `BurstAlbumUUIDs`;
  - trash without implicit filters; `Syndicated`, `SavedToLibrary`, `Shared`; `Info.SkippedAssets`.

  The description, the list of cases and how to add one are in [testdata/libraries/SOURCES.md](../testdata/libraries/SOURCES.md).
- **Templates and names:** tables, comparison with osxphotos rendering, parser fuzzing.
- **PhotoKit:** a stub and the Go layer over a fake bridge; Objective-C built with `-Werror`; "live" tests that modify the library run only by hand under a separate macOS account with its own Apple ID and a test library, never in CI and never against a primary library.
- `go test -race -shuffle=on`, a linter, coverage of at least 80%.

## Versioning

Semver, independent of the osxphotos version. `scripts/next-version.sh` computes the version from PR titles: `!` after the scope is an incompatible API change, `feat` is a new feature (minor), anything else is a fix (patch). Before `v1.0.0` an incompatible change gives a minor release. The release notes are the same titles in sections, on the GitHub Release page; there is no `CHANGELOG.md`. `v1.0.0` ships once a consuming application has proven the API.

## License

[MIT](LICENSE), (c) 2026 Windemiatrix. Credit for the ported project is in [NOTICE](NOTICE).
