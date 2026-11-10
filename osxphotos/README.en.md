# osxphotos

**English** | [Русский](README.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

A Go library for the macOS Photos library. An unofficial port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos), the Python project by Rhet Turnbull (MIT). The author of osxphotos is not involved in this library; but he is the one who decoded the database format ([NOTICE](NOTICE)).

What it does:

- reads the library catalog: photos and videos, albums and folders, people and faces, places, keywords, EXIF, scores, search labels, file paths;
- selects photos by filters and finds duplicates;
- builds paths from osxphotos templates, such as `{created.year}/{album}`;
- names export files the same way as `osxphotos export`;
- through PhotoKit, exports and imports photos, manages albums, deletes photos and changes their date, favorite, hidden state and location (macOS only).

No Python, and Photos does not need to be running.

**Status:** early development; the API may change before `v1.0.0`. Releases are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page.

## Quick start

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

```go
lib, err := photosdb.SystemLibraryPath()
if err != nil {
	log.Fatal(err)
}
db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
if err != nil {
	log.Fatal(err) // photosdb.ErrUnsupportedSchema: an unknown Photos version
}
defer db.Close()

for _, p := range db.Query(query.Options{}) { // like osxphotos query without flags: outside the trash, no unselected burst frames
	fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
}
```

Requires Go 1.26. Everything except `photokit` is pure Go without cgo and also works on Linux, given a copy of the library. `photokit` works only on macOS in a cgo build.

## Packages

| Package | What it does |
|---|---|
| `photosdb` | Opens a library and gives you the catalog; the only package that reads the database |
| `photoinfo`, `albuminfo`, `personinfo`, `placeinfo`, `exifinfo`, `momentinfo`, `scoreinfo`, `searchinfo`, `adjustmentsinfo` | Data of photos, albums and folders, people, places, EXIF, moments, scores, search labels and edits |
| `query` | Filters, sorting and duplicate search, as in `osxphotos query` |
| `template` | The osxphotos template language |
| `export` | Export file names |
| `photokit` | The system library through PhotoKit |

All functions and types are in the [package documentation](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos).

## Safety of your photos

