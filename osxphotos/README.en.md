# osxphotos (Go)

**English** | [Русский](README.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

`github.com/Windemiatrix/go-libs/osxphotos` is a Go library for reading the macOS Photos library. An unofficial port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos) by Rhet Turnbull (MIT): the osxphotos author is not involved in this project, and the credit for decoding the database format is his ([NOTICE](NOTICE)).

## Status

**Early development.** Releases are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page; the API may change before `v1.0.0`. Reading photos, albums, folders and people, query filters and duplicate search work (`photosdb.Open`); path templates and PhotoKit are planned. Details: ["What exists and what is next"](#what-exists-and-what-is-next).

## Quick start

Requires Go 1.26+. Pure Go, no cgo; tested in CI on Linux and macOS. On Linux the module reads a copy of a `.photoslibrary` directory; the system library lives on a Mac (permissions: see "Safety of your photos").

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

```go
lib, err := photosdb.SystemLibraryPath()
if err != nil {
	log.Fatal(err) // errors.Is(err, photosdb.ErrLibraryNotFound)
}
db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
if err != nil {
	log.Fatal(err) // errors.Is(err, photosdb.ErrUnsupportedSchema): an unknown Photos version
}
defer db.Close()

for _, p := range db.Photos() {
	if p.InTrash { // Photos() returns trashed photos too
		continue
	}
	fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
}
```

## Why, when osxphotos exists

osxphotos is the most complete open decoder of the Photos database, but it is Python and a command line. Use this module when you write Go:

- one binary, no Python or `pip`;
- the whole catalog in memory as plain Go structs;
- stable identifiers of photos and albums, iCloud ones included, to recognize them across runs;
- an unknown database version is an error, not silently wrong data.

What you can build: laying files out by a template such as `2024/05-01 - Holidays/IMG_0001.HEIC`, a library report, a duplicates report.

It was written for [photosync](https://github.com/Windemiatrix/photosync), a tool that syncs Photos with Yandex Disk (in development), but it knows nothing about syncing.

## macOS versions

The Photos schema is chosen by the database model number.

| macOS | Photos | Database model | osxphotos test library |
|---|---|---|---|
| 13 | 8 | 16320–16999 | 13.0, 13.1 with iCloud |
| 14.0–14.5 | 9 | 17000–17599 | 14.0 |
| 14.6+ | 9.6 | 17600–17999 | 14.6 |
| 15 | 10 | 18201–18999 | 15.4 |
| 26.0 | 11 | 19063–19319 | 26.0 beta |
| 26.1+ | 11.1 | 19320–19999 | 26.1 |
| 27 | 12 | 270000000–270999999 | 27.0 developer beta |

Other models, including beta builds outside the ranges, give `ErrUnsupportedSchema`. Support for a new macOS is added only from a library of that version: [open an issue](https://github.com/Windemiatrix/go-libs/issues/new?template=new-macos.yml).

## Safety of your photos

- **The original is never changed.** Only a copy of `Photos.sqlite` is read, together with its `-wal` file (without `-shm`, which SQLite rebuilds), opened with `mode=ro`. The copy goes into a temporary directory with mode `0700` (files `0600`) and is deleted before `Open` returns, after an error as well; a temporary directory inside the library is refused.
- **The read packages** (`photosdb`, `photoinfo`, `albuminfo`, `adjustmentsinfo`, `query`) cannot write to the library. Only the future `photokit` package (macOS, cgo) will change Photos, and only if you import it and grant the macOS permission; macOS confirms a deletion from Photos in its own dialog (one per batch). Its tests that modify a library will run only by hand, under a separate account, never in CI.
- **No network.** The module makes no network calls and starts no processes; the linter enforces this (the `library` rule in [.golangci.yml](https://github.com/Windemiatrix/go-libs/blob/main/.golangci.yml)). No telemetry.
- **macOS permissions.** The library in `~/Pictures` is protected by the system: the program that reads it (for example, Terminal) needs Full Disk Access in System Settings → Privacy & Security.
- **The copy is your data:** places, names of people, captions, comments of shared albums. If you set `Options.TempDir`, pick a directory only you can read.
- **Do not attach `Photos.sqlite` to an issue:** it holds album names, paths and identifiers. The macOS version and the database model number are enough. Report vulnerabilities privately, see [SECURITY.en.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.en.md).

## What exists and what is next

| Feature | Package | State |
|---|---|---|
| Schema detection, macOS 13–27 | `photosdb` | done |
| System library path (`SystemLibraryPath`) | `photosdb` | done |
| Opening a library (`Open`): a `.photoslibrary` directory or a database file, also outside its library (`Options.LibraryPath` says where the originals are); the database path and Photos version (`Info.DBPath`, `Info.PhotosVersion`) | `photosdb` | done |
| Photos: dates with time zone and its name, sizes, keywords, Live, RAW+JPEG, bursts, iCloud, Shared Library | `photosdb`, `photoinfo` | done |
| Photo flags: HDR, panorama, portrait, selfie, spatial, slow motion, time-lapse, screen recording, shared moment, edited in another app, visibility, who imported it | `photosdb`, `photoinfo` | done |
| Files on disk: original, Live, RAW, edited version and its Live video, adjustment files, previews (`PathEdited`, `PathEditedLive`, `AdjustmentsPath`, `OriginalAdjustmentsPath`, `PathDerivatives`); edit data: `Adjustments`, `OriginalAdjustments` | `photosdb`, `photoinfo`, `adjustmentsinfo` | done |
| People on photos: names (`Persons`; a face without a name is `_UNKNOWN_`), for the library `DB.Persons`, `DB.PersonCounts` | `photosdb`, `photoinfo` | done |
| Keywords of the library, their counts and those without a photo (`Keywords`, `KeywordCounts`, `KeywordsWithoutPhoto`) | `photosdb` | done |
| Albums: title, folder path, members, dates, trash, sort order and place in the sidebar (`AlbumInfo`, `Album`; photos in the order of the album and the place of a photo in it: `AlbumPhotos`, `PhotoIndex`); shared albums: `SharedAlbumInfo`, `SharedAlbumTitles`, the owner: `Owner` | `photosdb`, `albuminfo` | done |
| Import sessions: title, dates, photos, the session of each photo (`ImportInfo`, `Import`, `ImportUUID`) | `photosdb`, `albuminfo` | done |
| Projects ("My Projects"): title, dates, photos, the projects of each photo (`ProjectInfo`, `Project`, `ProjectInfoOf`, `ProjectUUIDs`) | `photosdb`, `albuminfo` | done |
| The shared moment and shared library of a photo (`SharedMomentInfo`, `ShareInfo`), the participants of that library (`ShareParticipantInfo`, `ShareParticipants`) | `photosdb`, `photoinfo` | done |
| The comments and likes of a photo in a shared album (`Comments`, `Likes`) | `photosdb`, `photoinfo` | done |
| Folders and their tree, place in the sidebar (`FolderInfo`, `Folder`, `FolderList`) | `photosdb`, `albuminfo` | done |
| Selection `Query` with `query.Options` filters (album, folder, dates, trash, type, flags, person and others), sorting; the selection with the arguments of `photos()`: `PhotosWith` | `photosdb`, `query` | done |
| Duplicates (`DB.Duplicates`, `query.Duplicates`, the `Duplicate` option) | `photosdb`, `query` | done |
| Path templates | `template` | planned |
| File names on export | `export` | planned |
| Import, albums, deletion through PhotoKit | `photokit` | planned |

**Not included:** a command line, exporting files and sidecars, face data other than the names of people (face position and size), place names (coordinates are read), EXIF, search by photo content, database repair, Photos 7 and older. osxphotos itself covers these.

## How it differs from osxphotos

Tag `v0.77.2` is ported. The same database reading plus: cloud identifiers of photos and albums, sizes of Live and RAW resources without downloading, a strict error instead of a guess on an unknown schema, and `Photos()` without hidden filters (`Query(query.Options{})` makes the osxphotos selection: photos and movies outside the trash, without unselected burst frames). Later: writing to Photos through PhotoKit. The full list of extensions and intentional differences, with links to the osxphotos source, is in [UPSTREAM.md](UPSTREAM.md); extensions are marked in code with `// extension: not in osxphotos`.

## How it is verified

Every PR passes tests with `-race` on Linux and macOS, a linter and a coverage threshold of 80%. The output of `Open` is compared with the osxphotos reference results (v0.77.2) for its 8 test libraries (7 schemas, from macOS 13.0 to 27.0 developer beta) and for a synthetic database; known divergences are recorded with reasons. Fields beyond osxphotos are checked with SQL migrations with synthetic values on a temporary copy of the database ([SOURCES.md](https://github.com/Windemiatrix/go-libs/blob/main/testdata/libraries/SOURCES.md)). With `make diff-local` (below) the port was also compared with osxphotos on the maintainer's own library, schema 11.1 (macOS 26.6): no differences.

### Checking against your own library

1. Install osxphotos 0.77.2 into its own environment (Python 3.10 or newer is required): `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Quit Photos, open the library package in Finder (Show Package Contents) and copy its `database/` folder to `~/Pictures/Copy.photoslibrary/database/`. The original photos are not needed.
3. From the repository root: `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`. The paths are absolute; `REPORT` is optional.

On a successful run the terminal gets only numbers and key names: how many objects and keys were compared, how many differences. Error messages of Go may name the paths you passed; the messages of osxphotos stay out of the terminal, and when osxphotos fails, run it by hand to see why. The summary holds the object count of your library: in an issue, paste only the per-key lines. The UUIDs and values of the differences go only to the `REPORT` file, with mode 0600 (an existing file is overwritten): it holds data of your library, do not publish it. The osxphotos output and the database copy live in a temporary directory and are removed after the check, even an interrupted one. After the check, delete the copy `~/Pictures/Copy.photoslibrary` too.

## Versioning

Semver by tags `osxphotos/vX.Y.Z`, independent of osxphotos. Before `v1.0.0` incompatible changes are possible (the minor number grows). Changes are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page. `v1.0.0` ships once a consuming application has proven the API. How the version is computed: [CONTRIBUTING.en.md](https://github.com/Windemiatrix/go-libs/blob/main/CONTRIBUTING.en.md#releases-for-the-maintainer).

## FAQ

- **Can I read while Photos is open?** Yes: a copy of the database is read together with its `-wal` file, and Photos is not blocked.
- **Does it read the photos themselves?** No: only the catalog (metadata), the paths of files in the library (originals, edited versions, adjustment files and previews) and the adjustment files themselves (property lists). Photo files are never opened.
- **Photos only in iCloud ("Optimize Storage")?** The catalog is read in full; for a photo whose original is not on disk, `Path` is empty, and `IsMissing`/`InCloud` show where it is.

## License

MIT. A port of [osxphotos](https://github.com/RhetTbull/osxphotos) (MIT, Rhet Turnbull); the test databases are taken from its tests unchanged (see [NOTICE](NOTICE)).

## Feedback

[Issues](https://github.com/Windemiatrix/go-libs/issues) for bugs and new macOS versions, [Discussions](https://github.com/Windemiatrix/go-libs/discussions) for questions and ideas.
