# osxphotos

[English](README.en.md) | **Русский**

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

Библиотека на Go для медиатеки «Фото» на macOS. Это неофициальный порт части [osxphotos](https://github.com/RhetTbull/osxphotos), Python-проекта Rhet Turnbull. Автор osxphotos в разработке этой библиотеки не участвует, но формат базы «Фото» разобрал он ([NOTICE](NOTICE)).

Дальше «фото» — это и фото, и видео. Библиотека подойдёт, если программе на Go нужно:

- прочитать фото, альбомы и папки, людей, места, ключевые слова, EXIF и пути к файлам;
- отобрать фото фильтрами `osxphotos query` и найти дубликаты;
- построить путь по шаблону osxphotos, например `{created.year}/{album}`;
- назвать файлы экспорта так же, как `osxphotos export`;
- выгрузить и импортировать фото, вести альбомы, удалять фото и менять их свойства.

Python не нужен. В библиотеке нет командной строки, записи файлов XMP и JSON рядом с фото и вызова exiftool: для этого возьмите сам osxphotos. До `v1.0.0` API может меняться.

## Подключение

Выполните `go get github.com/Windemiatrix/go-libs/osxphotos@latest`. Нужен Go 1.26 или новее. Все пакеты, кроме `photokit`, написаны на чистом Go без cgo и работают даже на Linux с копией медиатеки. `photokit` работает только на macOS в сборке с cgo. Пример печатает дату и исходное имя фото из системной медиатеки:

```go
lib, err := photosdb.SystemLibraryPath()
if err != nil {
	log.Fatal(err) // photosdb.ErrLibraryNotFound
}
db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
if err != nil {
	log.Fatal(err) // photosdb.ErrUnsupportedSchema: незнакомая версия «Фото»
}
defer db.Close()

for _, p := range db.Query(query.Options{}) { // как osxphotos query без флагов: без корзины и без невыбранных кадров серийной съёмки
	fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
}
```

## Пакеты

Все функции и типы — в [документации пакетов](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos).

| Пакет | Что делает |
|---|---|
| `photosdb` | Открывает медиатеку и отдаёт её данные; единственный пакет, который читает базу |
| `photoinfo`, `albuminfo`, `personinfo`, `placeinfo`, `exifinfo`, `momentinfo`, `scoreinfo`, `searchinfo`, `adjustmentsinfo` | Описывают фото, альбомы и папки, людей и лица, места, EXIF, моменты, оценки, поисковые метки и правки |
| `query` | Отбирает и сортирует фото, ищет дубликаты, как `osxphotos query` |
| `template` | Разбирает и выполняет шаблоны osxphotos |
| `export` | Даёт имена файлам экспорта |
| `photokit` | Читает и меняет системную медиатеку через PhotoKit |

## Безопасность данных и права macOS

- Медиатеку меняет только `photokit`, и только в методах импорта, альбомов, удаления и `Update`. Пакеты, кроме `photosdb` и `photokit`, с диском не работают.
- `photosdb` читает копию базы в новом каталоге с правами `0700`, только на чтение. `Open` удаляет копию до возврата, даже после ошибки. Если копия повреждена, `Open` копирует базу заново, до трёх попыток. Проверка копии находит только повреждение структуры. Полноту копии, сделанной во время записи «Фото», она не гарантирует.
- Копия содержит личные данные: места, имена людей, подписи. В `photosdb.Options.TempDir` укажите каталог вне медиатеки, доступный только вам. `DB.JSON`, `photokit.ImageInfo` и данные общего доступа (ссылки, участники, комментарии, лайки) тоже содержат личные данные: не пишите их в журналы.
- Библиотека сама не ходит в сеть и не запускает программ; это проверяет линтер. Телеметрии нет. При выгрузке `photokit` разрешает macOS скачать оригинал из iCloud.
- Программе, которая читает медиатеку через `photosdb`, нужен «Полный доступ к диску»; при запуске из Терминала — Терминалу. Для `photokit` нужно право «Фото», см. [PhotoKit](#photokit).
- Не прикладывайте `Photos.sqlite` к issue: достаточно указать версию macOS. Об уязвимостях пишите приватно, по [SECURITY.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.md).

## Версии macOS

Схему базы библиотека определяет по версии модели Core Data, записанной в базе. Поддержаны:

| macOS | «Фото» | Версия модели |
|---|---|---|
| 13 | 8 | 16320–16999 |
| 14.0–14.5 | 9 | 17000–17599 |
| 14.6+ | 9.6 | 17600–17999 |
| 15 | 10 | 18201–18999 |
| 26.0 | 11 | 19063–19319 |
| 26.1+ | 11.1 | 19320–19999 |
| 27 | 12 | 270000000–270999999 |

Для другой версии модели `Open` возвращает `ErrUnsupportedSchema` и схему не угадывает. Поддержку новой macOS добавляем, только когда есть тестовая медиатека этой версии: [заведите issue](https://github.com/Windemiatrix/go-libs/issues/new?template=new-macos.yml).

## Шаблоны путей

Пакет `template` выполняет шаблоны osxphotos: поля, фильтры, условия, замены, значения по умолчанию. Список полей — в [документации osxphotos](https://github.com/RhetTbull/osxphotos/blob/v0.77.2/osxphotos/phototemplate.md).

```go
t, err := template.Parse("{created.year}/{created.mm}-{created.dd} - {album,No album}/{photo.original_filename}")
if err != nil {
	log.Fatal(err) // template.ErrSyntax
}
paths, unknown, err := db.RenderTemplate(p, t, template.RenderOptions{})
// paths: [2024/05-01 - Holidays/IMG_0001.HEIC]; unknown — поля, которых нет в osxphotos
```

- Фото в двух альбомах даёт два пути: поле с несколькими значениями размножает результат.
- `RenderOptions.Filename` и `Dirname` очищают значения полей для имён файлов и каталогов. Но путь может содержать `..`, а с `Dirname` — и `/`: проверяйте, что он не выходит за ваш каталог.
- Поля, которые запускают код или программы (`{function}`, `{exiftool}` и другие), возвращают ошибку `ErrUnsupportedField`. Если путей больше `RenderOptions.MaxResults` (по умолчанию 10 000), возвращается `ErrTooManyValues`. Названия месяцев и дней недели — английские.
- Чтобы подставить свои данные, реализуйте интерфейс `template.Photo` с методом `Field(name)` и вызовите `Render` у `template.Renderer`.

## Имена файлов экспорта

Пакет `export` даёт файлам экспорта те же имена, что `osxphotos export`. Сами файлы пишет ваша программа.

```go
// taken — множество StemKey(имя) файлов целевого каталога; 0 — первым пробуется имя без номера.
name, _ := export.IncrementName(p.OriginalFilename, 0, taken) // IMG_0001.HEIC или IMG_0001 (1).HEIC
files := export.Names(p, name)                                 // files.Live — IMG_0001.mov
```

- `IncrementName` добавляет ` (1)`, ` (2)` и дальше, пока имя занято. Расширение не учитывается: если есть `foo.png`, имя `foo.jpeg` тоже занято.
- `Names` даёт имена сопутствующих файлов: видео Live Photo, RAW, превью, файлов правок. `EditedName` даёт имя правленой версии, например `IMG_0001_edited.jpeg`.
- Имена приходят из медиатеки как есть. Перед записью файла очистите имя, например функцией `template.SanitizeFilename`. Она не убирает `..` и NUL — проверьте их сами.

## PhotoKit

Пакет `photokit` читает и меняет системную медиатеку через PhotoKit. Он работает только на macOS в сборке с cgo, иначе каждый вызов возвращает `ErrNotSupported`.

В программе нужен `Info.plist` с ключом `NSPhotoLibraryUsageDescription`, иначе macOS завершит её при запросе доступа. Встройте файл сами, например флагом компоновщика `-sectcreate __TEXT __info_plist Info.plist`. `RequestAuthorization` спрашивает пользователя о доступе. `Open` не спрашивает: без полного доступа он возвращает `ErrNotAuthorized`, в том числе при доступе только к выбранным фото. Если программа запущена из Терминала, право «Фото» получает Терминал. После этого любая программа, запущенная из Терминала, может читать и менять всю медиатеку.

- `Fetch` находит фото по UUID из `photosdb`, `Resources` перечисляет его ресурсы — файлы: оригинал, правки, видео. Скрытые фото `photokit` не находит, как и osxphotos при работе через PhotoKit; `photosdb` их видит.
- `Export` выгружает ресурс как есть. `ExportImage`, `ExportVideo` и `ExportLivePhoto` выгружают версию с правками, версию без правок или исходный файл; у видео и Live Photo последние два варианта совпадают. Текущая версия замедленного видео рендерится заново, даже без правок, поэтому байты файла могут меняться от раза к разу.
- `Import` импортирует файлы, в том числе Live Photo и пары RAW+JPEG. Импорт не показывает диалог, даже если фото сразу попадают в скрытые.
- `CreateFolder`, `CreateAlbum`, `RenameCollection`, `DeleteCollections`, `AddToAlbum` и `RemoveFromAlbum` ведут папки и альбомы.
- `Delete` перемещает фото в «Недавно удалённые». `Update` меняет дату, избранное, скрытие и место фото, но снова показать скрытое фото не может.
- `CloudIdentifiers` и `LocalIdentifiers` переводят локальные идентификаторы фото в облачные и обратно.

Имя файла выгрузки — один элемент пути. Пустое имя, `.`, `..`, `/` или NUL в имени дают `ErrInvalidName`. Файл создаётся с правами `0600`. Существующий файл заменяется только с `ExportOptions.Overwrite`, иначе выгрузка возвращает `fs.ErrExist`. После ошибки созданный файл удаляется.

`Delete` и скрытие через `Update` показывают диалог macOS, `DeleteCollections` может его показать. Если пользователь отказал, вызов возвращает `ErrUserCanceled` и медиатека не меняется. `ctx` задаёт, сколько ждать итога любого изменения, включая импорт. Если `ctx` закончился после начала вызова, но до итога PhotoKit, ошибка содержит и `ErrOutcomeUnknown`, и ошибку `ctx`. Изменение могло уже примениться или применится позже, а диалог macOS, если был, остаётся открытым. Перечитайте медиатеку перед следующим изменением. С общей медиатекой iCloud `photokit` не проверялся.

## Отличия от osxphotos

Библиотека следует osxphotos `v0.77.2`. Сверх osxphotos она даёт облачные идентификаторы фото и альбомов и размеры всех файлов фото без скачивания. `photokit` сверх osxphotos импортирует, ведёт альбомы, удаляет и меняет фото. В библиотеке нет командной строки, базы экспорта, конвертации файлов, файлов XMP и JSON рядом с фото, exiftool и результатов анализа медиа. Полный список отличий со ссылками на код osxphotos — в [UPSTREAM.md](UPSTREAM.md).

## Как проверяется

Каждый PR проходит тесты с `-race` на Linux и macOS, линтер и порог покрытия 80 %. Результаты сравниваются с выводом osxphotos на восьми его тестовых медиатеках (macOS 13–27) и одной синтетической. `photokit` проверяется в CI на имитации PhotoKit. Тесты, которые меняют настоящую медиатеку, запускаются вручную под отдельной учётной записью macOS. Библиотеку можно сравнить с osxphotos и на своей медиатеке, не меняя её:

1. Поставьте osxphotos 0.77.2 в отдельное окружение Python 3.10+: `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Закройте «Фото», откройте медиатеку в Finder через «Показать содержимое пакета» и скопируйте каталог `database/` в `~/Pictures/Copy.photoslibrary/database/`.
3. В корне репозитория выполните `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`.
4. Прочитайте счётчики расхождений в терминале. Подробности — в файле, указанном в `REPORT`; в нём ваши данные, не публикуйте его. Затем удалите копию медиатеки.

## Вопросы

- **Можно ли читать медиатеку через `photosdb`, пока открыто приложение «Фото»?** Да, `photosdb` читает копию базы. Полноту копии, сделанной во время записи «Фото», библиотека не гарантирует.
- **Что `photosdb` знает о фото, которые хранятся только в iCloud?** Всё, что есть в базе. У такого фото `Path` пуст, а `IsMissing` равно true; оригинал скачает `photokit` при выгрузке.
- **Что делать, если `SystemLibraryPath` вернул `ErrLibraryNotFound`?** Дайте программе «Полный доступ к диску»; если не помогло, передайте путь к медиатеке в `Open` сами.

## Лицензия

MIT. Тестовые базы, кроме синтетической, взяты из тестов osxphotos без изменений, см. [NOTICE](NOTICE). Вопросы и идеи — в [Discussions](https://github.com/Windemiatrix/go-libs/discussions), ошибки — в [Issues](https://github.com/Windemiatrix/go-libs/issues).
