# Проверка

Синтетический benchmark `go test ./internal/planservice -run '^$' -bench '^BenchmarkLargePlan$' -benchtime=3x -count=1`: 40 записей и 80 уникальных запросов, искусственная задержка каждого Lookup — 5 мс. На Intel Core i7-1260P последовательный вариант занял 434 мс на план, параллельный — 208 мс. До изменения отдельный последовательный замер составил 424 мс. Это проверка параллелизма ожидания, а не скорость реальной генерации Gentoo metadata.

## Реальный план

6–7 ноября 2026 года сравнивались родитель `d726d63`, commit `8dc69e2`, и `d726d63`. Последний включает также изменения HTTP DTO, хранения промежуточного отчёта и поиска исторической ревизии; замер показывает эффект всей разницы между commits, а не изолированный эффект параллелизма. Вход `testdata/parser/ebuild-updates-2.txt`, SHA-256 `feaabaeb443165d9ccf2d3da31c920f016d9e71972ad79243cc0c5f274c60dae`. Использованы Gentoo `b3c5634f01d86a33bc5fc0a599f5c318989a2839`, gentoo-zh `53e6cacef79d87f347219314b357be7d2714526a`, umbriel-overlay `7b73f8dfac5f5896b088def7e1670e7073437947` и профиль `profiles/default/linux/amd64/23.0`.

Оба варианта Go-бинарников собраны на хосте `go1.27.2-X:nodwarf5` с `CGO_ENABLED=0`, `GOPROXY=off`, `-trimpath` и `-ldflags='-s -w'`. Они подключались к одним и тем же локальным runtime-образам: backend `sha256:447696b46cbe9cfc88f9d194609fcd660fa2a7832897905693163657da0c3181`, worker `sha256:9ccd9cc9a4f1c93a183730f0e2c2975ff5743718f8f7828bda4dce50db8c9cc5`, PostgreSQL `sha256:ab20e934b27ba3ff927c1b4c9ff8cf6c32f315f743efe68dd3441182a87fb085`. Штатная сборка Docker с `--network none` не нашла module cache; поэтому использована локальная сборка. Хэши бинарников: baseline backend `8e59c6a137d7d2a369ec61088b34eb5a7741c127369087c9e7b54eb995fb379e`, worker `6069ce5e22c2e42689bd2bff5666790b2a77d95f0b32f59177bbbf04aa28ff80`; текущие backend `d126e418dcda84a3f9bae6dc72ee6e6d70fdac8868a9a87f79d14e3bb7ede598`, worker `cfa1e464a596950678f7b293c8bc308de0f3b737982c5b3f5c6f5bf3a4511d2d`.

Отдельные Compose-проекты использовали отдельные базы и cache, идентичные копии overlays и общую read-only копию Gentoo mirror. Подготовленные публикации не обновлялись: попытки refresh overlays обращались к отсутствующим локальным путям и завершались с сохранением публикации. Установлены лимит одного анализа и лимиты worker двух генераций и двух подготовок; baseline не поддерживает отдельный лимит подготовки и выполняет запросы плана последовательно. Лимиты CPU и памяти контейнерам не задавались. Время загрузки зеркал и подготовки стенда в измерения не включено и отдельно не измерялось.

| Версия | Пустой generated-cache | Повторный запуск |
|---|---:|---:|
| baseline | 1318,033 с | 1395,833 с |
| текущая | 594,334 с | 18,135 с |

Время отсчитывалось от POST до первого наблюдения терминального состояния при опросе раз в две секунды. Каждый случай измерен один раз; файловый cache ОС не сбрасывался. Результаты не являются оценкой распределения времени или гарантией ускорения на другом оборудовании.

Все четыре задания завершились `completed`, `80/80`; наблюдавшийся счётчик не уменьшался. В каждом отчёте 41 запись. Журналы worker показывают по 80 cache hits в каждом повторном запуске. Холодный и повторный ответы каждой версии совпали целиком после удаления UUID задания. При прямом сравнении холодных ответов разных версий помимо UUID различались 320 значений provenance с автоматически созданными именами временных каталогов. После замены только числового суффикса `/var/tmp/gentoo-update-intelligence/metadata-<число>` остальные данные совпали, включая порядок пакетов, результаты, evidence, остальные части provenance, предупреждения и ошибки. Исходные ответы сохранены без такой замены; буквальное равенство provenance между независимыми генерациями не утверждается.

Полные ответы, наблюдения прогресса, журналы и конфигурация находятся в [`measurements/`](measurements/). Журналы `*-all-worker.log` включают холодный и повторный запуск соответствующей версии. Проверены `go test -count=1 ./...`, `go vet ./...` и `go test -race -count=1 ./internal/planservice ./cmd/metadata-worker ./internal/ebuildresolver` из `backend/`; существующие тесты проверяют ошибки, дедупликацию и пределы. Первый запуск полного suite в sandbox не смог открыть локальный сокет; повтор с доступом к локальным сокетам прошёл.

## Воспроизведение

