# Локальный запуск через Docker Compose

Полный стек включает frontend, backend, PostgreSQL, Gentoo sync и metadata worker. Нужны Docker Engine с Compose plugin и доступ к registry и источникам Gentoo. Gentoo-образы собираются для `linux/amd64`; все команды ниже выполняются из корня репозитория.

Сборка и отдельная проверка Gentoo-образов описаны в [gentoo/README.md](../gentoo/README.md), устройство сборки — в [gentoo/build.md](../gentoo/build.md). Первый запуск sync загружает полную Git-историю Gentoo: это может занять время и потребовать много места на диске.

## Локальные тестовые значения

Для текущего локального запуска в корне создан `.env` с `POSTGRES_PASSWORD=guit-dev-only`. Compose читает этот файл автоматически: export, отдельный скрипт и `--env-file` не нужны. `.env` исключён из Git. В новом checkout создайте его с тестовым значением вручную; для другого окружения задайте собственный пароль.

Обычный запуск выполняется из корня проекта:

```sh
docker compose up -d
docker compose ps -a
```

Команда запускает весь стек, включая PostgreSQL, metadata worker и sync. `--no-deps` используйте только для обновления отдельных сервисов, когда их зависимости уже работают. Для проверки остановившегося backend выполните `docker compose logs --tail 50 backend`.

Для существующей тестовой PostgreSQL пароль роли должен совпадать со значением в `.env`. Если он ещё не менялся на тестовый, при запущенном postgres выполните один раз:

```sh
docker compose exec postgres psql -U guit -d guit -c "ALTER ROLE guit WITH PASSWORD 'guit-dev-only';"
```

Команда меняет пароль роли guit и сохраняет данные. Изменение одного `.env` не меняет пароль существующей роли. Переменная POSTGRES_PASSWORD, ранее экспортированная в shell, имеет приоритет над файлом; для работы только через `.env` уберите её из окружения терминала.

## Первый отчёт

### 1. Запустите стек

Убедитесь, что в `.env` задан действующий пароль PostgreSQL из URL-safe символов, затем проверьте конфигурацию и запустите сервисы:

```sh
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

### Конфигурация backend

Обычные настройки backend находятся в [config/backend.yaml](../config/backend.yaml), подключённом read-only. Приоритет: defaults → YAML → непустые env. Поля, defaults и полный список env описаны в [руководстве конфигурации](backend-configuration.md).

При первом переходе на YAML пересоберите и пересоздайте backend:

```sh
docker compose up -d --no-deps --build backend
```

Далее после изменения того же файла достаточно `docker compose restart backend`. Смена env или источника файла требует пересоздания контейнера. Непустой env override продолжает перекрывать YAML после restart. DSN и пароль PostgreSQL остаются в env; их не нужно переносить в файл.

Другой файл на хосте выбирается `BACKEND_CONFIG_PATH`; по умолчанию это `./config/backend.yaml`. Он должен существовать. Изменение пути в конфигурации backend не меняет volumes Compose. Для предыдущего, ещё не применённого перехода на API v1 backend и frontend обновляются вместе по разделу [«Проверка API»](#проверка-api).

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

Для других overlays добавьте доверенные записи в `overlays.catalog` файла backend. Например:

```yaml
overlays:
  path: "/overlays"
  catalog:
    - name: gentoo-zh
      url: "https://example.invalid/gentoo-zh.git"
      masters: [gentoo]
    - name: umbriel-overlay
      url: "https://example.invalid/umbriel-overlay.git"
      masters: [gentoo]
