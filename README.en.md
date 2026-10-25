# go-libs

**English** | [Русский](README.md)

[![CI](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Read your Mac's Photos library from Go — no Python, and the library is never changed.**

The first library here is `osxphotos`. It gives you the Photos catalog: items, albums, dates, keywords, iCloud identifiers. No Python, and Photos does not need to be running: the module reads a copy of the `Photos.sqlite` database. An unofficial port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos).

The schemas of macOS 13–27 are data: an unknown schema is an error, not wrong data.

Requirements: Go 1.26. Reading a real library needs macOS 13 or later; database parsing also builds and is tested on Linux (no cgo).

## Status

Early development: the API may change before `v1.0.0`. What is done and what comes next: [osxphotos/README.en.md](osxphotos/README.en.md#status).

## Modules

| Module | Import path | What it is |
|---|---|---|
| [osxphotos](osxphotos/README.en.md) | `github.com/Windemiatrix/go-libs/osxphotos` | Reading the macOS Photos catalog. Path templates and PhotoKit are planned |

Each library is a separate Go module with its own version and tags (`osxphotos/vX.Y.Z`). The first is created for [photosync](https://github.com/Windemiatrix/photosync), a tool that syncs Photos with Yandex Disk (in development), but it suits any Go program.

## Why you can trust it

- The original library is never changed: a copy of the database is read ([details](osxphotos/README.en.md#safety-of-your-photos)).
- osxphotos's own results on its test libraries are stored in the repository, and the module's output is compared with them. The differences from osxphotos and everything added beyond it are listed in [UPSTREAM.md](osxphotos/UPSTREAM.md); a test keeps the list in agreement with the code.
- MIT, tests with `-race`, coverage of at least 80%, CI on Linux and macOS.

To contribute, see [CONTRIBUTING.en.md](CONTRIBUTING.en.md).

## License

[MIT](LICENSE), © 2026 Windemiatrix. The `osxphotos` module contains code and ideas ported from the osxphotos project (MIT, Rhet Turnbull); see [osxphotos/NOTICE](osxphotos/NOTICE).
