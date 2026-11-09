# osxphotos (Go)

[English](README.en.md) | **Русский**

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

`github.com/Windemiatrix/go-libs/osxphotos` — библиотека на Go для медиатеки «Фото» на macOS: читает каталог, выводит шаблоны путей osxphotos, называет файлы экспорта, а через PhotoKit выгружает, импортирует и меняет объекты. Неофициальный порт части [osxphotos](https://github.com/RhetTbull/osxphotos) Rhet Turnbull (MIT): автор osxphotos к этому проекту отношения не имеет, вся благодарность за разбор формата базы — ему ([NOTICE](NOTICE)).

## Статус

**Ранняя разработка.** Выпуски — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases); до `v1.0.0` API может меняться. Работают: чтение каталога (`photosdb.Open`) — фото, альбомы, папки, люди и лица, места, EXIF, оценки, поисковые метки; фильтры выборки и поиск дублей; шаблоны путей (`template`); имена файлов экспорта (`export`); PhotoKit (`photokit`, только macOS). Подробности — в разделе [«Возможности»](#возможности).

## Быстрый старт

Нужен Go 1.26+. Все пакеты, кроме `photokit`, — чистый Go без cgo; они проверяются в CI на Linux и macOS. `photokit` работает только на macOS в сборке с cgo, в остальных сборках каждый его вызов возвращает `photokit.ErrNotSupported`. На Linux модуль читает копию каталога `.photoslibrary`; системная медиатека лежит на Mac (права — в разделе [«Безопасность ваших фото»](#безопасность-ваших-фото)).

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

```go
lib, err := photosdb.SystemLibraryPath()
if err != nil {
	log.Fatal(err) // errors.Is(err, photosdb.ErrLibraryNotFound)
}
db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
if err != nil {
	log.Fatal(err) // errors.Is(err, photosdb.ErrUnsupportedSchema): незнакомая версия «Фото»
}
defer db.Close()

for _, p := range db.Photos() {
	if p.InTrash { // Photos() возвращает и фото из корзины
		continue
	}
	fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
}
```

## Зачем, если есть osxphotos

osxphotos — самый полный открытый разбор базы «Фото», но это Python и командная строка. Этот модуль нужен, если вы пишете на Go:

- один бинарник без Python и `pip`;
- весь каталог в памяти, обычные структуры Go;
- стабильные идентификаторы фото и альбомов, в том числе iCloud, чтобы узнавать их между запусками;
- незнакомая версия базы — ошибка, а не тихо неверные данные.

Что можно построить: раскладку файлов по шаблону вроде `2024/05-01 - Holidays/IMG_0001.HEIC`, отчёт о медиатеке, отчёт о дублях.

Модуль писался для [photosync](https://github.com/Windemiatrix/photosync) — утилиты синхронизации «Фото» с Яндекс Диском (в разработке), но о синхронизации ничего не знает.

## Версии macOS

Схема «Фото» определяется по номеру модели базы.

| macOS | «Фото» | Модель базы | Тестовая медиатека osxphotos |
|---|---|---|---|
| 13 | 8 | 16320–16999 | 13.0, 13.1 с iCloud |
| 14.0–14.5 | 9 | 17000–17599 | 14.0 |
| 14.6+ | 9.6 | 17600–17999 | 14.6 |
| 15 | 10 | 18201–18999 | 15.4 |
| 26.0 | 11 | 19063–19319 | 26.0 beta |
| 26.1+ | 11.1 | 19320–19999 | 26.1 |
| 27 | 12 | 270000000–270999999 | 27.0 developer beta |

Другие модели, включая бета-сборки вне диапазонов, дают `ErrUnsupportedSchema`. Поддержку новой macOS добавляем только по медиатеке этой версии: [заведите issue](https://github.com/Windemiatrix/go-libs/issues/new?template=new-macos.yml).

## Безопасность ваших фото

- **База медиатеки не меняется.** Читается только копия `Photos.sqlite` вместе с `-wal` (без `-shm`: SQLite восстанавливает его сам), в режиме `mode=ro`, и так же — копия базы поиска (`search/psi.sqlite` или `search/leo.sqlite`), если она есть. Копия создаётся во временном каталоге с правами `0700` (файлы `0600`) и удаляется до возврата из `Open`, в том числе после ошибки; каталог внутри медиатеки отклоняется.
- **Менять «Фото» умеет только `photokit`** (macOS, cgo), и только если вы его импортируете, macOS выдала право «Фото» и вы вызвали меняющий метод: `Import`, методы папок и альбомов, `Delete`, `Update`. Остальные пакеты писать в медиатеку не умеют: `photosdb` читает копию базы и файлы правок, а `photoinfo`, `albuminfo`, `adjustmentsinfo`, `query`, `template`, `export`, `exifinfo`, `placeinfo`, `personinfo`, `momentinfo`, `scoreinfo`, `searchinfo` ввода-вывода не делают.
- **Удаление и скрытие решает пользователь.** `Delete` и `Update` с `Hidden: true` спрашивают через диалог macOS, `DeleteCollections` может спросить; если `ctx` закончился раньше ответа (`ErrOutcomeUnknown`), объект ещё может измениться — подробности в разделе [«PhotoKit»](#photokit).
- **Сеть — только через PhotoKit.** Модуль сам не обращается к сети и не запускает процессов — это проверяет линтер (правило `library` в [.golangci.yml](https://github.com/Windemiatrix/go-libs/blob/main/.golangci.yml)). Телеметрии нет. Выгрузка через `photokit` разрешает PhotoKit скачать из iCloud оригинал, которого нет на диске.
- **Права macOS.** Медиатека в `~/Pictures` защищена системой: программе, которая её читает через `photosdb` (например, Терминалу), нужен «Полный доступ к диску» в «Системных настройках → Конфиденциальность и безопасность». Для `photokit` нужно право «Фото»; кто его получает — в разделе [«PhotoKit»](#photokit).
- **Копия — это ваши данные:** места, имена людей, подписи, комментарии общих альбомов. Если задаёте `Options.TempDir`, выберите каталог, доступный только вам. Байты `DB.JSON`, а также `Metadata` и `Path` из `photokit.ImageInfo` содержат пути и личные данные: не пишите их в журналы.
- **Не прикладывайте `Photos.sqlite` к issue:** в нём названия альбомов, пути и идентификаторы. Хватит версии macOS и номера модели базы. Об уязвимостях — приватно, по [SECURITY.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.md).

## Возможности

| Возможность | Пакет |
|---|---|
| Определение схемы, macOS 13–27 | `photosdb` |
| Путь системной медиатеки (`SystemLibraryPath`) | `photosdb` |
| Открытие медиатеки (`Open`): каталог `.photoslibrary` или файл базы, в том числе вне медиатеки (`Options.LibraryPath` — где лежат оригиналы); путь базы и версия «Фото» (`Info.DBPath`, `Info.PhotosVersion`) | `photosdb` |
| Фото: даты с поясом и его имя, размеры, ключевые слова, Live, RAW+JPEG, серии, iCloud, общая медиатека | `photosdb`, `photoinfo` |
| Признаки фото: HDR, панорама, портрет, селфи, пространственное фото, замедленная съёмка, таймлапс, запись экрана, общий момент, правка в другом приложении, видимость, кто импортировал | `photosdb`, `photoinfo` |
| Файлы на диске: оригинал, Live, RAW, правленая версия и её Live-видео, файлы правок, превью (`PathEdited`, `PathEditedLive`, `AdjustmentsPath`, `OriginalAdjustmentsPath`, `PathDerivatives`); данные правок — `Adjustments`, `OriginalAdjustments` | `photosdb`, `photoinfo`, `adjustmentsinfo` |
| Люди на фото: имена (`Persons`; лицо без имени — `_UNKNOWN_`), по медиатеке — `DB.Persons`, `DB.PersonCounts` | `photosdb`, `photoinfo` |
| Люди и лица: `DB.PersonInfo`, `Person`, `PersonInfoOf`, `PersonPhotos`; лица с положением и размером — `FaceInfoOf`, `PersonFaces` | `photosdb`, `personinfo` |
| EXIF, который «Фото» сохраняют в базе при импорте (`PhotoInfo.ExifInfo`) | `photosdb`, `photoinfo`, `exifinfo` |
| Место: названия и адрес (`PhotoInfo.Place`) | `photosdb`, `photoinfo`, `placeinfo` |
| Оценки снимка (`PhotoInfo.Score`): каждая — `*float64`, nil — значения нет | `photosdb`, `photoinfo`, `scoreinfo` |
| Моменты (`DB.MomentInfoOf`, `MomentPhotos`) | `photosdb`, `momentinfo` |
| Поисковые данные «Фото» и метки (`DB.SearchInfoOf`, `Labels`, `LabelsNormalized`, `LabelCounts`, `LabelCountsNormalized`) | `photosdb`, `searchinfo` |
| Облачные метаданные снимка (`DB.CloudMetadataOf`) | `photosdb` |
| Фото как словарь и JSON osxphotos (`DB.AsDict`, `DB.JSON`, `DB.Hexdigest`) | `photosdb` |
| Ключевые слова медиатеки, их счёт и слова без фото (`Keywords`, `KeywordCounts`, `KeywordsWithoutPhoto`) | `photosdb` |
| Альбомы: название, путь папок, состав, даты, корзина, порядок сортировки и место в боковой панели (`AlbumInfo`, `Album`; фото в порядке альбома и место фото в нём — `AlbumPhotos`, `PhotoIndex`); общие альбомы — `SharedAlbumInfo`, `SharedAlbumTitles`, владелец — `Owner` | `photosdb`, `albuminfo` |
| Сессии импорта: название, даты, фото, сессия каждого фото (`ImportInfo`, `Import`, `ImportUUID`) | `photosdb`, `albuminfo` |
| Проекты («Мои проекты»): название, даты, фото, проекты каждого фото (`ProjectInfo`, `Project`, `ProjectInfoOf`, `ProjectUUIDs`) | `photosdb`, `albuminfo` |
| Общий момент и общая медиатека фото (`SharedMomentInfo`, `ShareInfo`), её участники (`ShareParticipantInfo`, `ShareParticipants`) | `photosdb`, `photoinfo` |
| Комментарии и отметки «Нравится» фото общего альбома (`Comments`, `Likes`) | `photosdb`, `photoinfo` |
| Папки и их дерево, место в боковой панели (`FolderInfo`, `Folder`, `FolderList`) | `photosdb`, `albuminfo` |
| Выборка `Query` с фильтрами `query.Options` (альбом, папка, даты, корзина, тип, признаки, человек, метка `Label`, регулярное выражение по шаблону `Regex` и другие), сортировка; выборка с аргументами `photos()` — `PhotosWith` | `photosdb`, `query` |
| Дубликаты (`DB.Duplicates`, `query.Duplicates`, опция `Duplicate`) | `photosdb`, `query` |
| Шаблоны путей: разбор (`template.Parse`), вывод (`DB.RenderTemplate`, `Renderer.Render`), поля фото (`DB.Fields`), очистка имён (`SanitizeFilename`, `SanitizeDirname`) | `template`, `photosdb` |
| Имена файлов экспорта (`Names`, `EditedName`, `IncrementName`, `StemKey`) | `export` |
| PhotoKit: право доступа, поиск объектов, выгрузка ресурсов и версий, импорт, папки и альбомы, удаление, изменение, облачные идентификаторы | `photokit` |

**Чего нет:** командной строки; экспорта `osxphotos export` — от него перенесены только имена файлов (`export`), а файлы выгружает `photokit`; сайдкаров и exiftool; полей шаблона, которые запускают код или программы; результатов анализа медиа (`media_analysis`, `ai_caption`) — они лежат в базе другой программы; прямого доступа к таблицам базы (`PhotoTables`); ремонта баз; поддержки «Фото» 7 и более ранних версий. Для этого есть сам osxphotos.

## Шаблоны

Пакет `template` переносит грамматику языка шаблонов osxphotos целиком — поля, фильтры, условия, `[find,replace]`, `&`, `?` и значения по умолчанию; поля и значения, которых порт не выводит, перечислены ниже.

```go
t, err := template.Parse("{created.year}/{created.mm}-{created.dd} - {album,No album}/{photo.original_filename}")
if err != nil {
	log.Fatal(err) // errors.Is(err, template.ErrSyntax): ошибка называет смещение в байтах
}
for _, p := range db.Query(query.Options{}) {
	paths, unmatched, err := db.RenderTemplate(p, t, template.RenderOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(paths, unmatched) // [2024/05-01 - Holidays/IMG_0001.HEIC] []
}
```

Многозначное поле даёт строку на каждое значение: фото в двух альбомах — два пути. В `unmatched` попадают имена, которых нет в osxphotos. С `RenderOptions.Filename` результат — имя файла, с `RenderOptions.Dirname` — путь каталога: значения полей очищаются (`SanitizeFilename`, `SanitizeDirname`), но имя файла может оказаться `..`, а путь каталога — содержать `..` и `/` из текста шаблона, поэтому вызывающий сам держит путь внутри своего каталога.

`DB.RenderTemplate` читает поля через `DB.Fields(p)` — реализацию `template.Photo`. Интерфейс минимальный, один метод `Field(name)`: программа может реализовать его для своих данных и выводить шаблон через `template.NewRenderer().Render(t, photo, o)`.

| Группа | Поля |
|---|---|
| Текст и флаги | `name` и `original_name` (имя без расширения), `title`, `descr`, `uuid`, `shortuuid`, `id`, `hdr`, `edited`, `favorite`, `burst`, `media_type`, `photo_or_video`, `imported_by.name` |
| Даты | `created`, `modified`, `today` с частями `.year`, `.mm`, `.month`, `.dd`, `.dow`, `.hour` и другими; `.strftime,<формат>` |
| Многозначные | `album`, `folder_album`, `project`, `album_project`, `folder_album_project`, `keyword`, `person`, `comment`, `label`, `label_normalized` |
| Номер в альбоме | `album_seq`, `folder_album_seq` (по `RenderOptions.DestPath`) |
| Служебные | `{var:имя,значение}` и `%имя`, `counter`, `format`, `strip`, `shell_quote`, `export_dir`, `filepath`, `osxphotos_version`, `edited_version`, пунктуация (`comma`, `pipe`, `openbrace` и другие) |
| Свойства фото | `photo.X` — свойства `PhotoInfo` по именам osxphotos; вложенные `photo.score.X`, `photo.exif_info.X`, `photo.place.X`, `photo.search_info.X`, `photo.search_info_normalized.X`; `photo.hexdigest` |
| Поля модулей | `place.*` (например, `place.name`, `place.address.city`), `exif.camera_make`, `exif.camera_model`, `exif.lens_model`, `moment`, `searchinfo.season`, `searchinfo.holiday`, `searchinfo.activity`, `searchinfo.venue`, `searchinfo.venue_type` |

Полный список полей и синтаксис — в [phototemplate.md](https://github.com/RhetTbull/osxphotos/blob/v0.77.2/osxphotos/phototemplate.md) osxphotos, что даёт каждое поле фото — в godoc [`DB.Fields`](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos/photosdb#DB.Fields).

- **Не переносятся:** `{function}` и фильтр `function:` (код Python), `{exiftool}` (внешняя программа), `{detected_text}` (распознавание текста macOS), `{osxphotos_cmd_line}` (командная строка osxphotos). Библиотека не запускает чужой код и программы, поэтому такое поле — ошибка, которая оборачивает `template.ErrUnsupportedField`. Ту же ошибку даёт поле osxphotos, которого не даёт реализация `template.Photo`.
- **Пределы:** вывод больше `RenderOptions.MaxResults` строк (по умолчанию `DefaultMaxResults`, 10 000) или больше 16 МиБ строк — `ErrTooManyValues`. Многозначные поля перемножаются, а их значения приходят из медиатеки, в том числе от других участников общих альбомов.
- **Счётчики** `{counter}` принадлежат `template.Renderer`: у `DB` он один, и `{counter}` растёт от фото к фото, пока медиатека открыта. Начать заново — выводить своим `Renderer` (`NewRenderer`) с `DB.Fields(p)`; его счётчики сбрасывают `ResetCounters` и `ResetCounter`.
- **Имена месяцев и дней** — английские (локаль C), как в эталоне; osxphotos берёт локаль пользователя.
- **Ошибки вывода:** `ErrUnsupportedField`, `ErrTooManyValues`, `ErrUnsupportedConstruct` (например, директива `strftime` или спецификация формата, которых порт не знает), `ErrRender` (то, на чём osxphotos бросает исключение). Остальные отличия от osxphotos — в [UPSTREAM.md](UPSTREAM.md).

## Имена экспорта

Пакет `export` называет файлы так же, как `osxphotos export`, и сам файлов не пишет: каталог читает и файлы пишет вызывающий.

```go
taken := map[string]bool{} // export.StemKey(имя) каждого файла каталога
name, _ := export.IncrementName(p.OriginalFilename, 0, taken) // IMG_0001.HEIC или IMG_0001 (1).HEIC
files := export.Names(p, name)                                 // files.Live: IMG_0001.mov
```

- `IncrementName(name, count, taken)` добавляет ` (N)` перед расширением, пока основа имени занята; основы сравниваются без расширения и без учёта регистра, ключи `taken` даёт `StemKey`.
- `Names(p, name)` — спутники уже пронумерованного имени: Live-видео (`.mov`), RAW, превью, файлы правок AAE.
- `EditedName(p)` — имя правленой версии: `<основа>_edited` и расширение правленого файла.
- Имена приходят из медиатеки: очистите их перед записью файла.

## PhotoKit

Пакет `photokit` работает с системной медиатекой через PhotoKit — только на macOS в сборке с cgo; иначе каждый вызов возвращает `ErrNotSupported`.

**Право доступа.**

- Процесс, который обращается к PhotoKit, должен нести `Info.plist` с `NSPhotoLibraryUsageDescription`: без него macOS завершает процесс при первом запросе доступа. Библиотека его не встраивает — это делает программа, например флагом компоновщика `-sectcreate __TEXT __info_plist <файл>`, как тестовый бинарник модуля.
- `AuthorizationStatus` возвращает статус без диалога, `RequestAuthorization(ctx)` один раз спрашивает пользователя. `Open` не спрашивает: без права — `ErrNotAuthorized`; ограниченный доступ (`StatusLimited`) тоже отвергается, потому что ограниченная медиатека показывает только выбранные объекты.
- Право «Фото» после первого диалога получает Терминал учётной записи, и оно — доступ на чтение и изменение всей медиатеки — действует для всех программ, запущенных из этого Терминала; процесс, который запускает `launchd`, получает своё право. Поэтому тесты, меняющие медиатеку, запускаются только под отдельной учётной записью.

**Что даёт `Library`.**

- `Fetch`, `Resources` — объекты `Asset` и их ресурсы по UUID из `photosdb` или по `LocalIdentifier`; `FetchBurst` — кадры серии по её идентификатору.
- `Export` — один ресурс объекта (`ResourcePhoto`, `ResourcePairedVideo`, `ResourceAlternatePhoto` и другие) в том виде, в каком его хранит «Фото».
- `ExportImage`, `ExportVideo`, `ExportLivePhoto` — версия фото, видео или половины Live Photo (`VersionCurrent` с правками, `VersionUnadjusted`, `VersionOriginal`). Текущая версия замедленного видео рендерится заново, поэтому её байты могут различаться от вызова к вызову.
- `Import` — новый объект из файлов (`ImportRequest`: фото или видео, Live Photo, RAW+JPEG, альбомы).
- `Collections`, `CreateFolder`, `CreateAlbum`, `RenameCollection`, `DeleteCollections`, `AddToAlbum`, `RemoveFromAlbum` — папки и альбомы.
- `Delete` — перенос объектов в «Недавно удалённые»; `Update` (`AssetPatch`) — дата, избранное, скрытие, место.
- `CloudIdentifiers`, `LocalIdentifiers` — облачный идентификатор объекта и обратно.

**Файлы.** Выгрузка пишет в файл `name` каталога `dir` (на время вызова там же может появиться скрытый временный файл или каталог, до возврата он удаляется): `name` — один элемент пути, иначе `ErrInvalidName`. Файл создаётся с правами `0600`; существующий — `fs.ErrExist`, если не задан `ExportOptions.Overwrite`. После ошибки созданный файл удаляется, а существующий остаётся как был. Имена файлам даёт пакет `export`.

**Диалоги и срок.**

- `Delete` (один диалог на пачку) и `Update` с `Hidden: true` показывают диалог macOS, `DeleteCollections` может его показать; от импорта и остальных методов папок и альбомов диалога не ждут. Отказ пользователя — `ErrUserCanceled`, ничего не изменено.
- Срок задаёт только `ctx` (у `RequestAuthorization` без срока — 5 минут). `Fetch`, `FetchBurst`, `Resources`, `Collections`, `CloudIdentifiers` и `LocalIdentifiers` проверяют `ctx` только перед вызовом; выгрузка после конца `ctx` отменяется и возвращается не позже чем через 30 с. Если `ctx` закончился раньше, чем PhotoKit сообщил исход изменения, — `ErrOutcomeUnknown` вместе с ошибкой `ctx` (та же ошибка вместе с `context.DeadlineExceeded` приходит, если PhotoKit применил изменение, бросившее исключение): изменение ещё может произойти, перечитайте медиатеку перед следующим изменением.
- Скрытые объекты не находят `Fetch` и методы, которые ищут объект так же (выгрузка, `Delete`, `Update`, `AddToAlbum`), как и osxphotos; при заблокированном альбоме «Скрытые», как в «Фото» по умолчанию, macOS не отдаёт их и по особому запросу. Объект, скрытый через `Update`, больше не находят ни `Fetch`, ни `Update`.
- Работа `photokit` с общей медиатекой не проверялась.

Другие ошибки: `ErrNotFound`, `ErrMediaType`, `ErrNoResource`, `ErrInvalidVersion`, `ErrExport`, `ErrImport`, `ErrImportFile`, `ErrInvalidTitle`, `ErrChange`. Отличия от `photokit.py` — в [UPSTREAM.md](UPSTREAM.md).

## Чем отличается от osxphotos

Портируется тег `v0.77.2`. То же чтение базы плюс: облачные идентификаторы снимков и альбомов, размеры ресурсов Live и RAW без скачивания, строгая ошибка вместо догадки на неизвестной схеме, `Photos()` без скрытых фильтров (выборку osxphotos — фото и видео вне корзины без невыбранных кадров серий — делает `Query(query.Options{})`). В `photokit` сверх `photokit.py` — импорт, папки и альбомы, удаление, изменение объектов и облачные идентификаторы. Полный список расширений и намеренных отличий со ссылками на код osxphotos — в [UPSTREAM.md](UPSTREAM.md); в коде расширения помечены `// extension: not in osxphotos`.

## Как проверяется

Каждый PR проходит тесты с `-race` на Linux и macOS, линтер и порог покрытия 80 %. Вывод `Open` сверяется с эталоном osxphotos (v0.77.2): с `osxphotos query --json` (`TestGolden`), а вывод шаблонов — с `osxphotos query --field` по шаблонам `testdata/golden/templates.txt` (`TestTemplateGolden`). Эталон строит workflow `golden-regen` на 9 медиатеках `testdata/libraries`: 8 тестовых медиатеках osxphotos (7 схем, от macOS 13.0 до 27.0 developer beta) и синтетической базе; известные расхождения записаны с причинами. Поля сверх osxphotos проверяются SQL-миграциями с синтетическими значениями на временной копии базы ([SOURCES.md](https://github.com/Windemiatrix/go-libs/blob/main/testdata/libraries/SOURCES.md)). С помощью `make diff-local` (ниже) порт сверен с osxphotos и на медиатеке мейнтейнера, схема 11.1 (macOS 26.6): расхождений нет.

`photokit` проверяется в три яруса:

1. Go-слой — на фейке моста к PhotoKit, на Linux и macOS.
2. Сборка с cgo и вызовы без диалогов (`AuthorizationStatus`, `Open` без права) — в job `macos` CI.
3. Тесты с тегом `photokit_live`, которые меняют настоящую медиатеку, — только вручную под отдельной учётной записью. Они не стартуют без переменной `PHOTOSYNC_LIVE=1`, без маркерного альбома и в медиатеке больше предела объектов. CI их только компилирует и не запускает.

### Как проверить на своей медиатеке

1. Поставьте osxphotos 0.77.2 в отдельное окружение (нужен Python 3.10 или новее): `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Закройте «Фото», откройте пакет медиатеки в Finder («Показать содержимое пакета») и скопируйте папку `database/` в `~/Pictures/Copy.photoslibrary/database/`. Оригиналы снимков не нужны.
3. Из корня репозитория: `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`. Пути — абсолютные; `REPORT` необязателен.

Если проверка прошла без ошибок, в терминал выводятся только числа и имена ключей: сколько объектов и ключей сравнено, сколько расхождений. Сообщения об ошибках Go могут содержать переданные пути; сообщения osxphotos в терминал не выводятся, а если osxphotos упал, запустите его вручную, чтобы увидеть причину. В сводке есть число объектов вашей медиатеки: в issue приводите только строки по ключам. UUID и значения расхождений пишутся только в файл `REPORT` с правами 0600 (существующий файл перезаписывается): в нём данные вашей медиатеки, не публикуйте его. Вывод osxphotos и копия базы лежат во временном каталоге и удаляются после проверки, даже прерванной. После проверки удалите и копию `~/Pictures/Copy.photoslibrary`.

## Версии

Semver по тегам `osxphotos/vX.Y.Z`, независимо от osxphotos. До `v1.0.0` несовместимые изменения возможны (увеличивается minor). Изменения — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases). `v1.0.0` выйдет, когда API проверит приложение-потребитель. Как вычисляется версия — в [CONTRIBUTING.md](https://github.com/Windemiatrix/go-libs/blob/main/CONTRIBUTING.md#выпуск-для-мейнтейнера).

## Вопросы

- **Можно читать, пока «Фото» открыты?** Да: читается копия базы вместе с `-wal`, «Фото» не блокируются.
- **Читает ли сами фото?** `photosdb` — нет: только каталог (метаданные), пути к файлам в медиатеке (оригиналам, правленым версиям, файлам правок и превью) и сами файлы правок (списки свойств); файлы фото он не открывает. Файлы фото читает и пишет только `photokit`, когда вы вызываете выгрузку или импорт.
- **Фото только в iCloud («Оптимизация хранения»)?** Каталог читается целиком; у фото без оригинала на диске `Path` пуст, а `IsMissing`/`InCloud` показывают, где он. Выгрузка через `photokit` скачивает такой оригинал из iCloud.
- **`SystemLibraryPath` вернул `ErrLibraryNotFound`?** Ключа `SystemLibraryPath` в настройках `photolibraryd` может не быть (это встречалось на macOS 26), а через PhotoKit модуль путь не ищет. Ошибка проверяется `errors.Is(err, photosdb.ErrLibraryNotFound)`; путь к медиатеке тогда выбирает вызывающий.

## Лицензия

MIT. Порт [osxphotos](https://github.com/RhetTbull/osxphotos) (MIT, Rhet Turnbull); тестовые базы взяты из его тестов без изменений (см. [NOTICE](NOTICE)).

## Обратная связь

[Issues](https://github.com/Windemiatrix/go-libs/issues) — ошибки и новые версии macOS, [Discussions](https://github.com/Windemiatrix/go-libs/discussions) — вопросы и идеи.
