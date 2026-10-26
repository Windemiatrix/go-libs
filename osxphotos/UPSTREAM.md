# Upstream: osxphotos

**English** | [Русский](#upstream-osxphotos-ru)

For contributors who compare behavior with Python osxphotos or update the port. Users need only the [README](README.md).

This module is a Go port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos). The port follows one fixed upstream release; changes made upstream after that release are not ported until the version below is updated.

```text
Upstream version: 0.77.2
Upstream tag: v0.77.2
Tag commit: da8dfb212c19c61238dd1a9da8c87fc289324450
```

<!-- A test and CI read these three lines; keep the exact form. -->

The version must equal the one in `tools/golden/requirements.txt` (the version used to generate reference data). CI fails when they differ, so moving to a new upstream version changes both files in the same pull request.

Upstream references below are written as `file:line` in that release. Paths are relative to the `osxphotos/` package of the release, as in the table.

## Correspondence

Only what is ported so far. Templates, export and PhotoKit appear together with their own packages.

| osxphotos module | Go package |
|---|---|
| `photosdb/photosdb.py` | `photosdb`, `internal/sqlread` |
| `photosdb/photosdb_utils.py` | `photosdb` |
| `_constants.py` | `internal/schema`, `photoinfo` |
| `sqlite_utils.py` | `internal/sqlread` |
| `photosdb/_photosdb_process_shared_library.py` | `photosdb` |
| `photosdb/_photosdb_process_syndicationinfo.py` | `photosdb` |
| `photoinfo.py` | `photoinfo`, `photosdb` |
| `albuminfo.py` | `albuminfo`, `photosdb` |
| `photoquery.py` | `query` |
| `photos_datetime.py` | `photosdb` |
| `unicode.py` | `photosdb` |
| `uti.py` | `internal/uti` |
| `utils.py` (only `get_system_library_path`) | `photosdb` |

## Intentional differences

If you only read three things: (1) models outside the supported table return `ErrUnsupportedSchema` instead of guessing; (2) the database is always read from a copy; (3) `Photos()` applies no implicit filters; `Query(query.Options{})` returns what `osxphotos query` returns.

