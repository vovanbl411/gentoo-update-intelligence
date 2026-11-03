# Spec Delta

## Purpose

Задать воспроизводимый локальный запуск Gentoo Update Intelligence через Compose с ограниченной публикацией frontend, закреплённым metadata snapshot и сохранением результатов анализа в PostgreSQL.

## ADDED Requirements

### Requirement: Локальный Compose запускает полный стек с ограниченной публикацией
Compose SHALL запускать frontend, backend и PostgreSQL в общей внутренней сети. На хост SHALL публиковаться только frontend, привязанный к loopback; backend и PostgreSQL SHALL оставаться доступны только контейнерам стека.

#### Scenario: Запуск стека
- **WHEN** пользователь запускает Compose по инструкции
- **THEN** frontend, backend и PostgreSQL запускаются, а опубликованный host port принадлежит только frontend и привязан к loopback

### Requirement: Собранный frontend поддерживает прямые ссылки и API proxy
Frontend SHALL отдавать собранное приложение, обслуживать прямое открытие `/analyses/<uuid>` и refresh этой страницы. Запросы `/api` SHALL проксироваться в backend со снятием префикса `/api`, сохраняя HTTP-статус, `Content-Type` и тело ответа backend.

#### Scenario: Прямое открытие анализа
- **WHEN** пользователь открывает `/analyses/<uuid>` напрямую или обновляет эту страницу
- **THEN** frontend возвращает приложение, которое запрашивает тот же UUID через API

#### Scenario: Ответ API с ошибкой
- **WHEN** backend возвращает ошибочный JSON-ответ для `/analyses/<uuid>`
- **THEN** frontend proxy передаёт браузеру тот же статус и содержимое ответа API, не заменяя его HTML-страницей приложения

### Requirement: Backend использует явно закреплённый read-only metadata snapshot
Путь к подготовленному Git-снимку и полный commit SHALL задаваться конфигурацией запуска. Backend SHALL читать снимок через read-only mount и использовать заданный commit для запросов Metadata Provider.

#### Scenario: Lookup версий из подготовленного снимка
- **GIVEN** подготовленный снимок содержит cache для установленной и целевой версий `dev-libs/libei` из плана
- **WHEN** пользователь отправляет этот план через frontend
- **THEN** API возвращает для первой записи `dev-libs/libei` статусы `found` в `BeforeLookup` и `AfterLookup`

#### Scenario: Снимок недоступен для записи из backend
- **WHEN** backend работает с настроенным снимком
- **THEN** файловая система снимка смонтирована в контейнер только для чтения

### Requirement: PostgreSQL сохраняет анализы между пересозданиями контейнеров
PostgreSQL SHALL хранить анализы в именованном Compose volume. Пересоздание контейнеров при сохранении этого volume SHALL сохранять ранее записанные анализы и выданные UUID.

#### Scenario: Повторное открытие сохранённого анализа
- **GIVEN** frontend создал анализ и показал UUID
- **WHEN** контейнеры пересозданы с тем же именованным PostgreSQL volume и пользователь открывает `/analyses/<uuid>` напрямую
- **THEN** API возвращает прежний сохранённый результат, а frontend отображает его для того же UUID

### Requirement: Инструкция описывает запуск из чистого checkout
Инструкция SHALL описывать необходимые инструменты и параметры, подготовку локального Git-снимка с указанным commit, запуск Compose и проверку сохранения результата после пересоздания контейнеров.

#### Scenario: Подготовка окружения по инструкции
- **WHEN** пользователь начинает с чистого checkout и подготовленного metadata snapshot
- **THEN** инструкция позволяет настроить обязательные параметры, запустить стек и выполнить приёмочный сценарий без изменения HTTP-контракта
