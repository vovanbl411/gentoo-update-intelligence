## ADDED Requirements

### Requirement: Proxy сохраняет версионированный путь
Nginx и Vite SHALL передавать `/api/v1/analyses` в backend без снятия `/api`. Неизвестный API-путь SHALL давать JSON-ошибку backend вместо SPA HTML. Страница `/analyses/{id}` SHALL сохранять прямое открытие и refresh.

#### Scenario: Запрос v1 через proxy
- **WHEN** клиент отправляет POST `/api/v1/analyses`
- **THEN** backend получает тот же путь, а proxy сохраняет 202, Content-Type, Location и тело ответа

#### Scenario: Неизвестная версия
- **WHEN** клиент обращается к `/api/v2/analyses`
- **THEN** получает JSON 404, а не HTML frontend

## MODIFIED Requirements

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

