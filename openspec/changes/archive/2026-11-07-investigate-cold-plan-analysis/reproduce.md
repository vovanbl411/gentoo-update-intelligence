# Воспроизведение исследования

Работайте в отдельном каталоге `/var/tmp/guit-cold-plan-analysis/`. Существующий каталог содержит исходные данные этого исследования; для нового опыта выберите другой путь и свободные порты. Сценарии отказываются перезаписывать результаты. Нужны KVM, QEMU, `mkisofs`, Docker, Python 3 и локальный Go module cache. Не используйте `/tmp` для диска VM.

## Исходники и сборки

Точное состояние опыта сохранено в `source.tar` постоянного каталога; его хэш — в [manifest.json](manifest.json). [Manifest файлов](inputs/source-manifest.json) включает незакоммиченные изменения, [patch](inputs/source.patch) относится к указанному HEAD. Untracked-файлы сохранены в архиве исходников; одного patch для их восстановления недостаточно.

Для нового снимка выполните `python3 scripts/snapshot.py <repository> <новый-каталог>`. Скрипт копирует только заданные исходники и fixtures, исключает `.env*` и не копирует Git-конфигурацию. Далее команды выполняются из каталога этого change; установите `RESEARCH_ROOT` в абсолютный путь к новому каталогу.

```sh
mkdir -p "$RESEARCH_ROOT/export" "$RESEARCH_ROOT/instrumented"
cp -a "$RESEARCH_ROOT/source/backend" "$RESEARCH_ROOT/instrumented/"
cp -a "$RESEARCH_ROOT/source/testdata" "$RESEARCH_ROOT/source/config" "$RESEARCH_ROOT/instrumented/"
(cd "$RESEARCH_ROOT/instrumented" && git apply "$CHANGE_DIR/instrumentation.patch")
```

Здесь `CHANGE_DIR` — абсолютный путь этого change. В каждой из двух копий `backend/` выполните проверки:

```sh
GOPROXY=off go test -count=1 ./...
GOPROXY=off go vet ./...
GOPROXY=off go test -race -count=1 ./cmd/metadata-worker ./internal/planservice ./internal/analysishttp ./internal/metadatacache ./internal/ebuildresolver ./internal/reportservice ./internal/parser
```

Соберите `./cmd/gentoo-update-intelligence-server` и `./cmd/metadata-worker` с `CGO_ENABLED=0 GOPROXY=off go build -trimpath -ldflags='-s -w'`, сохранив пары в `export/control-backend`, `export/control-worker`, `export/instrumented-backend`, `export/instrumented-worker`. Зафиксируйте SHA-256. Измерительный patch применяется только к отдельной копии.

## Mirrors и предварительная проверка

Скопируйте Gentoo и guru из подготовленных mirrors в `mirrors/gentoo.git` и `mirrors/guru.git`. Исходные mounts должны быть read-only. Отдельно клонируйте полные bare mirrors публичных overlays:

