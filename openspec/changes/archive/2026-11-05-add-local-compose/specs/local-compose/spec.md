# Spec Delta

## Purpose

Задать воспроизводимый локальный запуск Gentoo Update Intelligence через Compose, маршрутизацию frontend, сетевые границы компонентов и сохранение заданий в PostgreSQL.

## ADDED Requirements

### Requirement: Локальный Compose запускает полный стек
Compose SHALL запускать собранный frontend, backend, PostgreSQL, Gentoo repository runtime и metadata worker. На хост SHALL публиковаться только frontend, привязанный к loopback. Backend, metadata worker и PostgreSQL SHALL оставаться без опубликованных host ports.

#### Scenario: Запуск стека
- **WHEN** пользователь запускает Compose по инструкции
- **THEN** frontend, backend и PostgreSQL запускаются после готовности зависимостей, а Gentoo runtime синхронизирует репозиторий в постоянный volume

### Requirement: Metadata worker изолирован от внешней сети
Metadata worker SHALL работать в отдельной внутренней сети с backend, использовать закреплённые репозитории и ограниченные capabilities. Compose SHALL предоставлять Gentoo runtime сетевой доступ для синхронизации.

#### Scenario: Подготовка metadata
- **WHEN** задание запрашивает metadata, отсутствующие в закреплённом repository cache
- **THEN** backend обращается к worker через внутреннюю сеть, а worker использует закреплённые исходники и сохраняет результат в metadata cache volume

### Requirement: Gentoo repository volume доступен backend и worker только для чтения
Compose SHALL монтировать volume `gentoo-data` в backend и metadata worker только для чтения. Это требование не распространяется на `overlay-data`, который backend использует для обновления overlays.

#### Scenario: Чтение Gentoo repository без записи
- **GIVEN** backend и metadata worker запущены с подключённым `gentoo-data`
- **WHEN** каждый контейнер читает опубликованную Git revision или пытается записать в `gentoo-data`
- **THEN** чтение разрешено, а запись из обоих контейнеров запрещена

### Requirement: Frontend обслуживает ссылки на анализ и проксирует API
Frontend SHALL отдавать собранное приложение для прямого открытия `/analyses/<uuid>` и refresh этой страницы. Запросы `/api/` SHALL проксироваться в backend со снятием префикса `/api`, сохраняя HTTP status, `Content-Type` и тело ответа backend.

#### Scenario: Прямое открытие анализа
- **WHEN** пользователь открывает `/analyses/<uuid>` напрямую или обновляет страницу
- **THEN** frontend возвращает приложение, которое запрашивает тот же UUID через API

#### Scenario: Асинхронный ответ API
- **WHEN** backend принимает корректный POST `/analyses`
- **THEN** frontend proxy передаёт ответ `202 Accepted` с UUID и начальным состоянием задания без изменений

#### Scenario: Ошибочный JSON-ответ API
- **WHEN** backend возвращает JSON-ошибку для `/analyses/<unknown>` через запрос frontend proxy `/api/analyses/<unknown>`
- **THEN** proxy передаёт исходные HTTP status, `Content-Type` и тело ответа, не заменяя JSON страницей приложения

### Requirement: PostgreSQL сохраняет задания между пересозданиями
PostgreSQL SHALL хранить задания и результаты в именованном volume. Пересоздание контейнеров без удаления volume SHALL сохранять результаты для повторного чтения по прежнему UUID.

#### Scenario: Повторное открытие результата
- **GIVEN** задание завершено и сохранено в PostgreSQL
- **WHEN** Compose-контейнеры пересозданы без удаления volume и клиент запрашивает прежний UUID
- **THEN** API возвращает сохранённый статус и результат

#### Scenario: Восстановление незавершённого задания
- **GIVEN** задание имеет статус `pending` или `running` перед перезапуском backend
- **WHEN** backend запускается снова
- **THEN** задание восстанавливается согласно [контракту асинхронной подготовки metadata](../../../../../specs/automatic-metadata-preparation/spec.md)
