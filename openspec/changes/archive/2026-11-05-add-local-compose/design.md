# Design

## Context

Compose запускает frontend, backend, PostgreSQL, Gentoo repository runtime и metadata worker. Gentoo runtime публикует синхронизированные Git mirrors в именованный volume; задания закрепляют использованные commits. Backend читает готовый cache и передаёт отсутствующие metadata worker. Подробный контракт синхронизации, закрепления ревизий и генерации задан в [automatic-metadata-preparation](../2026-11-05-add-automatic-metadata-preparation/specs/automatic-metadata-preparation/spec.md).

## Goals / Non-Goals

**Goals:**

- Запускать собранный frontend и обслуживать прямые ссылки на `/analyses/<uuid>`.
- Публиковать на хост только frontend и привязывать его порт к loopback.
- Проксировать `/api/` в backend со снятием префикса, сохраняя HTTP status, `Content-Type` и тело ответа.
- Сохранять задания и результаты в PostgreSQL между пересозданиями контейнеров.
- Использовать автоматически синхронизируемые и закрепляемые репозитории и отдельный metadata worker.

**Non-Goals:**

- Менять HTTP API или контракт подготовки metadata.
- Требовать от пользователя ручной snapshot, commit SHA или host path к Git-репозиторию.
- Публиковать backend, worker или PostgreSQL на host ports.
- Добавлять Redis, аутентификацию или публичное развёртывание.

## Decisions

### Сетевые границы

Frontend публикуется на `127.0.0.1`; backend и PostgreSQL не имеют host ports. Worker подключён к отдельной внутренней сети вместе с backend, а Gentoo runtime получает сетевой доступ для синхронизации. Backend ожидает готовности PostgreSQL и worker по healthcheck.

### Автоматическая подготовка metadata

Gentoo runtime хранит Git mirrors в именованных volumes и автоматически синхронизирует Gentoo. Backend и metadata worker читают эти данные; worker также читает overlay volume и сохраняет сгенерированные cache в отдельном volume. Backend закрепляет commits для задания. Пользовательские URLs из плана не используются для загрузки.

### Асинхронный HTTP API за frontend proxy

POST возвращает `202 Accepted` с UUID и начальным статусом задания. Клиенты читают прогресс и результат через GET. Nginx снимает префикс `/api` и не подменяет ответы backend страницей приложения.

### Постоянное хранение

PostgreSQL использует именованный volume. Пересоздание контейнеров сохраняет задания; удаление volume удаляет их.

## Risks / Trade-offs

- Первая синхронизация Git может занять продолжительное время и потребовать заметное место на диске.
- Удаление именованных volumes удаляет локальные repository mirrors, metadata cache и сохранённые задания.
- Worker обрабатывает ebuild через `egencache`; он отделён от backend, работает с ограниченным пользователем и capabilities, а сеть worker изолирована.

## Migration Plan

1. Задать пароль PostgreSQL и при необходимости параметры Gentoo source и лимит генерации.
2. Запустить Compose и дождаться публикации первой ревизии Gentoo.
3. Создать анализ через frontend или API; получить статус, прогресс и результат по UUID.
4. Пересоздать контейнеры без удаления PostgreSQL volume и проверить повторное открытие задания.
