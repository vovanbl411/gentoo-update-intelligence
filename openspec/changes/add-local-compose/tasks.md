# Tasks

## 1. Собрать frontend и настроить раздачу маршрутов

- [ ] 1.1 Добавить `frontend/Dockerfile` с многоэтапной сборкой из lock-файла и статическим runtime-образом; проверить `npm run build --prefix frontend` и `docker build -f frontend/Dockerfile -t gentoo-update-intelligence-frontend .`.
- [ ] 1.2 Настроить SPA fallback для `/analyses/<uuid>` и отдельный `/api/` proxy со снятием префикса; на собранном контейнере проверить прямой URL, refresh и сохранение статуса, `Content-Type` и тела контролируемого ошибочного ответа backend.

## 2. Добавить backend и PostgreSQL в Compose

- [ ] 2.1 Добавить `backend/Dockerfile` для сборки Go binary и включить Git CLI в runtime-образ, поскольку Metadata Provider запускает Git через `exec.Command`; проверить сборку `docker build -f backend/Dockerfile -t gentoo-update-intelligence-backend .` и наличие `git` в runtime-образе.
- [ ] 2.2 Создать `compose.yaml` с frontend, backend, PostgreSQL, loopback-публикацией только frontend, PostgreSQL healthcheck и именованным volume; задать внутри backend-контейнера `GENTOO_UPDATE_INTELLIGENCE_LISTEN_ADDR=0.0.0.0:8080`, передать абсолютный путь и полный commit snapshot, смонтировать snapshot read-only. Проверить `docker compose config` и отсутствие host ports у backend и PostgreSQL.
- [ ] 2.3 Запустить стек и проверить, что backend начинает обслуживание после готовности PostgreSQL; из backend-контейнера выполнить Git-чтение закреплённого commit и cache-объектов snapshot, а также подтвердить read-only mount. Проверить состояние через `docker compose ps` и inspect конфигурации контейнеров.

## 3. Документировать подготовку и локальный запуск

- [ ] 3.1 Добавить `.env.example` с placeholders для обязательных параметров и исключить локальный `.env` из Git; проверить, что в примере нет действующих секретов, а Compose принимает настроенный env-файл.
- [ ] 3.2 Документировать требования к Docker Compose и подготовленному Git snapshot, запуск/остановку стека и команды приёмки; сверить инструкции с реальными именами переменных и командами Compose.

## 4. Проверить полный сценарий с сохранением анализа

- [ ] 4.1 Запустить стек из чистого checkout с подготовленным snapshot, отправить реальный план в браузере и проверить оба Lookup `dev-libs/libei` как `found`; сохранить UUID и сверить отображённый результат с GET API.
- [ ] 4.2 Пересоздать контейнеры, сохранив именованный PostgreSQL volume, затем открыть прежний UUID напрямую и подтвердить, что API и UI показывают тот же результат.
