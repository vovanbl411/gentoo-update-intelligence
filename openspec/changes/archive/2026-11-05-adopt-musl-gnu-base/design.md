# Design

## Context

См. proposal.md. Musl + GNU в эксперименте содержал установленный, но не используемый BusyBox. Для штатной базы пакет не нужен. Профиль анализируемых пакетов задаётся запросом worker и не должен автоматически меняться вслед за libc runtime.

## Goals / Non-Goals

Сделать musl + GNU стандартной локальной сборкой worker/sync. Не менять команды на BusyBox и не переключать профиль анализа на musl. Не публиковать образы и не пересоздавать существующие рабочие контейнеры.

## Decisions

- Основной Dockerfile.base использует проверенный digest musl stage3 и прежний snapshot/SHA-256. Git собирается из исходников с прежними USE; BusyBox не устанавливается.
- Экспорт вызывается с `--libc musl`; glibc ldconfig исключён из этого Dockerfile.
- Общий exporter сохраняет glibc-режим для сравнения и отката. Проверка sandbox в скрипте выбирает путь библиотеки по наличию musl loader и проверяет GNU cp/grep/sed/find/tar/awk.
- До обновления local сохранить прежние base/runtime как `glibc-baseline`. Экспериментальные команды сравнения используют этот явный тег вместо изменяемого local.

## Risks / Trade-offs

- Не все ebuild проверены на musl runtime → сохранять явное ограничение покрытия и возможность сравнения с glibc.
- Local-тег меняется при сборке → существующие контейнеры не пересоздавать; glibc-baseline позволяет откатить выбор базы.
- Registry ещё не используется → локальные image ID не выдавать за скачиваемые digest.

## Migration Plan

Сохранить glibc-теги, выполнить стандартную сборку, проверить metadata и инструменты на временных данных. Для отката собрать runtime с `GENTOO_BASE_IMAGE=gentoo-update-intelligence/gentoo-base:glibc-baseline`. Рабочие volumes не изменять.
