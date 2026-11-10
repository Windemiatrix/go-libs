# go-libs

[English](README.en.md) | **Русский**

[![CI](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

go-libs — библиотеки Windemiatrix на Go для разных задач. Каждая библиотека — отдельный модуль со своей версией, поэтому подключается только нужная.

## Библиотеки

| Библиотека | Что делает |
|---|---|
| [osxphotos](osxphotos/README.md) | Читает данные приложения «Фото» на macOS и меняет их через PhotoKit — порт части [osxphotos](https://github.com/RhetTbull/osxphotos) на Go. |

## Подключение

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

Нужен Go 1.26 или новее. Остальные требования — в README библиотеки.

## Версии

Каждая библиотека выпускается отдельно, с тегами `<библиотека>/vX.Y.Z`, например `osxphotos/v0.1.0`. До `v1.0.0` API может меняться. Выпуски — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases).

## Участие

Как предложить изменение — в [CONTRIBUTING.md](CONTRIBUTING.md). Об уязвимостях пишите приватно, по [SECURITY.md](SECURITY.md).

## Лицензия

[MIT](LICENSE), © 2026 Windemiatrix. Библиотека osxphotos содержит код и идеи Python-проекта osxphotos (MIT, Rhet Turnbull), см. [osxphotos/NOTICE](osxphotos/NOTICE).