- [gentoo-zh](https://github.com/gentoo-zh/overlay.git);
- [umbriel-overlay](https://github.com/vovanbl411/umbriel-overlay.git);
- [zed-overlay](https://github.com/vovanbl411/zed-overlay.git).

Не меняйте пользовательские Git-настройки. Закрепите commits из [repositories.json](inputs/repositories.json), проверьте их наличие, полноту истории и `metadata/layout.conf`. Все четыре overlays используют master `gentoo`. Backend уже включает `guru` в стандартный каталог. Для плана 3 не добавляйте его повторно в YAML. В сохранённой конфигурации плана 2 оставлена исходная неиспользуемая дублирующая запись: план 2 к `guru` не обращается, контрольная пара использует те же настройки. Сохраните ту же структуру mirrors и те же origin URL, что в [конфигурации плана 2](inputs/backend-2.yaml) и [конфигурации плана 3](inputs/backend-3.yaml). Не обновляйте mirrors во время матрицы.

Скопируйте `scripts/preflight.go` в `source/backend/cmd/cold-preflight/main.go`, соберите его из `source/backend/` в `$RESEARCH_ROOT/preflight`. Вызов с путём fixture выводит Parser status, число записей, after-only пересборок и список уникальных Query. Сохраните ответы как `evidence/plan-2-queries.json` и `evidence/plan-3-queries.json` либо используйте сохранённые входы при неизменных fixtures. Скопируйте `inputs/repositories.json` в `evidence/repositories.json`.

```sh
python3 scripts/preflight.py "$RESEARCH_ROOT" "$RESEARCH_ROOT/evidence/preflight.jsonl"
```

Код возврата должен быть `0`; для каждой строки требуется `resolved` и resolver `Status=found`. Любой `not_found`, `history_incomplete`, техническая ошибка или таймаут препятствует успешному прогону соответствующего плана. Не заменяйте версии в fixtures.

## VM

Образ загружен из [официального каталога Debian](https://cloud.debian.org/images/cloud/trixie/latest/). Сохраните `SHA512SUMS`, JSON описания образа и qcow2; сверьте SHA-512 конкретного файла перед запуском. `latest` изменяется, поэтому для повторения этого опыта нужен файл с checksum из manifest.

```sh
qemu-img create -f qcow2 -F qcow2 -b "$RESEARCH_ROOT/downloads/debian-13-genericcloud-amd64.qcow2" "$RESEARCH_ROOT/guest.qcow2" 40G
mkisofs -quiet -output "$RESEARCH_ROOT/seed.iso" -volid cidata -joliet -rock scripts/seed
sh scripts/launch-vm.sh "$RESEARCH_ROOT"
```

Последнюю команду оставьте в отдельном терминале. `scripts/qga.py` обращается только к локальному QEMU guest-agent socket; SSH-ключи для управления VM не используются. Для нестандартного каталога задайте `--socket <путь>/agent.sock`. После установки пакетов cloud-init должен создать `/opt/guit/ready`. Проверьте через guest-agent `nproc`, `free -b`, `/proc/swaps`, `lsblk -b`, `uname -a`, `/etc/debian_version` и версии Docker/образов. Загрузка образа, установка пакетов и перенос данных относятся к подготовке.

## Перенос и запуск

Сохраните Docker-образы с IDs из manifest командой `docker image save`, а отдельные mirrors — обычным tar. В каталоге `export/` разместите `images.tar`, `mirrors.tar`, четыре бинарника, `backend-2.yaml`, `backend-3.yaml`, `seed-2.sql`, `seed-3.sql`, два исходных fixture, `observe.py` и `matrix.py` из этого change. Отдавайте только этот каталог локальным HTTP-сервером:

```sh
python3 -m http.server 18962 --bind 127.0.0.1 --directory "$RESEARCH_ROOT/export"
```

Гость может читать его через `http://10.0.2.2:18962/`. Через `qga.py` загрузите файлы в `/opt/guit`, выполните `docker load`, распакуйте mirrors, установите executable-биты бинарникам и назначьте владельца `100:101` копиям mirrors. Этот UID используется пользователями `app` и `portage` сохранённых образов. Исходные mirrors хоста остаются нетронутыми.

`seed-*.sql` создаёт тестовые таблицы и триггер, который при POST добавляет заранее закреплённые revisions. Backend использует штатный `RestorePinned`, поэтому refresh не вызывается. Замер включает проверку и открытие закреплённых repositories; он не включает первоначальное обновление и выбор публикаций. Это особенность стенда, SQL не применяется к рабочей базе.

Перед POST запустите на хосте:

```sh
python3 scripts/observe.py "$RESEARCH_ROOT/evidence/host.jsonl" --stop-file "$RESEARCH_ROOT/evidence/host-observer.stop"
```

Затем через guest-agent запустите `/usr/bin/python3 /opt/guit/matrix.py`. Сценарий создаёт внутреннюю Docker-сеть, использует отдельные базы и cache, выполняет три пары каждого плана и контрольную пару плана 2. CPU и RAM ограничивает VM; лимиты активного анализа, подготовки и генерации равны `1`. Наблюдатель в госте собирает данные раз в секунду. Опрос `partial-report` идёт раз в секунду по адресу контейнера; полный GET выполняется после терминального состояния. Таймаут анализа — 7200 секунд. Если между соседними ответами опроса прошло больше 10 секунд, сценарий сохраняет прогон как неуспешный. При своде данных тот же порог применяется к секундным наблюдениям гостя и хоста.

После каждой пары сценарий останавливает и удаляет только три собственных контейнера. Их bind-каталоги, cache и базы остаются на диске. При ошибке сценарий сохраняет причину и прекращает матрицу; контейнеры остаются для диагностики. Compose хоста не используется. При продолжении успешной пары используйте `matrix.py --resume`: сценарий проверит путь bind-mounted cache, заново определит IP backend и пропустит только успешные сохранённые прогоны. Сначала разберите неуспешный прогон; его каталог нельзя перезаписывать. Для выбранных пар есть параметр `--only <label> ...`. Label `plan3-pair2-retry` создаёт отдельную чистую пару для замены неуспешного опыта.

## Проверка и хранение

Сохраните `/opt/guit/results/`, версии гостя, журналы подготовки и pilot в постоянный каталог хоста. Остановите наблюдатель, создав его stop-файл. Сформируйте SHA-256 всех сырых файлов. Нормализация ответа допустима только для UUID и автоматически созданных временных путей; оригинальные JSON сохраняйте отдельно. Сравните findings, порядок, Lookup и provenance, проверьте OOM, число генераций и cache hits, счётчики прогресса и завершение `80/80` либо `79/79`.

`cpu.stat` и `io.stat` cgroup включают дочерние и завершившиеся процессы. Снимки `/proc/<pid>/io` показывают логические записи только наблюдавшихся процессов: короткоживущие процессы между опросами могут быть пропущены. Эти величины нельзя выдавать за полный объём логической записи. Запись блочного устройства считайте отдельно по `diskstats`; она включает PostgreSQL, Docker и наблюдатель. `memory.stat` разделяет anon/file, а `memory.current` включает остальные учтённые cgroup расходы. Не складывайте вложенные интервалы инструментации и перекрывающиеся Lookup как полное время задания.
