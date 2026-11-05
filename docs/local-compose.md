# Локальный запуск через Docker Compose

Полный стек включает frontend, backend, PostgreSQL, Gentoo sync и metadata worker. Нужны Docker Engine с Compose plugin и доступ к registry и источникам Gentoo. Gentoo-образы собираются для `linux/amd64`; все команды ниже выполняются из корня репозитория.

Сборка и отдельная проверка Gentoo-образов описаны в [gentoo/README.md](../gentoo/README.md), устройство сборки — в [gentoo/build.md](../gentoo/build.md). Первый запуск sync загружает полную Git-историю Gentoo: это может занять время и потребовать много места на диске.

## Первый отчёт

### 1. Запустите стек

Задайте локальный пароль PostgreSQL из URL-safe символов, проверьте конфигурацию и запустите сервисы:

```sh
export POSTGRES_PASSWORD='<your-local-password>'
docker compose config -q
sh scripts/build-gentoo-images.sh
docker compose up --build -d
```

Скрипт подготавливает Gentoo-базу и общий runtime для worker/sync. После этого `up --build` собирает образы сервисов из Compose и запускает стек. Сам по себе `docker compose up --build` не собирает отдельный `gentoo/Dockerfile.base`. Для стандартной локальной сборки `GENTOO_BASE_IMAGE` не должна переопределять базу; выбор готовой базы описан [ниже](#общий-gentoo-образ).

### 2. Дождитесь публикации Gentoo revision

Проверьте, что Gentoo runtime опубликовал проверенную ревизию. Healthy-статус worker сам по себе не подтверждает готовность Git-репозитория:

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

### Общий Gentoo-образ

Сервисы `gentoo` и `metadata-worker` используют один локальный образ `gentoo-update-intelligence/gentoo-runtime:local`, собранный из `gentoo/Dockerfile`. Штатная база — Gentoo musl-hardened с GNU-инструментами, без BusyBox. Образ содержит Portage, Git, бинарник worker и скрипты синхронизации. По умолчанию образ запускает sync; сервис `metadata-worker` задаёт собственный entrypoint. Настройки сети, capabilities и volumes остаются отдельными для каждого сервиса.

Для сборки, запуска только worker/sync и воспроизведения их проверки используйте [README Gentoo runtime](../gentoo/README.md). Подробности стадий, закрепления версий, кеша и обновления базы находятся в [руководстве по сборке](../gentoo/build.md). Публикация в registry пока не выполнена; `/metadata/gentoo.git` хранится отдельно от образа в обновляемом volume.

Для выбора подготовленной базы задавайте `GENTOO_BASE_IMAGE` **в окружении shell** перед сборкой. Скрипт пропустит подготовку локальной базы, проверит выбранный образ, затем Compose использует ту же ссылку при сборке runtime. Значение только в `.env` влияет на Compose, но не на выбор базы самим shell-скриптом. Порядок выбора базы и отката описан в [build.md](../gentoo/build.md#использовать-другую-подготовленную-базу).

### Порт frontend

Только frontend публикует host port и привязан к loopback. Чтобы изменить порт, задайте `FRONTEND_PORT`. Backend, metadata-worker и PostgreSQL host ports не публикуют. Пример операторских переменных находится в [`.env.example`](../.env.example).

### Gentoo mirror

Чтобы использовать другое зеркало, задайте `GENTOO_REPOSITORY_URL` перед запуском. Например, для [зеркала Gentoo на GitHub](https://github.com/gentoo/gentoo):

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

Backend ищет точный cache в закреплённой Git-ревизии соответствующего repository. При `not_found` backend передаёт worker точный запрос и закреплённые ревизии; `metadata-worker` ищет ebuild в закреплённой истории и готовит cache, если исходники и профиль доступны.

`metadata-worker` объединяет одновременно поступившие одинаковые запросы подготовки, включая создание временных checkout. Чтобы изменить общий предел параллельной генерации, задайте `METADATA_WORKER_GENERATION_LIMIT` и пересоздайте worker:

```sh
docker compose up -d metadata-worker
```

### Переменные окружения

#### Параметры, которые оператор задаёт для Compose

Переменные Compose можно задать в `.env` или окружении перед запуском `docker compose`. Для `GENTOO_BASE_IMAGE` используйте окружение shell, чтобы её одинаково учитывали скрипт сборки и Compose. `.env.example` содержит пароль, порт frontend, URL Gentoo repository и предел генерации; каталог overlays и профиль metadata — дополнительные параметры Compose.

| Переменная | Роль и значение по умолчанию |
|---|---|
| `POSTGRES_PASSWORD` | Обязательный пароль PostgreSQL. Compose подставляет его в DSN backend и в начальную конфигурацию сервиса PostgreSQL. Используйте URL-safe символы, поскольку пароль входит в URL подключения. Для существующего volume укажите действующий пароль базы: изменение переменной не меняет пароль внутри PostgreSQL. |
| `FRONTEND_PORT` | Необязательный host port frontend; по умолчанию `8081`. Привязка остаётся к `127.0.0.1`. |
| `GENTOO_REPOSITORY_URL` | URL Gentoo Git repository для синхронизации и provenance; по умолчанию `https://anongit.gentoo.org/git/repo/gentoo.git`. |
| `METADATA_WORKER_GENERATION_LIMIT` | Общий предел одновременных запусков `egencache`; по умолчанию `2`. Задайте положительное целое число. |
| `GENTOO_BASE_IMAGE` | Ссылка на подготовленную Gentoo-базу, только для сборки runtime. По умолчанию — `gentoo-update-intelligence/gentoo-base:local`. Если переменная задана в окружении shell, скрипт пропускает сборку базы. |
| `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG` | Необязательный JSON-каталог дополнительных доверенных overlays; по умолчанию пустой. |
| `GENTOO_UPDATE_INTELLIGENCE_METADATA_PROFILE` | Необязательный профиль Portage для подготовки metadata; по умолчанию пустой, тогда backend выбирает профиль по умолчанию. |

#### Окружение backend

Compose задаёт backend следующие параметры. При запуске backend вне Compose задайте обязательный DSN PostgreSQL и параметры Gentoo repository (`GENTOO_UPDATE_INTELLIGENCE_REPOSITORY`, `GENTOO_UPDATE_INTELLIGENCE_SOURCE_URL`, `GENTOO_UPDATE_INTELLIGENCE_REPOSITORY_PATH`); остальные параметры включайте по необходимости.

| Переменная backend | Compose задаёт | Backend вне Compose |
|---|---|---|
| `GENTOO_UPDATE_INTELLIGENCE_DATABASE_URL` | `postgres://guit:${POSTGRES_PASSWORD}@postgres:5432/guit?sslmode=disable`. | Обязательный DSN доступной базы PostgreSQL. В ней хранятся задания и результаты. |
| `GENTOO_UPDATE_INTELLIGENCE_LISTEN_ADDR` | `0.0.0.0:8080` внутри сети Compose. | Необязательная настройка; если не задана, backend слушает `127.0.0.1:8080`. |
| `GENTOO_UPDATE_INTELLIGENCE_REPOSITORY` | `gentoo`. | Имя основного Gentoo repository, например `gentoo`. |
| `GENTOO_UPDATE_INTELLIGENCE_SOURCE_URL` | Значение `GENTOO_REPOSITORY_URL` или URL по умолчанию из предыдущей таблицы. | URL источника основного repository для provenance. |
| `GENTOO_UPDATE_INTELLIGENCE_REPOSITORY_PATH` | `/metadata/gentoo.git`; volume Gentoo подключён backend только для чтения. | Путь к локальному Git repository. |
| `GENTOO_UPDATE_INTELLIGENCE_COMMIT` | Compose не задаёт; backend разрешает `refs/gentoo/published`. | Необязательный полный commit SHA. Если не задан, используется `refs/gentoo/published`. |
| `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH` | `/overlays`. | Необязательный путь для локальных копий overlays; пустое значение отключает их подготовку. |
| `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG` | Значение из предыдущей таблицы или пустая строка. | Необязательный JSON-каталог доверенных репозиториев; применяется при настроенном `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH`. |
| `GENTOO_UPDATE_INTELLIGENCE_METADATA_WORKER_URL` | `http://metadata-worker:8090` во внутренней сети Compose. | Необязательный URL worker; если не задан, backend не передаёт ему metadata-запросы. |
| `GENTOO_UPDATE_INTELLIGENCE_METADATA_PROFILE` | Значение из предыдущей таблицы или пустая строка. | Необязательный профиль Portage. Пустое значение означает `profiles/default/linux/amd64/23.0`; для профиля overlay укажите master, например `gentoo:profiles/default/linux/amd64/23.0`. |

Compose задаёт самому metadata worker `METADATA_WORKER_LISTEN_ADDR=:8090`, `METADATA_WORKER_CACHE_PATH=/metadata-cache`, `METADATA_WORKER_TEMP_PATH=/var/tmp/gentoo-update-intelligence`, `METADATA_WORKER_PORTAGE_USER=portage` и `METADATA_WORKER_PORTAGE_GROUP=portage`. Это параметры worker, не backend. Именованный volume `metadata-cache` хранит подготовленные файлы; задания и результаты хранит PostgreSQL.

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

`POST /analyses` принимает план, сохраняет задание и возвращает `202 Accepted` с UUID, не ожидая завершения анализа. `GET /analyses/<UUID>` показывает статусы задания `pending`, `running`, `completed` или `failed`. При `completed` ответ содержит `result`; внутри него отдельный metadata lookup может иметь статус `not_found`. При `failed` ответ содержит `error` с причиной сбоя.

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

Если POST возвращает `201 Created` и готовый `result` без статуса задания, запущен прежний синхронный backend. В работающем стеке пересоберите и пересоздайте backend и frontend из текущих исходников: `docker compose up -d --no-deps --build backend frontend`. Команда `docker compose start` запускает существующие контейнеры без обновления их образов.

## Сохранение данных и остановка

Compose использует именованные volumes: `gentoo-data` хранит Gentoo Git-репозиторий, `overlay-data` — загруженные overlays, `metadata-cache` — сгенерированные результаты, а `postgres-data` — задания и анализы. `gentoo-data` доступен backend и metadata-worker только для чтения. В `overlay-data` backend может записывать при синхронизации, а worker имеет доступ только для чтения. Worker не подключён к PostgreSQL.

Пересоздание контейнеров без удаления volumes сохраняет завершённые результаты по UUID. Backend восстанавливает незавершённые задания после перезапуска. Чтобы пересоздать контейнеры и открыть прежний отчёт, выполните:

```sh
docker compose up --build -d --force-recreate
```

Для уже созданного `postgres-data` используйте тот же пароль: изменение `POSTGRES_PASSWORD` не меняет пароль в существующей базе. В новом shell задайте пароль снова перед командами Compose. Пример переменных приведён в [`.env.example`](../.env.example).

Команда `docker compose down` останавливает стек и сохраняет volumes. `docker compose down -v` удаляет volumes, включая задания и результаты анализов, Gentoo дерево, overlays и metadata cache.
