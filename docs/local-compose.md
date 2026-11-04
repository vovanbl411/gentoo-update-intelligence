# Локальный запуск через Docker Compose

Для запуска нужны Docker Engine с Compose plugin. Snapshot Gentoo готовится отдельно: backend не загружает и не изменяет его.

## Подготовка snapshot

Укажите абсолютный путь к локальному Git-репозиторию Gentoo и полный commit SHA, который содержит нужные cache-файлы. Для приемочного плана из `testdata/parser/ebuild-updates.txt` commit должен содержать `metadata/md5-cache/dev-libs/libei-1.6.0` и `metadata/md5-cache/dev-libs/libei-1.6.0-r1`. Snapshot и Git-объекты должны быть доступны backend-контейнеру для чтения; backend читает их через Git CLI.

Если нужен воспроизводимый снимок из предыдущего приёмочного прогона, его commit указан в [отчёте frontend](../frontend/acceptance.md). Сам путь к локальному snapshot зависит от машины.

## Настройка и запуск

Создайте локальный env-файл и укажите параметры snapshot и пароль PostgreSQL:

```sh
cp .env.example .env
```

Задайте в `.env`:

- `GENTOO_UPDATE_INTELLIGENCE_SNAPSHOT_PATH` — абсолютный путь к подготовленному Git-репозиторию;
- `GENTOO_UPDATE_INTELLIGENCE_REPOSITORY` — логическое имя репозитория, например `gentoo`;
- `GENTOO_UPDATE_INTELLIGENCE_SOURCE_URL` — URL происхождения для provenance; Provider не проверяет его по сети;
- `GENTOO_UPDATE_INTELLIGENCE_COMMIT` — полный commit SHA, содержащий cache;
- `POSTGRES_PASSWORD` — локальный пароль из URL-safe символов;
- `FRONTEND_PORT` — необязательный loopback-порт frontend, по умолчанию `8081`.

Проверьте Compose-конфигурацию и запустите сервисы:

```sh
docker compose config -q
docker compose up --build -d
docker compose ps
```

Frontend доступен по адресу `http://127.0.0.1:8081` (или заданному `FRONTEND_PORT`). Только frontend публикует host port. Backend ждёт успешный PostgreSQL healthcheck и слушает `0.0.0.0:8080` внутри сети Compose. Snapshot подключён в backend как `/metadata` только для чтения. Именованный volume `postgres-data` сохраняет анализы при пересоздании контейнеров.

Проверить закреплённый commit, cache-файлы и режим mount можно из backend-контейнера:

```sh
docker compose exec backend sh -ec '
  git -C /metadata --no-replace-objects cat-file -t "$GENTOO_UPDATE_INTELLIGENCE_COMMIT"
  git -C /metadata --no-replace-objects cat-file -e "$GENTOO_UPDATE_INTELLIGENCE_COMMIT:metadata/md5-cache/dev-libs/libei-1.6.0"
  git -C /metadata --no-replace-objects cat-file -e "$GENTOO_UPDATE_INTELLIGENCE_COMMIT:metadata/md5-cache/dev-libs/libei-1.6.0-r1"
  mount | grep " /metadata " | grep -q "(ro,"
'
```

Первый вывод должен быть `commit`; две следующие команды должны завершиться с кодом `0`. Последняя проверяет, что mount `/metadata` read-only.

## Приёмка API и сохранения

1. Откройте `http://127.0.0.1:8081`, вставьте содержимое `testdata/parser/ebuild-updates.txt` в поле плана и укажите `dev-libs/libei` в поле локально патченных пакетов.
2. Отправьте план и сохраните выданный UUID. В отчёте для `dev-libs/libei` оба Lookup, `BeforeLookup` и `AfterLookup`, должны иметь статус `found`.
3. Сверьте GET-ответ API с результатом интерфейса:

   ```sh
   curl -fsS "http://127.0.0.1:8081/api/analyses/<UUID>"
   ```

4. Проверьте прямое открытие `http://127.0.0.1:8081/analyses/<UUID>` и обновление страницы.
5. Пересоздайте контейнеры, не удаляя volume, и снова откройте сохранённый UUID:

   ```sh
   docker compose up --build -d --force-recreate
   docker compose ps
   ```

   API и UI должны вернуть тот же результат. Запрос `GET /api/analyses/<UUID>` через frontend сохраняет HTTP status, `Content-Type` и тело backend; неизвестный UUID можно использовать для проверки ошибочного JSON-ответа.

Остановить Compose без удаления данных можно командой `docker compose down`. Не используйте `docker compose down -v`, если нужно сохранить анализы: она удаляет именованный volume.
