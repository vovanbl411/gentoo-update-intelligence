# Локальный Metadata Provider

`internal/metadataprovider` читает точный cache-файл из Git-объектов одного закреплённого commit и передаёт содержимое существующей `metadatadiff.Normalize`. Provider обслуживает один логический repository и один commit. Он не интегрирован с `ChangeReport` или `Analyzer`.

## API

```go
func Open(config Config) (*Provider, error)
func (p *Provider) Lookup(query Query) (LookupResult, error)
```

`Config` — доверенная настройка приложения. Все поля обязательны:

- `Repository` — логическое имя Gentoo repository;
- `SourceURL` — заявленное происхождение для provenance;
- `LocalPath` — путь к подготовленному локальному Git-репозиторию;
- `Commit` — полный hexadecimal commit SHA.

`SourceURL` не проверяется по сети. Он не доказывает, что локальные Git-объекты получены из указанного источника.

`Query` содержит `Repository`, `Category`, `Name` и `Version`. Он не может переопределить путь репозитория, URL источника или commit.

`LookupResult` содержит статус `found`, `not_found` или `repository_unavailable` и указатель `Snapshot`. Снимок не задан (`nil`) для двух последних статусов и при ошибке. При любой ошибке `Lookup` возвращает нулевой `LookupResult`.

## Проверка конфигурации и закрепление commit

`Open` выполняет следующие действия:

1. Проверяет наличие всех полей и синтаксис логического имени `Repository`.
2. Проверяет доступность локального Git-репозитория командой `git rev-parse --show-object-format`.
3. Определяет длину полного object ID по формату `sha1` или `sha256`.
4. Проверяет, что `Commit` содержит полный hexadecimal object ID требуемой длины.
5. Проверяет через `git cat-file -t`, что объект существует и имеет тип `commit`.
6. Сохраняет разрешённый полный SHA, возвращённый Git.

Ветка, tag, сокращённый SHA, blob и revision expression не принимаются вместо полного commit SHA. Tag не разворачивается в commit автоматически. Последующие изменения `HEAD`, ветки, индекса и рабочего дерева не меняют выбранное Provider содержимое. Git replace refs отключены при чтении и не подменяют объекты закреплённого commit.

## Проверка Query и путь cache

Все поля `Query` проверяются до сравнения `Repository`. Поэтому некорректный запрос с несовпадающим repository возвращает `ErrInvalidQuery`. Если запрос корректен, но `Repository` отличается от настройки Provider, возвращается `repository_unavailable` без чтения дерева.

Путь строится внутри Provider:

```text
metadata/md5-cache/<category>/<name>-<version>
```

