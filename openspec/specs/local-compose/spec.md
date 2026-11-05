# local-compose Specification

## Purpose
Задать воспроизводимый локальный запуск Gentoo Update Intelligence через Compose, маршрутизацию frontend, сетевые границы компонентов и сохранение заданий в PostgreSQL.

## Requirements

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
Frontend SHALL отдавать собранное приложение для прямого открытия `/analyses/<uuid>` и refresh этой страницы. Запросы `/api/` SHALL проксироваться в backend без снятия префикса `/api`, сохраняя HTTP status, `Content-Type` и тело ответа backend.

#### Scenario: Прямое открытие анализа
- **WHEN** пользователь открывает `/analyses/<uuid>` напрямую или обновляет страницу
- **THEN** frontend возвращает приложение, которое запрашивает тот же UUID через API

#### Scenario: Асинхронный ответ API
- **WHEN** backend принимает корректный POST `/analyses`
- **THEN** frontend proxy передаёт ответ `202 Accepted` с UUID и начальным состоянием задания без изменений

#### Scenario: Ошибочный JSON-ответ API
- **WHEN** backend возвращает JSON-ошибку для `/api/v1/analyses/<unknown>` через запрос frontend proxy `/api/v1/analyses/<unknown>`
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
- **THEN** задание восстанавливается согласно [контракту асинхронной подготовки metadata](../automatic-metadata-preparation/spec.md)

### Requirement: Настройка числа активных анализов
Compose SHALL передавать backend `GENTOO_UPDATE_INTELLIGENCE_ANALYSIS_CONCURRENCY_LIMIT`, по умолчанию `1`. Backend SHALL принимать неотрицательное целое, включая `0` без ограничения, и отклонять некорректное значение при запуске. Руководство SHALL объяснять применение нового значения через пересоздание backend и отличие от `METADATA_WORKER_GENERATION_LIMIT=2`.

#### Scenario: Изменение предела
- **WHEN** оператор задаёт положительное число или `0` и пересоздаёт backend
- **THEN** backend использует новый предел, сохраняя задания и результаты в существующем PostgreSQL volume

#### Scenario: Неверная настройка
- **WHEN** значение отрицательное, нечисловое или не помещается в целое число
- **THEN** backend завершается с явной ошибкой настройки до начала обработки заданий

### Requirement: Proxy сохраняет версионированный путь
Nginx и Vite SHALL передавать `/api/v1/analyses` в backend без снятия `/api`. Неизвестный API-путь SHALL давать JSON-ошибку backend вместо SPA HTML. Страница `/analyses/{id}` SHALL сохранять прямое открытие и refresh.

#### Scenario: Запрос v1 через proxy
- **WHEN** клиент отправляет POST `/api/v1/analyses`
- **THEN** backend получает тот же путь, а proxy сохраняет 202, Content-Type, Location и тело ответа

#### Scenario: Неизвестная версия
- **WHEN** клиент обращается к `/api/v2/analyses`
- **THEN** получает JSON 404, а не HTML frontend

### Requirement: YAML backend в Compose
Compose SHALL подключать BACKEND_CONFIG_PATH, по умолчанию `./config/backend.yaml`, read-only как `/etc/gentoo-update-intelligence/backend.yaml` и передавать --config. Отсутствие файла SHALL NOT создавать каталог. Обычные env SHALL передаваться как явные overrides без принудительных defaults. SOURCE_URL SHALL иметь приоритет над общим GENTOO_REPOSITORY_URL; DSN SHALL оставаться в env.

#### Scenario: Настройка через файл
- **WHEN** оператор меняет YAML и перезапускает backend
- **THEN** применяются настройки файла без пересборки, если они не перекрыты непустыми env

#### Scenario: Изменение env или источника файла
- **WHEN** оператор меняет env либо BACKEND_CONFIG_PATH
- **THEN** новое значение применяется через пересоздание backend; простой restart не объявляется способом изменения окружения или bind source
