# Приёмка компактного glibc runtime

Исторический отчёт. Сейчас используется [musl + GNU](../../../../gentoo/acceptance.md). Указанные ниже команды относятся к выполненной приёмке; временный тег `before-compact` удалён при уборке.

Проверка выполнена на `linux/amd64`. Go-код, backend и frontend в этой задаче не менялись. Проверялся общий образ worker/sync; полный стек с HTTP API и PostgreSQL повторно не запускался.

## Размер и состав

Размеры получены через `docker image inspect`, в байтах; это размеры локальных образов, а не сжатой загрузки из registry.

| Образ | Размер |
|---|---:|
| Прежний runtime, тег `gentoo-update-intelligence/gentoo-runtime:before-compact` | 1 434 586 421 |
| Компактная база `gentoo-update-intelligence/gentoo-base:local` | 332 902 787 |
| Новый runtime `gentoo-update-intelligence/gentoo-runtime:local` | 340 105 258 |

Уменьшение runtime — примерно 76%. База содержит 99 выбранных пакетов и дополнительные ELF-библиотеки. В ней нет дерева `/var/db/repos/gentoo` и исполняемого GCC. В списках provenance сохранены выбранные пакеты и библиотеки.

Image ID базы: `sha256:653c90ce0e36f52360b47e4ac178d84b9a3e62e5675a46ed9568ed30ebc5d2b1`.

Image ID runtime: `sha256:b0c7ac37c05372faba324491767b73aa4b83c96a1abef62b1a80f600e185c2ff`.

Это локальные image ID, а не registry digest для скачивания образов.

## Сборка и проверки

Выполнен workflow без чтения `.env`, с тестовым значением обязательной переменной Compose:

```sh
POSTGRES_PASSWORD=compose-validation-only sh scripts/build-gentoo-images.sh --env-file /dev/null -f compose.yaml
```

Snapshot `gentoo-20261104.tar.xz` прошёл проверку SHA-256 и подписи ключами закреплённой stage3. Git 2.56.0-r1 собран из исходников, без binhost. После исправлений экспорт файлов пересобирался с использованием кеша установки Git. Полная побайтовая воспроизводимость повторной сборки с нуля не проверялась.

В ходе проверки генерации обнаружена ошибка загрузки `libgcc_s.so.1` в окружении sandbox. Исправление включает экспорт ELF-зависимостей и сгенерированной конфигурации `/etc/ld.so.conf.d`. Скрипт сборки теперь отдельно проверяет запуск Bash с `LD_PRELOAD=/usr/lib64/libsandbox.so` в настоящем контейнере. Portage при установке пакетов выдавал предупреждения `Unable to unshare: EPERM` для namespace sandbox; сборка завершилась успешно, privileged-режим и дополнительные capabilities для неё не включались.

Четыре fault-теста выбора зависимостей прошли: отсутствующая обязательная зависимость, выбор целой альтернативной ветки, игнорирование blocker и обработка цикла с USE-условиями. Запуск:

```sh
docker run --rm --network none -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$PWD/gentoo:/checks:ro" -w /checks --entrypoint python3 \
  gentoo-update-intelligence/gentoo-base-builder:test test_export_runtime.py
```

Для fixture отдельно собрана стадия `build` из Dockerfile.base с тегом `gentoo-update-intelligence/gentoo-base-builder:test`. После этого выполнено сравнение:

```sh
python3 gentoo/test-runtime.py --baseline-image gentoo-update-intelligence/gentoo-runtime:before-compact
```

Реальные ebuild, eclass и профили взяты из закреплённого snapshot. Тест не использует готовые md5-cache. Для исторического поиска создан дополнительный commit, удаляющий ebuild; тестовый overlay содержит неизменённый ebuild `libei` с master Gentoo. Это контролируемые Git-fixtures, а не полная история Gentoo или живой сторонний overlay.

| Запрос | Что проверено |
|---|---|
| `dev-libs/libei-1.6.0-r1` | Поиск удалённого ebuild в предыдущем commit и реальная генерация |
| `dev-libs/libxml2-2.15.4` | Реальная генерация |
| `sys-libs/glibc-2.44-r4` | Реальная генерация |
| `dev-vcs/git-2.56.0` | Реальная генерация |
| `dev-libs/libei-1.6.0-r1::test-overlay` | Реальная генерация с закреплённым master и его профилем |

Все пять содержимых metadata побайтово совпали с прежним runtime. Повторный запрос вернул сохранённый cache. Также подтверждены публикация ожидаемого commit sync-сервисом, одинаковый image ID двух сервисов, healthcheck, UID/GID `portage`, доступность рабочих каталогов, read-only `/metadata` и единственная внутренняя сеть worker.

Внешний HTTPS-доступ Git отдельно проверен командой:

```sh
docker run --rm --entrypoint git gentoo-update-intelligence/gentoo-base:local \
  ls-remote https://github.com/gentoo/gentoo.git HEAD
```

Команда успешно вернула ref. Worker во время metadata-проверок оставался во внутренней сети без внешнего доступа.

Сырые результаты последней приёмки сохранены локально в `/tmp/guit-compact-test-n9i7310s`: `baseline/results.json`, `compact/results.json`, логи и fixture revisions. Тестовые контейнеры и сети удалены; существующие volumes не подключались.

## Границы результата

Это проверка выбранных реальных пакетов и тестового overlay, а не подтверждение поддержки всех исторических ebuild. Долговременное хранение snapshot/distfiles, побайтовая воспроизводимость и публикация в Docker Hub остаются отдельными задачами. Сборочные стадии и их кеш занимают дополнительное место локально и не входят в конечный runtime.
