# Контракт HTTP API для сохранённого анализа

API принимает текст emerge plan, сохраняет задание в PostgreSQL и сразу возвращает UUID. Анализ выполняется в фоне; `GET` по UUID показывает состояние, прогресс и итог. API не запускает `emerge` или генерацию metadata напрямую.

## Конечные точки

### `POST /analyses`

Запрос должен иметь `Content-Type: application/json` и тело:

```json
{
  "plan": "[ebuild U ] dev-libs/libei-1.6.0-r1",
  "locally_patched_packages": [
    {"category": "dev-libs", "name": "libei"}
  ]
}
```

`plan` — обязательная непустая строка с исходным текстом плана. `locally_patched_packages` — необязательный список точных Gentoo category/name; пустое или пропущенное поле означает пустой Analyzer context. Размер тела ограничен 4 MiB. Неизвестные JSON-поля и некорректные типы отклоняются как `400 Bad Request`; другой `Content-Type` — как `415 Unsupported Media Type`. Статусы Parser `partial` и `unsupported` являются результатами анализа и сами по себе не отклоняют запрос.

После проверки запроса backend сохраняет его как задание. Ответ `202 Accepted` содержит UUID и начальное состояние:

```json
{
  "id": "<uuid>",
  "status": "pending",
  "progress": {
    "stage": "queued",
    "completed": 0,
    "total": 0
  }
}
```

UUID выдаётся только после успешного сохранения задания. Ответ также содержит `Location: /analyses/<uuid>`. Provider открывается и `planservice` выполняется в фоне; недоступность Provider или ошибка анализа переводит задание в `failed` и сохраняет причину. Во время разбора плана общий объём прогресса ещё неизвестен (`total: 0`). После разбора `total` показывает число уникальных metadata `Query`, а `completed` увеличивается после каждого Lookup; одинаковые запросы учитываются один раз. Прогресс доступен и пока backend открывает Provider. GET не запускает повторный анализ. Повторный POST создаёт отдельное задание.

Результат завершённого задания сохраняет полные результаты Parser, Analyzer и reportservice с исходными полями и значениями. Поле `Err` каждого Lookup outcome сериализуется строкой из `error.Error()`. Также сохраняются `Query`, `SkipReason`, полный `LookupResult` со статусом и snapshot, отчёт, diagnostics, findings и evidence. Ошибка Lookup не прерывает анализ и остаётся в соответствующем outcome.

### `GET /analyses/{id}`

`id` — UUID, выданный `POST /analyses`. Ответ `200 OK` для нового задания имеет поля `id`, `status`, `progress` и, после завершения, `result`. Статусы: `pending`, `running`, `completed` и `failed`. Поле `error` содержит причину при `failed`; `result` присутствует при `completed`.

Существующие строки таблицы `analyses`, сохранённые до перехода на асинхронный API, также возвращаются как `completed` с их прежним `result`. Это сохраняет возможность открывать старые UUID.

Неизвестный или некорректный ID возвращает `404 Not Found`; ошибка обращения к PostgreSQL возвращает `503 Service Unavailable`.

При старте backend повторно ставит в обработку задания со статусами `pending` и `running`. Работа возобновляется с начала; прогресс текущего запуска начинается заново. До анализа backend сохраняет выбранные commits Provider. При восстановлении он открывает те же commits; если commit или его локальный Git-объект недоступен, задание получает статус `failed` с явной причиной. Если рестарт произошёл до сохранения ревизий, задание повторно проходит этап их выбора.

При включённой подготовке overlays `result.Repositories` содержит закреплённые commits, URL и masters доступных репозиториев; локальные пути в ответ не входят. Неизвестное или неоднозначное имя repository сохраняется как ошибка Lookup соответствующей стороны. Ошибка первой загрузки overlay также сохраняется на соответствующей стороне; при ошибке обновления используется последняя успешно опубликованная ревизия. Остальные записи продолжают обрабатываться.

Ошибки запроса возвращаются с JSON-объяснением и `400 Bad Request`; другой `Content-Type` — `415 Unsupported Media Type`; недоступность хранилища при сохранении задания — `503 Service Unavailable`. Эти ошибки не содержат ID.

## Хранение

PostgreSQL — постоянное хранилище заданий и анализов. Миграция создаёт `analysis_jobs` с исходным запросом, статусом, прогрессом, ошибкой и итогом; прежняя таблица `analyses` остаётся для чтения сохранённых результатов. Redis и кэш не участвуют.

На старте backend применяет встроенные идемпотентные миграции и восстанавливает незавершённые задания. Provider открывается для каждого задания; если открыть его не удалось, задание получает `failed`, а его UUID остаётся доступным через GET. После появления опубликованного commit новое задание может успешно открыть Provider без перезапуска backend. Переменные окружения:

| Переменная | Назначение |
|---|---|
| `GENTOO_UPDATE_INTELLIGENCE_DATABASE_URL` | PostgreSQL DSN для backend |
| `GENTOO_UPDATE_INTELLIGENCE_LISTEN_ADDR` | Адрес HTTP listener; по умолчанию `127.0.0.1:8080` |
| `GENTOO_UPDATE_INTELLIGENCE_REPOSITORY` | Логическое имя Gentoo repository |
| `GENTOO_UPDATE_INTELLIGENCE_SOURCE_URL` | Заявленный источник для provenance |
| `GENTOO_UPDATE_INTELLIGENCE_REPOSITORY_PATH` | Путь локального Git repository на backend |
| `GENTOO_UPDATE_INTELLIGENCE_COMMIT` | Полный commit SHA; если не задан, используется `refs/gentoo/published` |
| `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH` | Путь для локальных копий известных overlays; если не задан, их загрузка отключена |
| `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG` | Необязательный JSON-массив доверенных записей `name`, `url`, `masters`, дополняющий встроенный каталог |
| `GENTOO_UPDATE_INTELLIGENCE_METADATA_WORKER_URL` | Внутренний URL metadata-worker; Compose задаёт `http://metadata-worker:8090` |
| `GENTOO_UPDATE_INTELLIGENCE_METADATA_PROFILE` | Профиль Portage для генерации; по умолчанию Gentoo profile `profiles/default/linux/amd64/23.0`, для overlay указывается с префиксом master, например `gentoo:profiles/default/linux/amd64/23.0` |

В Compose backend сначала читает точный cache из закреплённой ревизии. Только при `not_found` он передаёт точный запрос и закреплённые revisions worker. Worker использует постоянный файловый `metadata-cache` для результатов, совместимых по source ebuild, commits masters, профилю и версиям генератора/Portage. Результат и provenance генерации входят в `Lookup.Result`; ошибка подготовки остаётся в Lookup соответствующей стороны.

Секреты и DSN задаются окружением процесса и не сохраняются в результате анализа. Backend по умолчанию слушает только loopback; способы внешней публикации и аутентификация относятся к отдельному этапу.

## Границы и проверки

API переиспользует `planservice` и локальный `metadataprovider`; он не повторяет Parser, Analyzer, reportservice или логику Gentoo metadata. CLI остаётся отдельным инструментом и сохраняет прежнее поведение.

Автоматические тесты покрывают `202`, переходы состояния, прогресс, восстановление прерванного задания, ошибки Provider и хранения, а также чтение прежнего результата. Интеграционная проверка с PostgreSQL должна подтвердить сохранение задания до ответа, восстановление после перезапуска и чтение старых строк `analyses`.
