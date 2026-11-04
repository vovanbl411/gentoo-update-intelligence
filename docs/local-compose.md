# Локальный запуск через Docker Compose

Нужны Docker Engine с Compose plugin и доступ к registry и источникам Gentoo. Первый запуск загружает полную Git-историю Gentoo: это может занять время и потребовать много места на диске.

## Первый отчёт

### 1. Запустите стек

Из корня репозитория задайте локальный пароль PostgreSQL из URL-safe символов, проверьте конфигурацию и запустите сервисы:

```sh
export POSTGRES_PASSWORD='<your-local-password>'
docker compose config -q
docker compose up --build -d
```

### 2. Дождитесь публикации Gentoo revision

Проверьте, что Gentoo runtime опубликовал проверенную ревизию:

```sh
docker compose exec gentoo git -C /metadata/gentoo.git rev-parse --verify refs/gentoo/published
```

Если команда вывела полный commit SHA и завершилась без ошибки, можно отправлять план. Если ref пока не существует, дождитесь окончания первой синхронизации и проверки, затем повторите команду.

### 3. Отправьте план из frontend

Откройте [http://127.0.0.1:8081](http://127.0.0.1:8081), вставьте пример в одну строку и отправьте его:

```text
[ebuild U] dev-libs/libxml2-2.14.6::gentoo [2.13.9::gentoo]
```

### 4. Сохраните и откройте отчёт

Дождитесь результата обработки. При принятии плана API возвращает `202 Accepted` и UUID задания; frontend показывает состояние и результат. Сохраните UUID и откройте этот же отчёт напрямую по адресу:

```text
http://127.0.0.1:8081/analyses/<UUID>
```

## Настройки

### Порт frontend

Frontend доступен на `http://127.0.0.1:8081`; только он публикует порт хоста и привязан к loopback. При необходимости задайте переменную `FRONTEND_PORT`. Backend, metadata-worker и PostgreSQL не публикуют host ports. Подробнее о переменных окружения см. в [`.env.example`](../.env.example).

### Gentoo mirror

По умолчанию используется `https://anongit.gentoo.org/git/repo/gentoo.git`. Источник можно заменить переменной `GENTOO_REPOSITORY_URL`; она применяется для синхронизации и provenance. Например, для [зеркала Gentoo на GitHub](https://github.com/gentoo/gentoo) задайте перед запуском:

```sh
export GENTOO_REPOSITORY_URL='https://github.com/gentoo/gentoo.git'
```

Если стек уже запущен, после смены URL выполните `docker compose up -d`, чтобы обновить конфигурацию Gentoo runtime и backend. В новом shell задайте выбранный URL снова перед командами Compose. `restart` не меняет окружение уже созданного контейнера.

### Overlays

Встроенный каталог содержит `guru` с master `gentoo`. При первом запросе backend загружает overlay в именованный volume `overlay-data`. Перед следующим анализом backend пытается обновить локальную копию и публикует только проверенный commit. При ошибке обновления используется последняя успешная публикация; уже начатый анализ продолжает использовать закреплённые ревизии. Источник и master GURU указаны в [каталоге Gentoo](https://raw.githubusercontent.com/gentoo/api-gentoo-org/master/files/overlays/repositories.xml) и [конфигурации GURU](https://raw.githubusercontent.com/gentoo-mirror/guru/master/metadata/layout.conf).

Для других overlays задайте доверенный JSON-каталог через `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG`. Замените URL и masters на значения своих репозиториев:

```sh
export GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG='[{"name":"gentoo-zh","url":"https://example.invalid/gentoo-zh.git","masters":["gentoo"]},{"name":"umbriel-overlay","url":"https://example.invalid/umbriel-overlay.git","masters":["gentoo"]}]'
```

Compose передаёт каталог backend. При локальном запуске backend задайте каталог вместе с `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH`. Поддерживаются HTTPS URL без учётных данных и абсолютные локальные пути, доступные backend; локальный путь в Compose нужно смонтировать в контейнер. URL из плана не используются. Повторное имя, в том числе встроенное, неоднозначно; неизвестные и неоднозначные имена указываются в ошибке соответствующей стороны отчёта. Неверный каталог или неизвестный master мешают запуску backend. После изменения каталога пересоздайте backend командой `docker compose up -d`. Если URL уже загруженного overlay изменён, backend сообщит о несовпадении источника и не станет использовать прежнюю копию под новым адресом.

Первая загрузка overlay происходит при обработке задания и может занять несколько минут.

### Metadata worker

Backend сначала ищет точный cache в закреплённом commit Gentoo. Если записи нет, он обращается к `metadata-worker`, который ищет ebuild в закреплённой истории и генерирует cache.

`metadata-worker` объединяет одновременно поступившие одинаковые запросы подготовки, включая создание временных checkout. Переменная `METADATA_WORKER_GENERATION_LIMIT` задаёт общий для всех анализов предел одновременных запусков `egencache`; значение по умолчанию — `2`. Укажите положительное целое число. После изменения пересоздайте worker:

```sh
docker compose up -d metadata-worker
```

## Диагностика

Gentoo runtime синхронизирует основное дерево при запуске и повторяет синхронизацию через три часа после завершения предыдущей попытки. Посмотреть журнал и опубликованный commit можно командами:

```sh
docker compose logs gentoo
docker compose exec gentoo git -C /metadata/gentoo.git rev-parse --verify refs/gentoo/published
```

Если ref ещё не опубликован, проверьте журнал и дождитесь завершения загрузки и проверки. После успешной синхронизации ref остаётся на последней проверенной ревизии даже при ошибке следующей попытки. Анализ использует commit, выбранный при его начале. Опубликованные commits хранятся в Git refs; автоматической очистки нет.

После завершившейся ошибкой попытки можно сразу запустить повторную синхронизацию, перезапустив только Gentoo runtime:

```sh
docker compose restart gentoo
```

Если Gentoo revision пока недоступна, `POST /analyses` всё равно сохранит задание и вернёт `202 Accepted`, а `GET` покажет `failed` и причину недоступности Provider. После публикации отправьте новое задание; перезапуск backend не требуется.

## Проверка API

`POST /analyses` принимает план, сохраняет задание и возвращает `202 Accepted` с UUID. Обработка идёт асинхронно; frontend proxy ждёт ответ POST не более часа. `GET /analyses/<UUID>` показывает статусы задания `pending`, `running`, `completed` или `failed`. При `completed` ответ содержит `result`; внутри него отдельный metadata lookup может иметь статус `not_found`. При `failed` ответ содержит `error` с причиной сбоя.

Проверить отправку плана через frontend proxy можно командой:

```sh
curl -i -H 'Content-Type: application/json' \
  -d '{"plan":"[ebuild U] dev-libs/libxml2-2.14.6::gentoo [2.13.9::gentoo]"}' \
  http://127.0.0.1:8081/api/analyses
```

Успешный ответ имеет статус `202 Accepted` и содержит `id`, `status` и `progress`. Чтобы проверить состояние, подставьте UUID из поля `id`:

```sh
curl -fsS 'http://127.0.0.1:8081/api/analyses/<UUID>'
```

При другом `FRONTEND_PORT` замените порт в URL. Прямую ссылку на отчёт и его API можно открыть по адресам `http://127.0.0.1:8081/analyses/<UUID>` и `http://127.0.0.1:8081/api/analyses/<UUID>`. Подробный контракт приведён в [документе HTTP API](http-api.md).

## Сохранение данных и остановка

Compose использует именованные volumes: `gentoo-data` хранит Gentoo Git-репозиторий, `overlay-data` — загруженные overlays, `metadata-cache` — сгенерированные результаты, а `postgres-data` — задания и анализы. `gentoo-data` доступен backend и metadata-worker только для чтения. В `overlay-data` backend может записывать при синхронизации, а worker имеет доступ только для чтения. Worker не подключён к PostgreSQL.

Пересоздание контейнеров без удаления volumes сохраняет завершённые результаты по UUID. Backend восстанавливает незавершённые задания после перезапуска. Чтобы пересоздать контейнеры и открыть прежний отчёт, выполните:

```sh
docker compose up --build -d --force-recreate
```

Для уже созданного `postgres-data` используйте тот же пароль: изменение `POSTGRES_PASSWORD` не меняет пароль в существующей базе. В новом shell задайте пароль снова перед командами Compose. Пример переменных приведён в [`.env.example`](../.env.example).

Команда `docker compose down` останавливает стек и сохраняет volumes. `docker compose down -v` удаляет volumes, включая задания и результаты анализов, Gentoo дерево, overlays и metadata cache.
