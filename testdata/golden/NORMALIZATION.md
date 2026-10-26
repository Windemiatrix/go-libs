# Reference data comparison rules

**English** | [Русский](#normalization-ru)

`query.json` and `deleted.json` of every library in this directory are the output of `osxphotos query --json` (osxphotos 0.77.2), reduced to the keys of `fields.txt`. Tests compare what the module reads from the same library with these files; the code is `osxphotos/internal/golden`. This file says what "equal" means, key by key. A rule that cannot hold is not bent here: the difference is a record in `divergences.json` with a reason.

Upstream places are written as `file:line` relative to the `osxphotos/` package of the upstream sources, as in `osxphotos/UPSTREAM.md`.

## General rules

1. **By UUID.** The reference holds the objects of `query.json` and `deleted.json` together; a UUID repeated in them, or repeated on the Go side, is an error of the input. The Go side is the photos that upstream `photos()` would return: not an unselected burst frame (pick type not 0 and with neither the selected nor the key bit) and a photo or a movie (other kinds have no type upstream). An object on one side only is a difference of the key `uuid`; the other keys are compared for the objects both sides have.
2. **A date is a moment**, to the microsecond (the resolution of Python `datetime`). A reference string (ISO 8601, offset `±HH:MM`, or `±HH:MM:SS` for some zones) is read as a moment; the Go value is compared as it is, without rounding, in UTC. The offset written in the string `date` is **not** compared: upstream takes it from the time zone name (`tzname`), which the module does not apply to `Date`. The offset of the asset is the key `tzoffset`.
3. **`null` equals a zero Go value** only for the keys whose row says so: a string is `""`, an integer is `0`, a pointer is `nil`. `null` where a row does not allow it (a boolean, a strict string) is an error of the form of the reference.
4. **Integers are exact** (a JSON integer that fits `int64`; `3024.0` is not accepted). **Floats** (`latitude`, `longitude`) are exact too: both sides read one IEEE double and neither Python `repr` nor Go changes it, so there is no tolerance (a tolerance would hide a wrong column).
5. **Lists of strings** are sorted by upstream in `json()` (`photoinfo.py:2279-2285`), except `location`; they are compared in order, and the Go side has to give the same order (by bytes of UTF-8, which is the order of code points). `null` in a list equals `""`. A list that is a set (`album_info`, `burst_album_info`) is compared as a set by `uuid`; `keywords` and `persons` are lists and keep repeats.
6. **Strings are compared strictly.** Both sides are in Unicode NFC (`unicode.py:12, 76-92`); the comparison does not normalize, so a decomposed string on the Go side is a difference.
7. **Paths** (`path*`) are not in `fields.txt`: the test libraries have no original files. When they appear, they are compared relative to the root of the library.
8. **Shared albums are not read.** Upstream lists albums of kind 1505 (shared) together with kind 2 (`photoinfo.py:2023-2034`; `_constants.py:364-365`), the module reads only kind 2. A shared album is told by `owner` not `null` (`albuminfo.py:113-133`); such elements, and their titles when no kept element has the title, are dropped from the reference before the comparison. The rule is applied to the reference, never to Go: the owner of an album of the user is always `null`, so an album of the user that Go lost is not hidden. A shared album with `owner: null` (owner not found) and a shared and a user album with one title on one object (`folders` keeps one value) are not caught: they are records of `divergences.json` by UUID.

## Keys

One row per key of `fields.txt`. "Go" is the field of `photoinfo.PhotoInfo` (and `albuminfo.AlbumInfo`).

| Key | Go | Rule | Upstream |
|---|---|---|---|
| `adjustment_type` | `AdjustmentType` | integer; `null` = `nil`; `0` is not `null` | `photoinfo.py:802-805`; `photosdb/photosdb.py:2183` |
| `album_info` | `AlbumUUIDs` → `Library.Albums`, albums in the trash left out | Set by `uuid` after rule 8. Per element: `uuid`; `title` (`null` = `""`); `parent` (`null` = `ParentUUID` `""`); `folder_names` in order; `creation_date` (a moment; 2001-01-01T00:00:00 = `nil`, as upstream defaults a missing value to 0 seconds of Core Data); `start_date`, `end_date` (`null` = `nil`). `folder_list`, `owner` and `sort_order` are not compared: Go has no counterpart | `photoinfo.py:655-662, 1991-2034`; `albuminfo.py:76-107, 135-143, 289-297`; `photosdb/photosdb.py:1937-1938` |
| `albums` | titles of the same albums | Sorted set of titles, repeats merged; titles of dropped shared albums are removed unless a kept element of `album_info` has the title | `photoinfo.py:633-644` |
| `burst` | `Burst` | boolean | `photosdb/photosdb.py:2212-2223` |
| `burst_album_info` | `Library.BurstAlbums` (`photosdb.DB.BurstAlbumInfoOf`) → `Library.Albums`, albums in the trash left out | as `album_info` | `photoinfo.py:664-671` |
| `burst_albums` | titles of the same albums | as `albums` (by `burst_album_info`) | `photoinfo.py:646-653` |
| `burst_default_pick` | `BurstDefaultPick` | boolean | `photoinfo.py:1021-1029`; `_constants.py:678` |
| `burst_key` | `BurstKey` | boolean | `photoinfo.py:999-1007` |
| `burst_photos` | other photos of the burst in `Library.Photos` | With `Burst`: UUIDs of the other photos that have `Burst` and the same `BurstUUID` (an empty one too: upstream groups by "is not None"), selected or not, in the trash or not, photos or not; sorted; without the photo itself. Without `Burst`: `[]` | `photosdb/photosdb.py:2212-2220`; `photoinfo.py:1031-1044` |
| `burst_selected` | `BurstSelected` | boolean | `photoinfo.py:989-997` |
| `cloud_guid` | `CloudMasterGUID` | `null` = `""` | `photoinfo.py:1762-1765` |
| `cloud_owner_hashed_id` | `CloudOwnerHashedID` | `null` = `""` | `photoinfo.py:1767-1770`; `photosdb/photosdb.py:2320` |
| `date` | `Date` | moment (rule 2) | `photosdb/photosdb.py:2159-2167`; `photos_datetime.py:27-67` |
| `date_added` | `DateAdded` | moment; the reference 1970-01-01T00:00:00 (the default of upstream) = `nil` or that moment | `photosdb/photosdb.py:2317`; `photos_datetime.py:19-21, 77-78`; `photoinfo.py:849-855` |
| `date_modified` | `DateModified` | moment; `null` = `nil` | `photosdb/photosdb.py:2156-2157`; `photoinfo.py:165-177` |
| `date_trashed` | `DateTrashed` | moment; `null` = `nil` | `photosdb/photosdb.py:2292-2293`; `photoinfo.py:842-847` |
| `description` | `Description` | `null` = `""` | `photoinfo.py:592-595` |
| `external_edit` | `ExternalEdit` | boolean | `photoinfo.py:807-810`; `photosdb/photosdb.py:2423-2439` |
| `favorite` | `Favorite` | boolean | `photoinfo.py:812-815` |
| `filename` | `Filename` | strict string | `photoinfo.py:114-140` |
| `fingerprint` | `Fingerprint` | `null` = `""` | `photoinfo.py:1772-1775` |
| `folders` | albums of `album_info` | Dictionary "title → `folder_names`" after rule 8: the same titles; with several Go albums of one title the value of the reference must equal `FolderNames` of one of them. An album without a title is the key `"null"` (`json.dumps` writes the key `None` as `null`; a mix of `None` and strings in keys cannot be sorted, so the reference has only that one key then) | `photoinfo.py:2125, 2286-2288` |
| `has_raw` | `HasRAW` | boolean | `photoinfo.py:1363-1366` |
| `hasadjustments` | `HasAdjustments` | boolean | `photoinfo.py:738-741` |
| `hdr` | `HDR` | boolean | `photoinfo.py:1291-1294`; `photosdb/photosdb.py:2248-2249` |
| `height` | `Height` | integer, `null` = 0 | `photoinfo.py:1380-1383` |
| `hidden` | `Hidden` | boolean | `photoinfo.py:827-830` |
| `imported_by` | `[ImportedByName, ImportedByBundleID]` | pair, the order matters, each `null` = `""` | `photoinfo.py:682-694`; `photosdb/photosdb.py:2070-2075, 2324-2325` |
| `incloud` | `InCloud` | `null` = `nil`; `false` is not `null` | `photoinfo.py:958-964` |
| `intrash` | `InTrash` | boolean | `photoinfo.py:837-840` |
| `iscloudasset` | `IsCloudAsset` | boolean | `photoinfo.py:966-977` |
| `ismissing` | `IsMissing` | boolean | `photoinfo.py:724-736` |
| `ismovie` | `IsMovie` | boolean | `photoinfo.py:948-951` |
| `isphoto` | `IsPhoto` | boolean | `photoinfo.py:953-956` |
| `israw` | `IsRAW` | boolean | `photoinfo.py:1368-1371` |
| `isreference` | `IsReference` | boolean | `photoinfo.py:979-982` |
| `keywords` | `Keywords` | list in the order of the reference, repeats kept; the Go list is not sorted by the comparison | `photoinfo.py:705-708` |
| `latitude` | `Latitude` | `null` = `nil`; exact | `photosdb/photosdb.py:2174-2181` |
| `live_photo` | `Live` | boolean | `photoinfo.py:1046-1049` |
| `location` | `[Latitude, Longitude]` | pair, the order matters, each `null` = `nil`; exact | `photoinfo.py:857-860` |
| `longitude` | `Longitude` | `null` = `nil`; exact | `photosdb/photosdb.py:2174-2181` |
| `orientation` | `Orientation` | integer, `null` = 0; **compared only when `hasadjustments` of the reference is `false`**: for an adjusted photo upstream reads a plist of the adjustments, which the test libraries do not have, and gives 0 | `photoinfo.py:1390-1404` |
| `original_filename` | `OriginalFilename` | strict string | `photoinfo.py:114-140` |
| `original_filesize` | `OriginalFilesize` | integer, `null` = 0 | `photoinfo.py:1421-1424` |
| `original_height` | `OriginalHeight` | integer, `null` = 0 | `photoinfo.py:1406-1409` |
| `original_orientation` | `OriginalOrientation` | integer, `null` = 0 | `photoinfo.py:1416-1419` |
| `original_width` | `OriginalWidth` | integer, `null` = 0 | `photoinfo.py:1411-1414` |
| `panorama` | `Panorama` | boolean | `photoinfo.py:1276-1279`; `photosdb/photosdb.py:2257-2258` |
| `persons` | `Persons` | list in the order of the reference, a name per face, repeats kept; a person without a name is `_UNKNOWN_` on both sides; the Go list is not sorted by the comparison | `photoinfo.py:597-603`; `photosdb/photosdb.py:1790-1796, 1844-1869`; `_constants.py:337` |
| `portrait` | `Portrait` | boolean | `photoinfo.py:1306-1309`; `photosdb/photosdb.py:2250-2251` |
| `raw_original` | `RawOriginal` | boolean | `photoinfo.py:1373-1378` |
| `saved_to_library` | `SavedToLibrary` | boolean | `photoinfo.py:1586-1599` |
| `screen_recording` | `ScreenRecording` | boolean | `photoinfo.py:1301-1304`; `photosdb/photosdb.py:2237` |
| `screenshot` | `Screenshot` | boolean | `photoinfo.py:1296-1299` |
| `selfie` | `Selfie` | boolean | `photoinfo.py:1322-1325`; `photosdb/photosdb.py:2260-2261` |
| `shared` | `Shared` | boolean | `photoinfo.py:872-879` |
| `shared_library` | `SharedLibrary` | boolean | `photoinfo.py:1628-1632` |
| `shared_moment` | `SharedMoment` | boolean | `photoinfo.py:1601-1604`; `photosdb/photosdb.py:2322` |
| `slow_mo` | `SlowMo` | boolean | `photoinfo.py:1281-1284`; `photosdb/photosdb.py:2239` |
| `spatial` | `Spatial` | integer, `null` = 0; 0 for a library without `ZASSET.ZSPATIALTYPE` | `photoinfo.py:1311-1320`; `photosdb/photosdb.py:1751-1759, 2253-2255` |
| `syndicated` | `Syndicated` | boolean | `photoinfo.py:1569-1584` |
| `time_lapse` | `TimeLapse` | boolean | `photoinfo.py:1286-1289`; `photosdb/photosdb.py:2240` |
| `title` | `Title` | `null` = `""` (upstream turns `""` into `None`) | `photoinfo.py:710-717` |
| `tzname` | `TZName` | `null` = `""` | `photoinfo.py:184-187`; `photosdb/photosdb.py:2160` |
| `tzoffset` | `TZOffset` | integer; `null` = `nil`; `0` is not `null` | `photoinfo.py:179-182`; `photosdb/photosdb.py:2159` |
| `uti` | `UTI` | strict string | `photoinfo.py:881-921` |
| `uti_edited` | `UTIEdited` | `null` = `""` | `photoinfo.py:923-932` |
| `uti_original` | `UTIOriginal` | strict string | `photoinfo.py:881-921` |
| `uti_raw` | `UTIRaw` | `null` = `""` | `photoinfo.py:934-946` |
| `uuid` | `UUID` | the sets of rule 1 | `photosdb/photosdb.py:2197-2204, 3346-3360` |
| `visible` | `Visible` | boolean | `photoinfo.py:832-835`; `photosdb/photosdb.py:2304-2307` |
| `width` | `Width` | integer, `null` = 0 | `photoinfo.py:1385-1388` |

## divergences.json

A JSON array of records; each is a known difference between osxphotos and this module, with the reason. Example (the shape only):

```json
[
  {
    "library": "synthetic",
    "uuid": "*",
    "field": "has_raw",
    "want": false,
    "got": true,
    "reason": "The second original of a RAW+JPEG pair is a resource of subtype 3 and resource type 4; upstream reads only subtype 17.",
    "upstream": "photosdb/photosdb.py:2537-2551"
  }
]
```

- `library` is a directory name of `testdata/libraries`, `synthetic`, or `local` (a run on the library of the person who runs the tool); `uuid` is the UUID of an object or `*` (all objects of the library); `field` is a key of `fields.txt`; `reason` is not empty, in English; `upstream` is `path.py:line` or `path.py:first-last` from the `osxphotos/` package.
- `want` and `got` are the normalized values of the two sides, in any JSON form; they are compared in the compact form with sorted keys that the report writes. They are optional for a record with a UUID and **required for `*`**: a `*` without values would hide the whole key of a library. A record explains a difference when `library`, `field` and `uuid` (or `*`) match and, where given, `want` and `got`. `"want": null` is a value; an absent `want` matches any.
- `local` is only for `uuid: "*"`, with `true`, `false`, `null` or a number as values: the file is public and holds no UUID and no string of a library of its owner.
- Two records may not have the same library, uuid, field, want and got.
- A **stale record** is a record of the compared library whose field was compared (or is `uuid`) and which explained nothing; the test is red. A **new difference** with no record is red too. Records of other libraries and of keys that were not compared are not stale.
- A record by UUID is preferred. `*` is for one reason and one pair of values on many objects (more than ten); it is added only together with a test that finds the candidate objects by direct read-only SQL and checks that the UUIDs of all differences of the key are among them and their number is the expected one. Without that, `*` would hide errors of the port in the same key.

## Order of the reference

Objects of `query.json` and of `deleted.json` are in the order of osxphotos: by `date`, then `date_added` (`null` is 1970-01-01T00:00:00Z), then `uuid`, ascending; dates as moments, `uuid` by bytes (`photoquery.py:888-898`). A test checks it for every library; the order is kept by the tool that regenerates the files.

<a id="normalization-ru"></a>

## Правила сравнения с эталоном (по-русски)

`query.json` и `deleted.json` каждой медиатеки в этом каталоге — вывод `osxphotos query --json` (osxphotos 0.77.2), сокращённый до ключей `fields.txt`. Тесты сравнивают с этими файлами то, что модуль читает из той же медиатеки; код — `osxphotos/internal/golden`. Здесь сказано, что значит «равно», по ключам. Правило, которое не выполняется, не подгоняется: расхождение записывается в `divergences.json` с причиной.

Места в upstream — `файл:строка` относительно пакета `osxphotos/` исходников upstream, как в `osxphotos/UPSTREAM.md`.

### Общие правила

1. **По UUID.** Эталон — объекты `query.json` и `deleted.json` вместе; UUID, повторённый в них или на стороне Go, — ошибка входа. Сторона Go — фото, которые вернул бы `photos()` upstream: не невыбранный кадр серии (отметка не 0 и без бита выбранного и бита ключевого кадра) и фото или видео (у остальных видов в upstream нет типа). Объект только с одной стороны — расхождение ключа `uuid`; остальные ключи сравниваются у объектов, которые есть с обеих сторон.
2. **Дата — момент**, с точностью до микросекунды (разрешение `datetime` Python). Строка эталона (ISO 8601, смещение `±HH:MM`, у некоторых зон `±HH:MM:SS`) читается как момент; значение Go сравнивается как есть, без округления, в UTC. Смещение в строке `date` **не** сравнивается: upstream берёт его из названия часового пояса (`tzname`), которое модуль к `Date` не применяет. Смещение объекта — ключ `tzoffset`.
3. **`null` равно нулевому значению Go** только у ключей, где это сказано в строке: строка — `""`, целое — `0`, указатель — `nil`. `null` там, где строка его не допускает (логическое значение, строгая строка), — ошибка формы эталона.
4. **Целые — точно** (целое JSON, помещающееся в `int64`; `3024.0` не принимается). **Дробные** (`latitude`, `longitude`) — тоже точно: обе стороны читают один IEEE double, ни `repr` Python, ни Go его не меняют, допуска нет (допуск спрятал бы чтение не той колонки).
5. **Списки строк** upstream сортирует в `json()` (`photoinfo.py:2279-2285`), кроме `location`; сравниваются по порядку, Go обязан дать тот же порядок (по байтам UTF-8, это порядок кодовых точек). `null` в списке равно `""`. Список-множество (`album_info`, `burst_album_info`) сравнивается как множество по `uuid`; `keywords` и `persons` — списки, повторы сохраняются.
6. **Строки сравниваются строго.** Обе стороны в Unicode NFC (`unicode.py:12, 76-92`); сравнение не нормализует, поэтому строка Go в разложенной форме — расхождение.
7. **Пути** (`path*`) в `fields.txt` не входят: оригиналов в тестовых медиатеках нет. Появятся — сравниваются относительно корня медиатеки.
8. **Общие альбомы не переносятся.** Upstream включает альбомы вида 1505 (общие) вместе с видом 2 (`photoinfo.py:2023-2034`; `_constants.py:364-365`), модуль читает только вид 2. Общий альбом узнаётся по `owner` не `null` (`albuminfo.py:113-133`); такие элементы и их названия (если названия нет ни у одного оставшегося элемента) убираются из эталона до сравнения. Правило применяется к эталону, не к Go: у альбома пользователя `owner` всегда `null`, поэтому потерянный Go альбом пользователя не прячется. Общий альбом с `owner: null` (владелец не найден) и общий и пользовательский альбом с одним названием у одного объекта (`folders` хранит одно значение) правило не ловит: это записи `divergences.json` по UUID.

### Ключи

Одна строка на ключ `fields.txt`. «Go» — поле `photoinfo.PhotoInfo` (и `albuminfo.AlbumInfo`).

| Ключ | Go | Правило | Upstream |
|---|---|---|---|
| `adjustment_type` | `AdjustmentType` | целое; `null` = `nil`; `0` — не `null` | `photoinfo.py:802-805`; `photosdb/photosdb.py:2183` |
| `album_info` | `AlbumUUIDs` → `Library.Albums`, альбомы в корзине не берутся | Множество по `uuid` после правила 8. У элемента: `uuid`; `title` (`null` = `""`); `parent` (`null` = `ParentUUID` `""`); `folder_names` по порядку; `creation_date` (момент; 2001-01-01T00:00:00 = `nil`: upstream подставляет вместо пустого значения 0 секунд Core Data); `start_date`, `end_date` (`null` = `nil`). `folder_list`, `owner` и `sort_order` не сравниваются: в Go их нет | `photoinfo.py:655-662, 1991-2034`; `albuminfo.py:76-107, 135-143, 289-297`; `photosdb/photosdb.py:1937-1938` |
| `albums` | названия тех же альбомов | Отсортированное множество названий, повторы склеены; названия отброшенных общих альбомов убираются, если такого названия нет у оставшегося элемента `album_info` | `photoinfo.py:633-644` |
| `burst` | `Burst` | логическое | `photosdb/photosdb.py:2212-2223` |
| `burst_album_info` | `Library.BurstAlbums` (`photosdb.DB.BurstAlbumInfoOf`) → `Library.Albums`, альбомы в корзине не берутся | как `album_info` | `photoinfo.py:664-671` |
| `burst_albums` | названия тех же альбомов | как `albums` (по `burst_album_info`) | `photoinfo.py:646-653` |
| `burst_default_pick` | `BurstDefaultPick` | логическое | `photoinfo.py:1021-1029`; `_constants.py:678` |
| `burst_key` | `BurstKey` | логическое | `photoinfo.py:999-1007` |
| `burst_photos` | другие фото серии в `Library.Photos` | У кадра с `Burst`: UUID других фото с `Burst` и тем же `BurstUUID` (в том числе пустым: upstream группирует по «не None»), выбранных и нет, в корзине и нет, фото и нет; отсортированные; без самого кадра. Без `Burst`: `[]` | `photosdb/photosdb.py:2212-2220`; `photoinfo.py:1031-1044` |
| `burst_selected` | `BurstSelected` | логическое | `photoinfo.py:989-997` |
| `cloud_guid` | `CloudMasterGUID` | `null` = `""` | `photoinfo.py:1762-1765` |
| `cloud_owner_hashed_id` | `CloudOwnerHashedID` | `null` = `""` | `photoinfo.py:1767-1770`; `photosdb/photosdb.py:2320` |
| `date` | `Date` | момент (правило 2) | `photosdb/photosdb.py:2159-2167`; `photos_datetime.py:27-67` |
| `date_added` | `DateAdded` | момент; эталон 1970-01-01T00:00:00 (умолчание upstream) = `nil` или этот момент | `photosdb/photosdb.py:2317`; `photos_datetime.py:19-21, 77-78`; `photoinfo.py:849-855` |
| `date_modified` | `DateModified` | момент; `null` = `nil` | `photosdb/photosdb.py:2156-2157`; `photoinfo.py:165-177` |
| `date_trashed` | `DateTrashed` | момент; `null` = `nil` | `photosdb/photosdb.py:2292-2293`; `photoinfo.py:842-847` |
| `description` | `Description` | `null` = `""` | `photoinfo.py:592-595` |
| `external_edit` | `ExternalEdit` | логическое | `photoinfo.py:807-810`; `photosdb/photosdb.py:2423-2439` |
| `favorite` | `Favorite` | логическое | `photoinfo.py:812-815` |
| `filename` | `Filename` | строгая строка | `photoinfo.py:114-140` |
| `fingerprint` | `Fingerprint` | `null` = `""` | `photoinfo.py:1772-1775` |
| `folders` | альбомы `album_info` | Словарь «название → `folder_names`» после правила 8: названия те же; при нескольких альбомах Go с одним названием значение эталона должно совпасть с `FolderNames` одного из них. Альбом без названия — ключ `"null"` (`json.dumps` пишет ключ `None` как `null`; смесь `None` и строк в ключах не сортируется, поэтому в эталоне тогда только этот ключ) | `photoinfo.py:2125, 2286-2288` |
| `has_raw` | `HasRAW` | логическое | `photoinfo.py:1363-1366` |
| `hasadjustments` | `HasAdjustments` | логическое | `photoinfo.py:738-741` |
| `hdr` | `HDR` | логическое | `photoinfo.py:1291-1294`; `photosdb/photosdb.py:2248-2249` |
| `height` | `Height` | целое, `null` = 0 | `photoinfo.py:1380-1383` |
| `hidden` | `Hidden` | логическое | `photoinfo.py:827-830` |
| `imported_by` | `[ImportedByName, ImportedByBundleID]` | пара, порядок значим, каждый `null` = `""` | `photoinfo.py:682-694`; `photosdb/photosdb.py:2070-2075, 2324-2325` |
| `incloud` | `InCloud` | `null` = `nil`; `false` — не `null` | `photoinfo.py:958-964` |
| `intrash` | `InTrash` | логическое | `photoinfo.py:837-840` |
| `iscloudasset` | `IsCloudAsset` | логическое | `photoinfo.py:966-977` |
| `ismissing` | `IsMissing` | логическое | `photoinfo.py:724-736` |
| `ismovie` | `IsMovie` | логическое | `photoinfo.py:948-951` |
| `isphoto` | `IsPhoto` | логическое | `photoinfo.py:953-956` |
| `israw` | `IsRAW` | логическое | `photoinfo.py:1368-1371` |
| `isreference` | `IsReference` | логическое | `photoinfo.py:979-982` |
| `keywords` | `Keywords` | список в порядке эталона, повторы сохраняются; список Go сравнение не сортирует | `photoinfo.py:705-708` |
| `latitude` | `Latitude` | `null` = `nil`; точно | `photosdb/photosdb.py:2174-2181` |
| `live_photo` | `Live` | логическое | `photoinfo.py:1046-1049` |
| `location` | `[Latitude, Longitude]` | пара, порядок значим, каждый `null` = `nil`; точно | `photoinfo.py:857-860` |
| `longitude` | `Longitude` | `null` = `nil`; точно | `photosdb/photosdb.py:2174-2181` |
| `orientation` | `Orientation` | целое, `null` = 0; **сравнивается, только если `hasadjustments` эталона `false`**: для правленого фото upstream читает plist правок, которого в тестовых медиатеках нет, и даёт 0 | `photoinfo.py:1390-1404` |
| `original_filename` | `OriginalFilename` | строгая строка | `photoinfo.py:114-140` |
| `original_filesize` | `OriginalFilesize` | целое, `null` = 0 | `photoinfo.py:1421-1424` |
| `original_height` | `OriginalHeight` | целое, `null` = 0 | `photoinfo.py:1406-1409` |
| `original_orientation` | `OriginalOrientation` | целое, `null` = 0 | `photoinfo.py:1416-1419` |
| `original_width` | `OriginalWidth` | целое, `null` = 0 | `photoinfo.py:1411-1414` |
| `panorama` | `Panorama` | логическое | `photoinfo.py:1276-1279`; `photosdb/photosdb.py:2257-2258` |
| `persons` | `Persons` | список в порядке эталона, имя на каждое лицо, повторы сохраняются; человек без имени — `_UNKNOWN_` с обеих сторон; список Go сравнение не сортирует | `photoinfo.py:597-603`; `photosdb/photosdb.py:1790-1796, 1844-1869`; `_constants.py:337` |
| `portrait` | `Portrait` | логическое | `photoinfo.py:1306-1309`; `photosdb/photosdb.py:2250-2251` |
| `raw_original` | `RawOriginal` | логическое | `photoinfo.py:1373-1378` |
| `saved_to_library` | `SavedToLibrary` | логическое | `photoinfo.py:1586-1599` |
| `screen_recording` | `ScreenRecording` | логическое | `photoinfo.py:1301-1304`; `photosdb/photosdb.py:2237` |
| `screenshot` | `Screenshot` | логическое | `photoinfo.py:1296-1299` |
| `selfie` | `Selfie` | логическое | `photoinfo.py:1322-1325`; `photosdb/photosdb.py:2260-2261` |
| `shared` | `Shared` | логическое | `photoinfo.py:872-879` |
| `shared_library` | `SharedLibrary` | логическое | `photoinfo.py:1628-1632` |
| `shared_moment` | `SharedMoment` | логическое | `photoinfo.py:1601-1604`; `photosdb/photosdb.py:2322` |
| `slow_mo` | `SlowMo` | логическое | `photoinfo.py:1281-1284`; `photosdb/photosdb.py:2239` |
| `spatial` | `Spatial` | целое, `null` = 0; 0 у медиатеки без `ZASSET.ZSPATIALTYPE` | `photoinfo.py:1311-1320`; `photosdb/photosdb.py:1751-1759, 2253-2255` |
| `syndicated` | `Syndicated` | логическое | `photoinfo.py:1569-1584` |
| `time_lapse` | `TimeLapse` | логическое | `photoinfo.py:1286-1289`; `photosdb/photosdb.py:2240` |
| `title` | `Title` | `null` = `""` (upstream превращает `""` в `None`) | `photoinfo.py:710-717` |
| `tzname` | `TZName` | `null` = `""` | `photoinfo.py:184-187`; `photosdb/photosdb.py:2160` |
| `tzoffset` | `TZOffset` | целое; `null` = `nil`; `0` — не `null` | `photoinfo.py:179-182`; `photosdb/photosdb.py:2159` |
| `uti` | `UTI` | строгая строка | `photoinfo.py:881-921` |
| `uti_edited` | `UTIEdited` | `null` = `""` | `photoinfo.py:923-932` |
| `uti_original` | `UTIOriginal` | строгая строка | `photoinfo.py:881-921` |
| `uti_raw` | `UTIRaw` | `null` = `""` | `photoinfo.py:934-946` |
| `uuid` | `UUID` | множества правила 1 | `photosdb/photosdb.py:2197-2204, 3346-3360` |
| `visible` | `Visible` | логическое | `photoinfo.py:832-835`; `photosdb/photosdb.py:2304-2307` |
| `width` | `Width` | целое, `null` = 0 | `photoinfo.py:1385-1388` |

### divergences.json

JSON-массив записей; каждая — известное расхождение между osxphotos и этим модулем с причиной. Форма записи — в примере английского раздела.

- `library` — имя каталога `testdata/libraries`, `synthetic` или `local` (прогон на медиатеке того, кто запускает инструмент); `uuid` — UUID объекта или `*` (все объекты медиатеки); `field` — ключ `fields.txt`; `reason` — непусто, по-английски; `upstream` — `путь.py:строка` или `путь.py:первая-последняя` от пакета `osxphotos/`.
- `want` и `got` — нормализованные значения сторон в любом виде JSON; сравниваются они в компактном виде с отсортированными ключами, как их пишет отчёт. У записи с UUID они необязательны, у `*` **обязательны**: `*` без значений спрятала бы весь ключ медиатеки. Запись объясняет расхождение, если совпали `library`, `field` и `uuid` (или `*`) и, где заданы, `want` и `got`. `"want": null` — значение; отсутствующий `want` подходит к любому.
- `local` — только с `uuid: "*"` и значениями `true`, `false`, `null` или числом: файл публичный, в нём нет UUID и строк медиатеки владельца.
- Две записи не могут совпадать по медиатеке, uuid, полю, `want` и `got`.
- **Устаревшая запись** — запись сравниваемой медиатеки, поле которой сравнивалось (или это `uuid`) и которая ничего не объяснила; тест красный. **Новое расхождение** без записи тоже красное. Записи других медиатек и ключей, которые не сравнивались, устаревшими не считаются.
- Предпочтительна запись по UUID. `*` — для одной причины и одной пары значений у многих объектов (больше десяти); её добавляют только вместе с тестом, который прямым SQL только для чтения находит множество объектов-кандидатов и проверяет, что UUID всех расхождений ключа входят в него, а их число ожидаемое. Без этого `*` спрятала бы ошибки порта в том же ключе.

### Порядок эталона

Объекты `query.json` и `deleted.json` идут в порядке osxphotos: по `date`, затем `date_added` (`null` — 1970-01-01T00:00:00Z), затем `uuid`, по возрастанию; даты как моменты, `uuid` побайтно (`photoquery.py:888-898`). Тест проверяет порядок у каждой медиатеки; порядок сохраняет инструмент, который пересоздаёт файлы.
