# Локальный запуск через Docker Compose

Для запуска нужны Docker Engine с Compose plugin и доступ к registry и источникам Gentoo. Gentoo runtime загружает основное дерево в именованный volume `gentoo-data` при запуске и повторяет синхронизацию через пять часов после завершения предыдущей попытки. Первая загрузка полной Git-истории может занять продолжительное время и значительное место на диске.

## Запуск

Задайте локальный пароль PostgreSQL из URL-safe символов и запустите стек:

```sh
export POSTGRES_PASSWORD='<your-local-password>'
docker compose config -q
docker compose up --build -d
docker compose ps
```

Команды запускайте из корня репозитория. Для уже созданного `postgres-data` используйте тот же пароль: изменение `POSTGRES_PASSWORD` в окружении не меняет пароль в существующей базе. При запуске из нового shell задайте пароль снова.

Frontend доступен на `http://127.0.0.1:8081`. При необходимости задайте `FRONTEND_PORT`; только frontend публикует host port. Backend ждёт PostgreSQL и читает volume Gentoo через mount только для чтения. Именованный volume `postgres-data` сохраняет анализы при пересоздании контейнеров.

По умолчанию используется `https://anongit.gentoo.org/git/repo/gentoo.git`. При необходимости источник можно задать параметром `GENTOO_REPOSITORY_URL`; он применяется к синхронизации и provenance. Проверенный commit `e99406921148088a6990e6b921440a2808a223a6` этого Git-источника не содержит `metadata/md5-cache`: текущий backend читает готовый cache и не запускает генерацию metadata, поэтому Lookup в таком снимке возвращает `not_found`.

Например, для [зеркала Gentoo на GitHub](https://github.com/gentoo/gentoo) перед запуском задайте:

```sh
export GENTOO_REPOSITORY_URL='https://github.com/gentoo/gentoo.git'
```

Если стек уже запущен, после изменения переменной выполните `docker compose up -d`, чтобы обновить конфигурацию и Gentoo runtime, и backend. Перед последующими командами Compose в новом shell задайте выбранный URL снова; простой `restart` не меняет окружение существующего контейнера.

Каталог известных overlays пока содержит `guru` с master `gentoo`. При первом запросе плана с `::guru` backend загружает его в отдельный именованный volume `overlay-data`; последующие анализы используют локальную копию. Источник и master проверены по [каталогу Gentoo](https://raw.githubusercontent.com/gentoo/api-gentoo-org/master/files/overlays/repositories.xml) и [конфигурации GURU](https://raw.githubusercontent.com/gentoo-mirror/guru/master/metadata/layout.conf). Автоматическое обновление overlay пока не реализовано; неизвестные имена не разрешаются в сетевые адреса.

Первая загрузка overlay выполняется во время синхронного POST и может занимать несколько минут; timeout frontend proxy составляет один час. Асинхронная обработка с прогрессом относится к следующему этапу.

До первой успешной публикации POST возвращает `503`; сохранённые анализы доступны через GET. После публикации повторите POST: перезапуск backend не требуется. Проверить синхронизацию и опубликованный commit можно командами:

```sh
docker compose logs gentoo
docker compose exec gentoo git -C /metadata/gentoo.git rev-parse --verify refs/gentoo/published
```

Вторая команда выводит полный commit. При сбое следующей синхронизации ссылка остаётся на последней успешно проверенной ревизии. Анализ использует commit, выбранный при его начале. Опубликованные commits сохраняются в Git refs; автоматическая очистка хранилища не предусмотрена.

Если публикации ещё нет, `rev-parse` завершится с ошибкой; проверьте журнал `gentoo` и дождитесь окончания загрузки и проверки. При неудачной попытке runtime повторит её через пять часов. Чтобы повторить завершившуюся с ошибкой попытку сразу, перезапустите только этот сервис:

```sh
docker compose restart gentoo
```

## Проверка API и сохранения

1. Откройте frontend, вставьте текст плана и отправьте его. Текущий API возвращает результат синхронно; если cache для версии отсутствует, результат содержит соответствующий статус.
2. Сохраните UUID и откройте `http://127.0.0.1:8081/analyses/<UUID>` напрямую или запросите `http://127.0.0.1:8081/api/analyses/<UUID>`.
3. Пересоздайте контейнеры командой `docker compose up --build -d --force-recreate` и снова откройте тот же UUID.

Проверить POST без браузера можно после публикации commit:

```sh
curl -fsS -H 'Content-Type: application/json' \
  -d '{"plan":"[ebuild U] dev-libs/libxml2-2.14.6::gentoo [2.13.9::gentoo]"}' \
  http://127.0.0.1:8081/api/analyses
```

При другом `FRONTEND_PORT` замените порт в URL. Успешный ответ содержит `id` и `result`; статус `not_found` для Lookup возможен, если в опубликованном commit нет cache запрошенной версии.

Остановить Compose без удаления данных можно командой `docker compose down`. Команда `docker compose down -v` удаляет volumes, включая сохранённые анализы и загруженное дерево.
