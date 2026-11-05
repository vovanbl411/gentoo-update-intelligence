# Приёмка Gentoo musl + GNU

Исторические сравнения сохранены в архиве: [компактный glibc](../2026-11-05-compact-gentoo-runtime/acceptance.md) и [musl + BusyBox](../2026-11-05-experiment-musl-busybox-runtime/acceptance.md).

Основной `Dockerfile.base` переведён на закреплённую Gentoo musl stage3. GNU-инструменты сохранены, BusyBox не устанавливается. Исторический glibc-runtime сохранён как `gentoo-update-intelligence/gentoo-runtime:glibc-baseline`, база — как `gentoo-update-intelligence/gentoo-base:glibc-baseline`.

Выполнен штатный workflow без чтения `.env`:

```sh
POSTGRES_PASSWORD=compose-validation-only sh scripts/build-gentoo-images.sh --env-file /dev/null -f compose.yaml
```

Snapshot и подпись проверены; Git 2.56.0-r1 собран из исходников. Экспорт содержит 104 пакета и необходимые ELF-библиотеки. Проверка sandbox использует `/usr/lib/libsandbox.so` и musl loader. Для сохранённой glibc-базы скрипт поддерживает прежний путь `/usr/lib64/libsandbox.so`.

| Образ | Размер, байт | Image ID |
|---|---:|---|
| Штатная база `gentoo-base:local` | 265 964 161 | `sha256:04cfb871faece736ee065a186e14c4950f8018c19ba7330b61bf712c126457f7` |
| Штатный runtime `gentoo-runtime:local` | 273 166 632 | `sha256:f338701bdef48c1fa7ffd7a3f89666d961221273fa8fff65ea246e1332e6d016` |
| Сохранённый `gentoo-runtime:glibc-baseline` | 340 105 258 | `sha256:b0c7ac37c05372faba324491767b73aa4b83c96a1abef62b1a80f600e185c2ff` |

Размеры относятся к локальным образам, а image ID не являются registry digest. Новый runtime примерно на 19,7% меньше компактного glibc-варианта.

В базе подтверждены musl loader, отсутствие исполняемых BusyBox и GCC. Команды возвращают GNU-версии: Bash 5.3.20, coreutils 9.12, grep 3.12, sed 4.10, findutils 4.11.0, tar 1.35 и gawk 5.3.2. Четыре теста выбора runtime-зависимостей прошли внутри новой базы с текущим exporter. HTTPS Git успешно прочитал HEAD публичного Gentoo-репозитория на GitHub.

Сборочная стадия основного Dockerfile.base собрана под тегом `gentoo-base-builder:test`, затем выполнено сравнение:

```sh
python3 gentoo/test-runtime.py --baseline-image gentoo-update-intelligence/gentoo-runtime:glibc-baseline
```

Пять результатов для `libei-1.6.0-r1`, `libxml2-2.15.4`, `glibc-2.44-r4`, `git-2.56.0` и `libei-1.6.0-r1::test-overlay` побайтово совпали с glibc-baseline. Подтверждены исторический поиск, cache reuse, sync, healthcheck, общий image ID, UID/GID `portage`, права рабочих каталогов, read-only `/metadata` и внутренняя сеть worker. Профиль анализа остался `profiles/default/linux/amd64/23.0`, для overlay — через Gentoo master; переход libc runtime не меняет профиль анализируемого пакета.

Сырые результаты находятся в `/tmp/guit-compact-test-0ineuc5o`, с отдельными `baseline/results.json`, `compact/results.json` и логами. Тестовые контейнеры и сети удалены. Существующие рабочие контейнеры не пересоздавались, их volumes не подключались. Полный стек с HTTP API и PostgreSQL повторно не запускался. Публикация в registry не выполнялась.

Ограничения покрытия ebuild, долговременного хранения исходников и побайтовой воспроизводимости остаются в силе. Эксперимент BusyBox завершён; его сборочные файлы и временные Docker-теги удалены, результаты сохранены в архиве OpenSpec.

После уборки повторно выполнен `python3 gentoo/test-runtime.py`: пять генераций metadata, sync, cache reuse, healthcheck, права и изоляция worker прошли. Результаты сохранены в `/tmp/guit-compact-test-76l_qp4n`. Сравнение с glibc в этом повторном запуске не выполнялось; результаты предыдущего сравнения приведены выше. Временные контейнеры и сети удалены.
