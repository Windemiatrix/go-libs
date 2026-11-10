# go-libs

[English](README.en.md) | **Русский**

[![CI](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

Библиотеки на Go для разных задач. Каждая — отдельный модуль со своей версией: подключается только та, что нужна.

## Библиотеки

| Библиотека | Что делает |
|---|---|
| [osxphotos](osxphotos/README.md) | Медиатека «Фото» на macOS: чтение каталога, шаблоны путей и имена файлов экспорта, изменение медиатеки через PhotoKit. Неофициальный порт части [osxphotos](https://github.com/RhetTbull/osxphotos) на Go. |

## Подключение

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

Нужен Go 1.26. Остальные требования — в README библиотеки.

## Версии

У каждой библиотеки свои теги: `<библиотека>/vX.Y.Z`, например `osxphotos/v0.1.0`. До `v1.0.0` API может меняться. Выпуски — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases).

## Разработка

Тесты с `-race`, покрытие не ниже 80 %, CI на Linux и macOS. Как участвовать — в [CONTRIBUTING.md](CONTRIBUTING.md).

## Лицензия

[MIT](LICENSE), © 2026 Windemiatrix. В `osxphotos` есть код и идеи проекта osxphotos (MIT, Rhet Turnbull), см. [osxphotos/NOTICE](osxphotos/NOTICE).