Нужны локальные образы с указанными выше ID, подготовленный Gentoo volume с указанным commit и локальный Go module cache. Перед командами установите `CHANGE_DIR` в путь к каталогу этого архивного change. Файлы стенда и опроса сохранены в `measurements/`; `measure.py` использует абсолютный путь этого checkout и порт `18951`. Команды ниже создают новые каталоги и Compose volumes; имя проекта должно быть свободно. Загрузка публичных overlays требует сетевого доступа до начала замеров.

```sh
mkdir -p /tmp/guit-speed-acceptance/baseline /tmp/guit-speed-acceptance/sources /tmp/guit-speed-acceptance/gentoo /tmp/guit-speed-acceptance/base-overlays /tmp/guit-speed-acceptance/new-overlays
git archive 8dc69e2 | tar -x -C /tmp/guit-speed-acceptance/baseline
cp "$CHANGE_DIR"/measurements/compose.yaml "$CHANGE_DIR"/measurements/backend.yaml "$CHANGE_DIR"/measurements/measure.py /tmp/guit-speed-acceptance/
git clone --bare --single-branch https://github.com/gentoo-zh/overlay.git /tmp/guit-speed-acceptance/sources/gentoo-zh.git
git clone --bare --single-branch https://github.com/vovanbl411/umbriel-overlay.git /tmp/guit-speed-acceptance/sources/umbriel-overlay.git
cp -a /tmp/guit-speed-acceptance/sources/. /tmp/guit-speed-acceptance/base-overlays/
cp -a /tmp/guit-speed-acceptance/sources/. /tmp/guit-speed-acceptance/new-overlays/
docker run --rm --network none -v gentoo-update-intelligence_gentoo-data:/from:ro -v /tmp/guit-speed-acceptance/gentoo:/to --entrypoint sh gentoo-update-intelligence/gentoo-runtime:local -c 'cp -a /from/. /to/'
```

Закрепите публикации, задайте недоступные локальные источники и права, затем соберите бинарники обоих commits. Текущий checkout для воспроизведения должен соответствовать `d726d63`.

```sh
for variant in base new; do
  git -C "/tmp/guit-speed-acceptance/$variant-overlays/gentoo-zh.git" update-ref refs/gentoo/published 53e6cacef79d87f347219314b357be7d2714526a
  git -C "/tmp/guit-speed-acceptance/$variant-overlays/umbriel-overlay.git" update-ref refs/gentoo/published 7b73f8dfac5f5896b088def7e1670e7073437947
  git -C "/tmp/guit-speed-acceptance/$variant-overlays/gentoo-zh.git" config remote.origin.url /unavailable/gentoo-zh.git
  git -C "/tmp/guit-speed-acceptance/$variant-overlays/umbriel-overlay.git" config remote.origin.url /unavailable/umbriel-overlay.git
done
docker run --rm --network none -v /tmp/guit-speed-acceptance/base-overlays:/base -v /tmp/guit-speed-acceptance/new-overlays:/new --entrypoint sh gentoo-update-intelligence/gentoo-runtime:local -c 'chown -R 100:101 /base /new'
(cd /tmp/guit-speed-acceptance/baseline/backend && CGO_ENABLED=0 GOPROXY=off go build -trimpath -ldflags='-s -w' -o /tmp/guit-speed-acceptance/baseline-backend ./cmd/gentoo-update-intelligence-server)
(cd /tmp/guit-speed-acceptance/baseline/backend && CGO_ENABLED=0 GOPROXY=off go build -trimpath -ldflags='-s -w' -o /tmp/guit-speed-acceptance/baseline-worker ./cmd/metadata-worker)
(cd backend && CGO_ENABLED=0 GOPROXY=off go build -trimpath -ldflags='-s -w' -o /tmp/guit-speed-acceptance/current-backend ./cmd/gentoo-update-intelligence-server)
(cd backend && CGO_ENABLED=0 GOPROXY=off go build -trimpath -ldflags='-s -w' -o /tmp/guit-speed-acceptance/current-worker ./cmd/metadata-worker)
```

Для baseline задать `ACCEPT_ROOT=/tmp/guit-speed-acceptance`, `ACCEPT_VERSION=baseline`, `ACCEPT_OVERLAY=base` и выполнить `docker compose --env-file /dev/null -p guit-speed-base -f /tmp/guit-speed-acceptance/compose.yaml up -d`. Последовательно выполнить `python3 /tmp/guit-speed-acceptance/measure.py baseline-cold` и `python3 /tmp/guit-speed-acceptance/measure.py baseline-warm`; сохранить `docker logs guit-speed-base-metadata-worker-1` с обоими потоками. Остановить этот проект через `docker compose ... stop`. Для текущей версии повторить с `ACCEPT_VERSION=current`, `ACCEPT_OVERLAY=new`, проектом `guit-speed-current` и именами измерений `current-cold`, `current-warm`. Не повторять с уже заполненными volumes под теми же именами проектов, если нужен холодный запуск.