- A model from 13000 to 16319 (Photos 5-7 and the pre-release builds 16000-16319) or from 18000 to 18200 gives `ErrUnsupportedSchema`. Upstream reads all of these with their own schemas (`_constants.py:56-62`).
- Any other model that is not in the schema table also gives `ErrUnsupportedSchema`: below 13000, 19000-19062, 20000-269999999, above 270999999, or a missing `PLModelVersion`. Upstream assumes the latest schema for a model outside all of its ranges and for a missing one (`photosdb/photosdb_utils.py:131-136`, `photosdb/photosdb_utils.py:160-163`).
- A string `PLModelVersion` is accepted only as ASCII decimal digits. Upstream `int()` also accepts a sign, surrounding spaces and non-ASCII digits (`photosdb/photosdb_utils.py:137`).
- `Z_PLIST` over 1 MiB, or a binary property list with more than 4096 objects, gives `ErrUnsupportedSchema`. `SystemLibraryPath` skips a preferences file over 1 MiB or one that is not a property list. Upstream `plistlib` has no such limits (`photosdb/photosdb_utils.py:114`, `utils.py:193`).
- The database is always read from a copy that includes the `-wal` file. Upstream copies the database only when it is locked (`photosdb/photosdb.py:438-443`, `sqlite_utils.py:125-137`).
- `Info.LibraryPath` is the directory as given to `Open`, or, for a database file, the parent of its directory made absolute; upstream makes the derived path absolute and resolves symbolic links, and takes `library_path` as given (`photosdb/photosdb.py:349-350`, `photosdb/photosdb.py:452-458`). `Options.LibraryPath` must exist; upstream does not check it (`photosdb/photosdb.py:456-458`). `Info.DBPath` is absolute but its symbolic links are not resolved (`photosdb/photosdb.py:349-350`, `photosdb/photosdb.py:698-701`). A `photos.db` without `Photos.sqlite` beside it, or a library with only `database/photos.db`, gives a not-exist error; upstream reads it as a database of Photos 4 or earlier (`photosdb/photosdb.py:359-362`, `photosdb/photosdb.py:370-376`).
- `Photos()` has no implicit filters: it returns trashed photos and unselected burst frames as well. `Query` makes the selection of osxphotos `photos()`, a photo or a movie outside the trash that is not an unselected burst frame (one whose pick type is not 0 and has neither the selected nor the key bit), as `PhotosDB.query` does (`photosdb/photosdb.py:3269-3276`, `photosdb/photosdb.py:3346-3353`, `photosdb/photosdb.py:3355-3358`, `photoquery.py:426-445`).
- `query.Options` ports `QueryOptions` field by field (`photoquery.py:147-241`). Not ported: `exif` (exiftool), `label` (the search index), `place` and `no_place` (reverse geolocation), `query_eval` (Python expressions), `regex` (templates), `selected` (the selection in the Photos app, through AppleScript), `has_comment`, `no_comment`, `has_likes` and `no_likes` (comments of shared albums).
- A `DateAdded` that the database does not have is 1970-01-01T00:00:00Z for `AddedAfter`, `AddedBefore` and `AddedInLast`; upstream takes midnight of 1970-01-01 in the time zone of the computer (`photos_datetime.py:19-21`, `photoquery.py:822-837`).
- The first `Function` gets the photos in catalog (UUID) order; in upstream the order depends on which filters ran, some of them pass through a set (`photoquery.py:859-861`); with `Duplicate`, the order of the duplicate step, as upstream (`photoquery.py:754-765`).
- A photo that matches several values of `Title` or `Description` is returned once; upstream returns it once per value (`photoquery.py:480-516`).
- `IgnoreCase` folds case with `strings.ToLower`; Python `str.lower` gives another result for a few characters, such as U+0130 (`photoquery.py:484-491`).
- `query.Duplicates`, `DB.Duplicates` and `Duplicate` compare `OriginalFilesize`, `Height`, `Width` and `UTI` as the module reads them: a `NULL` column is 0 or `""` and matches 0 or `""`, where upstream compares `None`, which matches only `None`; dates are compared to the nanosecond, upstream compares Python datetimes to the microsecond (`photosdb/photosdb.py:3411-3420`).
- `Persons()` and `PersonCounts()` count the faces on the photos that `Photos()` returns; upstream counts the faces of every asset, those without a `ZADDITIONALASSETATTRIBUTES` row included, and keeps the order of a set and of the count (`photosdb/photosdb.py:498-509`, `photosdb/photosdb.py:553-557`).
- `NULL` in the pick type of a burst frame is 0. Upstream fails on `None & int` (`photosdb/photosdb.py:3347-3351`).
- `BurstPhotos` is in UUID order, and `BurstKeyPhoto` of a burst with several key photos is the first of them by UUID; upstream iterates a set (`photoinfo.py:1009-1044`).
- The second original of a RAW+JPEG pair is a resource of subtype 17, or of subtype 3 with resource type 4. Upstream looks for subtype 17 only (`photosdb/photosdb.py:2537-2551`). None of the test libraries has a resource of subtype 3 with type 4; the rule is checked only on the synthetic library.
- `IsMissing` looks for the resource with the fingerprint of the asset only among the resources of the asset, and of several the one with the highest `Z_PK` decides. Upstream takes such a resource of any asset, and the last row read decides (`photosdb/photosdb.py:2498-2516`).
- `Date` is in the fixed zone of `TZOffset`; `TZName` is not used for it. Upstream takes the zone of the name when it resolves one, so a name that disagrees with the offset gives another clock time (`photos_datetime.py:53-62`).
- Paths are looked up only inside the library: a symbolic link is never a file, and a link in a directory of the path is followed only while it stays inside the library. An absolute `ZDIRECTORY` (a referenced file) or an empty one gives the path ""; upstream uses an absolute one as is and looks for an empty one in `originals/<file name>`. Referenced files have no `PathLive` and `PathRaw`. Photos of shared albums and "Shared with you" photos not saved to the library have `Path` and `PathLive` ""; their `PathRaw` is looked up in `originals/`, as upstream does. The shared moment directory is not searched: a photo of a shared moment has a `Path` only when its file is in `originals/` (`photoinfo.py:209-234`, `photoinfo.py:545-571`, `photoinfo.py:1063-1071`). With several candidate RAW files the first in name order is used; upstream takes the first of `os.listdir`, whose order is unspecified (`utils.py:364-389`, `photoinfo.py:554-555`).
- `PathEdited`, `PathEditedLive`, `AdjustmentsPath`, `OriginalAdjustmentsPath` and `PathDerivatives` hold a file only when it is a regular file inside the library; a `ZUUID` with `/` gives them all "", where upstream builds the path from it as is. Upstream follows symbolic links, and `path_exists` accepts a directory too (`photoinfo.py:336-341`, `photoinfo.py:761-762`, `photoinfo.py:779-780`, `photoinfo.py:1218`, `utils.py:573-587`, `utils.py:772-773`); a derivatives directory that cannot be read gives no previews, where upstream raises `PermissionError` (`utils.py:762`). Previews of equal size are in name order; upstream keeps the order of `os.listdir` (`utils.py:762-778`). `adjustments` and `original_adjustments` (`adjustmentsinfo.py`, `photoinfo.py:783-800`) are not ported yet.
- `Orientation` of an edited asset is the database column. Upstream reads it from the property list of the edit (`photoinfo.py:1396-1404`).
- The UTI of an extension comes only from the table of `uti.py`, without `mdls` (`uti.py:642-653`).
- A repeated or missing `ZUUID` of an asset with a `ZADDITIONALASSETATTRIBUTES` row is an error of `Open`; assets without that row are left out unchecked, as upstream drops them (`photosdb/photosdb.py:2086-2090`). Upstream keeps the last row (`photosdb/photosdb.py:2356`). The album memberships of such assets are left out too, while upstream finds the photo of a membership row by its `ZUUID` (`photosdb/photosdb.py:1873-1901`, `photosdb/photosdb.py:3378-3394`).
- A chain of `ZPARENTFOLDER` of an album or a folder that does not reach the root folder or a row without a parent (a missing parent or a cycle) is an error of `Open`; a parent row without `ZUUID`, or of a `ZKIND` other than 2, 3999 and 4000, counts as missing. Upstream fails on a missing parent of any row (`photosdb/photosdb.py:1977-1980`) and on a cycle above an album (`photosdb/photosdb.py:2944-2965`), and accepts a cycle of folders that holds no album.
- `FolderInfo()`, `FolderTitles()` and the `Subfolders` and `Albums` of a folder are in UUID order; upstream keeps the order in which SQLite returns the rows of `ZGENERICALBUM` (`photosdb/photosdb.py:570-610`, `albuminfo.py:421-491`).
- `LibraryListOrder` of a folder in the trash, and of the folders and albums in it, starts with the orders of its enclosing folders. Upstream drops them when the enclosing folder holds no folder or album outside the trash, as an empty `FolderInfo` is false (`albuminfo.py:508-511`, `albuminfo.py:525-527`).
- A `TZOffset` outside (-86400, 86400) seconds always gives `Date` 1970-01-01T00:00:00Z. Upstream gives the same date (`ValueError`), but for an offset outside the range of `timedelta` it fails with an uncaught `OverflowError` (`photos_datetime.py:64-67`).
- Shared albums (`ZKIND` 1505) are not in `AlbumUUIDs`, and the album fields of `query.Options` (`Album`, `Folder`, `InAlbum`, `NotInAlbum`) do not see them. Upstream includes them in `album_info` and `albums` (`photoinfo.py:2023-2034`, `_constants.py:364-365`, `photoquery.py:671-674`, `photoquery.py:948-993`).
- An empty GUID string means no GUID, as `NULL` does.
- `AlbumUUIDs` includes albums in the trash; `AlbumInfoOf`, `AlbumTitlesOf`, `BurstAlbumInfoOf` and `BurstAlbumTitlesOf` leave them out, as osxphotos `album_info`, `albums`, `burst_album_info` and `burst_albums` of a photo do (`photoinfo.py:633-671`, `photoinfo.py:2029-2034`).
- `AlbumInfo()` returns every album of `ZKIND` 2 outside the trash. Upstream `album_info` also requires an empty `ZCLOUDOWNERHASHEDPERSONID` (`photosdb/photosdb.py:3206`); no album of `ZKIND` 2 in the test libraries has one.
- `AlbumInfo()` is in UUID order, `AlbumTitles()` and `AlbumTitlesOf` are sorted, `AlbumInfoOf` sorts by title, then by UUID. Upstream keeps the order of the rows of `ZGENERICALBUM`, of a Python set and of the membership rows (`photosdb/photosdb.py:612-650`, `photosdb/photosdb.py:3212-3225`, `photoinfo.py:655-662`).
- `Keywords()` and `KeywordsWithoutPhoto()` are sorted; upstream keeps the order of the rows. `KeywordCounts` is a Go map, without the order by count of upstream (`photosdb/photosdb.py:473-496`, `photosdb/photosdb.py:547-551`). A keyword without a title is `""` in all three; upstream has `None` (`unicode.py:79-80`, `photosdb/photosdb.py:2006-2014`).
- `BurstAlbumInfoOf` lists an album once. Upstream lists it twice when both the photo and the key photo of its burst are in it: it makes a set of `AlbumInfo` objects, which have no equality (`photoinfo.py:664-671`).
- `AlbumCounts` counts the photos of each album that `Photos()` returns, and an empty album adds 0. Upstream counts the membership rows, members without a `ZADDITIONALASSETATTRIBUTES` row included, sets the count of a title to 0 when an empty album of that title comes after another, and returns a dictionary ordered by the count, which a Go map does not keep (`photosdb/photosdb.py:511-526`, `photosdb/photosdb.py:1873-1901`).
- `PhotoUUIDs` of an album is a set; `AlbumPhotos` gives the photos in the order of the album. A `NULL` position of a photo in an album comes before the numbers; upstream fails when an album has both (`albuminfo.py:37-53`, `albuminfo.py:165-187`). `PhotoIndex` of a photo not in the album returns -1, false; upstream raises `ValueError` (`albuminfo.py:280-287`).
- `SystemLibraryPath` has no PhotoKit fallback (`utils.py:206-228`). When neither photolibraryd preferences file holds an absolute path without NUL, it returns `ErrLibraryNotFound`. Upstream returns any value of the key as is (`utils.py:180-206`).

