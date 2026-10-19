# go-libs

**English** | [Русский](README.md)

A monorepo of Go libraries by Windemiatrix. Each library is a separate Go module in its own subdirectory, with its own `go.mod`, version and tags.

## Status

**Design phase. There is no code and there are no versions yet.** This repository describes what will be built and how; no tags or working API exist so far.

## Modules

| Module | Import path | What it is |
|---|---|---|
| [osxphotos](osxphotos/README.en.md) | `github.com/Windemiatrix/go-libs/osxphotos` | Reading the macOS Photos database, path templates and PhotoKit access in Go. A port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos) |

The first library is created for the [photosync](https://github.com/Windemiatrix/photosync) application but is meant to be usable on its own.

## Principles

- One module per subdirectory; versions are independent and follow semver.
- Release tags include the module path: `osxphotos/v0.1.0` (Go requires this for a module that is not at the repository root).
- A published tag is immutable; a faulty version is withdrawn with a `retract` directive in a later version.
- New functionality starts with a test; results are compared with a reference wherever one exists.
- Few dependencies: each has a reason, and licenses are compatible with MIT.
- Documentation in Russian and English.

## License

[MIT](LICENSE), (c) 2026 Windemiatrix. The `osxphotos` module contains code and ideas ported from the osxphotos project (MIT, Rhet Turnbull); see [osxphotos/NOTICE](osxphotos/NOTICE).
