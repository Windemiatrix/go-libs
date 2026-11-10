# go-libs

**English** | [Русский](README.md)

[![CI](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

Go libraries for different tasks. Each one is a separate module with its own version, so you add only the one you need.

## Libraries

| Library | What it does |
|---|---|
| [osxphotos](osxphotos/README.en.md) | The macOS Photos library: reading the catalog, path templates and export file names, changing the library through PhotoKit. An unofficial Go port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos). |

## Installation

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

Requires Go 1.26. Other requirements are in the library's README.

## Versions

Each library has its own tags: `<library>/vX.Y.Z`, for example `osxphotos/v0.1.0`. The API may change before `v1.0.0`. Releases are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page.

## Development

Tests with `-race`, coverage of at least 80%, CI on Linux and macOS. How to contribute: [CONTRIBUTING.en.md](CONTRIBUTING.en.md).

## License

[MIT](LICENSE), © 2026 Windemiatrix. `osxphotos` contains code and ideas from the osxphotos project (MIT, Rhet Turnbull); see [osxphotos/NOTICE](osxphotos/NOTICE).
