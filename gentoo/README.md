# Gentoo runtime для worker и sync

Этот каталог содержит сборку Gentoo-окружения для двух сервисов проекта:

| Сервис Compose | Назначение |
|---|---|
| `gentoo` | Загружает Git-историю Gentoo и публикует проверенные ревизии для анализа |
| `metadata-worker` | Подготавливает metadata выбранных ebuild через Portage |

Оба сервиса запускаются из одного образа `gentoo-update-intelligence/gentoo-runtime:local` с разными командами и подключёнными данными. Штатная база — Gentoo **musl-hardened с GNU-инструментами**, без BusyBox. При последней приёмке база занимала около 268 MB, runtime — 275 MB; точные значения и покрытие проверок записаны в [acceptance.md](acceptance.md).

Подробный разбор стадий, закрепления версий, экспорта rootfs, прав и команды запуска находится в [build.md](build.md). Здесь — порядок сборки, запуска и повторения проверок.

## Собрать образы

Нужны Docker Engine с Compose plugin; для первой сборки — доступ к registry, Gentoo snapshot и исходникам пакетов. Целевая платформа — `linux/amd64`. Все команды выполняются **из корня репозитория**. Go и Portage на хост устанавливать не нужно: они используются внутри сборочных стадий.

```sh
export POSTGRES_PASSWORD='<your-local-password>'
sh scripts/build-gentoo-images.sh
```

Пароль PostgreSQL должен состоять из URL-safe символов. Он обязателен для разбора общей конфигурации Compose, даже если сейчас собираются только Gentoo-сервисы; сама база PostgreSQL этой командой не запускается.

При стандартной локальной сборке скрипт сначала собирает и проверяет базу, затем runtime:

| Образ | Как собирается |
|---|---|
| `gentoo-update-intelligence/gentoo-base:local` | Из [Dockerfile.base](Dockerfile.base): закреплённая stage3, snapshot, Git и runtime-зависимости |
| `gentoo-update-intelligence/gentoo-runtime:local` | Из [Dockerfile](Dockerfile): база, скомпилированный worker и sync-скрипты |

Первая сборка компилирует Git из исходников. Следующие сборки могут использовать кеш. Образы сохраняются локально; публикация в registry не выполняется. Скрипт не запускает рабочие сервисы. Другую базу задавайте через `GENTOO_BASE_IMAGE` в окружении shell, чтобы её одинаково учитывали скрипт и Compose; подробности — в [руководстве по сборке](build.md#использовать-другую-подготовленную-базу).

## Запустить worker и sync

В том же shell, где задан `POSTGRES_PASSWORD`:

```sh
docker compose up -d --no-build --pull never gentoo metadata-worker
docker compose ps gentoo metadata-worker
```

Эти команды запускают только два Gentoo-сервиса. Sync начинает загрузку истории в постоянный volume; первая загрузка может занять время и много места на диске. Проверьте публикацию ревизии:

```sh
docker compose exec gentoo git -C /metadata/gentoo.git rev-parse --verify refs/gentoo/published
```

Полный commit SHA означает, что проверенная ревизия доступна. Если ref ещё отсутствует, посмотрите `docker compose logs gentoo`, дождитесь синхронизации и повторите проверку. Healthy-статус worker сам по себе не подтверждает готовность Git-репозитория.

Полный стек с frontend, backend и PostgreSQL запускается по [руководству Compose](../docs/local-compose.md).

## Воспроизвести проверку runtime

После сборки образов нужны Python 3 на хосте и Compose с поддержкой `!override`, используемой тестовым файлом. Подготовьте builder с закреплёнными ebuild для fixture и запустите тест:

```sh
docker build --target build -f gentoo/Dockerfile.base -t gentoo-update-intelligence/gentoo-base-builder:test .
python3 gentoo/test-runtime.py
```

Тест проверяет sync, готовность и изоляцию worker, пять генераций metadata, исторический поиск ebuild, тестовый overlay и повторное использование кеша. Он использует временные данные, не подключает рабочие volumes и удаляет свои контейнеры и сети после завершения. Результаты и логи остаются в напечатанном каталоге `/tmp/guit-compact-test-*`.

Это повторение функциональной проверки на контролируемых данных, а не доказательство побайтовой воспроизводимости образа или поддержки всех исторических ebuild. Сравнение с другим образом, тесты exporter и ограничения сборки разобраны в [build.md](build.md#что-именно-проверяют-тесты).

## Где смотреть подробности

- [build.md](build.md) — что делает каждая стадия и скрипт, зачем нужны каталоги и права, как обновлять базу и выбрать сохранённую baseline-базу.
- [acceptance.md](acceptance.md) — проверенные версии, размеры, image ID и результаты приёмки.
- [runtime-packages](runtime-packages) и [portage/package.use/git](portage/package.use/git) — исходный состав runtime и настройки сборки Git.
- [Руководство Compose](../docs/local-compose.md) — полный запуск приложения и операторские настройки.
