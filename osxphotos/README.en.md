# osxphotos

**English** | [Русский](README.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

A Go module for the macOS Photos library. It is an unofficial port of part of [osxphotos](https://github.com/RhetTbull/osxphotos), the Python project by Rhet Turnbull. The author of osxphotos is not involved in this port, but he decoded the Photos database format ([NOTICE](NOTICE)).

Below, "photo" means a photo or a video. The module fits if your Go program needs to:

- read photos, albums and folders, people, places, keywords, EXIF and file paths;
- select photos with the filters of `osxphotos query` and find duplicates;
- build a path from an osxphotos template, such as `{created.year}/{album}`;
- name export files the way `osxphotos export` does;
- export and import photos, manage albums, delete photos and change their properties.

No Python is needed. The module has no command line, does not write XMP or JSON sidecar files and does not call exiftool: use osxphotos itself for those. The API may change before `v1.0.0`.

## Installation

Run `go get github.com/Windemiatrix/go-libs/osxphotos@latest`. Requires Go 1.26 or later. Every package except `photokit` is pure Go without cgo and works even on Linux with a copy of a Photos library. `photokit` works only on macOS in a cgo build. This example prints the date and original name of the photos in the system library:

```go
lib, err := photosdb.SystemLibraryPath()
if err != nil {
	log.Fatal(err) // photosdb.ErrLibraryNotFound
}
db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
if err != nil {
	log.Fatal(err) // photosdb.ErrUnsupportedSchema: an unknown Photos version
}
defer db.Close()

for _, p := range db.Query(query.Options{}) { // like osxphotos query without flags: no trash, no unselected burst frames
	fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
}
```

## Packages

All functions and types are in the [package documentation](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos).

| Package | What it does |
|---|---|
| `photosdb` | Opens a library and returns its data; the only package that reads the database |
| `photoinfo`, `albuminfo`, `personinfo`, `placeinfo`, `exifinfo`, `momentinfo`, `scoreinfo`, `searchinfo`, `adjustmentsinfo` | Describe photos, albums and folders, people and faces, places, EXIF, moments, scores, search labels and edits |
| `query` | Selects and sorts photos and finds duplicates, as `osxphotos query` does |
| `template` | Parses and renders osxphotos templates |
| `export` | Names export files |
| `photokit` | Reads and changes the system library through PhotoKit |

## Data safety and macOS permissions

- Only `photokit` changes a library, and only in its import, album, delete and `Update` methods. Packages other than `photosdb` and `photokit` do not touch the disk.
- `photosdb` reads a read-only copy of the database in a new directory with mode `0700`. `Open` deletes the copy before it returns, even after an error. If the copy is damaged, `Open` copies the database again, up to three attempts. The check of the copy finds only structural damage. It does not guarantee that a copy made while Photos is writing to the database is complete.
- The copy holds personal data: places, names of people, captions. Set `photosdb.Options.TempDir` to a directory outside the library that only you can access. `DB.JSON`, `photokit.ImageInfo` and sharing data (links, participants, comments, likes) hold personal data too: do not log them.
- The module does not go online or run programs by itself; the linter checks this. There is no telemetry. When exporting, `photokit` lets macOS download the original from iCloud.
- A program that reads a library through `photosdb` needs Full Disk Access; when run from Terminal, Terminal needs it. `photokit` needs the Photos permission; see [PhotoKit](#photokit).
- Do not attach `Photos.sqlite` to an issue; just state your macOS version. Report vulnerabilities privately, as described in [SECURITY.en.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.en.md).

## macOS versions

The module detects the database schema by the Core Data model version stored in the database. Supported:

| macOS | Photos | Model version |
|---|---|---|
| 13 | 8 | 16320–16999 |
| 14.0–14.5 | 9 | 17000–17599 |
| 14.6+ | 9.6 | 17600–17999 |
| 15 | 10 | 18201–18999 |
| 26.0 | 11 | 19063–19319 |
| 26.1+ | 11.1 | 19320–19999 |
| 27 | 12 | 270000000–270999999 |

For any other model version `Open` returns `ErrUnsupportedSchema` and does not guess the schema. Support for a new macOS is added only when a test library from that version is available: [open an issue](https://github.com/Windemiatrix/go-libs/issues/new?template=new-macos.yml).

## Path templates

The `template` package renders osxphotos templates: fields, filters, conditions, replacements, default values. The list of fields is in the [osxphotos documentation](https://github.com/RhetTbull/osxphotos/blob/v0.78.1/osxphotos/phototemplate.md).

```go
t, err := template.Parse("{created.year}/{created.mm}-{created.dd} - {album,No album}/{photo.original_filename}")
if err != nil {
	log.Fatal(err) // template.ErrSyntax
}
paths, unknown, err := db.RenderTemplate(p, t, template.RenderOptions{})
// paths: [2024/05-01 - Holidays/IMG_0001.HEIC]; unknown: fields that osxphotos does not have
```

- A photo in two albums gives two paths: a field with several values multiplies the result.
- `RenderOptions.Filename` and `Dirname` clean field values for file and directory names. The path may still hold `..`, and with `Dirname` also `/`: check that it stays inside your directory.
- Fields that run code or programs (`{function}`, `{exiftool}` and others) return `ErrUnsupportedField`. If there are more paths than `RenderOptions.MaxResults` (10,000 by default), rendering returns `ErrTooManyValues`. Month and weekday names are in English.
- To render your own data, implement the `template.Photo` interface with its `Field(name)` method and call `Render` on a `template.Renderer`.

## Export file names

The `export` package gives export files the same names as `osxphotos export`. Your program writes the files itself.

```go
// taken: set of StemKey(name) for files in the target directory; 0 (start number): try the unnumbered name first.
name, _ := export.IncrementName(p.OriginalFilename, 0, taken) // IMG_0001.HEIC or IMG_0001 (1).HEIC
files := export.Names(p, name)                                 // files.Live is IMG_0001.mov
```

- `IncrementName` adds ` (1)`, ` (2)` and so on while the name is taken. The extension is ignored: an existing `foo.png` makes `foo.jpeg` taken too.
- `Names` gives the names of the companion files: the Live Photo video, RAW, previews, edit files. `EditedName` gives the name of the edited version, such as `IMG_0001_edited.jpeg`.
- Names come from the library as they are. Clean a name before you write a file, for example with `template.SanitizeFilename`. It does not remove `..` or NUL, so check for them yourself.

## PhotoKit

The `photokit` package reads and changes the system library through PhotoKit. It works only on macOS in a cgo build; otherwise every call returns `ErrNotSupported`.

Your program needs an `Info.plist` with the `NSPhotoLibraryUsageDescription` key, or macOS terminates it when it asks for access. Embed the file yourself, for example with the linker flag `-sectcreate __TEXT __info_plist Info.plist`. `RequestAuthorization` asks the user for access. `Open` does not ask: without full access it returns `ErrNotAuthorized`, also when access is limited to selected photos. If the program runs from Terminal, Terminal gets the Photos permission. After that, any program started from Terminal can read and change the whole library.

- `Fetch` finds photos by UUID from `photosdb`, and `Resources` lists a photo's resources, its files: original, edits, video. Like osxphotos working through PhotoKit, `photokit` does not find hidden photos; `photosdb` does see them.
- `Export` exports a resource as stored. `ExportImage`, `ExportVideo` and `ExportLivePhoto` export the edited version, the unedited version or the original file; for video and Live Photo the last two are the same. The current version of any slow-motion video is rendered again, even without edits, so the file bytes may differ between calls.
- `Import` imports files, Live Photos and RAW+JPEG pairs included. Import shows no dialog, even when it puts photos straight into the Hidden album.
- `CreateFolder`, `CreateAlbum`, `RenameCollection`, `DeleteCollections`, `AddToAlbum` and `RemoveFromAlbum` manage folders and albums.
- `Delete` moves photos to Recently Deleted. `Update` changes the date, favorite, hidden state and location of a photo, but cannot unhide it.
- `CloudIdentifiers` and `LocalIdentifiers` convert local identifiers of photos to cloud identifiers and back.

The name of an export file is one path element. An empty name, `.`, `..`, or a `/` or NUL in it returns `ErrInvalidName`. The file is created with mode `0600`. An existing file is replaced only with `ExportOptions.Overwrite`; otherwise the export returns `fs.ErrExist`. After an error the created file is deleted.

`Delete` and hiding through `Update` show a macOS dialog; `DeleteCollections` may show one. If the user refuses, the call returns `ErrUserCanceled` and the library is unchanged. `ctx` sets how long any change, including an import, waits for its outcome. If `ctx` ends after the call starts but before PhotoKit reports the outcome, the error wraps `ErrOutcomeUnknown` and `ctx.Err()`. The change may have been applied or may apply later, and the macOS dialog, if any, stays open. Read the library again before the next change. `photokit` has not been tested with an iCloud Shared Photo Library.

## Differences from osxphotos

The module follows osxphotos `v0.78.1`. The full list of differences, with links to the osxphotos code, is in [UPSTREAM.md](UPSTREAM.md).

### What osxphotos does not have

`photokit` changes a library through PhotoKit. osxphotos changes it only through AppleScript.

- `Import` imports files. It can also set the file name, date, location, favorite, hidden state and albums.
- `Delete` deletes photos; `Update` changes their date, favorite, hidden state and location.
- `CreateAlbum`, `CreateFolder`, `RenameCollection`, `DeleteCollections`, `AddToAlbum` and `RemoveFromAlbum` manage albums and folders. `Collections` lists them.
- `CloudIdentifiers` and `LocalIdentifiers` convert local identifiers of photos to cloud identifiers and back.
- `ExportOptions.Progress` reports how much of a file is exported.

`photosdb` returns database data that osxphotos does not return:

- `PhotoInfo.CloudAssetGUID`: the cloud identifier of a photo;
- `PhotoInfo.DateModifiedRaw`: the date the photo's record last changed; `DateModified`, as in osxphotos, is the date of the last edit;
- `PhotoInfo.LibraryScopeShareState`: the state of a photo in an iCloud Shared Photo Library;
- `PhotoInfo.Resources`: all files of a photo, with their sizes and whether each is on disk;
- `AlbumInfo.CloudGUID` and `FolderInfo.CloudGUID`: cloud identifiers of albums and folders;
- `AlbumInfo.InTrash`: whether an album is in the trash;
- `Info.LibraryUUID`: the identifier of the library;
- `Info.SkippedAssets`: how many photos were skipped because their database record is incomplete; osxphotos skips them silently.

The module is also more careful than osxphotos:

- `photosdb` always reads a copy of the database; osxphotos copies it only when it is locked.
- For an unknown database version `Open` returns an error; osxphotos reads it with the latest schema it knows.
- The size of parsed data and of template results is limited.
- Paths to photo files never lead outside the library.
- A `photokit` export does not overwrite a file without `ExportOptions.Overwrite`, and `ctx` stops it at any moment.
- The order of items in lists is stable; in osxphotos it may change between runs.

### What the module does not have

Will not be ported:

- libraries of Photos 7 and earlier (macOS 12 and earlier), iPhoto and macOS betas;
- file export: copying, conversion, the export database, XMP and JSON sidecar files, exiftool; `export` gives only file names;
- the `osxphotos` command line with all its commands;
- template fields and query filters that run code or programs: `{function}`, `{exiftool}`, `{detected_text}`, `query_eval`, `exif`;
- the filter by photos selected in the Photos app;
- text recognition in photos, media analysis results (`media_analysis`, `ai_caption`) and `PhotoInfo.tables()`;
- changing a library through AppleScript.

Not ported yet:

- finding a library through PhotoKit, finding the last opened library and listing all libraries;
- paths to files stored outside the library and to photos of shared moments;
- detecting a file type through macOS; the type comes from the osxphotos table;
- the orientation of an edited photo from its edit file; it now comes from the database;
- the file fingerprint as Photos computes it, and finding a photo by it;
- comparing two libraries;
- reading metadata from photo files;
- repairing a damaged database;
- month and day names in the system language; they are now in English.

## How it is tested

Every PR runs tests with `-race` on Linux and macOS, the linter and an 80% coverage threshold. The results are compared with the output of osxphotos on its eight test libraries (macOS 13–27) and one synthetic library. `photokit` is tested in CI against a simulated PhotoKit. Tests that change a real library run only by hand, under a separate macOS account. You can also compare the module with osxphotos on your own library without changing it:

1. Install osxphotos 0.78.1 in a separate Python 3.10+ environment: `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.78.1`.
2. Quit Photos, open the library in Finder with "Show Package Contents" and copy the `database/` directory to `~/Pictures/Copy.photoslibrary/database/`.
3. From the repository root, run `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`.
4. Read the counts of differences in the terminal. The details go to the file set in `REPORT`; it holds your data, so do not publish it. Then delete the library copy.

## Questions

- **Can `photosdb` read a library while the Photos app is open?** Yes, `photosdb` reads a copy of the database. The module does not guarantee that a copy made while Photos is writing to the database is complete.
- **What does `photosdb` know about photos stored only in iCloud?** Everything in the database. Such a photo has an empty `Path` and `IsMissing` set to true; `photokit` downloads the original when exporting.
- **What if `SystemLibraryPath` returns `ErrLibraryNotFound`?** Give the program Full Disk Access; if that does not help, pass the library path to `Open` yourself.

## License

MIT. The test databases, except the synthetic one, are taken unchanged from the osxphotos tests; see [NOTICE](NOTICE). Questions and ideas go to [Discussions](https://github.com/Windemiatrix/go-libs/discussions), bugs to [Issues](https://github.com/Windemiatrix/go-libs/issues).
