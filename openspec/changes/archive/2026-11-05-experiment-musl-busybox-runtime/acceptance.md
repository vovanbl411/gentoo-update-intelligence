# Приёмка musl + BusyBox

Проверено 5 ноября 2026 года на `linux/amd64` в рабочем дереве ветки `main`. Текущий glibc-runtime и штатные значения Compose не переключались; публикации в registry не было.

Это историческая приёмка эксперимента до внедрения musl + GNU. После завершения эксперимента сборочные файлы и временные Docker-теги удалены. Команды ниже фиксируют выполненные проверки и не являются действующей инструкцией запуска. Glibc-контроль сохранён под тегом `glibc-baseline`; текущее состояние описано в [приёмке runtime](../../../../gentoo/acceptance.md).

## Сборка

Выполнено:

```sh
sh gentoo/experimental/musl-busybox/build.sh
```

Официальная musl stage3 выбрана по amd64-манифесту `edb059ec2a434c504c1f778cff20865abdd40205bbb444ac61f9f9da3bf0aa2e`. Это отдельная Gentoo-база, не замена libc в glibc stage3. В ней Portage 3.0.82.2, Python 3.14.10 и musl 1.2.6-r1. Snapshot 20261104 прошёл проверку SHA-256 и подписи. Git 2.56.0-r1 и BusyBox 1.36.1-r4 скомпилированы из исходников, без binhost.

Для musl сохраняется `/etc/ld-musl-x86_64.path`, glibc ldconfig не используется. В обоих экспериментальных rootfs успешно запущены Git, Python и Bash с preload библиотеки sandbox. Исполняемого GCC нет. Сборочная стадия сохраняет GNU-инструменты; symlinks BusyBox создаются только в копии конечного rootfs.

## Размеры и образы

Измерено через `docker image inspect`; значения относятся к локальным образам.

| Образ | Размер, байт | Image ID |
|---|---:|---|
| Рабочий glibc runtime | 340 105 258 | `sha256:b0c7ac37c05372faba324491767b73aa4b83c96a1abef62b1a80f600e185c2ff` |
| База musl + GNU | 267 606 820 | `sha256:a5f0893cc582cc30feb3c43c5a2846eabed80c519c1b8127eb6d89eae75222f9` |
| База musl + BusyBox | 256 695 488 | `sha256:a4e56a9c4efec9162c9ec8e298d8a137f6d057adc094cc8c0987712f17d34993` |
| Runtime musl + GNU | 274 809 291 | `sha256:ea69e5ecb67c2ee11894a3668017dcb38cbded59348adb3d80874ef053dab637` |
| Runtime musl + BusyBox | 263 897 959 | `sha256:c6cda66d05c139a2604f371916424fb5d305a3b6d595acec789d29597e838e26` |

Это image ID исторических образов, не registry digest для скачивания. Архитектурные решения эксперимента описаны в [design.md](design.md).

## Metadata и эксплуатационные проверки

Выполнены обе команды проверки из README, с рабочим `gentoo-runtime:local` как baseline и `gentoo-musl-builder:experimental` как источником fixture. Проверяемые реальные ebuild и профили одинаковы для обоих runtime. Использован профиль анализа `profiles/default/linux/amd64/23.0`, а для overlay — тот же профиль через закреплённый Gentoo master. Среда выполнения musl не подменяла профиль анализируемого пакета.

В каждом варианте реально сгенерированы metadata для:

- `dev-libs/libei-1.6.0-r1` с поиском в предыдущем commit после искусственного удаления ebuild;
- `dev-libs/libxml2-2.15.4`;
- `sys-libs/glibc-2.44-r4`;
- `dev-vcs/git-2.56.0`;
- `dev-libs/libei-1.6.0-r1::test-overlay` с Gentoo master.

Все пять содержимых metadata совпали с glibc побайтово. Подтверждены cache reuse, публикация ожидаемого commit sync-сервисом, одинаковый image ID worker/sync, healthcheck, UID/GID `portage`, права рабочих каталогов, read-only `/metadata` и единственная внутренняя сеть worker.

Fixture использует неизменённые ebuild, eclass и профили snapshot, но контролируемую Git-историю удаления и созданный для теста overlay. Полная история Gentoo и живой сторонний overlay не загружались.

Сырые результаты сохранены локально:

- musl + GNU: `/tmp/guit-compact-test-47q3gxav`;
- musl + BusyBox: `/tmp/guit-compact-test-sk7ppsuc`.

В каждом каталоге есть `baseline/results.json`, `compact/results.json`, fixture revisions и логи. Временные контейнеры и сети удалены, существующие volumes не подключались.

HTTPS Git отдельно проверен:

```sh
docker run --rm --entrypoint git gentoo-update-intelligence/gentoo-base:musl-busybox \
  ls-remote https://github.com/gentoo/gentoo.git HEAD
```

Запрос успешно вернул HEAD публичного репозитория. Worker во время metadata-тестов оставался без внешней сети.

## Проверка общего кода

Четыре теста выбора зависимостей прошли в musl builder с текущим `export-runtime.py`:

```sh
docker run --rm --network none -e PYTHONDONTWRITEBYTECODE=1 \
  -v "$PWD/gentoo:/checks:ro" -w /checks --entrypoint python3 \
  gentoo-update-intelligence/gentoo-musl-builder:experimental test_export_runtime.py
```

С изменённым экспортом также заново собраны glibc-база и runtime под отдельными тегами `glibc-export-check`. Bash с sandbox запускается; все пять metadata-сценариев и побайтовое сравнение с рабочим glibc-runtime прошли. Результаты: `/tmp/guit-compact-test-gj8thilh`. Штатный тег `local` сохранил прежний image ID.

## Вывод и ограничения

Gentoo musl + BusyBox работоспособен на проверенном наборе metadata-сценариев и даёт дополнительное уменьшение размера. Это гибрид: BusyBox заменяет 153 существующие команды, но Bash и GNU-программы без заменяемых applets, включая gawk, остаются. База Portage описывает исходную выборку пакетов; после подмены `CONTENTS` не является точной инвентаризацией заменённых файлов.

Не подтверждены поддержка всех исторических ebuild, все режимы 153 заменённых команд, работа с произвольными overlays, смена профиля анализа на musl и побайтовая воспроизводимость сборки. Перевод штатного Compose и публикация в Docker Hub остаются отдельными решениями.

Официальные исходные сведения: [Gentoo Docker Images](https://github.com/gentoo/gentoo-docker-images), [Gentoo stage3 в Docker Hub](https://hub.docker.com/r/gentoo/stage3).
