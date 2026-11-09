# osxphotos (Go)

**English** | [Русский](README.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

`github.com/Windemiatrix/go-libs/osxphotos` is a Go library for the macOS Photos library: it reads the catalog, renders osxphotos path templates, names export files, and through PhotoKit exports, imports and changes assets. An unofficial port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos) by Rhet Turnbull (MIT): the osxphotos author is not involved in this project, and the credit for decoding the database format is his ([NOTICE](NOTICE)).

## Status

**Early development.** Releases are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page; the API may change before `v1.0.0`. Working: reading the catalog (`photosdb.Open`): photos, albums, folders, people and faces, places, EXIF, scores, search labels; query filters and duplicate search; path templates (`template`); export file names (`export`); PhotoKit (`photokit`, macOS only). Details: ["Features"](#features).

## Quick start

Requires Go 1.26+. Every package but `photokit` is pure Go without cgo; they are tested in CI on Linux and macOS. `photokit` works only on macOS in a cgo build; in other builds every call of it returns `photokit.ErrNotSupported`. On Linux the module reads a copy of a `.photoslibrary` directory; the system library lives on a Mac (permissions: see ["Safety of your photos"](#safety-of-your-photos)).

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

- **The library database is never changed.** Only a copy of `Photos.sqlite` is read, together with its `-wal` file (without `-shm`, which SQLite rebuilds), opened with `mode=ro`, and so is a copy of the search database (`search/psi.sqlite` or `search/leo.sqlite`) when there is one. The copy goes into a temporary directory with mode `0700` (files `0600`) and is deleted before `Open` returns, after an error as well; a temporary directory inside the library is refused.
- **Only `photokit` can change Photos** (macOS, cgo), and only if you import it, macOS granted the Photos permission and you call a method that changes: `Import`, the folder and album methods, `Delete`, `Update`. The other packages cannot write to the library: `photosdb` reads the database copy and the adjustment files, and `photoinfo`, `albuminfo`, `adjustmentsinfo`, `query`, `template`, `export`, `exifinfo`, `placeinfo`, `personinfo`, `momentinfo`, `scoreinfo`, `searchinfo` do no I/O.
- **The user decides on deletion and hiding.** `Delete` and `Update` with `Hidden: true` ask through a macOS dialog, `DeleteCollections` may ask; when `ctx` ends before the answer (`ErrOutcomeUnknown`), the asset may still change; see ["PhotoKit"](#photokit).
- **Network only through PhotoKit.** The module itself makes no network calls and starts no processes; the linter enforces this (the `library` rule in [.golangci.yml](https://github.com/Windemiatrix/go-libs/blob/main/.golangci.yml)). No telemetry. An export through `photokit` lets PhotoKit download from iCloud an original that is not on disk.
- **macOS permissions.** The library in `~/Pictures` is protected by the system: the program that reads it through `photosdb` (for example, Terminal) needs Full Disk Access in System Settings → Privacy & Security. `photokit` needs the Photos permission; who gets it is described in ["PhotoKit"](#photokit).
- **The copy is your data:** places, names of people, captions, comments of shared albums. If you set `Options.TempDir`, pick a directory only you can read. The bytes of `DB.JSON`, and `Metadata` and `Path` of `photokit.ImageInfo`, hold paths and personal data: do not log them.
- **Do not attach `Photos.sqlite` to an issue:** it holds album names, paths and identifiers. The macOS version and the database model number are enough. Report vulnerabilities privately, see [SECURITY.en.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.en.md).

## Features

| Feature | Package |
|---|---|
| Schema detection, macOS 13–27 | `photosdb` |
| System library path (`SystemLibraryPath`) | `photosdb` |
| Opening a library (`Open`): a `.photoslibrary` directory or a database file, also outside its library (`Options.LibraryPath` says where the originals are); the database path and Photos version (`Info.DBPath`, `Info.PhotosVersion`) | `photosdb` |
| Photos: dates with time zone and its name, sizes, keywords, Live, RAW+JPEG, bursts, iCloud, Shared Library | `photosdb`, `photoinfo` |
| Photo flags: HDR, panorama, portrait, selfie, spatial, slow motion, time-lapse, screen recording, shared moment, edited in another app, visibility, who imported it | `photosdb`, `photoinfo` |
| Files on disk: original, Live, RAW, edited version and its Live video, adjustment files, previews (`PathEdited`, `PathEditedLive`, `AdjustmentsPath`, `OriginalAdjustmentsPath`, `PathDerivatives`); edit data: `Adjustments`, `OriginalAdjustments` | `photosdb`, `photoinfo`, `adjustmentsinfo` |
| People on photos: names (`Persons`; a face without a name is `_UNKNOWN_`), for the library `DB.Persons`, `DB.PersonCounts` | `photosdb`, `photoinfo` |
| People and faces: `DB.PersonInfo`, `Person`, `PersonInfoOf`, `PersonPhotos`; faces with position and size: `FaceInfoOf`, `PersonFaces` | `photosdb`, `personinfo` |
| The EXIF data that Photos stores in its database on import (`PhotoInfo.ExifInfo`) | `photosdb`, `photoinfo`, `exifinfo` |
| Place: names and address (`PhotoInfo.Place`) | `photosdb`, `photoinfo`, `placeinfo` |
| Scores of a photo (`PhotoInfo.Score`): each is a `*float64`, nil when there is no value | `photosdb`, `photoinfo`, `scoreinfo` |
| Moments (`DB.MomentInfoOf`, `MomentPhotos`) | `photosdb`, `momentinfo` |
| The search data of Photos and labels (`DB.SearchInfoOf`, `Labels`, `LabelsNormalized`, `LabelCounts`, `LabelCountsNormalized`) | `photosdb`, `searchinfo` |
| Cloud metadata of a photo (`DB.CloudMetadataOf`) | `photosdb` |
| A photo as the dictionary and JSON of osxphotos (`DB.AsDict`, `DB.JSON`, `DB.Hexdigest`) | `photosdb` |
| Keywords of the library, their counts and those without a photo (`Keywords`, `KeywordCounts`, `KeywordsWithoutPhoto`) | `photosdb` |
| Albums: title, folder path, members, dates, trash, sort order and place in the sidebar (`AlbumInfo`, `Album`; photos in the order of the album and the place of a photo in it: `AlbumPhotos`, `PhotoIndex`); shared albums: `SharedAlbumInfo`, `SharedAlbumTitles`, the owner: `Owner` | `photosdb`, `albuminfo` |
| Import sessions: title, dates, photos, the session of each photo (`ImportInfo`, `Import`, `ImportUUID`) | `photosdb`, `albuminfo` |
| Projects ("My Projects"): title, dates, photos, the projects of each photo (`ProjectInfo`, `Project`, `ProjectInfoOf`, `ProjectUUIDs`) | `photosdb`, `albuminfo` |
| The shared moment and the Shared Library of a photo (`SharedMomentInfo`, `ShareInfo`), the participants of that Shared Library (`ShareParticipantInfo`, `ShareParticipants`) | `photosdb`, `photoinfo` |
| The comments and likes of a photo in a shared album (`Comments`, `Likes`) | `photosdb`, `photoinfo` |
| Folders and their tree, place in the sidebar (`FolderInfo`, `Folder`, `FolderList`) | `photosdb`, `albuminfo` |
| Selection `Query` with `query.Options` filters (album, folder, dates, trash, type, flags, person, the label `Label`, a regular expression over a template `Regex` and others), sorting; the selection with the arguments of `photos()`: `PhotosWith` | `photosdb`, `query` |
| Duplicates (`DB.Duplicates`, `query.Duplicates`, the `Duplicate` option) | `photosdb`, `query` |
| Path templates: parsing (`template.Parse`), rendering (`DB.RenderTemplate`, `Renderer.Render`), the fields of a photo (`DB.Fields`), name sanitizing (`SanitizeFilename`, `SanitizeDirname`) | `template`, `photosdb` |
| Export file names (`Names`, `EditedName`, `IncrementName`, `StemKey`) | `export` |
| PhotoKit: authorization, fetching assets, exporting resources and versions, import, folders and albums, deletion, updates, cloud identifiers | `photokit` |

**Not included:** a command line; `osxphotos export` itself, of which only the file names are ported (`export`), while files are exported by `photokit`; sidecars and exiftool; template fields that run code or programs; media analysis results (`media_analysis`, `ai_caption`), which live in the database of another program; direct access to the database tables (`PhotoTables`); database repair; Photos 7 and older. osxphotos itself covers these.

## Templates

The `template` package ports the whole grammar of the osxphotos template language — fields, filters, conditions, `[find,replace]`, `&`, `?` and default values; the fields and values the port does not render are listed below.

```go
t, err := template.Parse("{created.year}/{created.mm}-{created.dd} - {album,No album}/{photo.original_filename}")
if err != nil {
	log.Fatal(err) // errors.Is(err, template.ErrSyntax): the error names the byte offset
}
for _, p := range db.Query(query.Options{}) {
	paths, unmatched, err := db.RenderTemplate(p, t, template.RenderOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(paths, unmatched) // [2024/05-01 - Holidays/IMG_0001.HEIC] []
}
```

A multi-valued field gives one string per value: a photo in two albums gives two paths. `unmatched` gets the names that osxphotos does not have. With `RenderOptions.Filename` the result is a file name, with `RenderOptions.Dirname` a directory path: the values of fields are sanitized (`SanitizeFilename`, `SanitizeDirname`), but a file name may still be `..`, and a directory path may hold `..` and the `/` of the template text, so the caller keeps the path inside its directory.

`DB.RenderTemplate` reads the fields through `DB.Fields(p)`, an implementation of `template.Photo`. The interface is minimal, one method `Field(name)`: a program can implement it for its own data and render through `template.NewRenderer().Render(t, photo, o)`.

| Group | Fields |
|---|---|
| Text and flags | `name` and `original_name` (the name without extension), `title`, `descr`, `uuid`, `shortuuid`, `id`, `hdr`, `edited`, `favorite`, `burst`, `media_type`, `photo_or_video`, `imported_by.name` |
| Dates | `created`, `modified`, `today` with the parts `.year`, `.mm`, `.month`, `.dd`, `.dow`, `.hour` and others; `.strftime,<format>` |
| Multi-valued | `album`, `folder_album`, `project`, `album_project`, `folder_album_project`, `keyword`, `person`, `comment`, `label`, `label_normalized` |
| Number in an album | `album_seq`, `folder_album_seq` (by `RenderOptions.DestPath`) |
| Utility | `{var:name,value}` and `%name`, `counter`, `format`, `strip`, `shell_quote`, `export_dir`, `filepath`, `osxphotos_version`, `edited_version`, punctuation (`comma`, `pipe`, `openbrace` and others) |
| Photo properties | `photo.X`: the properties of `PhotoInfo` by their osxphotos names; the nested `photo.score.X`, `photo.exif_info.X`, `photo.place.X`, `photo.search_info.X`, `photo.search_info_normalized.X`; `photo.hexdigest` |
| Module fields | `place.*` (such as `place.name`, `place.address.city`), `exif.camera_make`, `exif.camera_model`, `exif.lens_model`, `moment`, `searchinfo.season`, `searchinfo.holiday`, `searchinfo.activity`, `searchinfo.venue`, `searchinfo.venue_type` |

The full list of fields and the syntax are in [phototemplate.md](https://github.com/RhetTbull/osxphotos/blob/v0.77.2/osxphotos/phototemplate.md) of osxphotos; what each photo field gives is in the godoc of [`DB.Fields`](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos/photosdb#DB.Fields).

- **Not ported:** `{function}` and the filter `function:` (Python code), `{exiftool}` (an external program), `{detected_text}` (macOS text recognition), `{osxphotos_cmd_line}` (the osxphotos command line). The library runs no foreign code or programs, so such a field is an error that wraps `template.ErrUnsupportedField`. A field of osxphotos that the implementation of `template.Photo` does not give is the same error.
- **Limits:** a render of more than `RenderOptions.MaxResults` strings (by default `DefaultMaxResults`, 10,000) or of more than 16 MiB of strings is `ErrTooManyValues`. Multi-valued fields multiply, and their values come from the library, from other participants of shared albums too.
- **Counters** of `{counter}` belong to a `template.Renderer`: a `DB` has one, and `{counter}` grows from photo to photo while the library is open. To start anew, render with a `Renderer` of your own (`NewRenderer`) and `DB.Fields(p)`; `ResetCounters` and `ResetCounter` reset its counters.
- **Month and day names** are English (the C locale), as in the reference; osxphotos takes the locale of the user.
- **Render errors:** `ErrUnsupportedField`, `ErrTooManyValues`, `ErrUnsupportedConstruct` (such as a `strftime` directive or a format specification the port does not know), `ErrRender` (what osxphotos raises an exception on). The other differences from osxphotos are in [UPSTREAM.md](UPSTREAM.md).

## Export names

The `export` package names files as `osxphotos export` does and writes no files itself: the caller reads the directory and writes the files.

```go
taken := map[string]bool{} // export.StemKey(name) of every file of the directory
name, _ := export.IncrementName(p.OriginalFilename, 0, taken) // IMG_0001.HEIC or IMG_0001 (1).HEIC
files := export.Names(p, name)                                 // files.Live: IMG_0001.mov
```

- `IncrementName(name, count, taken)` adds ` (N)` before the extension while the stem of the name is taken; stems are compared without the extension and ignoring case, and `StemKey` gives the keys of `taken`.
- `Names(p, name)` gives the companions of the already numbered name: the Live video (`.mov`), RAW, preview, AAE adjustment files.
- `EditedName(p)` is the name of the edited version: `<stem>_edited` and the extension of the edited file.
- The names come from the library: sanitize them before you write a file.

## PhotoKit

The `photokit` package works with the system library through PhotoKit, only on macOS in a cgo build; otherwise every call returns `ErrNotSupported`.

**Authorization.**

- The process that calls PhotoKit must carry an `Info.plist` with `NSPhotoLibraryUsageDescription`: without it macOS ends the process on the first request for access. The library does not embed one; the program does, for example with the linker flag `-sectcreate __TEXT __info_plist <file>`, as the test binary of the module does.
- `AuthorizationStatus` returns the status without a dialog, `RequestAuthorization(ctx)` asks the user once. `Open` does not ask: without the permission it is `ErrNotAuthorized`; limited access (`StatusLimited`) is refused too, because a limited library shows only the selected items.
- After the first dialog the Photos permission goes to the Terminal of the account and this read-write access to the whole library holds for every program started from that Terminal; a process started by `launchd` gets a permission of its own. So the tests that modify a library run only under a separate account.

**What `Library` gives.**

- `Fetch`, `Resources`: `Asset` values and their resources by the UUID of `photosdb` or by `LocalIdentifier`; `FetchBurst`: the frames of a burst by its identifier.
- `Export`: one resource of an asset (`ResourcePhoto`, `ResourcePairedVideo`, `ResourceAlternatePhoto` and others) as Photos stores it.
- `ExportImage`, `ExportVideo`, `ExportLivePhoto`: a version of a photo, a video or one half of a Live Photo (`VersionCurrent` with edits, `VersionUnadjusted`, `VersionOriginal`). The current version of a slow-motion video is rendered anew, so its bytes may differ between calls.
- `Import`: a new asset from files (`ImportRequest`: a photo or a video, a Live Photo, RAW+JPEG, albums).
- `Collections`, `CreateFolder`, `CreateAlbum`, `RenameCollection`, `DeleteCollections`, `AddToAlbum`, `RemoveFromAlbum`: folders and albums.
- `Delete` moves assets to Recently Deleted; `Update` (`AssetPatch`) changes the date, the favorite and hidden flags and the location.
- `CloudIdentifiers`, `LocalIdentifiers`: the cloud identifier of an asset and back.

**Files.** An export writes into the file `name` of the directory `dir` (during the call a hidden temporary file or directory may appear there too; it is removed before the return): `name` is one path element, otherwise `ErrInvalidName`. The file is created with mode `0600`; an existing one is `fs.ErrExist` unless `ExportOptions.Overwrite` is set. After an error a file the export created is removed and an existing one is left as it was. The `export` package gives the names.

**Dialogs and deadlines.**

- `Delete` (one dialog per batch) and `Update` with `Hidden: true` show a macOS dialog, `DeleteCollections` may show one; no dialog is expected from an import or the other folder and album methods. A refusal of the user is `ErrUserCanceled`, and nothing was changed.
- Only `ctx` sets the deadline (`RequestAuthorization` without one waits 5 minutes). `Fetch`, `FetchBurst`, `Resources`, `Collections`, `CloudIdentifiers` and `LocalIdentifiers` check `ctx` only before the call; an export is canceled when `ctx` ends and returns within 30 s. When `ctx` ends before PhotoKit reports the outcome of a change, the error is `ErrOutcomeUnknown` together with the error of `ctx` (the same error comes with `context.DeadlineExceeded` when PhotoKit commits a change that raised an exception): the change may still happen; read the library again before the next change.
- `Fetch` and the methods that look an asset up the same way (exports, `Delete`, `Update`, `AddToAlbum`) do not find hidden assets, as in osxphotos; with the Hidden album locked, as Photos has it by default, macOS does not give them out even on request. An asset hidden through `Update` is found neither by `Fetch` nor by `Update` any more.
- `photokit` has not been tested with a Shared Library.

Other errors: `ErrNotFound`, `ErrMediaType`, `ErrNoResource`, `ErrInvalidVersion`, `ErrExport`, `ErrImport`, `ErrImportFile`, `ErrInvalidTitle`, `ErrChange`. The differences from `photokit.py` are in [UPSTREAM.md](UPSTREAM.md).

## How it differs from osxphotos

Tag `v0.77.2` is ported. The same database reading plus: cloud identifiers of photos and albums, sizes of Live and RAW resources without downloading, a strict error instead of a guess on an unknown schema, and `Photos()` without hidden filters (`Query(query.Options{})` makes the osxphotos selection: photos and movies outside the trash, without unselected burst frames). In `photokit`, beyond `photokit.py`: import, folders and albums, deletion, updates of assets and cloud identifiers. The full list of extensions and intentional differences, with links to the osxphotos source, is in [UPSTREAM.md](UPSTREAM.md); extensions are marked in code with `// extension: not in osxphotos`.

## How it is verified

Every PR passes tests with `-race` on Linux and macOS, a linter and a coverage threshold of 80%. The output of `Open` is compared with the osxphotos reference (v0.77.2): with `osxphotos query --json` (`TestGolden`), and the rendered templates with `osxphotos query --field` over the templates of `testdata/golden/templates.txt` (`TestTemplateGolden`). The `golden-regen` workflow builds the reference on the 9 libraries of `testdata/libraries`: 8 osxphotos test libraries (7 schemas, from macOS 13.0 to 27.0 developer beta) and a synthetic database; known divergences are recorded with reasons. Fields beyond osxphotos are checked with SQL migrations with synthetic values on a temporary copy of the database ([SOURCES.md](https://github.com/Windemiatrix/go-libs/blob/main/testdata/libraries/SOURCES.md)). With `make diff-local` (below) the port was also compared with osxphotos on the maintainer's own library, schema 11.1 (macOS 26.6): no differences.

`photokit` is tested at three levels:

1. The Go layer, on a fake of the bridge to PhotoKit, on Linux and macOS.
2. The cgo build and the calls without dialogs (`AuthorizationStatus`, `Open` without the permission), in the `macos` CI job.
3. The tests with the `photokit_live` tag, which modify a real library, only by hand under a separate account. They do not start without the variable `PHOTOSYNC_LIVE=1`, without a marker album, or in a library above a limit of assets. CI only compiles them and never runs them.

### Checking against your own library

1. Install osxphotos 0.77.2 into its own environment (Python 3.10 or newer is required): `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Quit Photos, open the library package in Finder (Show Package Contents) and copy its `database/` folder to `~/Pictures/Copy.photoslibrary/database/`. The original photos are not needed.
3. From the repository root: `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`. The paths are absolute; `REPORT` is optional.

On a successful run the terminal gets only numbers and key names: how many objects and keys were compared, how many differences. Error messages of Go may name the paths you passed; the messages of osxphotos stay out of the terminal, and when osxphotos fails, run it by hand to see why. The summary holds the object count of your library: in an issue, paste only the per-key lines. The UUIDs and values of the differences go only to the `REPORT` file, with mode 0600 (an existing file is overwritten): it holds data of your library, do not publish it. The osxphotos output and the database copy live in a temporary directory and are removed after the check, even an interrupted one. After the check, delete the copy `~/Pictures/Copy.photoslibrary` too.

## Versioning

Semver by tags `osxphotos/vX.Y.Z`, independent of osxphotos. Before `v1.0.0` incompatible changes are possible (the minor number grows). Changes are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page. `v1.0.0` ships once a consuming application has proven the API. How the version is computed: [CONTRIBUTING.en.md](https://github.com/Windemiatrix/go-libs/blob/main/CONTRIBUTING.en.md#releases-for-the-maintainer).

## FAQ

- **Can I read while Photos is open?** Yes: a copy of the database is read together with its `-wal` file, and Photos is not blocked.
- **Does it read the photos themselves?** `photosdb` does not: only the catalog (metadata), the paths of files in the library (originals, edited versions, adjustment files and previews) and the adjustment files themselves (property lists); it never opens photo files. Only `photokit` reads and writes photo files, when you call an export or an import.
- **Photos only in iCloud ("Optimize Storage")?** The catalog is read in full; for a photo whose original is not on disk, `Path` is empty, and `IsMissing`/`InCloud` show where it is. An export through `photokit` downloads such an original from iCloud.
- **`SystemLibraryPath` returned `ErrLibraryNotFound`?** The `photolibraryd` preferences may have no `SystemLibraryPath` key (as seen on macOS 26), and the module does not look for the path through PhotoKit. Check the error with `errors.Is(err, photosdb.ErrLibraryNotFound)`; the caller then chooses the library path.

## License

MIT. A port of [osxphotos](https://github.com/RhetTbull/osxphotos) (MIT, Rhet Turnbull); the test databases are taken from its tests unchanged (see [NOTICE](NOTICE)).

## Feedback

[Issues](https://github.com/Windemiatrix/go-libs/issues) for bugs and new macOS versions, [Discussions](https://github.com/Windemiatrix/go-libs/discussions) for questions and ideas.
