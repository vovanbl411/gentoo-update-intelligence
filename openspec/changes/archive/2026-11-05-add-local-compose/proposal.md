# Proposal

## Why

Локальный Compose обеспечивает полный путь от frontend до backend и PostgreSQL. После принятия автоматической подготовки metadata прежнее описание Compose стало неверным: оно требовало вручную подготовить Git snapshot и задавало синхронный HTTP-контракт.

## What Changes

- Зафиксировать Compose-запуск frontend, backend, PostgreSQL, Gentoo repository runtime и metadata worker.
- Описать автоматическую синхронизацию Gentoo и доверенных overlays, закрепление ревизий для заданий и подготовку недостающих metadata через worker согласно [действующей спецификации](../2026-11-05-add-automatic-metadata-preparation/specs/automatic-metadata-preparation/spec.md).
- Сохранить публикацию только frontend на loopback, API proxy со снятием `/api`, сетевые границы сервисов, а также постоянное хранение заданий в PostgreSQL.
- Согласовать локальную проверку с асинхронным API и уже выполненной приёмкой чистого Compose в `frontend/acceptance.md`.

## Capabilities

### New Capabilities
- `local-compose`: воспроизводимый локальный запуск и маршрутизация frontend, сетевые границы сервисов и сохранение заданий PostgreSQL.

### Modified Capabilities

Нет. Контракт подготовки metadata остаётся в `automatic-metadata-preparation`, а HTTP API — в `docs/http-api.md`.

## Impact

Затрагиваются Compose-конфигурация и документация локального запуска. HTTP API, автоматическая синхронизация и подготовка metadata описываются своими действующими контрактами и этим change не меняются.
