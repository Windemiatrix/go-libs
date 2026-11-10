# go-libs

**English** | [Русский](README.md)

[![CI](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

go-libs is a set of Go libraries by Windemiatrix for different tasks. Each library is a separate module with its own version, so you add only the one you need.

## Libraries

| Library | What it does |
|---|---|
| [osxphotos](osxphotos/README.en.md) | Reads the data of the macOS Photos app and changes it through PhotoKit — a Go port of part of [osxphotos](https://github.com/RhetTbull/osxphotos). |

## Installation

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

Requires Go 1.26 or later. Other requirements are in the README of each library.

## Versions

Each library is released on its own, with tags `<library>/vX.Y.Z`, such as `osxphotos/v0.1.0`. The API may change before `v1.0.0`. Releases are on the [Releases](https://github.com/Windemiatrix/go-libs/releases) page.

## Contributing

See [CONTRIBUTING.en.md](CONTRIBUTING.en.md) for how to propose a change. Report vulnerabilities privately, as described in [SECURITY.en.md](SECURITY.en.md).

## License

[MIT](LICENSE), © 2026 Windemiatrix. The osxphotos library contains code and ideas from the Python project osxphotos (MIT, Rhet Turnbull); see [osxphotos/NOTICE](osxphotos/NOTICE).
