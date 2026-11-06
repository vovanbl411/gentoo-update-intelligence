# http-api Specification

## Purpose
Задать публичный транспортный контракт HTTP API v1 независимо от доменных Go-структур и формата хранения результатов.

## Requirements

### Requirement: Версионированные маршруты
Backend SHALL обслуживать POST `/api/v1/analyses` и GET `/api/v1/analyses/{id}`. Успешный POST SHALL сохранять задание перед ответом `202` и возвращать Location `/api/v1/analyses/{id}`. Прежние API-маршруты SHALL NOT обслуживаться или перенаправляться. GET SHALL только читать сохранённый результат.

#### Scenario: Создание и получение
- **WHEN** клиент отправляет корректный план и опрашивает выданный UUID через v1
- **THEN** получает `202` с UUID, затем состояние и сохранённый результат без повторного запуска анализа при GET

#### Scenario: Старые адреса
- **WHEN** клиент использует `/analyses` в backend или `/api/analyses` через proxy
- **THEN** API отвечает `404` и не создаёт задание

### Requirement: Явные DTO и пустые значения
API SHALL использовать явные DTO с snake_case, сохраняя существующую вложенность и данные. Списки SHALL быть массивами, включая `[]`, словари — объектами, включая `{}`, отсутствующие объекты и необязательные значения — `null`. Значения Gentoo, кодов правил, metadata content и provenance SHALL сохраняться без переименования.

#### Scenario: Неполные данные и evidence
- **WHEN** результат содержит пустые коллекции, отсутствующие значения, пустую строку или индекс `0`
- **THEN** DTO сохраняет различия, порядок и ссылки evidence; все пять действующих правил представлены типизированными evidence

### Requirement: Envelope задания
POST и GET SHALL возвращать id, status, progress, result и error. Progress SHALL содержать stage, completed и total. У completed SHALL быть result и error null, у failed — result null и структурированная error; у pending/running оба поля SHALL быть null.

#### Scenario: Состояния задания
- **WHEN** клиент получает pending, running, completed или failed
- **THEN** envelope имеет одну схему, а result/error соответствуют состоянию

### Requirement: Структурированные ошибки
HTTP-ошибка SHALL иметь error с code и message. Коды SHALL включать INVALID_REQUEST (400), UNSUPPORTED_MEDIA_TYPE (415), ANALYSIS_NOT_FOUND и NOT_FOUND (404), METHOD_NOT_ALLOWED (405 с Allow), STORAGE_UNAVAILABLE (503), INVALID_STORED_RESULT и INTERNAL_ERROR (500). Ошибка задания SHALL иметь ANALYSIS_FAILED, Lookup — METADATA_LOOKUP_FAILED.

#### Scenario: Разные уровни ошибки
- **WHEN** запрос отклонён, задание failed или не удался отдельный Lookup
- **THEN** клиент получает соответствующий код и сообщение на нужном уровне, без потери остальных частей отчёта

### Requirement: Совместимость сохранённых результатов
GET v1 SHALL читать прежние результаты из analysis_jobs и analyses через адаптер без повторного анализа, изменения UUID или перезаписи результата. Доменные JSON-теги, CLI и внутренний worker protocol SHALL сохраняться.

#### Scenario: Старый отчёт без Provider
- **WHEN** клиент открывает UUID прежнего результата, а Provider недоступен
- **THEN** API возвращает completed с DTO v1 и сохранёнными evidence/provenance, не обращаясь к Provider

### Requirement: Правила развития v1
Удаление, переименование, изменение типа или смысла существующего поля SHALL требовать новой major-версии. Клиенты SHALL допускать дополнительные поля; несовместимое расширение состояний SHALL NOT вводиться в v1.

#### Scenario: Дополнительное поле
- **WHEN** ответ получает новое совместимое необязательное поле
- **THEN** клиент продолжает обрабатывать известные поля

### Requirement: Компактное представление задания
API SHALL предоставлять GET `/api/v1/analyses/{id}/report` с envelope задания и компактным результатом. Компактный результат SHALL сохранять факты плана, findings, evidence, metadata diff, ограничения и исходы Lookup, исключая snapshots, provenance и повторяющиеся диагностики Parser в отчётах пакетов.

#### Scenario: Завершённое задание
- **WHEN** клиент запрашивает компактный результат завершённого задания
- **THEN** API возвращает сохранённые факты без повторного анализа и без полных snapshots и provenance

#### Scenario: Ожидание и ошибка
- **WHEN** клиент запрашивает незавершённое или failed задание
- **THEN** API возвращает состояние, прогресс и ошибку по правилам envelope v1

#### Scenario: Прежний результат
- **WHEN** клиент запрашивает сохранённый до изменения UUID
- **THEN** API возвращает компактное представление без изменения сохранённого результата

### Requirement: Промежуточный отчёт
API SHALL предоставлять отдельный GET `/api/v1/analyses/{id}/partial-report` для чтения сохранённых фактов Parser и Analyzer, готовых отчётов и индексов ожидающих metadata. Прежние маршруты и смысл их полей SHALL оставаться неизменными.

#### Scenario: Подготовка metadata
- **WHEN** Parser и Analyzer завершены, а подготовка metadata продолжается
- **THEN** маршрут возвращает факты, готовые отчёты и явно обозначенные ожидающие записи с текущим состоянием и прогрессом

#### Scenario: Ранее сохранённый результат
- **WHEN** запрошен завершённый анализ без промежуточного отчёта
- **THEN** маршрут возвращает итоговые сохранённые данные без повторного анализа