`Category`, `Name` и `Version` проверяются по правилам PMS 8, разделам 3.1–3.2. Category допускает ASCII `[A-Za-z0-9+_.-]`, кроме начальных `-`, `.` и `+`. Package name допускает ASCII `[A-Za-z0-9+_-]`, кроме начальных `-` и `+`, и не может оканчиваться на `-` с последующей строкой, соответствующей синтаксису версии. Версия начинается с одного или нескольких числовых компонентов, может иметь строчную букву, цепочку `_alpha`, `_beta`, `_pre`, `_rc`, `_p` с необязательными числами и необязательный суффикс `-rN`. Эти правила допускают корректные Gentoo revisions и version suffixes. ([Gentoo PMS 8](https://projects.gentoo.org/pms/8/pms.html))

Логическое имя repository проверяется по правилам PMS 8: `[A-Za-z0-9_-]`, без начального дефиса и с дополнительными ограничениями package name. Разделители путей, абсолютные пути и управляющие символы не проходят грамматики компонентов. Корректная подстрока `..` не запрещается отдельно, если она допустима соответствующей грамматикой.

## Чтение Git-объектов

Каждый вызов Git через `runGit` передаёт `--no-replace-objects` и `--no-lazy-fetch`. Это отключает подмену объектов через replace refs и запрещает Git загружать отсутствующие объекты из promisor remote. Поэтому отсутствующий blob в partial clone приводит к `ErrRead`; Provider не запускает lazy fetch и не обращается к remote. Описание флага: [global options в документации Git](https://git-scm.com/docs/git).

Provider запускает Git CLI через `os/exec` с отдельными аргументами, без shell. В `git ls-tree` передаётся сформированный Provider путь как literal pathspec, после `--`. Вывод `ls-tree` разделён NUL; Provider ищет точное совпадение пути, извлекает mode, тип записи и object ID. Рекурсивный список дерева не используется.

Только записи с mode `100644` или `100755` и типом `blob` поддерживаются. Symlink, каталог, submodule и другие записи возвращают `ErrUnsupportedEntry`. Исполняемый бит не запускает содержимое. Для обычного файла Provider выполняет `git cat-file blob <object-id>` без filters или textconv и передаёт байты в `Normalize` без изменений.

Успешное чтение дерева без точной записи даёт `not_found`. Ошибка Git при чтении дерева или blob возвращает `ErrRead`, а не `not_found`. Неподдерживаемая запись возвращает `ErrUnsupportedEntry`; чтение рабочего дерева как запасной путь не выполняется.

## Ошибки

Ошибки распознаются через `errors.Is`:

| Sentinel | Значение |
|---|---|
| `ErrInvalidConfig` | обязательное поле или логическое имя `Repository` в конфигурации некорректно |
| `ErrInvalidQuery` | одно или несколько полей Query не соответствуют синтаксису |
| `ErrRepositoryAccess` | локальный репозиторий или Git CLI недоступен при `Open` |
| `ErrInvalidCommit` | commit отсутствует, имеет неверный формат либо не является commit-объектом |
| `ErrRead` | ошибка чтения дерева или blob |
| `ErrUnsupportedEntry` | по точному пути находится неподдерживаемый тип записи или mode |

Ошибка содержит контекст операции или пути и оборачивает соответствующий sentinel. Категория определяется кодом возврата и результатом Git-команд, а не поиском текста локализованного stderr.

## Snapshot и provenance

При `found` Provider создаёт `metadatadiff.Source` из проверенных полей Query и настройки Provider:

| Поле `Source` | Значение |
|---|---|
| `Category`, `Name`, `Version` | проверенные значения Query |
| `Repository` | имя из Config |
| `SourceURL` | заявленный URL из Config |
| `Commit` | фактически выбранный полный SHA commit |
| `CachePath` | точный путь объекта в commit |

`Normalize` сохраняет точное содержимое cache в `Snapshot.Content`, вычисляет SHA-256 этого содержимого и добавляет диагностики. SHA-256 содержимого не является Git object ID. Если обычный файл найден, но metadata повреждены или неполны, статус остаётся `found`; диагностики доступны в `Snapshot`.

`not_found` означает только отсутствие точной записи в выбранном commit. Provider не подбирает другую версию или ревизию. Snapshot описывает metadata выбранного repository, но не подтверждает, что они соответствуют установленной пользовательской сборке, полны или происходят из заявленного `SourceURL`.

## Git CLI и проверки

Для работы Provider необходим установленный `git` с поддержкой global option `--no-lazy-fetch`, доступный через `PATH`, и локальный репозиторий, содержащий выбранный commit и требуемые blobs. Provider не клонирует и не синхронизирует repository, не делает сетевых запросов или lazy fetch, не выполняет checkout, ebuild или генерацию metadata и не изменяет исходный репозиторий.

Тесты создают временные локальные репозитории в `t.TempDir`, задают test-only author/committer и отключают подпись только для Git-команд теста. SHA-1 проверяется обязательно; SHA-256 проверяется, если текущий Git поддерживает `git init --object-format=sha256`.

Покрываются обычные файлы mode `100644` и `100755`, точное содержимое, SHA-256 и provenance, `not_found`, `repository_unavailable`, приоритет валидации, Gentoo имена и версии, ошибки конфигурации и commit, закреплённый commit при изменённом `HEAD`, игнорирование рабочих изменений и untracked-файлов, symlink/каталог/submodule, диагностики повреждённых metadata, ошибка blob-чтения и отсутствие изменений репозитория при повторном чтении.

Validation gates:

```text
gofmt -w internal/metadataprovider/*.go
go test -count=1 ./...
go vet ./...
git diff --check
```

Provider реализован отдельно и пока не подключён к `ChangeReport`, `Analyzer` или другим компонентам.
