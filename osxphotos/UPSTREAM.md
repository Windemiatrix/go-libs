# Upstream: osxphotos

**English** | [Русский](#upstream-osxphotos-ru)

This module is a Go port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos). The port follows one fixed upstream release; changes made upstream after that release are not ported until the version below is updated.

The three lines below are read by a test in this module and by CI. Keep exactly this form.

```text
Upstream version: 0.77.2
Upstream tag: v0.77.2
Tag commit: da8dfb212c19c61238dd1a9da8c87fc289324450
```

The version must equal the one in `tools/golden/requirements.txt` (the version used to generate reference data). CI fails when they differ, so moving to a new upstream version changes both files in the same pull request.

Upstream references below are written as `file:line` in that release. Paths are relative to the `osxphotos/` package of the release, as in the table.

## Correspondence

Only what is ported in v0.1.0. Templates, export and PhotoKit appear together with their own packages.

| osxphotos module | Go package |
|---|---|
| `photosdb/photosdb.py` | `photosdb`, `internal/sqlread` |
| `photosdb/photosdb_utils.py` | `photosdb` |
| `_constants.py` | `internal/schema` |
| `sqlite_utils.py` | `internal/sqlread` |
| `photosdb/_photosdb_process_shared_library.py` | `photosdb` |
| `photosdb/_photosdb_process_syndicationinfo.py` | `photosdb` |
| `photoinfo.py` | `photoinfo` |
| `albuminfo.py` | `albuminfo` |
| `photoquery.py` | `query` |
| `photos_datetime.py` | `photosdb` |
| `unicode.py` | `photosdb` |
| `uti.py` | `internal/uti` |
| `utils.py` (only `get_system_library_path`) | `photosdb` |

## Intentional differences

