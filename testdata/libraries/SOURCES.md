# Test libraries

**English** | [Русский](#test-libraries-ru)

Databases of the test Photos libraries of [osxphotos](https://github.com/RhetTbull/osxphotos) v0.77.2 (MIT, Copyright (c) 2019-2026 Rhet Turnbull), copied unchanged from `tests/<name>.photoslibrary/database/` of that tag. Only `Photos.sqlite` and, where it is not empty, `Photos.sqlite-wal` are kept: the expected values in the upstream tests were computed on exactly this state. Images and other files of the libraries are not copied.

The databases live outside the `osxphotos` module directory, so they are not part of the module zip that consumers download. Tests of the module read them by relative path from the repository.

| Directory | Upstream library | Model | Schema | macOS | `-wal` | Upstream test |
|---|---|---|---|---|---|---|
| `Test-13.0.0` | `tests/Test-13.0.0.photoslibrary` | 16320 | photos8 | 13.0 | no | `tests/test_ventura_13_0_0.py` |
| `Test-Cloud-13.1` | `tests/Test-Cloud-13.1.photoslibrary` | 16320 | photos8 | 13.1, iCloud and shared library | yes | `tests/test_shared_ventura_13_1.py` |
| `Test-14.0.0` | `tests/Test-14.0.0.photoslibrary` | 17120 | photos9 | 14.0 | no | `tests/test_sonoma_14_0_0.py` |
| `Test-14.6.0` | `tests/Test-14.6.0.photoslibrary` | 17600 | photos9_14_6 | 14.6 | yes | `tests/test_sonoma_14_6_0.py` |
| `Test-15.4.1` | `tests/Test-15.4.1.photoslibrary` | 18508 | photos10 | 15.4 | yes | `tests/test_sequoia_15_4_1.py` |
| `Test-16.0.0-beta` | `tests/Test-16.0.0-beta.photoslibrary` | 19063 | photos11 | 26.0 beta | yes | `tests/test_tahoe_16_0_0_beta.py` |
| `Test-26.1` | `tests/Test-26.1.photoslibrary` | 19320 | photos11_1 | 26.1 | no | `tests/test_tahoe_26_1.py` |
| `Test-27.0_DevBeta` | `tests/Test-27.0_DevBeta.photoslibrary` | 270008501 | photos12 | 27.0 developer beta | yes | `tests/test_gg_dev_beta_27_0.py` |

To add a library: take the database from the same upstream tag, put it in `<name>/database/` (no `.photoslibrary` suffix, so that `.gitignore` lets it through), and add a row here.

## Migrations

The module reads more from `Photos.sqlite` than osxphotos does (the fields marked `// extension: not in osxphotos`), and some states that these fields need are missing from the libraries (see the cases below). osxphotos gives no reference for them, so each such case is an SQL migration:

```text
<name>/database/Photos.sqlite        unchanged upstream database
<name>/migrations/NNN_<case>.sql     applied in file-name order to a temporary copy
```

- The databases themselves are never modified: the test helper `osxphotos/internal/testlib` copies `Photos.sqlite` and `Photos.sqlite-wal` into a temporary directory, applies all migrations of the library in one transaction and hands the copy to the test. Reference (golden) tests use the untouched databases.
- Any `UPDATE` of `ZASSET` fires triggers of `Photos.sqlite` (R*Tree and functions that exist only inside the Photos app, Core Data). The helper registers stand-ins for the functions that do nothing, so a migration that fires such a trigger (for example, moving an asset to the trash) runs on the copy.
- A migration contains only `UPDATE` and `DELETE`, no primary keys or UUIDs of the database: a row is chosen by a rule (the first eligible asset in `ZUUID` order that no earlier migration has marked) and marked with a title `mig:<case>` (`ZADDITIONALASSETATTRIBUTES.ZTITLE` for assets, `ZGENERICALBUM.ZTITLE` for albums and folders); tests find rows by that title. One exception: `skipped-asset` deletes the `ZADDITIONALASSETATTRIBUTES` row of its asset, and the title goes with it, so that asset is marked in `ZASSET` instead, before the row is deleted: `ZFILENAME = 'mig-skipped-asset.jpg'`. Every statement must change at least one row, otherwise the helper fails the test (a statement that may legitimately match nothing is preceded by the line `-- rows: any`).
- All values are synthetic: identifiers carry the prefix `TEST-` (`TEST-GUID-0001`), dates are round values (`694224000` = 2023-01-01T00:00:00Z in Core Data seconds). Nothing from a real library is used.
- The `Z_METADATA` table is not touched, so the model version and the schema of the copy are those of the upstream library.

File format: a header comment (`--`, in English: the case, what it changes, what the test expects); statements separated by a line ending in `;`; comments only as separate lines starting with `--`; no `;` inside string literals. The markers a library has (titles, and the file name of `skipped-asset`) are listed in the test of the helper, which fails if a marker appears or disappears unnoticed.

Cases (the expected values are in the tests of the package that reads the field):

| Case | What it covers | Libraries |
|---|---|---|
| `asset-cloud-guid` | `CloudAssetGUID`, `IsCloudAsset`; two assets with one GUID are both returned (a duplicate is an error of the application, not of the library) | `Test-13.0.0`, `Test-26.1` |
| `asset-cloud-guid-absent` | `NULL` and an empty string in an iCloud library both mean "no GUID" (an intentional difference from osxphotos) | `Test-Cloud-13.1` |
| `album-cloud-guid` | `AlbumInfo.CloudGUID`, `FolderInfo.CloudGUID` | `Test-13.0.0`, `Test-26.1` |
| `album-cloud-guid-null` | `NULL` album `CloudGUID` in an iCloud library means "no GUID" | `Test-Cloud-13.1` |
| `cloud-master` | `InCloud` is `false` (master not in iCloud) or `nil` (no master), `CloudMasterGUID` | `Test-Cloud-13.1` |
| `fingerprint-schema` | `FingerprintSchema` for the two fingerprint columns (`ZMASTERFINGERPRINT`, `ZORIGINALSTABLEHASH`); empty fingerprint | `Test-13.0.0`, `Test-26.1`, `Test-27.0_DevBeta` |
| `resources` | `Resources`, `IsMissing`; `HasRAW` only in `Test-14.0.0` (`Test-26.1` has no RAW resource) | `Test-14.0.0`, `Test-26.1` |
| `shared-library` | `SharedLibrary`, `LibraryScopeShareState`, `LibraryScope` | `Test-14.6.0` |
| `attrs-null` | `NULL` in the time zone offset, original file name and creation date | `Test-14.6.0` |
| `date-modified-raw` | `DateModifiedRaw` apart from `DateModified` | `Test-16.0.0-beta` |
| `burst-frames` | every burst pick type, `BurstUnselected()`, `BurstAlbumUUIDs`, `BurstSelectedOnly` | `Test-13.0.0`, `Test-26.1` |
| `trash` | a trashed asset is returned when no filters are set | `Test-14.0.0` |
| `syndication-shared` | `Syndicated`, `SavedToLibrary`, `Shared` | `Test-15.4.1` |
| `skipped-asset` | `Info.SkippedAssets`: an asset without a row in `ZADDITIONALASSETATTRIBUTES` is absent from `Photos()` and counted (marked by `ZASSET.ZFILENAME`, see above) | `Test-16.0.0-beta` |

Planned with the album tests: `albums-edge` in `Test-15.4.1` (two albums with one title in one folder, an empty album, a trashed album).

Files per library, in the order of application:

| Library | `migrations/` |
|---|---|
| `Test-13.0.0` | `001_asset-cloud-guid`, `002_album-cloud-guid`, `003_fingerprint-schema`, `004_burst-frames` |
| `Test-Cloud-13.1` | `001_asset-cloud-guid-absent`, `002_cloud-master`, `003_album-cloud-guid-null` |
| `Test-14.0.0` | `001_resources`, `002_trash` |
| `Test-14.6.0` | `001_shared-library`, `002_attrs-null` |
| `Test-15.4.1` | `001_syndication-shared` |
| `Test-16.0.0-beta` | `001_date-modified-raw`, `002_skipped-asset` |
| `Test-26.1` | `001_asset-cloud-guid`, `002_album-cloud-guid`, `003_fingerprint-schema`, `004_burst-frames`, `005_resources` |
| `Test-27.0_DevBeta` | `001_fingerprint-schema` |

To add a case: a new file `NNN_<case>.sql` (lowercase letters, digits and hyphens in `<case>`) with the next number in the library whose schema reads the field in question, a title marker `mig:<case>` (with a suffix when the case has several rows), synthetic values, and the expected values in the test of the package that reads the field; then add the case to the tables above and the marker to the list in the helper's test. A migration contains no `INSERT`; if a state cannot be reached by changing existing rows, discuss it in the PR first.

<a id="test-libraries-ru"></a>

## Тестовые медиатеки (по-русски)

Базы тестовых медиатек [osxphotos](https://github.com/RhetTbull/osxphotos) v0.77.2 (MIT, Copyright (c) 2019-2026 Rhet Turnbull), скопированные без изменений из `tests/<имя>.photoslibrary/database/` этого тега. Хранятся только `Photos.sqlite` и, где он не пуст, `Photos.sqlite-wal`: ожидаемые значения в тестах upstream вычислены ровно на этом состоянии. Изображения и другие файлы медиатек не копируются.

Базы лежат вне каталога модуля `osxphotos`, поэтому не входят в zip модуля, который скачивают потребители. Тесты модуля читают их по относительному пути от репозитория. Таблица каталогов, моделей и схем — выше (она одна).

Чтобы добавить медиатеку: взять базу из того же тега upstream, положить в `<имя>/database/` (без суффикса `.photoslibrary`, иначе `.gitignore` её не пропустит) и добавить строку в таблицу.

### Миграции

Модуль читает из `Photos.sqlite` больше, чем osxphotos (поля, помеченные `// extension: not in osxphotos`), а некоторых состояний, нужных этим полям, в медиатеках нет (см. случаи ниже). Эталона от osxphotos для них нет, поэтому каждый такой случай — SQL-миграция:

```text
<имя>/database/Photos.sqlite        база upstream без изменений
<имя>/migrations/NNN_<случай>.sql   применяются по порядку имён к временной копии
```

- Сами базы никогда не меняются: тестовый помощник `osxphotos/internal/testlib` копирует `Photos.sqlite` и `Photos.sqlite-wal` во временный каталог, применяет все миграции медиатеки в одной транзакции и отдаёт копию тесту. Эталонные тесты работают на нетронутых базах.
- Любой `UPDATE` таблицы `ZASSET` запускает триггеры `Photos.sqlite` (R*Tree и функции, которые есть только внутри приложения «Фото», Core Data). Помощник регистрирует для функций заглушки, которые ничего не делают, поэтому миграция, запускающая такой триггер (например, перенос объекта в корзину), выполняется на копии.
- Миграция содержит только `UPDATE` и `DELETE`, без первичных ключей и UUID из базы: строка выбирается правилом (первый подходящий объект в порядке `ZUUID`, не помеченный предыдущей миграцией) и помечается названием `mig:<случай>` (`ZADDITIONALASSETATTRIBUTES.ZTITLE` у объектов, `ZGENERICALBUM.ZTITLE` у альбомов и папок); тесты находят строки по этому названию. Одно исключение: `skipped-asset` удаляет строку `ZADDITIONALASSETATTRIBUTES` своего объекта, и название исчезает вместе с ней, поэтому этот объект помечается в `ZASSET` до удаления строки: `ZFILENAME = 'mig-skipped-asset.jpg'`. Каждая инструкция обязана изменить хотя бы одну строку, иначе помощник валит тест (перед инструкцией, которой законно не на чем сработать, стоит строка `-- rows: any`).
- Все значения синтетические: идентификаторы с префиксом `TEST-` (`TEST-GUID-0001`), даты круглые (`694224000` = 2023-01-01T00:00:00Z в секундах Core Data). Ничего из реальной медиатеки не используется.
- Таблица `Z_METADATA` не трогается: номер модели и схема копии — те же, что у медиатеки upstream.

Формат файла: заголовок-комментарий (`--`, на английском: случай, что меняет, что ожидает тест); инструкции разделены строкой, оканчивающейся на `;`; комментарии только отдельными строками, начинающимися с `--`; `;` внутри строковых литералов нет. Какие маркеры есть у медиатеки (названия и имя файла у `skipped-asset`), перечислено в тесте помощника: он падает, если маркер появился или пропал незаметно.

Случаи (ожидаемые значения — в тестах пакета, который читает поле):

| Случай | Что покрывает | Медиатеки |
|---|---|---|
| `asset-cloud-guid` | `CloudAssetGUID`, `IsCloudAsset`; два объекта с одним GUID возвращаются оба (дубликат — ошибка приложения, не библиотеки) | `Test-13.0.0`, `Test-26.1` |
| `asset-cloud-guid-absent` | `NULL` и пустая строка в облачной медиатеке означают «GUID нет» (намеренное отличие от osxphotos) | `Test-Cloud-13.1` |
| `album-cloud-guid` | `AlbumInfo.CloudGUID`, `FolderInfo.CloudGUID` | `Test-13.0.0`, `Test-26.1` |
| `album-cloud-guid-null` | `NULL` в `CloudGUID` альбома в облачной медиатеке означает «GUID нет» | `Test-Cloud-13.1` |
| `cloud-master` | `InCloud` равно `false` (мастер не в iCloud) или `nil` (мастера нет), `CloudMasterGUID` | `Test-Cloud-13.1` |
| `fingerprint-schema` | `FingerprintSchema` для двух колонок отпечатка (`ZMASTERFINGERPRINT`, `ZORIGINALSTABLEHASH`); пустой отпечаток | `Test-13.0.0`, `Test-26.1`, `Test-27.0_DevBeta` |
| `resources` | `Resources`, `IsMissing`; `HasRAW` — только в `Test-14.0.0` (в `Test-26.1` нет RAW-ресурса) | `Test-14.0.0`, `Test-26.1` |
| `shared-library` | `SharedLibrary`, `LibraryScopeShareState`, `LibraryScope` | `Test-14.6.0` |
| `attrs-null` | `NULL` в смещении пояса, исходном имени файла и дате создания | `Test-14.6.0` |
| `date-modified-raw` | `DateModifiedRaw` отдельно от `DateModified` | `Test-16.0.0-beta` |
| `burst-frames` | все виды отметок серий, `BurstUnselected()`, `BurstAlbumUUIDs`, `BurstSelectedOnly` | `Test-13.0.0`, `Test-26.1` |
| `trash` | объект в корзине возвращается, когда фильтров нет | `Test-14.0.0` |
| `syndication-shared` | `Syndicated`, `SavedToLibrary`, `Shared` | `Test-15.4.1` |
| `skipped-asset` | `Info.SkippedAssets`: объект без строки в `ZADDITIONALASSETATTRIBUTES` не попадает в `Photos()` и считается (помечен `ZASSET.ZFILENAME`, см. выше) | `Test-16.0.0-beta` |

Запланировано вместе с тестами альбомов: `albums-edge` в `Test-15.4.1` (два альбома с одним названием в одной папке, пустой альбом, альбом в корзине).

Файлы по медиатекам, в порядке применения:

| Медиатека | `migrations/` |
|---|---|
| `Test-13.0.0` | `001_asset-cloud-guid`, `002_album-cloud-guid`, `003_fingerprint-schema`, `004_burst-frames` |
| `Test-Cloud-13.1` | `001_asset-cloud-guid-absent`, `002_cloud-master`, `003_album-cloud-guid-null` |
| `Test-14.0.0` | `001_resources`, `002_trash` |
| `Test-14.6.0` | `001_shared-library`, `002_attrs-null` |
| `Test-15.4.1` | `001_syndication-shared` |
| `Test-16.0.0-beta` | `001_date-modified-raw`, `002_skipped-asset` |
| `Test-26.1` | `001_asset-cloud-guid`, `002_album-cloud-guid`, `003_fingerprint-schema`, `004_burst-frames`, `005_resources` |
| `Test-27.0_DevBeta` | `001_fingerprint-schema` |

Чтобы добавить случай: новый файл `NNN_<случай>.sql` (в `<случай>` строчные буквы, цифры и дефисы) со следующим номером в медиатеке той схемы, где это поле читается, маркер-название `mig:<случай>` (с суффиксом, если у случая несколько строк), синтетические значения и ожидаемые значения в тесте пакета, который читает поле; затем внести случай в таблицы выше, а маркер — в список в тесте помощника. `INSERT` в миграции нет; если состояние нельзя получить правкой существующих строк, сначала обсудить это в PR.
