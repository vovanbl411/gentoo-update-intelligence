# Проверка корректности checkout pool

Дата: 2026-11-08. Приёмка checkout pool завершена; change остаётся активным до отдельного review.

## Реализация

Worker использует комплект основного checkout и упорядоченных masters после прежних проверок repository/generated-cache. Ключ исключает package/version и включает repository names, URLs, mirror paths, selected commit, commits masters, запрошенный профиль, версию Portage и фиксированную конфигурацию генератора. Комплект выдаётся одному запросу; generation temp directory создаётся отдельно для каждого запроса.

После использования pool восстанавливает деревья через `git reset --hard` и `git clean -ffdx`, проверяет HEAD и чистоту. Проверки ebuild blob и границ профиля перед генерацией сохранены. Ошибка валидации исключает комплект; ошибка удаления блокирует последующие выделения до перезапуска. Cache hits доступны до обращения к pool. Отмена одного HTTP-клиента прекращает его ожидание и не отменяет подготовку для остальных клиентов; pool также поддерживает отмену ожидающей lease через context.

Idle LRU ограничен 16 комплектами и настроенным бюджетом (по умолчанию 4 GiB); учитываются выделенные блоки без перехода по symlinks. Oversized-комплект удаляется после использования. Значение `0` создаёт и удаляет отдельный комплект на каждый запрос, без reuse и восстановления. Блокировка выделенного каталога получается после перехода к пользователю Portage; startup очищает остатки только после получения блокировки.

## Ограничение конкурентности

Создание checkout, восстановление деревьев, подсчёт размера и удаление комплектов выполняются под общим mutex pool. Эти операции сериализуются даже для разных ключей; сами генерации могут выполняться параллельно в пределах настроенных лимитов. Для этой фиксации mutex сохранён. Его влияние на время ожидания и cold-анализ нужно оценить в следующих измерениях; ускорение этой части не подтверждено.

## Автоматические сценарии

- Ключи: все компоненты repository/master identity, порядок masters, selected commit, profile и runtime; package/version исключены.
- Reuse разных пакетов; отдельные generation directories; восстановление tracked, untracked и ignored файлов основного дерева и master.
- Ошибка генерации, повреждение checkout, отказ удаления и последующая блокировка выделений; generated-cache hit при заблокированном pool.
- Ожидающая эксклюзивная lease, отмена ожидания, независимые активные ключи; существующие тесты preparation/generation limits и отмены одного HTTP-клиента.
- LRU по количеству, байтам и порядку обращения; oversized и disabled mode; symlink при подсчёте размера.
- Второй владелец не удаляет данные первого; stale cleanup после закрытия владельца и после принудительного завершения отдельного процесса.

## Реальный egencache

На локальном хосте два разных пакета тестового Git-репозитория (`app-test/alpha-1`, `app-test/beta-1`) последовательно обработаны настоящим `egencache` в одном комплекте. После каждого запроса подтверждена чистота checkout, совпадение пути повторно используемого комплекта и различие временных каталогов генерации. Рабочий Compose и его volumes не использовались.

Воспроизведение из `backend/`:

```sh
METADATA_WORKER_TEST_REAL_EGENCACHE=1 go test -count=1 -v ./cmd/metadata-worker -run TestPooledGeneration
```

Этот сценарий проверяет корректность на небольшом репозитории. Он не заменяет измерения двух закреплённых планов в KVM 1 vCPU / 2 GiB.

## Проверки и оставшаяся работа

Успешно выполнены из `backend/`: `go test -count=1 ./...`, `go vet ./...`, сборки CLI, server и worker, а также `METADATA_WORKER_TEST_REAL_EGENCACHE=1 go test -race -count=1 ./...` (включая реальный egencache). Выполнены `git diff --check`, проверка whitespace новых файлов и `sh scripts/openspec.sh validate reuse-pinned-metadata-checkouts --strict`. Следующий этап — 16 запусков по согласованной матрице, отчёт о производительности и проверка бюджета на реальных деревьях. До этого ускорение cold-анализа и завершение change не заявляются.

## Финальная проверка общего коммита — 2026-11-08

Документация согласована с состоянием реализации: pool проверен на корректность, задача измерений 3.3 остаётся открытой. Финальный редакторский проход humanize-docs выполнен для изменённой прозы. В тесте генерации проверяются непосредственно различие путей `--config-root`, удаление generation directory и чистота дерева после ошибки.

Успешно выполнены:

