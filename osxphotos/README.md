# osxphotos (Go)

[English](README.en.md) | **Русский**

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

`github.com/Windemiatrix/go-libs/osxphotos` — библиотека на Go для чтения медиатеки «Фото» на macOS. Неофициальный порт части [osxphotos](https://github.com/RhetTbull/osxphotos) Rhet Turnbull (MIT): автор osxphotos к этому проекту отношения не имеет, вся благодарность за разбор формата базы — ему ([NOTICE](NOTICE)).

## Статус

**Ранняя разработка.** Выпуски — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases); до `v1.0.0` API может меняться. Чтение фото, альбомов, папок и людей, фильтры выборки и поиск дублей работают (`photosdb.Open`); шаблоны путей и PhotoKit запланированы. Подробности — в разделе [«Что есть и что будет»](#что-есть-и-что-будет).

## Быстрый старт

Нужен Go 1.26+. Код на чистом Go, без cgo; проверяется в CI на Linux и macOS. На Linux модуль читает копию каталога `.photoslibrary`; системная медиатека лежит на Mac (права — в разделе «Безопасность ваших фото»).

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

- **Оригинал не меняется.** Читается только копия `Photos.sqlite` вместе с `-wal` (без `-shm`: SQLite восстанавливает его сам), в режиме `mode=ro`. Копия создаётся во временном каталоге с правами `0700` (файлы `0600`) и удаляется до возврата из `Open`, в том числе после ошибки; каталог внутри медиатеки отклоняется.
- **Пакеты чтения** (`photosdb`, `photoinfo`, `albuminfo`, `adjustmentsinfo`, `query`) писать в медиатеку не умеют. Менять «Фото» сможет только будущий пакет `photokit` (macOS, cgo), и только если вы его импортируете и выдадите разрешение macOS; удаление из «Фото» macOS подтверждает своим окном (одним на пачку). Его тесты, меняющие медиатеку, будут запускаться только вручную, под отдельной учётной записью, и никогда в CI.
- **Без сети.** Модуль не обращается к сети и не запускает процессов — это проверяет линтер (правило `library` в [.golangci.yml](https://github.com/Windemiatrix/go-libs/blob/main/.golangci.yml)). Телеметрии нет.
- **Права macOS.** Медиатека в `~/Pictures` защищена системой: программе, которая её читает (например, Терминалу), нужен «Полный доступ к диску» в «Системных настройках → Конфиденциальность и безопасность».
- **Копия — это ваши данные:** места, имена людей, подписи, комментарии общих альбомов. Если задаёте `Options.TempDir`, выберите каталог, доступный только вам.
- **Не прикладывайте `Photos.sqlite` к issue:** в нём названия альбомов, пути и идентификаторы. Хватит версии macOS и номера модели базы. Об уязвимостях — приватно, по [SECURITY.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.md).

## Что есть и что будет

| Возможность | Пакет | Состояние |
|---|---|---|
| Определение схемы, macOS 13–27 | `photosdb` | есть |
| Путь системной медиатеки (`SystemLibraryPath`) | `photosdb` | есть |
| Открытие медиатеки (`Open`): каталог `.photoslibrary` или файл базы, в том числе вне медиатеки (`Options.LibraryPath` — где лежат оригиналы); путь базы и версия «Фото» (`Info.DBPath`, `Info.PhotosVersion`) | `photosdb` | есть |
| Фото: даты с поясом и его имя, размеры, ключевые слова, Live, RAW+JPEG, серии, iCloud, общая медиатека | `photosdb`, `photoinfo` | есть |
| Признаки фото: HDR, панорама, портрет, селфи, пространственное фото, замедленная съёмка, таймлапс, запись экрана, общий момент, правка в другом приложении, видимость, кто импортировал | `photosdb`, `photoinfo` | есть |
| Файлы на диске: оригинал, Live, RAW, правленая версия и её Live-видео, файлы правок, превью (`PathEdited`, `PathEditedLive`, `AdjustmentsPath`, `OriginalAdjustmentsPath`, `PathDerivatives`); данные правок — `Adjustments`, `OriginalAdjustments` | `photosdb`, `photoinfo`, `adjustmentsinfo` | есть |
| Люди на фото: имена (`Persons`; лицо без имени — `_UNKNOWN_`), по медиатеке — `DB.Persons`, `DB.PersonCounts` | `photosdb`, `photoinfo` | есть |
| Ключевые слова медиатеки, их счёт и слова без фото (`Keywords`, `KeywordCounts`, `KeywordsWithoutPhoto`) | `photosdb` | есть |
| Альбомы: название, путь папок, состав, даты, корзина, порядок сортировки и место в боковой панели (`AlbumInfo`, `Album`; фото в порядке альбома и место фото в нём — `AlbumPhotos`, `PhotoIndex`); общие альбомы — `SharedAlbumInfo`, `SharedAlbumTitles`, владелец — `Owner` | `photosdb`, `albuminfo` | есть |
| Сессии импорта: название, даты, фото, сессия каждого фото (`ImportInfo`, `Import`, `ImportUUID`) | `photosdb`, `albuminfo` | есть |
| Проекты («Мои проекты»): название, даты, фото, проекты каждого фото (`ProjectInfo`, `Project`, `ProjectInfoOf`, `ProjectUUIDs`) | `photosdb`, `albuminfo` | есть |
| Общий момент и общая медиатека фото (`SharedMomentInfo`, `ShareInfo`), её участники (`ShareParticipantInfo`, `ShareParticipants`) | `photosdb`, `photoinfo` | есть |
| Комментарии и отметки «Нравится» фото общего альбома (`Comments`, `Likes`) | `photosdb`, `photoinfo` | есть |
| Папки и их дерево, место в боковой панели (`FolderInfo`, `Folder`, `FolderList`) | `photosdb`, `albuminfo` | есть |
| Выборка `Query` с фильтрами `query.Options` (альбом, папка, даты, корзина, тип, признаки, человек и другие), сортировка; выборка с аргументами `photos()` — `PhotosWith` | `photosdb`, `query` | есть |
| Дубликаты (`DB.Duplicates`, `query.Duplicates`, опция `Duplicate`) | `photosdb`, `query` | есть |
| Шаблоны путей | `template` | запланировано |
| Имена файлов при выгрузке | `export` | запланировано |
| Импорт, альбомы, удаление через PhotoKit | `photokit` | запланировано |

**Чего нет:** командной строки, выгрузки файлов и sidecar, данных о лицах на снимках, кроме имён людей (положение и размер лица), названий мест (координаты читаются), EXIF, поиска по содержимому снимков, ремонта баз, «Фото» 7 и старше. Для этого есть сам osxphotos.

## Чем отличается от osxphotos

Портируется тег `v0.77.2`. То же чтение базы плюс: облачные идентификаторы снимков и альбомов, размеры ресурсов Live и RAW без скачивания, строгая ошибка вместо догадки на неизвестной схеме, `Photos()` без скрытых фильтров (выборку osxphotos — фото и видео вне корзины без невыбранных кадров серий — делает `Query(query.Options{})`). Позже — запись в «Фото» через PhotoKit. Полный список расширений и намеренных отличий со ссылками на код osxphotos — в [UPSTREAM.md](UPSTREAM.md); в коде расширения помечены `// extension: not in osxphotos`.

## Как проверяется

Каждый PR проходит тесты с `-race` на Linux и macOS, линтер и порог покрытия 80 %. Вывод `Open` сверяется с эталонными результатами osxphotos (v0.77.2) для 8 его тестовых медиатек (7 схем, от macOS 13.0 до 27.0 developer beta) и для синтетической базы; известные расхождения записаны с причинами. Поля сверх osxphotos проверяются SQL-миграциями с синтетическими значениями на временной копии базы ([SOURCES.md](https://github.com/Windemiatrix/go-libs/blob/main/testdata/libraries/SOURCES.md)). С помощью `make diff-local` (ниже) порт сверен с osxphotos и на медиатеке мейнтейнера, схема 11.1 (macOS 26.6): расхождений нет.

### Как проверить на своей медиатеке

1. Поставьте osxphotos 0.77.2 в отдельное окружение (нужен Python 3.10 или новее): `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Закройте «Фото», откройте пакет медиатеки в Finder («Показать содержимое пакета») и скопируйте папку `database/` в `~/Pictures/Copy.photoslibrary/database/`. Оригиналы снимков не нужны.
3. Из корня репозитория: `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`. Пути — абсолютные; `REPORT` необязателен.

Если проверка прошла без ошибок, в терминал выводятся только числа и имена ключей: сколько объектов и ключей сравнено, сколько расхождений. Сообщения об ошибках Go могут содержать переданные пути; сообщения osxphotos в терминал не выводятся, а если osxphotos упал, запустите его вручную, чтобы увидеть причину. В сводке есть число объектов вашей медиатеки: в issue приводите только строки по ключам. UUID и значения расхождений пишутся только в файл `REPORT` с правами 0600 (существующий файл перезаписывается): в нём данные вашей медиатеки, не публикуйте его. Вывод osxphotos и копия базы лежат во временном каталоге и удаляются после проверки, даже прерванной. После проверки удалите и копию `~/Pictures/Copy.photoslibrary`.

## Версии

Semver по тегам `osxphotos/vX.Y.Z`, независимо от osxphotos. До `v1.0.0` несовместимые изменения возможны (увеличивается minor). Изменения — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases). `v1.0.0` выйдет, когда API проверит приложение-потребитель. Как вычисляется версия — в [CONTRIBUTING.md](https://github.com/Windemiatrix/go-libs/blob/main/CONTRIBUTING.md#выпуск-для-мейнтейнера).

## Вопросы

- **Можно читать, пока «Фото» открыты?** Да: читается копия базы вместе с `-wal`, «Фото» не блокируются.
- **Читает ли сами фото?** Нет: только каталог (метаданные), пути к файлам в медиатеке (оригиналам, правленым версиям, файлам правок и превью) и сами файлы правок (списки свойств). Файлы фото не открываются.
- **Фото только в iCloud («Оптимизация хранения»)?** Каталог читается целиком; у фото без оригинала на диске `Path` пуст, а `IsMissing`/`InCloud` показывают, где он.
- **`SystemLibraryPath` вернул `ErrLibraryNotFound`?** Ключа `SystemLibraryPath` в настройках `photolibraryd` может не быть (это встречалось на macOS 26), а через PhotoKit модуль путь не ищет. Ошибка проверяется `errors.Is(err, photosdb.ErrLibraryNotFound)`; путь к медиатеке тогда выбирает вызывающий.

## Лицензия

MIT. Порт [osxphotos](https://github.com/RhetTbull/osxphotos) (MIT, Rhet Turnbull); тестовые базы взяты из его тестов без изменений (см. [NOTICE](NOTICE)).

## Обратная связь

[Issues](https://github.com/Windemiatrix/go-libs/issues) — ошибки и новые версии macOS, [Discussions](https://github.com/Windemiatrix/go-libs/discussions) — вопросы и идеи.
