# Upstream: osxphotos

**English** | [Русский](#upstream-osxphotos-ru)

This module is a Go port of parts of [osxphotos](https://github.com/RhetTbull/osxphotos). The port follows one fixed upstream release; changes made upstream after that release are not ported until the version below is updated.

The three lines below are read by a test in this module and by CI. Keep exactly this form.

```text
Upstream version: 0.77.2
Upstream tag: v0.77.2
Tag commit: not recorded yet
```

The version must equal the one in `tools/golden/requirements.txt` (the version used to generate reference data). CI fails when they differ, so moving to a new upstream version changes both files in the same pull request.

The table of correspondence between upstream modules and Go packages, the list of intentional differences and the list of extensions beyond upstream are added together with the first ported packages.

<a id="upstream-osxphotos-ru"></a>

## Upstream: osxphotos (по-русски)

Модуль — порт частей [osxphotos](https://github.com/RhetTbull/osxphotos) на Go. Порт следует одному зафиксированному выпуску upstream; правки upstream после этого выпуска не переносятся, пока версия выше не обновлена.

Три строки в блоке выше читает тест этого модуля и CI. Формат менять нельзя.

Версия должна совпадать с версией в `tools/golden/requirements.txt` (по ней строятся эталонные данные). При расхождении CI падает, поэтому переход на новую версию upstream меняет оба файла в одном PR.

Таблица соответствия модулей upstream и пакетов Go, список намеренных отличий и список расширений сверх upstream появятся вместе с первыми портированными пакетами.