```

Замените URL и masters значениями своих репозиториев. Примените изменение через restart backend. Существующий JSON env `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG` также поддерживается; непустое значение заменяет список YAML-дополнений целиком. Для env-пути используется `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH`; после изменения env нужен recreate.

Поддерживаются HTTPS URL без учётных данных и абсолютные локальные пути, доступные backend; локальный путь в Compose нужно смонтировать. URL из плана не используются. Повторное имя, в том числе встроенное, неоднозначно; неизвестные и неоднозначные имена указываются в ошибке соответствующей стороны отчёта. Неверный каталог или неизвестный master мешают запуску backend. Если URL уже загруженного overlay изменён, backend сообщит о несовпадении источника и не станет использовать прежнюю копию под новым адресом.

Первая загрузка overlay происходит при обработке задания и может занять несколько минут.

### Число активных анализов

По умолчанию backend выполняет один анализ, а остальные принятые задания хранятся в PostgreSQL со статусом `pending` и этапом `queued`. UUID возвращается сразу после сохранения задания. Ожидающий план ещё не разбирается и не создаёт Git checkout. После завершения или ошибки активного анализа backend берёт следующее задание по `created_at, id`.

Число активных анализов задаётся `analysis.concurrency_limit` в YAML:

| Значение | Поведение |
|---|---|
| `1` | Один анализ одновременно, default. |
| Положительное целое, например `4` | Не больше указанного числа активных анализов. |
| `0` | Число активных анализов не ограничено. |

После изменения поля перезапустите backend:

```sh
docker compose restart backend
```

Непустой `GENTOO_UPDATE_INTELLIGENCE_ANALYSIS_CONCURRENCY_LIMIT` имеет приоритет над YAML. При изменении этого env пересоздайте backend командой `docker compose up -d --no-deps backend`; пересборка для изменения настройки не нужна. Отрицательное, нечисловое значение или переполнение мешают запуску.

Большой предел и `0` могут увеличить расход CPU, памяти и временного диска. Предел не ограничивает число принятых заданий и размер очереди в PostgreSQL. Сохранённые задания и результаты остаются в базе; прерванные задания возвращаются в общую очередь с прежними закреплёнными ревизиями. Пока задание ждёт первого запуска, ревизии ещё не выбираются.

Очередь рассчитана на **один backend**. Лимит относится к этому процессу; запуск нескольких backend с общей базой не поддерживается, поскольку восстановление возвращает все `running` в очередь. Полный контракт очереди находится в [спецификации подготовки metadata](../openspec/specs/automatic-metadata-preparation/spec.md).

### Metadata worker

Backend ищет точный cache в закреплённой Git-ревизии соответствующего repository. При `not_found` backend передаёт worker точный запрос и закреплённые ревизии; `metadata-worker` ищет ebuild в закреплённой истории и готовит cache, если исходники и профиль доступны.

`metadata-worker` объединяет одновременно поступившие одинаковые запросы подготовки, включая создание временных checkout. Чтобы изменить общий предел параллельной генерации, задайте `METADATA_WORKER_GENERATION_LIMIT` и пересоздайте worker:

```sh
docker compose up -d metadata-worker
```

Предел генераций по умолчанию — `2`; он независим от числа активных анализов. Например, `4` анализа могут выполняться одновременно, но всего worker запустит не больше двух `egencache`. `METADATA_WORKER_GENERATION_LIMIT` принимает только положительное целое: `0` здесь недопустим. Для изменения задайте, например, `export METADATA_WORKER_GENERATION_LIMIT=4`, затем выполните команду выше. Повышайте пределы по результатам измерений на целевой машине; начальные значения `1` и `2` не гарантируют определённую производительность.

### Переменные окружения

#### Параметры, которые оператор задаёт для Compose

Переменные Compose можно задать в `.env` или окружении перед запуском `docker compose`. Для `GENTOO_BASE_IMAGE` используйте окружение shell, чтобы её одинаково учитывали скрипт сборки и Compose. `.env.example` содержит пароль, порт frontend, URL Gentoo repository и предел генерации; каталог overlays и профиль metadata — дополнительные параметры Compose.

| Переменная | Роль и значение по умолчанию |
|---|---|
| `BACKEND_CONFIG_PATH` | Файл YAML на хосте; по умолчанию `./config/backend.yaml`. Смена источника bind mount требует recreate backend. |
| `POSTGRES_PASSWORD` | Обязательный пароль PostgreSQL. Compose подставляет его в DSN backend и в начальную конфигурацию сервиса PostgreSQL. Используйте URL-safe символы, поскольку пароль входит в URL подключения. Для существующего volume укажите действующий пароль базы: изменение переменной не меняет пароль внутри PostgreSQL. |
| `FRONTEND_PORT` | Необязательный host port frontend; по умолчанию `8081`. Привязка остаётся к `127.0.0.1`. |
| `GENTOO_REPOSITORY_URL` | URL Gentoo Git repository для синхронизации и provenance; по умолчанию `https://anongit.gentoo.org/git/repo/gentoo.git`. |
| `METADATA_WORKER_GENERATION_LIMIT` | Общий предел одновременных запусков `egencache`; по умолчанию `2`. Задайте положительное целое число. |
| `GENTOO_UPDATE_INTELLIGENCE_ANALYSIS_CONCURRENCY_LIMIT` | Непустой override для `analysis.concurrency_limit` из YAML; `0` отключает предел. Смена env применяется через recreate. |
| `GENTOO_BASE_IMAGE` | Ссылка на подготовленную Gentoo-базу, только для сборки runtime. По умолчанию — `gentoo-update-intelligence/gentoo-base:local`. Если переменная задана в окружении shell, скрипт пропускает сборку базы. |
| `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG` | Необязательный JSON-каталог дополнительных доверенных overlays; по умолчанию пустой. |
| `GENTOO_UPDATE_INTELLIGENCE_METADATA_PROFILE` | Непустой override для `metadata.profile` в YAML. |

#### Окружение backend