- A model from 13000 to 16319 (Photos 5-7 and the pre-release builds 16000-16319) or from 18000 to 18200 gives `ErrUnsupportedSchema`. Upstream reads all of these with their own schemas (`_constants.py:56-62`).
- Any other model that is not in the schema table also gives `ErrUnsupportedSchema`: below 13000, 19000-19062, 20000-269999999, above 270999999, or a missing `PLModelVersion`. Upstream assumes the latest schema for a model outside all of its ranges and for a missing one (`photosdb/photosdb_utils.py:131-136`, `photosdb/photosdb_utils.py:160-163`).
- A string `PLModelVersion` is accepted only as ASCII decimal digits. Upstream `int()` also accepts a sign, surrounding spaces and non-ASCII digits (`photosdb/photosdb_utils.py:137`).
- `Z_PLIST` over 1 MiB, or a binary property list with more than 4096 objects, gives `ErrUnsupportedSchema`. `SystemLibraryPath` skips a preferences file over 1 MiB or one that is not a property list. Upstream `plistlib` has no such limits (`photosdb/photosdb_utils.py:114`, `utils.py:193`).
- The database is always read from a copy that includes the `-wal` file. Upstream copies the database only when it is locked (`photosdb/photosdb.py:438-443`, `sqlite_utils.py:125-137`).
- `Photos()` has no implicit filters: it returns trashed photos and unselected burst frames as well. The selection of osxphotos `photos()` is made by the caller: `!p.InTrash && !p.BurstUnselected() && (p.IsPhoto || p.IsMovie)` (`photosdb/photosdb.py:3269-3276`, `photosdb/photosdb.py:3346-3353`, `photosdb/photosdb.py:3355-3358`).
- `NULL` in the pick type of a burst frame is 0. Upstream fails on `None & int` (`photosdb/photosdb.py:3347-3351`).
- The second original of a RAW+JPEG pair is a resource of subtype 17, or of subtype 3 with resource type 4. Upstream looks for subtype 17 only (`photosdb/photosdb.py:2537-2551`). None of the test libraries has a resource of subtype 3 with type 4; the rule is checked only on the synthetic library.
- `IsMissing` looks for the resource with the fingerprint of the asset only among the resources of the asset, and of several the one with the highest `Z_PK` decides. Upstream takes such a resource of any asset, and the last row read decides (`photosdb/photosdb.py:2498-2516`).
- `Date` is in the fixed zone of `TZOffset`; `ZTIMEZONENAME` is not used (`photos_datetime.py:53-62`).
- Paths are looked up only inside `originals/` of the library, without following symbolic links. An absolute `ZDIRECTORY` (a referenced file) or an empty one gives the path ""; upstream uses an absolute one as is and looks for an empty one in `originals/<file name>`. Referenced files have no `PathLive` and `PathRaw`. Photos of shared albums and "Shared with you" photos not saved to the library have `Path` and `PathLive` ""; their `PathRaw` is looked up in `originals/`, as upstream does. The shared moment directory is not searched: a photo of a shared moment has a `Path` only when its file is in `originals/` (`photoinfo.py:209-234`, `photoinfo.py:545-571`, `photoinfo.py:1063-1071`). With several candidate RAW files the first in name order is used; upstream takes the first of `os.listdir`, whose order is unspecified (`utils.py:364-389`, `photoinfo.py:554-555`).
- `Orientation` of an edited asset is the database column. Upstream reads it from the property list of the edit (`photoinfo.py:1396-1404`).
- The UTI of an extension comes only from the table of `uti.py`, without `mdls` (`uti.py:642-653`).
- A repeated or missing `ZUUID` of an asset with a `ZADDITIONALASSETATTRIBUTES` row is an error of `Open`; assets without that row are left out unchecked, as upstream drops them (`photosdb/photosdb.py:2086-2090`). Upstream keeps the last row (`photosdb/photosdb.py:2356`).
- A `TZOffset` outside (-86400, 86400) seconds always gives `Date` 1970-01-01T00:00:00Z. Upstream gives the same date (`ValueError`), but for an offset outside the range of `timedelta` it fails with an uncaught `OverflowError` (`photos_datetime.py:64-67`).
- Shared albums (`ZKIND` 1505) are not in `AlbumUUIDs` and `BurstAlbumUUIDs`. Upstream includes them in `album_info` and `albums` (`photoinfo.py:2023-2034`, `_constants.py:364-365`).
- An empty GUID string means no GUID, as `NULL` does.
- `AlbumUUIDs` includes albums in the trash; the caller filters by `AlbumInfo.InTrash`.
- The order of photos in an album is not stored: `PhotoUUIDs` is a set.
- `SystemLibraryPath` has no PhotoKit fallback (`utils.py:206-228`). When neither photolibraryd preferences file holds an absolute path without NUL, it returns `ErrLibraryNotFound`. Upstream returns any value of the key as is (`utils.py:180-206`).

## Extensions

Exported names that have no counterpart in osxphotos. Each is marked `// extension: not in osxphotos` in the code, and a test keeps this list and the code in agreement.

- `albuminfo.AlbumInfo.CloudGUID`
- `albuminfo.FolderInfo.CloudGUID`
- `photoinfo.PhotoInfo.BurstAlbumUUIDs`
- `photoinfo.PhotoInfo.BurstUnselected()`
- `photoinfo.PhotoInfo.CloudAssetGUID`
- `photoinfo.PhotoInfo.DateModifiedRaw`
- `photoinfo.PhotoInfo.FingerprintSchema`
- `photoinfo.PhotoInfo.HasPersons`
- `photoinfo.PhotoInfo.LibraryScope`
- `photoinfo.PhotoInfo.LibraryScopeShareState`
- `photoinfo.PhotoInfo.Resources`
- `photoinfo.Resource`
- `photosdb.Info.LibraryUUID`
- `photosdb.Info.SkippedAssets`

<a id="upstream-osxphotos-ru"></a>

## Upstream: osxphotos (по-русски)

