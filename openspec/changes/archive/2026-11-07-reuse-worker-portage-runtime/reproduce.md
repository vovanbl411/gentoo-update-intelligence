# Воспроизведение замеров этапа 1

## Условия

Использовалась отдельная KVM с 1 vCPU, 2 GiB RAM без swap, на overlay-копии гостевого диска `/var/tmp/guit-cold-plan-analysis/guest.qcow2`. Данные эксперимента и выходы хранились в `/var/tmp/guit-worker-optimization/`. Рабочий Compose и его volumes не подключались. В госте матрица использовала прежние закреплённые mirrors, планы и Compose images; синхронизация репозиториев не запускалась.

Сборки сделаны с `CGO_ENABLED=0`, `GOPROXY=off`, `-trimpath` и `-ldflags='-s -w'`. Control worker собран из снимка дерева до изменения Portage runtime; новый worker — из дерева после изменения. Для обоих вариантов использован один бинарник backend. SHA-256 сборок и данных сохранены в `/var/tmp/guit-worker-optimization/evidence/manifest.json`. Матрица состоит из трёх cold→warm пар нового worker и одной свежей контрольной пары для каждого плана. Runner проверяет структуру отчёта, число записей и lookup, after-only пересборки и ошибки.

## Повторный запуск

При повторном опыте используйте каталог, которого ещё нет. Снимки этого запуска сохранены в `/var/tmp/guit-worker-optimization/source/` и `/var/tmp/guit-worker-optimization/control-src/`. Пример создания отдельного нового снимка:

```sh
python3 openspec/changes/archive/2026-11-07-investigate-cold-plan-analysis/scripts/snapshot.py "$PWD" /var/tmp/guit-worker-optimization-next
```

Сборки из текущего дерева и сохранённого pre-change снимка выполнялись так:

```sh
cd backend
GOPROXY=off CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /var/tmp/guit-worker-optimization/bin/backend ./cmd/gentoo-update-intelligence-server
GOPROXY=off CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /var/tmp/guit-worker-optimization/bin/worker-optimized ./cmd/metadata-worker
```

Контрольный worker собирается из `control-src/backend`, извлечённого из `/var/tmp/guit-before-optimization-working-tree.tar`:

```sh
cd /var/tmp/guit-worker-optimization/control-src/backend
GOPROXY=off CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /var/tmp/guit-worker-optimization/bin/worker-control ./cmd/metadata-worker
```

Сборки передаются в изолированную VM. Использованный runner сохранён в `/var/tmp/guit-worker-optimization/matrix.py`. Он отличается от исходного `reproduction-scripts/matrix.py` расписанием: три новые пары и одна контрольная пара на каждый план. Оба файла сохранены; контроль различий выполнен сравнением расписаний. QEMU запускается с 1 vCPU, 2048 MiB RAM, отдельным qcow2 overlay и собственным guest-agent socket. Host observer запускается из `reproduction-scripts/observe.py`; после завершения runner результаты сводятся командами:

```sh
python3 reproduction-scripts/summarize.py evidence/guest/results evidence/host.jsonl evidence/summary.json
python3 reproduction-scripts/render_tables.py evidence/summary.json evidence/measurement-tables.md
cd reproduction-scripts
python3 -m unittest -v test_measurements.py
```

Существующие каталоги VM и результаты не перезаписывайте: создавайте новый рабочий каталог для повторения опыта. Сохраняйте исходные ответы отдельно от сводки. Нормализуйте только UUID анализа и автоматически созданные временные пути.