- `gofmt` для всех изменённых Go-файлов backend; `go test -count=1 ./...`, `go vet ./...`, сборки CLI, server и worker.
- `METADATA_WORKER_TEST_REAL_EGENCACHE=1 go test -race -count=1 ./...`; после уточнения теста — тот же прогон для `./cmd/metadata-worker`.
- Frontend: `npm test --prefix frontend` (13 тестов), `npm run typecheck --prefix frontend`, `npm run build --prefix frontend`.
- Browser tests с Flatpak Chrome: 39 passed, 1 skipped. Пропущен live Compose/PostgreSQL test: `LIVE_ACCEPTANCE` не включён; рабочий стек не запускался.
- `python3 -m unittest test_measurements.py` из каталога scripts архивного исследования cold-анализа: 10 тестов обработки evidence.
- `sh scripts/openspec.sh validate --all --strict`: 6 passed; `sh scripts/openspec.sh validate --archived --strict`: 16 passed.

Whitespace проверяется также для новых файлов. В двух архивных `.patch` сохраняются строки контекста из одного пробела — обязательный маркер unified diff; они проверяются отдельно без удаления этого маркера. Результаты измерений и исходные fixtures не переписываются.

## Phase-two performance acceptance — 2026-11-08

Evidence проверены из `/var/tmp/guit-checkout-pool-phase2/evidence/`. SHA-256 отчёта, summary и reproduction совпали с записями manifest; записи manifest для исходных гостевых артефактов сохранены в manifest. Summary содержит 8 пар и 16 успешных запусков от `cab93148963f952ecf9454f47d41a5c23ad78679`. Для всех запусков нормализованный hash совпал с hash соответствующего плана; отчёты Pool ON/OFF равны внутри плана. Медианы и guest device writes пересчитаны из JSON:

| План | Pool OFF cold | Pool ON cold, медиана (n=3) | Снижение cold | Pool OFF device writes | Pool ON device writes, медиана |
| --- | ---: | ---: | ---: | ---: | ---: |
| 2 | 1063.03 s (n=1) | 603.06 s | 43.3% | 62.55 GiB | 11.30 GiB |
| 3 | 1081.03 s (n=1) | 492.06 s | 54.5% | 59.47 GiB | 8.80 GiB |

Измерения проведены в Debian 13 KVM с 1 vCPU, 2 GiB RAM и без swap; использовались одинаковые backend/worker, pinned revisions и два плана. Pool OFF задан значением `METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES=0`, Pool ON — `4294967296`. Полные normalized JSON отчёты совпали после нормализации UUID и временных путей. Device writes измерялись для гостевой системы целиком. Pool ON имеет по три cold/warm пары на план, Pool OFF — по одной; host contention не измерялся. Поэтому результаты описывают эту среду и не доказывают статистически ускорение на другом оборудовании. Первый неудачный pilot исключён из статистики. Рабочий Compose не использовался, полная матрица повторно не запускалась.

Воспроизведение: `phase2-reproduce.md`. Исходный отчёт: `phase2-report.md`; машинные данные: `phase2-summary.json`; полный список artifact hashes: `phase2-manifest.sha256` в локальном каталоге evidence. SHA-256 доступных локальных итоговых файлов:

- `phase2-report.md`: `aac750ebf91a57fc2d5f054bebd5309655170c06f5bec2342d53ba1b9d39f626`
- `phase2-summary.json`: `b44795392de00aa9816be4658d6225a39af30400e0c3905186405d3a12a0b5fd`
- `phase2-reproduce.md`: `234d2ccbf5ac5ed5842d800366128384790c0e50f609777ba8219dcd34e33ea2`
- `phase2-manifest.sha256`: `348e10f5728a9aeb5c3b62f74d2577803ceeed08d43c82b7b635ac4f6643c245`

## Исправление проверки Git checkout — 2026-11-08

Проверки `git rev-parse HEAD` и `git status` теперь оценивают exit code и stdout отдельно от stderr. Предупреждение в stderr при успешной Git-команде и ожидаемом stdout не считается повреждением checkout; ненулевой exit code и несовпадение pinned HEAD или чистого статуса по-прежнему приводят к ошибке и fail-closed удалению комплекта. Добавлен регрессионный тест с Git wrapper, который выдаёт указанное предупреждение и выполняет настоящую команду Git. `TestCheckoutCorruptionIsDiscarded` сохраняет проверку повреждённого checkout.

Проверены `TestRestoreCheckoutVerificationIgnoresStderrWarning`, `TestCheckoutCorruptionIsDiscarded` и остальные тесты пакета `cmd/metadata-worker`. Команда `METADATA_WORKER_TEST_REAL_EGENCACHE=1 go test -count=1 ./cmd/metadata-worker -run TestPooledGeneration -v` успешно выполнила stub- и real-egencache сценарии.
