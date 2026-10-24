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
- A property list over 1 MiB, or a binary property list with more than 4096 objects, is refused: `Z_PLIST` gives `ErrUnsupportedSchema`, and `SystemLibraryPath` skips a preferences file. Upstream `plistlib` has no such limits (`photosdb/photosdb_utils.py:114`, `utils.py:193`).
- The database is always read from a copy that includes the `-wal` file. Upstream copies the database only when it is locked (`photosdb/photosdb.py:438-443`, `sqlite_utils.py:125-137`).
- `Photos()` has no implicit filters. The selection of `osxphotos query` is `query.OSXPhotosDefaults()`, which leaves out trashed photos and unselected burst frames (`photosdb/photosdb.py:3237`, `photosdb/photosdb.py:3346-3353`).
- `NULL` in the pick type of a burst frame is 0. Upstream fails on `None & int` (`photosdb/photosdb.py:3347-3351`).
- The second original of a RAW+JPEG pair is a resource of subtype 3 or 17. Upstream looks for 17 only (`photosdb/photosdb.py:2537-2551`).
- `IsMissing` finds the original resource by the asset, not only by the fingerprint (`photosdb/photosdb.py:2498-2516`).
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
- `query.Options.BurstSelectedOnly`

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
- Список свойств больше 1 МиБ или двоичный список свойств с более чем 4096 объектами отклоняется: `Z_PLIST` даёт `ErrUnsupportedSchema`, а `SystemLibraryPath` пропускает такой файл настроек. В `plistlib` upstream таких ограничений нет (`photosdb/photosdb_utils.py:114`, `utils.py:193`).
- База всегда читается из копии вместе с файлом `-wal`. Upstream копирует базу, только когда она заблокирована (`photosdb/photosdb.py:438-443`, `sqlite_utils.py:125-137`).
- `Photos()` без неявных фильтров. Выборка `osxphotos query` — это `query.OSXPhotosDefaults()`: она исключает фото из корзины и невыбранные кадры серий (`photosdb/photosdb.py:3237`, `photosdb/photosdb.py:3346-3353`).
- `NULL` в отметке кадра серии — 0. Upstream падает на `None & int` (`photosdb/photosdb.py:3347-3351`).
- Второй оригинал пары RAW+JPEG — ресурс подтипа 3 или 17. Upstream ищет только 17 (`photosdb/photosdb.py:2537-2551`).
- `IsMissing` ищет ресурс оригинала по записи объекта (`ZASSET`), а не только по отпечатку (`photosdb/photosdb.py:2498-2516`).
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
- `query.Options.BurstSelectedOnly`
