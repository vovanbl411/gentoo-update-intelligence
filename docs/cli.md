# Контракт CLI

`backend/cmd/gentoo-update-intelligence` запускает ручной анализ текстового плана через принятый `planservice`. CLI читает вход, открывает локальный Metadata Provider с явно указанными repository и commit, передаёт результат анализа в JSON и не выполняет обновление системы.

Команды запускайте из каталога `backend/`.

## Требования

Для запуска нужны Go, Git CLI и локальный Git-репозиторий, содержащий выбранный commit и нужные объекты `metadata/md5-cache`. CLI не клонирует и не синхронизирует repository. Пользователь сам выбирает полный commit SHA; CLI передаёт его существующему Metadata Provider для проверки. Provider читает закреплённые объекты и не использует рабочее дерево как fallback.

Для provenance CLI задаёт `SourceURL` как `local://<repo>`. Локальный путь передаётся отдельно через `LocalPath` и не выдаётся за upstream URL.

## Запуск и параметры

Пример чтения плана из файла:

```sh
go run ./cmd/gentoo-update-intelligence \
  -plan /tmp/emerge-plan.txt \
  -repo gentoo \
  -repo-path /var/db/repos/gentoo \
  -commit FULL_COMMIT_SHA \
  -local-patched dev-libs/example
```

Для чтения того же текста из stdin укажите `-plan -`:

```sh
cat /tmp/emerge-plan.txt | go run ./cmd/gentoo-update-intelligence \
  -plan - \
  -repo gentoo \
  -repo-path /var/db/repos/gentoo \
  -commit FULL_COMMIT_SHA
```

Поддерживаются только флаги, без подкоманд и позиционных аргументов:

| Флаг | Обязательность | Значение |
|---|---|---|
| `-plan` | Обязателен | Путь к текстовому файлу; `-` читает stdin. |
| `-repo` | Обязателен | Логическое имя repository для Metadata Provider. |
| `-repo-path` | Обязателен | Путь к локальному Git-репозиторию с metadata cache. |
| `-commit` | Обязателен | Замените `FULL_COMMIT_SHA` полным SHA commit. Provider проверяет его; HEAD, ветка, tag и сокращённый SHA не подставляются. |
| `-local-patched` | Необязателен, повторяем | Точная пара `category/name`. Порядок значений сохраняется, повторения допустимы. |

Для `-local-patched` разрешены только символы имени категории и пакета в виде одной пары `category/name`: category использует `[A-Za-z0-9+_.-]`, package name — `[A-Za-z0-9+_-]`, с правилами начального символа Metadata Provider. Имя, оканчивающееся на суффикс версии, не принимается. Версии, операторы, wildcards, SLOT, repository и пробелы не принимаются. CLI не разбирает dependency atoms.

Все четыре обязательных флага должны быть заданы непустыми значениями. Неизвестный флаг, лишний позиционный аргумент или некорректная пара `category/name` — ошибка аргументов. `-h` и `-help` печатают справку в stderr.

## JSON

В stdout записывается JSON с отступами. Верхний уровень содержит `Plan`, `Analysis` и `Reports`; вложенные поля сохраняют имена экспортируемых полей существующих типов. Результат включает Parser status и факты, диагностики с `SourceLines`, findings с `EntryIndex` и типизированным evidence, отчёты, metadata comparison и provenance, а также обе стороны Lookup с Query, Provider Result и SkipReason.

В JSON-представлении `LookupOutcome.Err` имеет тип string-or-null: `null`, если ошибки нет, и строка с `Err.Error()`, если она есть. Go `error` напрямую не сериализуется. Остальные поля выводятся без `omitempty`, включая нулевые индексы и nil-ссылки. Диагностики Parser и findings отдельно в stderr не дублируются.

Этот JSON предназначен для ручного изучения результата и не является стабильным публичным API. Код 0 означает, что CLI сформировал и записал результат. Он не означает, что план безопасен, обновление одобрено или его можно выполнять. Approval не реализуется.

## Коды завершения

| Код | Значение |
|---:|---|
| `0` | Результат анализа записан. Включает Parser `partial` / `unsupported`, findings `BLOCKER`, отсутствующие metadata и ошибки внутри Lookup outcomes. |
| `1` | Ошибка чтения входа, открытия Provider, внешняя ошибка planservice, сериализации или записи stdout. |
| `2` | Некорректные аргументы. |

При ошибке аргументов или выполнения успешный JSON не выводится. Ошибки и справка идут в stderr; stdout содержит только результат JSON.

## Границы

Парсер поддерживает только входной формат и строки, описанные в [контракте Parser](parser.md). Неполный и неподдерживаемый вход остаются данными результата и не меняют код завершения `0`. Предупреждения о пропущенных обновлениях из-за dependency conflict пока остаются отложенным этапом Parser.

CLI не запускает `emerge`, не меняет систему или metadata repository, не выполняет Git fetch/clone/pull/sync и не выбирает commit автоматически. Он не повторяет правила анализа или проверки ChangeReport, не ищет настройки Portage, не добавляет retry, кэш, approval, БД или другой формат вывода. Git-чтение выполняет только существующий Metadata Provider.

`planservice` принят после review `013523e`. CLI подготовлен и ожидает review.
