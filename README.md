# go-libs

[English](README.en.md) | **Русский**

[![CI](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/go-libs/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Читайте медиатеку «Фото» на Mac из Go — без Python, медиатека не меняется.**

Первая библиотека здесь — `osxphotos`. Она отдаёт каталог «Фото»: фото и видео, альбомы и папки, даты, ключевые слова, людей на снимках, пути к оригиналам, правленым версиям, файлам правок и превью, iCloud-идентификаторы; умеет выбирать фото по фильтрам и находить дубликаты. Python не нужен, «Фото» запускать не нужно: модуль читает копию базы `Photos.sqlite`. Это неофициальный порт части [osxphotos](https://github.com/RhetTbull/osxphotos).

Схемы macOS 13–27 заданы как данные: неизвестная схема даёт ошибку, а не неверные данные.

Требования: Go 1.26. Чтение настоящей медиатеки — на macOS 13 и новее; разбор базы собирается и тестируется и на Linux (без cgo).

## Статус

Ранняя разработка: до `v1.0.0` API может меняться. Что готово и что дальше — в [osxphotos/README.md](osxphotos/README.md#статус).

## Модули

| Модуль | Путь импорта | Что это |
|---|---|---|
| [osxphotos](osxphotos/README.md) | `github.com/Windemiatrix/go-libs/osxphotos` | Чтение каталога «Фото» macOS. Шаблоны путей и PhotoKit — в планах |

Каждая библиотека — отдельный Go-модуль со своей версией и тегами (`osxphotos/vX.Y.Z`). Первая создаётся для [photosync](https://github.com/Windemiatrix/photosync) — утилиты синхронизации «Фото» с Яндекс Диском (в разработке), но годится для любой Go-программы.

## Почему можно доверять

- Оригинальная медиатека не меняется: читается копия базы ([подробнее](osxphotos/README.md#безопасность-ваших-фото)).
- Результаты самого osxphotos на его тестовых медиатеках лежат в репозитории, с ними сверяется вывод модуля. Отличия от osxphotos и всё, что добавлено сверх него, перечислены в [UPSTREAM.md](osxphotos/UPSTREAM.md); тест держит список в согласии с кодом.
- MIT, тесты с `-race`, покрытие не ниже 80 %, CI на Linux и macOS.

Участие в разработке — [CONTRIBUTING.md](CONTRIBUTING.md).

## Лицензия

[MIT](LICENSE), © 2026 Windemiatrix. Модуль `osxphotos` содержит портированный код и идеи проекта osxphotos (MIT, Rhet Turnbull); см. [osxphotos/NOTICE](osxphotos/NOTICE).