Модуль — порт частей [osxphotos](https://github.com/RhetTbull/osxphotos) на Go. Порт следует одному зафиксированному выпуску upstream; правки upstream после этого выпуска не переносятся, пока версия выше не обновлена.

Три строки в блоке выше читает тест этого модуля и CI. Формат менять нельзя.

Версия должна совпадать с версией в `tools/golden/requirements.txt` (по ней строятся эталонные данные). При расхождении CI падает, поэтому переход на новую версию upstream меняет оба файла в одном PR.

Ссылки на upstream ниже записаны как `файл:строка` в этом выпуске. Пути отсчитываются от пакета `osxphotos/` выпуска, как в таблице.

### Соответствие

Только то, что перенесено в v0.1.0. Шаблоны, экспорт и PhotoKit появятся вместе со своими пакетами.

| Модуль osxphotos | Пакет Go |
|---|---|
| `photosdb/photosdb.py` | `photosdb`, `internal/sqlread` |
| `photosdb/photosdb_utils.py` | `photosdb` |
| `_constants.py` | `internal/schema` |
| `sqlite_utils.py` | `internal/sqlread` |
| `photosdb/_photosdb_process_shared_library.py` | `photosdb` |
| `photosdb/_photosdb_process_syndicationinfo.py` | `photosdb` |
| `photoinfo.py` | `photoinfo` |
| `albuminfo.py` | `albuminfo` |
| `photoquery.py` | `query` |
| `photos_datetime.py` | `photosdb` |
| `unicode.py` | `photosdb` |
| `uti.py` | `internal/uti` |
| `utils.py` (только `get_system_library_path`) | `photosdb` |

### Намеренные отличия

- Номер модели от 13000 до 16319 (Photos 5-7 и предварительные сборки 16000-16319) или от 18000 до 18200 даёт `ErrUnsupportedSchema`. Upstream читает все эти модели по их собственным схемам (`_constants.py:56-62`).
- Любой другой номер модели, которого нет в таблице схем, тоже даёт `ErrUnsupportedSchema`: меньше 13000, 19000-19062, 20000-269999999, больше 270999999 или отсутствующий `PLModelVersion`. Upstream для номера вне всех своих диапазонов и для отсутствующего берёт последнюю схему (`photosdb/photosdb_utils.py:131-136`, `photosdb/photosdb_utils.py:160-163`).
- Строковый `PLModelVersion` принимается, только если это десятичные цифры ASCII. `int()` в upstream принимает ещё знак, пробелы по краям и цифры других алфавитов (не ASCII) (`photosdb/photosdb_utils.py:137`).
- `Z_PLIST` больше 1 МиБ или двоичный список свойств с более чем 4096 объектами даёт `ErrUnsupportedSchema`. `SystemLibraryPath` пропускает файл настроек больше 1 МиБ или не являющийся списком свойств. В `plistlib` upstream таких ограничений нет (`photosdb/photosdb_utils.py:114`, `utils.py:193`).
- База всегда читается из копии вместе с файлом `-wal`. Upstream копирует базу, только когда она заблокирована (`photosdb/photosdb.py:438-443`, `sqlite_utils.py:125-137`).
- `Photos()` без неявных фильтров: возвращает и фото из корзины, и невыбранные кадры серий. Выборку `photos()` из osxphotos делает вызывающий: `!p.InTrash && !p.BurstUnselected() && (p.IsPhoto || p.IsMovie)` (`photosdb/photosdb.py:3269-3276`, `photosdb/photosdb.py:3346-3353`, `photosdb/photosdb.py:3355-3358`).
- `NULL` в отметке кадра серии — 0. Upstream падает на `None & int` (`photosdb/photosdb.py:3347-3351`).
- Второй оригинал пары RAW+JPEG — ресурс подтипа 17 или ресурс подтипа 3 с типом ресурса 4. Upstream ищет только подтип 17 (`photosdb/photosdb.py:2537-2551`). Ни в одной тестовой медиатеке нет ресурса подтипа 3 с типом 4; правило проверено только на синтетической медиатеке.
- `IsMissing` ищет ресурс с отпечатком объекта только среди ресурсов этого объекта, а из нескольких решает ресурс с наибольшим `Z_PK`. Upstream берёт такой ресурс любого объекта, и решает последняя прочитанная строка (`photosdb/photosdb.py:2498-2516`).
- `Date` — в фиксированной зоне смещения `TZOffset`; `ZTIMEZONENAME` не используется (`photos_datetime.py:53-62`).
- Пути ищутся только внутри `originals/` медиатеки, без перехода по символическим ссылкам. Абсолютный `ZDIRECTORY` (файл-ссылка) или пустой даёт путь ""; upstream берёт абсолютный как есть, а пустой ищет в `originals/<имя файла>`. У файлов-ссылок нет `PathLive` и `PathRaw`. У фото общих альбомов и фото «Отправлено вам», не сохранённых в медиатеку, `Path` и `PathLive` — ""; их `PathRaw` ищется в `originals/`, как в upstream. Каталог общих моментов не просматривается: у фото общего момента `Path` есть, только если его файл лежит в `originals/` (`photoinfo.py:209-234`, `photoinfo.py:545-571`, `photoinfo.py:1063-1071`). Из нескольких файлов-кандидатов RAW берётся первый по порядку имён; upstream берёт первый из `os.listdir`, порядок которого не задан (`utils.py:364-389`, `photoinfo.py:554-555`).
- `Orientation` правленого объекта — колонка базы. Upstream читает её из списка свойств правки (`photoinfo.py:1396-1404`).
- UTI по расширению берётся только из таблицы `uti.py`, без `mdls` (`uti.py:642-653`).
- Повтор или отсутствие `ZUUID` у объекта со строкой `ZADDITIONALASSETATTRIBUTES` — ошибка `Open`; объекты без этой строки отбрасываются без проверки, как в upstream (`photosdb/photosdb.py:2086-2090`). Upstream оставляет последнюю строку (`photosdb/photosdb.py:2356`).
- `TZOffset` вне диапазона (-86400, 86400) секунд всегда даёт `Date` 1970-01-01T00:00:00Z. Upstream даёт ту же дату (`ValueError`), но на смещении вне диапазона `timedelta` падает с непойманным `OverflowError` (`photos_datetime.py:64-67`).
- Общие альбомы (`ZKIND` 1505) не входят в `AlbumUUIDs` и `BurstAlbumUUIDs`. Upstream включает их в `album_info` и `albums` (`photoinfo.py:2023-2034`, `_constants.py:364-365`).
- Пустая строка GUID означает отсутствие GUID, как и `NULL`.
- `AlbumUUIDs` включает альбомы в корзине; вызывающий фильтрует по `AlbumInfo.InTrash`.
- Порядок фото в альбоме не хранится: `PhotoUUIDs` — множество.
- У `SystemLibraryPath` нет запасного пути через PhotoKit (`utils.py:206-228`). Если ни один из двух файлов настроек photolibraryd не содержит абсолютный путь без NUL, возвращается `ErrLibraryNotFound`. Upstream возвращает любое значение ключа как есть (`utils.py:180-206`).

### Расширения

Экспортируемые имена, у которых нет пары в osxphotos. Каждое помечено в коде `// extension: not in osxphotos`, а тест проверяет, что список совпадает с кодом.

- `albuminfo.AlbumInfo.CloudGUID`
- `albuminfo.FolderInfo.CloudGUID`
- `photoinfo.PhotoInfo.BurstAlbumUUIDs`
- `photoinfo.PhotoInfo.BurstUnselected()`
- `photoinfo.PhotoInfo.CloudAssetGUID`
- `photoinfo.PhotoInfo.DateModifiedRaw`
- `photoinfo.PhotoInfo.FingerprintSchema`
- `photoinfo.PhotoInfo.HasPersons`
- `photoinfo.PhotoInfo.LibraryScope`
- `photoinfo.PhotoInfo.LibraryScopeShareState`
- `photoinfo.PhotoInfo.Resources`
- `photoinfo.Resource`
- `photosdb.Info.LibraryUUID`
- `photosdb.Info.SkippedAssets`
