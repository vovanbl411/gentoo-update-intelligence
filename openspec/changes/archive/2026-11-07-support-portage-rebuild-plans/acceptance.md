# Приёмка support-portage-rebuild-plans

## Результат

Неизменённый `testdata/parser/ebuild-updates-3.txt` разбирается целиком: 43 записи, 36 upgrade и 7 reinstall, статус `parsed`, без диагностик. Исходный SHA-256: `45a17bd538e2688536b09d644735b5f36c65b17551c3eb664839d4d6fa6738eb`. Вход не содержит сводки Portage; Summary остаётся nil.

До исправления регрессионный тест воспроизвёл `partial`, 17 записей и диагностики размеров с запятыми и операций R. Теперь отдельная таблица проверяет все 43 записи в порядке плана, версии, repository, операции, номера строк, размеры и явно показанные SLOT/SUBSLOT. Полностью проверены группы флагов семи пересборок, отдельный ~ и отсутствие выдуманного installed.

## Проверенные сценарии

- Размеры без разделителей, с одним и несколькими разделителями, ноль и int64 max. Ошибки группировки, дроби, отрицательные числа и overflow отклоняются. Сводка поддерживает reinstall/reinstalls, неизвестные и повторные счётчики отклоняются.
- R и R с отдельным ~ поддерживаются; неизвестные комбинации остаются неподдерживаемыми. Локальные патчи и явно изменённые USE сохраняют прежние правила Analyzer; пересборка сама по себе findings не создаёт.
- R без installed получает только целевой Lookup. Before остаётся без запроса с BINDING_REJECTED; INSTALLED_CONTEXT_MISSING сохраняется, Compare не вызывается. Проверены found, not_found, repository_unavailable, ошибка Provider, неизвестный repository, неоднозначный и явный установленный контекст.
- Целевой snapshot проверяется по category/name, version, repository и явно показанным SLOT/SUBSLOT. Несоответствия не создают сравнение.
- Последовательный, параллельный и progressive planservice возвращают 43 отчёта, 79 уникальных запросов и прогресс 79/79. При корректных найденных snapshots доступны ровно 36 сравнений. Повторы записей не повторяют Lookup.
- HTTP с управляемым Provider сохраняет 43 отчёта, семь reinstall и прогресс 79/79; full, compact и partial-report маршруты сохраняют факты. Отдельная синтетическая сводка подтверждает summary.reinstalls = 7.
- Старый сохранённый partial читается без нового Provider Lookup и изменения хранения; отсутствующий Reinstalls даёт 0. Legacy golden fixture сохранён без этого поля.
- Браузер показывает 43 карточки и семь пересборок, отсутствие установленной стороны, отдельные ограничения metadata и предупреждение для partial/unsupported. Без сводки общий счётчик Portage не показывается.

## Выполненные проверки

Из `backend/`:

- `gofmt -w` для изменённых и новых Go-файлов.
- `go test -count=1 ./...` — успешно.
- `go vet ./...` — успешно.
- `go build -o /tmp/gentoo-update-intelligence ./cmd/gentoo-update-intelligence` — успешно.
- `go test -race -count=1 ./internal/parser ./internal/analyzer ./internal/changereport ./internal/reportservice ./internal/planservice ./internal/analysishttp` — успешно.
- После уточнения регрессионных проверок: `go test -race -count=1 ./internal/parser ./internal/reportservice ./internal/planservice` — успешно.

Из корня:

- `npm run typecheck --prefix frontend` — успешно.
- `npm test --prefix frontend` — 13 тестов успешно.
- `npm run build --prefix frontend` — успешно.
- `PLAYWRIGHT_BROWSER_EXECUTABLE=/var/lib/flatpak/app/com.google.Chrome/current/active/files/extra/chrome npm run test:browser --prefix frontend` — 39 passed, 1 skipped.
- `git diff --check` и whitespace-проверка новых файлов — успешно.
- Итоговый diff проверен; исходные изменения предыдущей приёмки сохранены.
- `sh scripts/openspec.sh archive support-portage-rebuild-plans --yes` — change архивирован, применены четыре требования к действующим спецификациям.
- `sh scripts/openspec.sh validate --all --strict` — 5 спецификаций успешно.
- `sh scripts/openspec.sh validate --archived --strict` — 14 changes успешно.

Документация прошла завершающий precision-проход humanize-docs с сохранением технических идентификаторов, условий и порядка проверок.

## Ограничения

Live Compose/PostgreSQL browser test пропущен: LIVE_ACCEPTANCE не включён, Compose в рамках этой приёмки не запускался. HTTP проверен с in-memory Store и управляемым Provider; браузер — с контролируемыми API-ответами. Реальные репозитории Gentoo и генерация metadata не использовались.

Сохранённые анализы автоматически не переразбираются: для прежнего частичного результата нужен новый запуск. After metadata пересборки не подтверждают установленную конфигурацию и не делают сравнение доступным. Причины пересборок не определяются.

Commit и push не выполнялись. Результат доступен в рабочем diff, включая новые файлы.
