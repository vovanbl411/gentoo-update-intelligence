# Контракт HTTP API для сохранённого анализа

Контракт задаёт первый backend-подэтап: принять текст emerge plan, вызвать существующий `planservice`, сохранить полный результат в PostgreSQL и вернуть его по ID. API не запускает `emerge` или генерацию metadata.

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

Для каждого анализа backend открывает Metadata Provider с закреплённым commit и передаёт запрос в `planservice.Analyze`. Путь локального Git repository и способ выбора commit задаются конфигурацией backend, не HTTP-запросом. При отсутствии явно заданного commit backend один раз читает `refs/gentoo/published`; последующее обновление ссылки не меняет commit текущего анализа.

Успешный анализ сохраняется до отправки ответа. Ответ `201 Created`:

```json
{
  "id": "<uuid>",
  "result": {
    "Plan": {},
    "Analysis": {},
    "Reports": []
  }
}
```

`result` сохраняет полные результаты Parser, Analyzer и reportservice с исходными полями и значениями. Поле `Err` каждого Lookup outcome сериализуется строкой из `error.Error()`. Также сохраняются `Query`, `SkipReason`, полный `LookupResult` со статусом и snapshot, отчёт, diagnostics, findings и evidence. Ошибка Lookup не прерывает анализ и остаётся в соответствующем outcome.

При включённой подготовке overlays `result.Repositories` содержит закреплённые commits, URL и masters доступных репозиториев; локальные пути в ответ не входят. Неизвестное или неоднозначное имя repository сохраняется как ошибка Lookup соответствующей стороны. Ошибка первой загрузки overlay также сохраняется на соответствующей стороне; при ошибке обновления используется последняя успешно опубликованная ревизия. Остальные записи продолжают обрабатываться.

Если Metadata Provider недоступен, `POST` возвращает `503 Service Unavailable` без ID. Эта ошибка не мешает чтению ранее сохранённых анализов через `GET`. Если анализ не может быть собран или запись не подтверждена PostgreSQL, API не отправляет `201 Created` и не возвращает ID. Ошибка запроса возвращает JSON с объяснением и `400 Bad Request`; другой `Content-Type` — `415 Unsupported Media Type`; внутренняя ошибка анализа — `500 Internal Server Error`; недоступность Provider или хранилища — `503 Service Unavailable`. Ответ об ошибке не содержит ID.

### `GET /analyses/{id}`

`id` — UUID, выданный `POST /analyses`. Успешный ответ `200 OK` возвращает тот же объект `{ "id", "result" }`, сохранённый при создании. Неизвестный или некорректный ID возвращает `404 Not Found`; ошибка обращения к PostgreSQL возвращает `503 Service Unavailable`.

## Хранение

PostgreSQL — постоянное хранилище этого подэтапа. Минимальная миграция создаёт таблицу `analyses` с UUID primary key, JSONB полного результата и временем создания. Запись результата и получение созданного ID — единая операция вставки; API выдаёт ID только после её успешного завершения. Redis и кэш не участвуют.

На старте backend применяет встроенную идемпотентную миграцию. Provider открывается для каждого POST; если открыть его не удалось, POST возвращает `503 Service Unavailable`, а GET остаётся доступным. После появления опубликованного commit новый POST может успешно открыть Provider без перезапуска backend. Переменные окружения:

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

Секреты и DSN задаются окружением процесса и не сохраняются в результате анализа. Backend по умолчанию слушает только loopback; способы внешней публикации и аутентификация относятся к отдельному этапу.

## Границы и проверки

API переиспользует `planservice` и локальный `metadataprovider`; он не повторяет Parser, Analyzer, reportservice или логику Gentoo metadata. CLI остаётся отдельным инструментом и сохраняет прежнее поведение.

Автоматические тесты покрывают контракт HTTP, недоступность Provider и ошибки хранения через контролируемые реализации. Проверка с PostgreSQL должна подтвердить сохранение результата, его получение после перезапуска backend с недоступным Provider, равенство JSON POST/GET и `404` для неизвестного ID.