Compose передаёт обязательный DSN `GENTOO_UPDATE_INTELLIGENCE_DATABASE_URL`, сформированный из `POSTGRES_PASSWORD`, и обычные env overrides. Если override не задан, он пуст и не перекрывает файл. Основные настройки контейнерного запуска находятся в YAML, а полный список соответствий — в [руководстве backend](backend-configuration.md#переменные-окружения-и-секреты).

При запуске backend вне Compose задайте DSN и при необходимости `--config /path/to/backend.yaml`. Без файла используются defaults для standalone: loopback, отключённые overlays и worker. Непустые прежние env сохраняют поддержку. Конфигурация проверяется до подключения к БД и восстановления очереди, но не требует готового Git mirror.

Сам metadata worker по-прежнему получает из Compose `METADATA_WORKER_LISTEN_ADDR=:8090`, `METADATA_WORKER_CACHE_PATH=/metadata-cache`, `METADATA_WORKER_TEMP_PATH=/var/tmp/gentoo-update-intelligence`, `METADATA_WORKER_PORTAGE_USER=portage` и `METADATA_WORKER_PORTAGE_GROUP=portage`. Эти параметры и положительный лимит генераций не входят в YAML backend. Именованный volume metadata-cache хранит подготовленные файлы; задания и результаты хранит PostgreSQL.

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

Если Gentoo revision пока недоступна, `POST /api/v1/analyses` всё равно сохранит задание и вернёт `202 Accepted`, а `GET` покажет `failed` и причину недоступности Provider. После публикации отправьте новое задание; перезапуск backend не требуется.

## Проверка API

`POST /api/v1/analyses` принимает план, сохраняет задание и возвращает `202 Accepted` с UUID, не ожидая завершения анализа. `GET /api/v1/analyses/<UUID>` показывает статусы задания `pending`, `running`, `completed` или `failed`. При `completed` ответ содержит `result`; внутри него отдельный metadata lookup может иметь статус `not_found`. При `failed` ответ содержит `error` с `code` и `message`. Поля `result` и `error` всегда присутствуют; неиспользуемое поле равно `null`.

Проверить отправку плана через frontend proxy можно командой:

```sh
curl -i -H 'Content-Type: application/json' \
  -d '{"plan":"[ebuild U] dev-libs/libxml2-2.14.6::gentoo [2.13.9::gentoo]"}' \
  http://127.0.0.1:8081/api/v1/analyses
```

Успешный ответ имеет статус `202 Accepted` и содержит `id`, `status` и `progress`. Чтобы проверить состояние, подставьте UUID из поля `id`:

```sh
curl -fsS 'http://127.0.0.1:8081/api/v1/analyses/<UUID>'
```

При другом `FRONTEND_PORT` замените порт в URL. Прямую ссылку на отчёт и его API можно открыть по адресам `http://127.0.0.1:8081/analyses/<UUID>` и `http://127.0.0.1:8081/api/v1/analyses/<UUID>`. Подробный контракт приведён в [документе HTTP API](http-api.md).

После перехода на API v1 пересоберите и пересоздайте backend и frontend вместе:

```sh
docker compose up -d --no-deps --build backend frontend
```

Команда предполагает, что зависимости уже запущены, а в `.env` задан действующий `POSTGRES_PASSWORD`. Прежние API-адреса больше не работают; сохранённые страницы `/analyses/<UUID>` остаются доступными. `Location` из POST теперь указывает на API `/api/v1/analyses/<UUID>`. Сначала откройте прежний отчёт, затем отправьте новый план. `start` и `restart` не заменяют сборку и пересоздание контейнеров.

## Сохранение данных и остановка

Compose использует именованные volumes: `gentoo-data` хранит Gentoo Git-репозиторий, `overlay-data` — загруженные overlays, `metadata-cache` — сгенерированные результаты, а `postgres-data` — задания и анализы. `gentoo-data` доступен backend и metadata-worker только для чтения. В `overlay-data` backend может записывать при синхронизации, а worker имеет доступ только для чтения. Worker не подключён к PostgreSQL.

Пересоздание контейнеров без удаления volumes сохраняет завершённые результаты по UUID. Backend восстанавливает незавершённые задания после перезапуска. Чтобы пересоздать контейнеры и открыть прежний отчёт, выполните:

```sh
docker compose up --build -d --force-recreate
```

Для уже созданного `postgres-data` используйте тот же пароль: изменение `POSTGRES_PASSWORD` не меняет пароль в существующей базе. Compose автоматически читает пароль из `.env` в корне проекта. Пример переменных приведён в [`.env.example`](../.env.example).

Команда `docker compose down` останавливает стек и сохраняет volumes. `docker compose down -v` удаляет volumes, включая задания и результаты анализов, Gentoo дерево, overlays и metadata cache.
