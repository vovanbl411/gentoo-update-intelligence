# Приёмка Gentoo musl-hardened + GNU

Проверено 2026-11-05 для `linux/amd64`. Штатная база переведена на `gentoo/stage3:musl-hardened` с amd64 digest `sha256:44605d8ddaa109d6f60c737713fefe10fa97ea44d50f27fd17c8e25a9c678cf4`. В builder подтверждены профиль `default/linux/amd64/23.0/musl/hardened`, `PROFILE_IS_HARDENED=1` и USE `hardened cet`. GNU-инструменты сохранены; BusyBox, GCC и дерево Gentoo в конечный runtime не включены. Init-система не запускается.

Предыдущие отчёты: [musl + GNU до hardened](../openspec/changes/archive/2026-11-05-adopt-musl-gnu-base/acceptance.md), [компактный glibc](../openspec/changes/archive/2026-11-05-compact-gentoo-runtime/acceptance.md), [эксперимент BusyBox](../openspec/changes/archive/2026-11-05-experiment-musl-busybox-runtime/acceptance.md).

## Сборка и размеры

Сначала base/runtime собраны под отдельными candidate-тегами. Проверка подписи snapshot прошла; Git 2.56.0-r1 скомпилирован из исходников. Дата snapshot, checksum, настройки Git и список runtime-пакетов сохранены. Экспорт содержит 104 пакета и необходимые ELF-библиотеки.

После функциональной приёмки выполнен штатный workflow без чтения `.env`:

```sh
POSTGRES_PASSWORD=compose-validation-only sh scripts/build-gentoo-images.sh --env-file /dev/null -f compose.yaml
```

| Образ | Размер, байт | Image ID |
|---|---:|---|
| Штатная база `gentoo-base:local` | 267 749 890 | `sha256:c68d890c610d8dbbeaab6ef717838b01005e829fca0030a2051406f5e7edd49c` |
| Штатный runtime `gentoo-runtime:local` | 274 952 361 | `sha256:9f4c164f55e7c7c23d28587f0e1a34ff4e3eb7a8ac7395cbe2b7f8c9d01bd3c9` |
| Прежний `gentoo-base:musl-baseline` | 265 964 161 | `sha256:04cfb871faece736ee065a186e14c4950f8018c19ba7330b61bf712c126457f7` |
| Прежний `gentoo-runtime:musl-baseline` | 273 166 632 | `sha256:f338701bdef48c1fa7ffd7a3f89666d961221273fa8fff65ea246e1332e6d016` |

Namespace всех тегов — `gentoo-update-intelligence`. Новый runtime больше прежнего на 1 785 729 байт, около 0,65%. Размеры относятся к локальным образам; image ID не являются registry digest. Baseline-пара сохранена локально для отката, glibc-baseline также оставлен.

Проверенный candidate runtime имел ID `sha256:08150a6ae68501cf9f973987ca1c1175c7981c8db9682fd9bd818ef03e4b2da8`. У результата штатной Compose-сборки подтверждено полное совпадение `RootFS` и `Config` с кандидатом; image ID отличается. Штатная сборка использовала кеш кандидата и не является отдельной проверкой сборки с нуля.

## Измеренные ELF-свойства

[`check-hardening.py`](check-hardening.py) прочитал десять экспортированных программ: Git, Bash, Python, cp, grep, sed, find, tar, gawk и wget. Проверка выполнялась через `readelf` в builder, а не добавлением binutils в runtime. Исходные JSON: [обычная musl-база](../openspec/changes/archive/2026-11-05-adopt-musl-hardened-base/baseline-elf.json) и [musl-hardened](../openspec/changes/archive/2026-11-05-adopt-musl-hardened-base/hardened-elf.json).

| Свойство | musl-baseline | musl-hardened |
|---|---:|---:|
| PIE | 10/10 | 10/10 |
| Неисполняемый GNU_STACK | 10/10 | 10/10 |
| GNU_RELRO и BIND_NOW | 10/10 | 10/10 |
| Символ `__stack_chk_fail` или `__stack_chk_fail_local` | 2/10 | 9/10 |
| CET IBT / SHSTK notes | 0/10 | 0/10 |

Символ stack protector появился дополнительно у Bash, cp, find, gawk, grep, sed и wget; у Git и tar он уже был. У Python символ не обнаружен. Наличие символа не доказывает защиту каждой функции, а отсутствие символа не является исчерпывающим аудитом защиты программы. Переход не добавил измеренных CET notes, несмотря на USE `cet`.

`--require-basic` прошёл. Два теста ELF-анализатора прошли на C-файлах с явно включёнными и отключёнными защитами; тестовые бинарники только компилировались и читались, без запуска. Hardened-профиль C-окружения не меняет отдельную сборку Go worker и защитные настройки ядра хоста.

## Совместимость

Успешно проверены Git, Python 3.14.10, `egencache`, сертификаты, musl loader, Bash с preload `/usr/lib/libsandbox.so` и GNU-версии `cp`, `grep`, `sed`, `find`, `tar`, `awk`. Подтверждено отсутствие исполняемых GCC и BusyBox и стандартного дерева Gentoo. Git по HTTPS прочитал HEAD публичного Gentoo-репозитория на GitHub. Четыре теста exporter прошли в hardened builder.

Сравнение выполнено до обновления local-тегов:

```sh
python3 gentoo/test-runtime.py \
  --image gentoo-update-intelligence/gentoo-runtime:hardened-candidate \
  --builder-image gentoo-update-intelligence/gentoo-base-builder:hardened-check \
  --baseline-image gentoo-update-intelligence/gentoo-runtime:local
```

Пять результатов для `libei-1.6.0-r1`, `libxml2-2.15.4`, `glibc-2.44-r4`, `git-2.56.0` и `libei-1.6.0-r1::test-overlay` побайтово совпали с прежним musl-runtime. Прошли исторический поиск, cache reuse, sync, healthcheck, общий image ID, UID/GID `portage`, права рабочих каталогов, read-only `/metadata` и внутренняя сеть worker. Профиль анализа остался `profiles/default/linux/amd64/23.0`; overlay использует Gentoo master.

Сырые результаты и логи находятся в `/tmp/guit-compact-test-ks9t8obl`, отдельно для baseline и кандидата. Временные контейнеры и сети удалены; candidate-теги убраны после обновления штатных local-тегов и `gentoo-base-builder:test`.

Рабочие контейнеры не пересоздавались, их volumes не подключались. Полный стек с HTTP API и PostgreSQL повторно не запускался; публикация в registry не выполнялась. Проверка десяти ELF и пяти metadata-сценариев не подтверждает поддержку всех исторических ebuild, отсутствие уязвимостей или побайтовую воспроизводимость сборки. Ограничения хранения snapshot и distfiles описаны в [build.md](build.md#что-закреплено-и-как-обновлять-базу).
