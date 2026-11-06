# Приёмка compact-analysis-report

## Проверки 2026-11-06

- Backend: `go test -count=1 ./...` и `go vet ./...` прошли.
- Frontend: typecheck, 13 unit-тестов, proxy test и production build прошли.
- Playwright с Flatpak Chrome: 35 тестов прошли, live-тест пропущен без `LIVE_ACCEPTANCE=1`.
- `openspec validate --all --strict` и `openspec validate --archived --strict` прошли; `git diff --check` не обнаружил ошибок.
- Работающий Compose на `127.0.0.1:8081`: компактный API вернул `200` для сохранённого анализа `8f3f69b1-4519-46b9-b531-4dfbab9a492f`. В Flatpak Chrome открылась страница с `parsed`, одним пакетом `dev-libs/libxml2` и доступным сравнением metadata.

## Live-проверка обновлённого Compose

Тест `frontend/tests/live.spec.ts` адаптирован к новой странице и выполнен через работающий Compose на `127.0.0.1:8081` с Flatpak Chrome. Новый анализ `8bab32b1-362e-4ecf-be6f-734490449755` для `dev-libs/libei` завершился за 48,7 секунды; обе стороны Lookup имеют статус `found`, сравнение metadata — `available`. Проверены создание через форму, совпадение plan и analysis в компактном и полном API, открытие в новой вкладке, refresh и загрузка технических данных. Тестовое задание осталось в локальной базе.

Live-проверка использовала один пакет и работающий локальный стек; она не подтверждает доступность metadata для произвольных пакетов или производительность больших планов.