- **Only `photokit` changes the library**, and only when you call a method that changes it: import, albums, deletion, changing an asset. `photosdb` works on a copy of the database: it is created in a temporary directory with mode `0700`, opened read-only and deleted before `Open` returns. The other packages do not touch the disk.
- **The user confirms deleting or hiding photos already in the library** in a macOS dialog. Importing straight into Hidden shows no dialog.
- **The library does not go online by itself** and does not run programs; the linter checks this. No telemetry. When exporting, `photokit` lets macOS download the original from iCloud.
- **macOS permissions.** A program that reads the library through `photosdb` (for example, Terminal) needs Full Disk Access. `photokit` needs the Photos permission; see [PhotoKit](#photokit).
- **The database copy is your personal data:** places, names of people, captions. Point `photosdb.Options.TempDir` only at a directory that only you can access. `DB.JSON` and `photokit.ImageInfo` hold paths and personal data; do not write them to logs.
- Do not attach `Photos.sqlite` to an issue: the macOS version is enough. Report vulnerabilities privately, see [SECURITY.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.md).

## macOS versions

The database schema is detected by its model number. Supported:

| macOS | Photos | Database model |
|---|---|---|
| 13 | 8 | 16320–16999 |
| 14.0–14.5 | 9 | 17000–17599 |
| 14.6+ | 9.6 | 17600–17999 |
| 15 | 10 | 18201–18999 |
| 26.0 | 11 | 19063–19319 |
| 26.1+ | 11.1 | 19320–19999 |
| 27 | 12 | 270000000–270999999 |

For any other model `Open` returns `ErrUnsupportedSchema` instead of guessing. A new macOS is added only when a library of that version is available: [open an issue](https://github.com/Windemiatrix/go-libs/issues/new?template=new-macos.yml).

## Templates

The `template` package parses and renders osxphotos templates: fields, filters, conditions, replacements, default values.

```go
t, err := template.Parse("{created.year}/{created.mm}-{created.dd} - {album,No album}/{photo.original_filename}")
if err != nil {
	log.Fatal(err) // template.ErrSyntax
}
paths, unknown, err := db.RenderTemplate(p, t, template.RenderOptions{})
// paths: [2024/05-01 - Holidays/IMG_0001.HEIC]; unknown: fields osxphotos does not have
```

- A photo in two albums gives two paths: a field with several values multiplies the result.
- With `RenderOptions.Filename` and `Dirname`, field values are cleaned for file and directory names. The result may still hold `..` from the template text, and with only `Dirname` also `/`, so keeping the path inside your directory is up to you.
- Fields that run code or programs (`{function}`, `{exiftool}` and others) are not supported: they return `ErrUnsupportedField`.
- If a template yields more than `RenderOptions.MaxResults` paths (10,000 by default), it returns `ErrTooManyValues`.
- Month and weekday names are in English.
- To render your own data, implement the `template.Photo` interface, which has one method, `Field(name)`.

The list of fields is in the [osxphotos documentation](https://github.com/RhetTbull/osxphotos/blob/v0.77.2/osxphotos/phototemplate.md).

## Export names

The `export` package gives files the same names as `osxphotos export`. It does not write the files.

```go
// 0: try the name without a number first
name, _ := export.IncrementName(p.OriginalFilename, 0, taken) // IMG_0001.HEIC or IMG_0001 (1).HEIC
files := export.Names(p, name)                                 // files.Live is IMG_0001.mov
```

- `IncrementName` adds ` (1)`, ` (2)`… while the name is taken. `taken` is a set of `StemKey(name)` for every file in the directory: the extension is ignored, so `foo.png` also blocks `foo.jpeg`.
- `Names` gives the names of the companion files: the Live Photo video, RAW, previews, edit files.
- `EditedName` gives the name of the edited version, such as `IMG_0001_edited.jpeg`.

## PhotoKit

The `photokit` package works with the system library through PhotoKit. macOS and a cgo build only; otherwise every call returns `ErrNotSupported`.

**Access.**

- The program must contain an `Info.plist` with the `NSPhotoLibraryUsageDescription` key, or macOS terminates it on first access. Embedding it is up to the program, for example with the linker flag `-sectcreate __TEXT __info_plist Info.plist`.
- `RequestAuthorization` asks the user; `Open` does not: without the permission it returns `ErrNotAuthorized`. Limited access (selected photos only) is not accepted either.
- If you run the program from Terminal, Terminal gets the Photos permission. After that any program started from it can read and change the whole library.

**What it does.**

- Finds assets by UUID from `photosdb` (`Fetch`) and lists their resources (`Resources`).
- Exports a resource as stored (`Export`), or a version of a photo or video: with edits, without edits, original (`ExportImage`, `ExportVideo`, `ExportLivePhoto`). An edited slow-motion video is rendered again, so its bytes may differ between calls.
- Imports files, Live Photos and RAW+JPEG included (`Import`).
- Creates, renames and deletes folders and albums, adds photos to them.
- Deletes assets (`Delete`) and changes the date, favorite, hidden state and location (`Update`).
- Converts local identifiers to cloud identifiers and back.

The `photokit` methods do not find hidden photos, and neither does the PhotoKit code of osxphotos; `photosdb` does see them.

**Files.** An export writes one file with mode `0600` into the given directory. An existing file is not overwritten without `ExportOptions.Overwrite`. After an error the partial file is deleted.

**Dialogs and deadlines.**

- `Delete`, `DeleteCollections` and hiding through `Update` show a macOS dialog. If the user refuses, the error is `ErrUserCanceled` and nothing changed.
- `ctx` sets how long to wait. If it ends before PhotoKit reports the result of a change, the error is `ErrOutcomeUnknown`: the change may have been applied, or may still be, because the macOS dialog stays open. Read the library again before changing anything else.
- `photokit` has not been tested with an iCloud Shared Photo Library.

## Differences from osxphotos

The port follows osxphotos `v0.77.2`. Beyond it, the library gives cloud identifiers of photos and albums, sizes of every file of a photo, Live and RAW included, without downloading, and in `photokit` import, albums, deletion and changing assets. Not included: the command line, writing export files, sidecars and exiftool, media analysis results. The full list of differences, with links to the osxphotos code, is in [UPSTREAM.md](UPSTREAM.md).

## How it is tested

- Every PR: tests with `-race` on Linux and macOS, the linter, coverage of at least 80%.
- The output is compared with the output of osxphotos itself on its eight test libraries (macOS 13–27) and one synthetic library. Templates are checked the same way.
- `photokit` is tested against a fake PhotoKit in CI. Tests that change a real library run only by hand, under a separate macOS account.

### Checking on your own library

You can compare the library with osxphotos on your own data. Your library is not changed.

1. Install osxphotos 0.77.2 in a separate environment (Python 3.10+): `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Quit Photos, open the library in Finder with "Show Package Contents" and copy the `database/` folder to `~/Pictures/Copy.photoslibrary/database/`.
3. From the repository root: `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`.

The terminal shows only the counts of differences. The details go to the `REPORT` file: it holds your data, do not publish it. Delete the library copy afterwards.

## Questions

- **Can I read while Photos is open?** Yes.
- **Photos stored only in iCloud?** The catalog is read in full. Such a photo has an empty `Path` and `IsMissing` set to true. `photokit` downloads the original when exporting.
- **`SystemLibraryPath` returned `ErrLibraryNotFound`?** On some systems macOS does not keep the library path where the library looks for it. Pass the path to `Open` yourself.

## License

MIT. A port of [osxphotos](https://github.com/RhetTbull/osxphotos) (MIT, Rhet Turnbull); the test databases are taken from its tests unchanged, see [NOTICE](NOTICE).

Questions and ideas go to [Discussions](https://github.com/Windemiatrix/go-libs/discussions), bugs to [Issues](https://github.com/Windemiatrix/go-libs/issues).
