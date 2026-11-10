# osxphotos

[English](README.en.md) | **Русский**

[![Go Reference](https://pkg.go.dev/badge/github.com/Windemiatrix/go-libs/osxphotos.svg)](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos)

Библиотека на Go для медиатеки «Фото» на macOS. Неофициальный порт части [osxphotos](https://github.com/RhetTbull/osxphotos) — Python-проекта Rhet Turnbull (MIT). Автор osxphotos к этой библиотеке отношения не имеет; разбор формата базы — его заслуга ([NOTICE](NOTICE)).

Что умеет:

- читать каталог медиатеки: фото и видео, альбомы и папки, людей и лица, места, ключевые слова, EXIF, оценки, поисковые метки, пути к файлам;
- выбирать фото по фильтрам и находить дубликаты;
- строить пути по шаблонам osxphotos, например `{created.year}/{album}`;
- называть файлы экспорта так же, как `osxphotos export`;
- через PhotoKit выгружать и импортировать фото, вести альбомы, удалять фото и менять их дату, избранное, скрытие и место (только macOS).

Python не нужен, «Фото» запускать не нужно.

**Статус:** ранняя разработка, до `v1.0.0` API может меняться. Выпуски — на странице [Releases](https://github.com/Windemiatrix/go-libs/releases).

## Быстрый старт

```sh
go get github.com/Windemiatrix/go-libs/osxphotos@latest
```

```go
lib, err := photosdb.SystemLibraryPath()
if err != nil {
	log.Fatal(err)
}
db, err := photosdb.Open(context.Background(), lib, photosdb.Options{})
if err != nil {
	log.Fatal(err) // photosdb.ErrUnsupportedSchema — незнакомая версия «Фото»
}
defer db.Close()

for _, p := range db.Query(query.Options{}) { // как osxphotos query без флагов: вне корзины, без невыбранных кадров серий
	fmt.Println(p.Date.Format(time.DateOnly), p.OriginalFilename)
}
```

Нужен Go 1.26. Всё, кроме `photokit`, написано на чистом Go без cgo и работает даже на Linux, если дать ему копию медиатеки. `photokit` работает только на macOS в сборке с cgo.

## Пакеты

| Пакет | Что делает |
|---|---|
| `photosdb` | Открывает медиатеку и отдаёт каталог; единственный пакет, который читает базу |
| `photoinfo`, `albuminfo`, `personinfo`, `placeinfo`, `exifinfo`, `momentinfo`, `scoreinfo`, `searchinfo`, `adjustmentsinfo` | Данные фото, альбомов и папок, людей, мест, EXIF, моментов, оценок, поисковых меток и правок |
| `query` | Фильтры, сортировка и поиск дубликатов, как у `osxphotos query` |
| `template` | Язык шаблонов osxphotos |
| `export` | Имена файлов экспорта |
| `photokit` | Работа с системной медиатекой через PhotoKit |

Все функции и типы — в [документации пакетов](https://pkg.go.dev/github.com/Windemiatrix/go-libs/osxphotos).

## Безопасность ваших фото

- **Медиатеку меняет только `photokit`**, и только когда вы вызываете меняющий метод: импорт, альбомы, удаление, изменение объекта. `photosdb` работает с копией базы: она создаётся во временном каталоге с правами `0700`, открывается только на чтение и удаляется до возврата из `Open`. Остальные пакеты с диском не работают.
- **Удаление и скрытие фото, уже лежащих в медиатеке, подтверждает пользователь** в диалоге macOS. Импорт сразу в скрытые проходит без диалога.
- **В сеть библиотека сама не ходит** и не запускает программ; это проверяет линтер. Телеметрии нет. Выгружая фото, `photokit` разрешает macOS скачать оригинал из iCloud.
- **Права macOS.** Программе, которая читает медиатеку через `photosdb` (например, Терминалу), нужен «Полный доступ к диску». Для `photokit` нужно право «Фото» — см. [PhotoKit](#photokit).
- **Копия базы — ваши личные данные:** места, имена людей, подписи. `photosdb.Options.TempDir` указывайте только на каталог, доступный вам одному. `DB.JSON` и `photokit.ImageInfo` содержат пути и личные данные — не пишите их в журналы.
- Не прикладывайте `Photos.sqlite` к issue: хватит версии macOS. Об уязвимостях — приватно, по [SECURITY.md](https://github.com/Windemiatrix/go-libs/blob/main/SECURITY.md).

## Версии macOS

Схема базы определяется по номеру её модели. Поддержаны:

| macOS | «Фото» | Модель базы |
|---|---|---|
| 13 | 8 | 16320–16999 |
| 14.0–14.5 | 9 | 17000–17599 |
| 14.6+ | 9.6 | 17600–17999 |
| 15 | 10 | 18201–18999 |
| 26.0 | 11 | 19063–19319 |
| 26.1+ | 11.1 | 19320–19999 |
| 27 | 12 | 270000000–270999999 |

На другой модели `Open` возвращает `ErrUnsupportedSchema`, а не угадывает. Новую macOS добавляем, только когда есть медиатека этой версии: [заведите issue](https://github.com/Windemiatrix/go-libs/issues/new?template=new-macos.yml).

## Шаблоны

Пакет `template` разбирает и выполняет шаблоны osxphotos: поля, фильтры, условия, замены, значения по умолчанию.

```go
t, err := template.Parse("{created.year}/{created.mm}-{created.dd} - {album,No album}/{photo.original_filename}")
if err != nil {
	log.Fatal(err) // template.ErrSyntax
}
paths, unknown, err := db.RenderTemplate(p, t, template.RenderOptions{})
// paths: [2024/05-01 - Holidays/IMG_0001.HEIC]; unknown — поля, которых нет в osxphotos
```

- Фото в двух альбомах даёт два пути: поле с несколькими значениями размножает результат.
- С `RenderOptions.Filename` и `Dirname` значения полей очищаются для имён файлов и каталогов. Но в результате может остаться `..` из текста шаблона, а с одним `Dirname` — ещё и `/`, поэтому проверять, что путь не выходит за ваш каталог, — ваша задача.
- Поля, которые запускают код или программы (`{function}`, `{exiftool}` и другие), библиотека не поддерживает: они возвращают `ErrUnsupportedField`.
- Если путей больше `RenderOptions.MaxResults` (по умолчанию 10 000), возвращается `ErrTooManyValues`.
- Названия месяцев и дней недели — английские.
- Свои данные можно подставлять в шаблон, реализовав интерфейс `template.Photo` с одним методом `Field(name)`.

Список полей — в [документации osxphotos](https://github.com/RhetTbull/osxphotos/blob/v0.77.2/osxphotos/phototemplate.md).

## Имена экспорта

Пакет `export` даёт файлам те же имена, что `osxphotos export`. Сами файлы он не пишет.

```go
// 0 — сначала пробовать имя без номера
name, _ := export.IncrementName(p.OriginalFilename, 0, taken) // IMG_0001.HEIC или IMG_0001 (1).HEIC
files := export.Names(p, name)                                 // files.Live — IMG_0001.mov
```

- `IncrementName` добавляет ` (1)`, ` (2)`… если имя уже занято. `taken` — набор `StemKey(имя)` всех файлов каталога: расширение не учитывается, `foo.png` занимает и `foo.jpeg`.
- `Names` — имена сопутствующих файлов: видео Live Photo, RAW, превью, файлы правок.
- `EditedName` — имя правленой версии, например `IMG_0001_edited.jpeg`.

## PhotoKit

Пакет `photokit` работает с системной медиатекой через PhotoKit. Только macOS и сборка с cgo; иначе каждый вызов возвращает `ErrNotSupported`.

**Доступ.**

- Программа должна содержать `Info.plist` с ключом `NSPhotoLibraryUsageDescription`, иначе macOS завершит её при первом обращении. Встраивать его — задача программы, например флагом компоновщика `-sectcreate __TEXT __info_plist Info.plist`.
- `RequestAuthorization` спрашивает пользователя, `Open` — нет: без права он возвращает `ErrNotAuthorized`. Ограниченный доступ (только выбранные фото) тоже не принимается.
- Если запускать программу из Терминала, право «Фото» получает Терминал. После этого читать и менять всю медиатеку может любая программа, запущенная из него.

**Что умеет.**

- Найти объекты по UUID из `photosdb` (`Fetch`) и узнать их ресурсы (`Resources`).
- Выгрузить ресурс как есть (`Export`) или версию фото и видео: с правками, без правок, оригинал (`ExportImage`, `ExportVideo`, `ExportLivePhoto`). Замедленное видео с правками рендерится заново, поэтому байты могут отличаться от раза к разу.
- Импортировать файлы, в том числе Live Photo и RAW+JPEG (`Import`).
- Создавать, переименовывать и удалять папки и альбомы, добавлять в них фото.
- Удалять объекты (`Delete`) и менять дату, избранное, скрытие, место (`Update`).
- Переводить локальные идентификаторы в облачные и обратно.

Скрытые фото методы `photokit` не находят, как и PhotoKit-часть osxphotos; `photosdb` их видит.

**Файлы.** Выгрузка пишет один файл с правами `0600` в указанный каталог. Существующий файл не перезаписывается без `ExportOptions.Overwrite`. После ошибки недописанный файл удаляется.

**Диалоги и сроки.**

- `Delete`, `DeleteCollections` и скрытие через `Update` показывают диалог macOS. Если пользователь отказал — `ErrUserCanceled`, ничего не изменилось.
- Сколько ждать, задаёт `ctx`. Если срок вышел, пока PhotoKit ещё не сообщил результат изменения, — `ErrOutcomeUnknown`: изменение могло примениться или ещё применится, потому что диалог macOS остаётся открытым. Перечитайте медиатеку, прежде чем менять что-то снова.
- С общей медиатекой iCloud `photokit` не проверялся.

## Отличия от osxphotos

Порт следует osxphotos `v0.77.2`. Сверх него библиотека даёт облачные идентификаторы фото и альбомов, размеры всех файлов фото, в том числе Live и RAW, без скачивания, а в `photokit` — импорт, альбомы, удаление и изменение объектов. Чего нет: командной строки, записи файлов экспорта, сайдкаров и exiftool, результатов анализа медиа. Полный список отличий со ссылками на код osxphotos — в [UPSTREAM.md](UPSTREAM.md).

## Как проверяется

- Каждый PR: тесты с `-race` на Linux и macOS, линтер, покрытие не ниже 80 %.
- Результат сравнивается с выводом самого osxphotos на восьми его тестовых медиатеках (macOS 13–27) и одной синтетической. Шаблоны проверяются так же.
- `photokit` проверяется на имитации PhotoKit в CI. Тесты, которые меняют настоящую медиатеку, запускаются только вручную под отдельной учётной записью macOS.

### Проверка на своей медиатеке

Можно сравнить библиотеку с osxphotos на своих данных. Медиатека при этом не меняется.

1. Поставьте osxphotos 0.77.2 в отдельное окружение (Python 3.10+): `python3 -m venv "$HOME/venvs/osxphotos" && "$HOME/venvs/osxphotos/bin/pip" install osxphotos==0.77.2`.
2. Закройте «Фото», откройте медиатеку в Finder через «Показать содержимое пакета» и скопируйте папку `database/` в `~/Pictures/Copy.photoslibrary/database/`.
3. В корне репозитория: `make diff-local LIBRARY="$HOME/Pictures/Copy.photoslibrary" OSXPHOTOS="$HOME/venvs/osxphotos/bin/osxphotos" REPORT="$HOME/diff-local-report.txt"`.

В терминал выводятся только счётчики расхождений. Подробности пишутся в файл `REPORT`: в нём ваши данные, не публикуйте его. После проверки удалите копию медиатеки.

## Вопросы

- **Можно читать, пока открыто приложение «Фото»?** Да.
- **Фото только в iCloud?** Каталог читается целиком. У такого фото `Path` пуст, а `IsMissing` равно true. `photokit` при выгрузке скачает оригинал.
- **`SystemLibraryPath` вернул `ErrLibraryNotFound`?** На некоторых системах macOS не хранит путь к медиатеке там, где его ищет библиотека. Передайте путь в `Open` сами.

## Лицензия

MIT. Порт [osxphotos](https://github.com/RhetTbull/osxphotos) (MIT, Rhet Turnbull); тестовые базы взяты из его тестов без изменений, см. [NOTICE](NOTICE).

Вопросы и идеи — в [Discussions](https://github.com/Windemiatrix/go-libs/discussions), ошибки — в [Issues](https://github.com/Windemiatrix/go-libs/issues).