## Extensions

Exported names that have no counterpart in osxphotos. Each is marked `// extension: not in osxphotos` in the code, and a test keeps this list and the code in agreement.

- `albuminfo.AlbumInfo.CloudGUID`
- `albuminfo.AlbumInfo.InTrash`
- `albuminfo.FolderInfo.CloudGUID`
- `photoinfo.PhotoInfo.CloudAssetGUID`
- `photoinfo.PhotoInfo.DateModifiedRaw`
- `photoinfo.PhotoInfo.LibraryScope`
- `photoinfo.PhotoInfo.LibraryScopeShareState`
- `photoinfo.PhotoInfo.Resources`
- `photoinfo.Resource`
- `photosdb.Info.LibraryUUID`
- `photosdb.Info.SkippedAssets`

<a id="upstream-osxphotos-ru"></a>

## Upstream: osxphotos (по-русски)

Для тех, кто сравнивает поведение с Python osxphotos или обновляет порт; пользователю достаточно [README](README.md).

Модуль — порт частей [osxphotos](https://github.com/RhetTbull/osxphotos) на Go. Порт следует одному зафиксированному выпуску upstream; правки upstream после этого выпуска не переносятся, пока версия выше не обновлена.

Версия должна совпадать с версией в `tools/golden/requirements.txt` (по ней строятся эталонные данные). При расхождении CI падает, поэтому переход на новую версию upstream меняет оба файла в одном PR.

Ссылки на upstream ниже записаны как `файл:строка` в этом выпуске. Пути отсчитываются от пакета `osxphotos/` выпуска, как в таблице.

### Соответствие

Только то, что уже перенесено. Шаблоны, экспорт и PhotoKit появятся вместе со своими пакетами.

| Модуль osxphotos | Пакет Go |
|---|---|
| `photosdb/photosdb.py` | `photosdb`, `internal/sqlread` |
| `photosdb/photosdb_utils.py` | `photosdb` |
| `_constants.py` | `internal/schema`, `photoinfo` |
| `sqlite_utils.py` | `internal/sqlread` |
| `photosdb/_photosdb_process_shared_library.py` | `photosdb` |
| `photosdb/_photosdb_process_syndicationinfo.py` | `photosdb` |
| `photoinfo.py` | `photoinfo`, `photosdb` |
| `albuminfo.py` | `albuminfo`, `photosdb` |
| `photoquery.py` | `query` |
| `photos_datetime.py` | `photosdb` |
| `unicode.py` | `photosdb` |
| `uti.py` | `internal/uti` |
| `utils.py` (только `get_system_library_path`) | `photosdb` |

### Намеренные отличия

Если читать только три пункта: (1) модели вне таблицы поддерживаемых дают `ErrUnsupportedSchema`, а не догадку; (2) база всегда читается из копии; (3) `Photos()` без неявных фильтров; `Query(query.Options{})` отдаёт то же, что `osxphotos query`.

- Номер модели от 13000 до 16319 (Photos 5-7 и предварительные сборки 16000-16319) или от 18000 до 18200 даёт `ErrUnsupportedSchema`. Upstream читает все эти модели по их собственным схемам (`_constants.py:56-62`).
- Любой другой номер модели, которого нет в таблице схем, тоже даёт `ErrUnsupportedSchema`: меньше 13000, 19000-19062, 20000-269999999, больше 270999999 или отсутствующий `PLModelVersion`. Upstream для номера вне всех своих диапазонов и для отсутствующего берёт последнюю схему (`photosdb/photosdb_utils.py:131-136`, `photosdb/photosdb_utils.py:160-163`).
- Строковый `PLModelVersion` принимается, только если это десятичные цифры ASCII. `int()` в upstream принимает ещё знак, пробелы по краям и цифры других алфавитов (не ASCII) (`photosdb/photosdb_utils.py:137`).
- `Z_PLIST` больше 1 МиБ или двоичный список свойств с более чем 4096 объектами даёт `ErrUnsupportedSchema`. `SystemLibraryPath` пропускает файл настроек больше 1 МиБ или не являющийся списком свойств. В `plistlib` upstream таких ограничений нет (`photosdb/photosdb_utils.py:114`, `utils.py:193`).
- База всегда читается из копии вместе с файлом `-wal`. Upstream копирует базу, только когда она заблокирована (`photosdb/photosdb.py:438-443`, `sqlite_utils.py:125-137`).
- `Info.LibraryPath` — каталог, как передан в `Open`, а для файла базы — родитель её каталога, путь абсолютный; upstream делает выведенный путь абсолютным и разрешает символические ссылки, а `library_path` берёт как передан (`photosdb/photosdb.py:349-350`, `photosdb/photosdb.py:452-458`). `Options.LibraryPath` должен существовать; upstream его не проверяет (`photosdb/photosdb.py:456-458`). `Info.DBPath` абсолютный, но символические ссылки в нём не разрешаются (`photosdb/photosdb.py:349-350`, `photosdb/photosdb.py:698-701`). `photos.db` без `Photos.sqlite` рядом или медиатека, где есть только `database/photos.db`, дают ошибку «не существует»; upstream читает такой файл как базу Photos 4 и раньше (`photosdb/photosdb.py:359-362`, `photosdb/photosdb.py:370-376`).
- `Photos()` без неявных фильтров: возвращает и фото из корзины, и невыбранные кадры серий. Выборку `photos()` из osxphotos — фото или видео вне корзины, кроме невыбранных кадров серий (у такого кадра отметка не 0 и в ней нет ни бита выбранного, ни бита ключевого кадра), — делает `Query`, как `PhotosDB.query` (`photosdb/photosdb.py:3269-3276`, `photosdb/photosdb.py:3346-3353`, `photosdb/photosdb.py:3355-3358`, `photoquery.py:426-445`).
- `query.Options` переносит `QueryOptions` поле за полем (`photoquery.py:147-241`). Не переносятся: `exif` (exiftool), `label` (индекс поиска), `place` и `no_place` (обратное геокодирование), `query_eval` (выражения Python), `regex` (шаблоны), `selected` (выделение в приложении «Фото» через AppleScript), `has_comment`, `no_comment`, `has_likes` и `no_likes` (комментарии общих альбомов).
- `DateAdded`, которого нет в базе, для `AddedAfter`, `AddedBefore` и `AddedInLast` — 1970-01-01T00:00:00Z; upstream берёт полночь 1970-01-01 в поясе компьютера (`photos_datetime.py:19-21`, `photoquery.py:822-837`).
- Первая `Function` получает фото в порядке каталога (по UUID); в upstream порядок зависит от того, какие фильтры сработали, часть из них проходит через множество (`photoquery.py:859-861`); с `Duplicate` — в порядке шага дубликатов, как в upstream (`photoquery.py:754-765`).
- Фото, которое подходит под несколько значений `Title` или `Description`, возвращается один раз; upstream возвращает его по разу на значение (`photoquery.py:480-516`).
- `IgnoreCase` сравнивает без учёта регистра через `strings.ToLower`; `str.lower` в Python для нескольких символов даёт другой результат, например для U+0130 (`photoquery.py:484-491`).
- `query.Duplicates`, `DB.Duplicates` и `Duplicate` сравнивают `OriginalFilesize`, `Height`, `Width` и `UTI` в том виде, в каком их читает модуль: колонка `NULL` даёт 0 или `""` и совпадает с 0 или `""`, а upstream сравнивает `None`, которое совпадает только с `None`; даты сравниваются до наносекунды, upstream сравнивает datetime Python до микросекунды (`photosdb/photosdb.py:3411-3420`).
- `Persons()` и `PersonCounts()` считают лица на фото, которые отдаёт `Photos()`; upstream считает лица всех объектов, включая объекты без строки `ZADDITIONALASSETATTRIBUTES`, и сохраняет порядок множества и счёта (`photosdb/photosdb.py:498-509`, `photosdb/photosdb.py:553-557`).
- `NULL` в отметке кадра серии — 0. Upstream падает на `None & int` (`photosdb/photosdb.py:3347-3351`).
- `BurstPhotos` идёт по UUID, а `BurstKeyPhoto` серии с несколькими ключевыми кадрами — первый из них по UUID; upstream обходит множество (`photoinfo.py:1009-1044`).
- Второй оригинал пары RAW+JPEG — ресурс подтипа 17 или ресурс подтипа 3 с типом ресурса 4. Upstream ищет только подтип 17 (`photosdb/photosdb.py:2537-2551`). Ни в одной тестовой медиатеке нет ресурса подтипа 3 с типом 4; правило проверено только на синтетической медиатеке.
- `IsMissing` ищет ресурс с отпечатком объекта только среди ресурсов этого объекта, а из нескольких решает ресурс с наибольшим `Z_PK`. Upstream берёт такой ресурс любого объекта, и решает последняя прочитанная строка (`photosdb/photosdb.py:2498-2516`).
- `Date` — в фиксированной зоне смещения `TZOffset`; `TZName` для него не используется. Upstream берёт зону по имени, если находит её, поэтому имя, не согласное со смещением, даёт другое время на часах (`photos_datetime.py:53-62`).
- Пути ищутся только внутри медиатеки: символическая ссылка — никогда не файл, а ссылка в каталоге пути проходится, только пока остаётся внутри медиатеки. Абсолютный `ZDIRECTORY` (файл-ссылка) или пустой даёт путь ""; upstream берёт абсолютный как есть, а пустой ищет в `originals/<имя файла>`. У файлов-ссылок нет `PathLive` и `PathRaw`. У фото общих альбомов и фото «Отправлено вам», не сохранённых в медиатеку, `Path` и `PathLive` — ""; их `PathRaw` ищется в `originals/`, как в upstream. Каталог общих моментов не просматривается: у фото общего момента `Path` есть, только если его файл лежит в `originals/` (`photoinfo.py:209-234`, `photoinfo.py:545-571`, `photoinfo.py:1063-1071`). Из нескольких файлов-кандидатов RAW берётся первый по порядку имён; upstream берёт первый из `os.listdir`, порядок которого не задан (`utils.py:364-389`, `photoinfo.py:554-555`).
- `PathEdited`, `PathEditedLive`, `AdjustmentsPath`, `OriginalAdjustmentsPath` и `PathDerivatives` содержат файл, только если это обычный файл внутри медиатеки; при `ZUUID` с `/` все они "", а upstream строит путь из него как есть. Upstream проходит по символическим ссылкам, а `path_exists` принимает и каталог (`photoinfo.py:336-341`, `photoinfo.py:761-762`, `photoinfo.py:779-780`, `photoinfo.py:1218`, `utils.py:573-587`, `utils.py:772-773`); каталог превью, который нельзя прочитать, не даёт превью, а upstream выбрасывает `PermissionError` (`utils.py:762`). Превью одного размера идут по порядку имён; upstream сохраняет порядок `os.listdir` (`utils.py:762-778`). `adjustments` и `original_adjustments` (`adjustmentsinfo.py`, `photoinfo.py:783-800`) пока не перенесены.
- `Orientation` правленого объекта — колонка базы. Upstream читает её из списка свойств правки (`photoinfo.py:1396-1404`).
- UTI по расширению берётся только из таблицы `uti.py`, без `mdls` (`uti.py:642-653`).
- Повтор или отсутствие `ZUUID` у объекта со строкой `ZADDITIONALASSETATTRIBUTES` — ошибка `Open`; объекты без этой строки отбрасываются без проверки, как в upstream (`photosdb/photosdb.py:2086-2090`). Upstream оставляет последнюю строку (`photosdb/photosdb.py:2356`). Членство таких объектов в альбомах тоже отбрасывается, а upstream находит фото строки членства по её `ZUUID` (`photosdb/photosdb.py:1873-1901`, `photosdb/photosdb.py:3378-3394`).
- Цепочка `ZPARENTFOLDER` альбома или папки, которая не доходит до корневой папки или до строки без родителя (родителя нет или цикл), — ошибка `Open`; родительская строка без `ZUUID` или с `ZKIND`, отличным от 2, 3999 и 4000, считается отсутствующей. Upstream падает на отсутствующем родителе любой строки (`photosdb/photosdb.py:1977-1980`) и на цикле над альбомом (`photosdb/photosdb.py:2944-2965`), а цикл папок без альбомов принимает.
- `FolderInfo()`, `FolderTitles()`, а также `Subfolders` и `Albums` папки идут по UUID; upstream сохраняет порядок, в котором SQLite отдаёт строки `ZGENERICALBUM` (`photosdb/photosdb.py:570-610`, `albuminfo.py:421-491`).
- `LibraryListOrder` папки в корзине и вложенных в неё папок и альбомов начинается с порядков объемлющих папок. Upstream их отбрасывает, если в объемлющей папке нет папок и альбомов вне корзины: пустой `FolderInfo` ложен (`albuminfo.py:508-511`, `albuminfo.py:525-527`).
- `TZOffset` вне диапазона (-86400, 86400) секунд всегда даёт `Date` 1970-01-01T00:00:00Z. Upstream даёт ту же дату (`ValueError`), но на смещении вне диапазона `timedelta` падает с непойманным `OverflowError` (`photos_datetime.py:64-67`).
- Общие альбомы (`ZKIND` 1505) не входят в `AlbumUUIDs`, и поля альбомов `query.Options` (`Album`, `Folder`, `InAlbum`, `NotInAlbum`) их не видят. Upstream включает их в `album_info` и `albums` (`photoinfo.py:2023-2034`, `_constants.py:364-365`, `photoquery.py:671-674`, `photoquery.py:948-993`).
- Пустая строка GUID означает отсутствие GUID, как и `NULL`.
- `AlbumUUIDs` включает альбомы в корзине; `AlbumInfoOf`, `AlbumTitlesOf`, `BurstAlbumInfoOf` и `BurstAlbumTitlesOf` их не берут, как `album_info`, `albums`, `burst_album_info` и `burst_albums` фото в osxphotos (`photoinfo.py:633-671`, `photoinfo.py:2029-2034`).
- `AlbumInfo()` возвращает все альбомы с `ZKIND` 2 вне корзины. `album_info` в upstream требует ещё пустого `ZCLOUDOWNERHASHEDPERSONID` (`photosdb/photosdb.py:3206`); ни у одного альбома с `ZKIND` 2 в тестовых медиатеках его нет.
- `AlbumInfo()` идёт по UUID, `AlbumTitles()` и `AlbumTitlesOf` отсортированы, `AlbumInfoOf` сортирует по названию, затем по UUID. Upstream сохраняет порядок строк `ZGENERICALBUM`, множества Python и строк членства (`photosdb/photosdb.py:612-650`, `photosdb/photosdb.py:3212-3225`, `photoinfo.py:655-662`).
- `Keywords()` и `KeywordsWithoutPhoto()` отсортированы; upstream сохраняет порядок строк. `KeywordCounts` — `map` Go, без порядка по счёту, который есть у upstream (`photosdb/photosdb.py:473-496`, `photosdb/photosdb.py:547-551`). Ключевое слово без названия во всех трёх — `""`; у upstream — `None` (`unicode.py:79-80`, `photosdb/photosdb.py:2006-2014`).
- `BurstAlbumInfoOf` называет альбом один раз. Upstream называет его дважды, если в нём и само фото, и ключевой кадр его серии: это множество объектов `AlbumInfo`, у которых нет равенства (`photoinfo.py:664-671`).
- `AlbumCounts` считает фото каждого альбома из тех, что отдаёт `Photos()`; пустой альбом добавляет 0. Upstream считает строки членства, включая объекты без строки `ZADDITIONALASSETATTRIBUTES`, обнуляет счёт названия, если пустой альбом с этим названием идёт после другого, и отдаёт словарь по убыванию счёта, а у `map` Go порядка нет (`photosdb/photosdb.py:511-526`, `photosdb/photosdb.py:1873-1901`).
- `PhotoUUIDs` альбома — множество; фото в порядке альбома даёт `AlbumPhotos`. Позиция фото в альбоме `NULL` идёт раньше чисел; upstream падает, если в альбоме есть и то и другое (`albuminfo.py:37-53`, `albuminfo.py:165-187`). `PhotoIndex` фото не из альбома возвращает -1, false; upstream бросает `ValueError` (`albuminfo.py:280-287`).
- У `SystemLibraryPath` нет запасного пути через PhotoKit (`utils.py:206-228`). Если ни один из двух файлов настроек photolibraryd не содержит абсолютный путь без NUL, возвращается `ErrLibraryNotFound`. Upstream возвращает любое значение ключа как есть (`utils.py:180-206`).

### Расширения

Экспортируемые имена, у которых нет пары в osxphotos. Каждое помечено в коде `// extension: not in osxphotos`, а тест проверяет, что список совпадает с кодом.

- `albuminfo.AlbumInfo.CloudGUID`
- `albuminfo.AlbumInfo.InTrash`
- `albuminfo.FolderInfo.CloudGUID`
- `photoinfo.PhotoInfo.CloudAssetGUID`
- `photoinfo.PhotoInfo.DateModifiedRaw`
- `photoinfo.PhotoInfo.LibraryScope`
- `photoinfo.PhotoInfo.LibraryScopeShareState`
- `photoinfo.PhotoInfo.Resources`
- `photoinfo.Resource`
- `photosdb.Info.LibraryUUID`
- `photosdb.Info.SkippedAssets`
