# Локальный запуск через Docker Compose

Для запуска нужны Docker Engine с Compose plugin и доступ к registry и источникам Gentoo. Gentoo runtime загружает основное дерево в именованный volume `gentoo-data` при запуске и повторяет синхронизацию через три часа после завершения предыдущей попытки. Первая загрузка полной Git-истории может занять продолжительное время и значительное место на диске.

## Запуск

Задайте локальный пароль PostgreSQL из URL-safe символов и запустите стек:

```sh
export POSTGRES_PASSWORD='<your-local-password>'
docker compose config -q
docker compose up --build -d
docker compose ps
```

Команды запускайте из корня репозитория. Для уже созданного `postgres-data` используйте тот же пароль: изменение `POSTGRES_PASSWORD` в окружении не меняет пароль в существующей базе. При запуске из нового shell задайте пароль снова.

Frontend доступен на `http://127.0.0.1:8081`. При необходимости задайте `FRONTEND_PORT`; только frontend публикует host port. Backend ждёт PostgreSQL и metadata-worker. Backend и worker читают закреплённые Git-зеркала через read-only mounts. Worker не подключён к PostgreSQL; отдельный volume `metadata-cache` хранит сгенерированные результаты. Именованный volume `postgres-data` сохраняет анализы при пересоздании контейнеров.

`metadata-worker` объединяет одновременно поступившие одинаковые запросы подготовки, включая создание временных checkout. Переменная `METADATA_WORKER_GENERATION_LIMIT` задаёт общий для всех анализов предел одновременных запусков `egencache`; по умолчанию он равен `2`. Значение должно быть положительным целым числом. После его изменения пересоздайте worker командой `docker compose up -d metadata-worker`.

По умолчанию используется `https://anongit.gentoo.org/git/repo/gentoo.git`. При необходимости источник можно задать параметром `GENTOO_REPOSITORY_URL`; он применяется к синхронизации и provenance. Backend сначала читает точный cache из закреплённого commit. Если записи нет, он обращается к `metadata-worker`, который ищет ebuild в закреплённой истории и генерирует cache в общей Gentoo-базе.

Например, для [зеркала Gentoo на GitHub](https://github.com/gentoo/gentoo) перед запуском задайте:

```sh
export GENTOO_REPOSITORY_URL='https://github.com/gentoo/gentoo.git'
```

Если стек уже запущен, после изменения переменной выполните `docker compose up -d`, чтобы обновить конфигурацию и Gentoo runtime, и backend. Перед последующими командами Compose в новом shell задайте выбранный URL снова; простой `restart` не меняет окружение существующего контейнера.

Встроенный каталог содержит `guru` с master `gentoo`. При первом запросе backend загружает overlay в именованный volume `overlay-data`. Перед последующим анализом он пытается обновить локальную копию и публикует только проверенный commit. При ошибке обновления используется последняя успешная публикация; уже начатый анализ сохраняет свои ревизии. Источник и master GURU указаны в [каталоге Gentoo](https://raw.githubusercontent.com/gentoo/api-gentoo-org/master/files/overlays/repositories.xml) и [конфигурации GURU](https://raw.githubusercontent.com/gentoo-mirror/guru/master/metadata/layout.conf).

Для других overlays оператор задаёт доверенный JSON-каталог через `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG`. Например, замените адреса и masters на соответствующие вашим репозиториям:

```sh
export GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG='[{"name":"gentoo-zh","url":"https://example.invalid/gentoo-zh.git","masters":["gentoo"]},{"name":"umbriel-overlay","url":"https://example.invalid/umbriel-overlay.git","masters":["gentoo"]}]'
```

Compose передаёт переменную backend. Для локального запуска backend задайте её вместе с `GENTOO_UPDATE_INTELLIGENCE_OVERLAY_PATH`. Поддерживаются HTTPS URL без учётных данных и абсолютные локальные пути, доступные backend; локальный путь внутри Compose должен быть смонтирован в контейнер. Адреса из плана не используются. Повторное имя, включая встроенное, считается неоднозначным; неизвестные и неоднозначные имена указываются в ошибке соответствующей стороны отчёта. Неверный каталог или ссылка на неизвестный master препятствуют запуску backend. После изменения каталога пересоздайте backend через `docker compose up -d`. Если URL уже загруженного overlay изменён, backend сообщает о несовпадении источника и не использует прежнюю копию под новым адресом.

Первая загрузка overlay выполняется во время обработки задания и может занимать несколько минут. `POST /analyses` быстро сохраняет задание и возвращает `202 Accepted`; получение metadata и анализ продолжаются асинхронно. Frontend proxy ожидает ответ POST не более часа.

Дождитесь первой успешной публикации перед отправкой плана. Если Gentoo revision ещё недоступна, POST всё равно сохранит задание и вернёт `202 Accepted`, а GET покажет `failed` с причиной недоступности Provider. После публикации отправьте новое задание: перезапуск backend не требуется. Проверить синхронизацию и опубликованный commit можно командами:

```sh
docker compose logs gentoo
docker compose exec gentoo git -C /metadata/gentoo.git rev-parse --verify refs/gentoo/published
```

Вторая команда выводит полный commit. При сбое следующей синхронизации ссылка остаётся на последней успешно проверенной ревизии. Анализ использует commit, выбранный при его начале. Опубликованные commits сохраняются в Git refs; автоматическая очистка хранилища не предусмотрена.

Если публикации ещё нет, `rev-parse` завершится с ошибкой; проверьте журнал `gentoo` и дождитесь окончания загрузки и проверки. При неудачной попытке runtime повторит её через три часа. Чтобы повторить завершившуюся с ошибкой попытку сразу, перезапустите только этот сервис:

```sh
docker compose restart gentoo
```

## Проверка API и сохранения

1. Откройте frontend, вставьте текст плана и отправьте его. Ответ `POST /analyses` имеет статус `202 Accepted` и содержит UUID задания со статусом `pending`.
2. Frontend запрашивает `GET /analyses/<UUID>` и показывает прогресс. Статусы задания: `pending`, `running`, `completed` и `failed`; итоговый `result` доступен при `completed`, а причина ошибки — при `failed`.
3. Сохраните UUID и откройте `http://127.0.0.1:8081/analyses/<UUID>` напрямую или запросите состояние через `http://127.0.0.1:8081/api/analyses/<UUID>`.
4. Пересоздайте контейнеры командой `docker compose up --build -d --force-recreate` и снова откройте тот же UUID. Незавершённые задания backend восстанавливает после перезапуска, а завершённые результаты остаются в PostgreSQL volume.

Проверить POST без браузера можно после публикации commit:

```sh
curl -i -H 'Content-Type: application/json' \
  -d '{"plan":"[ebuild U] dev-libs/libxml2-2.14.6::gentoo [2.13.9::gentoo]"}' \
  http://127.0.0.1:8081/api/analyses
```

При другом `FRONTEND_PORT` замените порт в URL. Успешный ответ имеет статус `202 Accepted` и содержит `id`, `status` и `progress`. Чтобы следить за заданием до статуса `completed` или `failed`, задайте `ANALYSIS_ID` равным UUID из ответа POST:

```sh
curl -fsS "http://127.0.0.1:8081/api/analyses/${ANALYSIS_ID}"
```

Итоговый ответ содержит `result`; отдельный Lookup может иметь статус `not_found`, если нужные данные недоступны. Контракт статусов и полей API описан в [документе HTTP API](http-api.md).

Остановить Compose без удаления данных можно командой `docker compose down`. Команда `docker compose down -v` удаляет volumes, включая сохранённые анализы и загруженное дерево.
