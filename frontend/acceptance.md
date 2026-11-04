# Реальная браузерная проверка

## Асинхронный API

**Этап 3.2: асинхронный API.** В предыдущей сессии полный Playwright suite прошёл: 30 тестов успешно, 1 пропущен. Отдельный live-тест с `LIVE_ACCEPTANCE=1` тоже прошёл: 1 тест успешно. Дополнительно прошли typecheck, 10 unit-тестов, production build и `git diff --check`.

Live-тест отправлял план из `testdata/parser/ebuild-updates.txt` через браузер и проверял асинхронный ответ `202`, прогресс, завершение задания, отображение результата, открытие того же UUID в новой вкладке и после refresh. В отчёте проверены Lookup и metadata diff для `dev-libs/libei`, а finding с `ContextIndex: 0` отображался на странице. UUID анализа: `f62bbece-5f7e-44ac-960c-7c2a8fead82b`.

Ранее запущенный Compose backend оказался старой синхронной версией и отвечал `201`. Поэтому live-тест запускался с backend из текущего source и отдельной PostgreSQL, используя существующий Gentoo snapshot только для чтения. Эти результаты подтверждают UI и асинхронный API этапа 3.2; интеграционный прогон с пустыми volumes и десятью пакетами этапа 4.1 описан отдельно и здесь не подтверждается.

Ниже приведена более ранняя проверка интерфейса с синхронным API `201`; она не подтверждает поведение асинхронного задания.

Для проверки использованы backend проекта, временный PostgreSQL 18, Chrome из Flatpak и `testdata/parser/ebuild-updates.txt`. Metadata Provider читал локальный Git-снимок с commit `1d1580515aeae5bf0c3d33a9d7d099bdc4a2aed3` и именем repository `gentoo`.

SHA-256 содержимого cache в commit:

- `dev-libs/libei-1.6.0`: `0168526a96c712e1fad72f920ba5fbf3af34ddbcca1fd69db5c211448ffa0002`.
- `dev-libs/libei-1.6.0-r1`: `e0d82b81a0e872e19cefc32a1f0229a23628726acf8a2ecd2ad53e965f4337e8`.

CLI подтвердил оба Lookup `found` и доступное сравнение. Браузерный прогон создал анализ `8a576223-4046-4ddf-8d7f-a03f5483cb26`; JSON POST и GET совпали. Проверены шесть пакетов, операции и доступные версии, пустой diff EAPI, SLOT/SUBSLOT и IUSE, а также отображение результата в новой вкладке и после refresh. UUID относится к временному хранилищу и не является постоянной ссылкой.

В поле локально патченных пакетов указан `dev-libs/libei`: finding и его `ContextIndex: 0` подтверждены по JSON и на экране, в том числе после открытия в новой вкладке и refresh. Другие metadata в снимке отсутствуют. Непустой diff проверен отдельно на контролируемом ответе API. Чистая установка через `npm ci --prefix frontend` и запуск Vite проверены в отдельной копии; proxy вернул JSON backend, а реальный браузерный тест прошёл.

## Этап 4.1: чистый Compose и план из десяти пакетов

4 ноября 2026 года поднят Compose-проект `guit-acceptance-20261104` с frontend на `127.0.0.1:18933`. Для проекта Compose создал новые volumes: `guit-acceptance-20261104_postgres-data`, `guit-acceptance-20261104_gentoo-data`, `guit-acceptance-20261104_overlay-data` и `guit-acceptance-20261104_metadata-cache`. Первичная публикация Gentoo завершилась автоматически.

В доверенном каталоге backend настроены `gentoo-zh` (`https://github.com/gentoo-zh/overlay.git`) и `umbriel-overlay` (`https://github.com/vovanbl411/umbriel-overlay.git`), для обоих master — `gentoo`. В UI заполнено только поле «План emerge»; поле локально патченных пакетов оставлено пустым. SHA-256 текста плана: `d79625731052a7761033d0289fd267c7e64eb97bd1a243a3d777a22246e49196`.

```text
[ebuild     U ~] x11-terms/kitty-shell-integration-0.50.0::gentoo [0.49.2::gentoo] 12077 KiB
[ebuild     U ~] x11-terms/kitty-terminfo-0.50.0::gentoo [0.49.2::gentoo] 0 KiB
[ebuild     U  ] app-containers/docker-compose-5.6.0:2::gentoo [5.5.1:2::gentoo] 4157 KiB
[ebuild     U ~] www-client/firefox-l10n-159.0.1:0/rapid::gentoo [159.0:0/rapid::gentoo] L10N="-ach -af -an -ar -ast -az -be -bg -bn -br -bs -ca -ca-valencia -cak -cs -cy -da -de -dsb -el -en-CA -en-GB -eo -es-AR -es-CL -es-ES -es-MX -et -eu -fa -ff -fi -fr -fur -fy -ga -gd -gl -gn -gu -he -hi -hr -hsb -hu -hy -ia -id -it -ja -ka -kab -kk -km -kn -ko -lij -lt -lv -mk -mr -ms -my -nb -ne -nl -nn -oc -pa -pl -pt-BR -pt-PT -rm -ro -sc -sco -si -sk -skr -sl -son -sq -sr -sv -szl -ta -te -th -tl -tr -trs -uk -ur -uz -vi -xh -zh-CN -zh-TW" 0 KiB
[ebuild     U  ] dev-libs/expat-2.10.0::gentoo [2.9.0::gentoo] USE="unicode -examples -static-libs -test" 531 KiB
[ebuild     U  ] net-misc/rclone-1.75.2::gentoo [1.75.1::gentoo] 33956 KiB
[ebuild     U  ] dev-libs/libei-1.6.0-r1::gentoo [1.6.0::gentoo] USE="(systemd) (-elogind) -test" 0 KiB
[ebuild     U ~] app-admin/chezmoi-2.74.0::gentoo-zh [2.73.3::gentoo-zh] 11897 KiB
[ebuild     U ~] gui-libs/xdg-desktop-portal-umbriel-0.1.0_p20261101154307::umbriel-overlay [0.1.0_p20261030114808::umbriel-overlay] USE="picker" 104 KiB
[ebuild     U ~] gui-wm/umbriel-0.1.0_pre20261103013457::umbriel-overlay [0.1.0_pre20261031040829::umbriel-overlay] USE="jemalloc screencast" 2070 KiB
```

UI создал анализ `55dbe68a-3e8e-423d-a7a2-f5d44f085992`. Parser вернул `parsed`, результат содержит 10 записей и 10 отчётов. Все 20 Lookup завершились `found`, у обеих сторон каждой записи есть SHA-256 и provenance. Сохранённый анализ повторно открыт в браузере; для всех десяти карточек проверены статус, отображение `before`/`after`, ревизии в `Provenance` и причины в `Ограничения сравнения`.

Опубликованные ревизии репозиториев:

- `gentoo`: `e5e514491d2c320d947d12da59d399094367a0b0`.
- `gentoo-zh`: `16005b518245e2a5d63ed25c5f4338484689bbdc`.
- `umbriel-overlay`: `7b73f8dfac5f5896b088def7e1670e7073437947`.

Исторические ревизии, использованные для `before`: `gentoo` — `88a930a93edbc5b6e21fbf80748e649281dae402`; `gentoo-zh` — `ed2af7727494ac4a589c8bfb7b9e5fb5c377193e`; `umbriel-overlay` — `1b7e362cd9794cd83ff271c99c98d4c7c5b0f8f8`.

| Запись | Результат сравнения | Основание / сравниваемые поля | Ревизия `before` → `after` |
|---|---|---|---|
| `x11-terms/kitty-shell-integration` | `partial` | `EAPI`, `SLOT`; `IUSE_MISSING` на `before` и `after` | `gentoo` e5e51449 → `gentoo` e5e51449 |
| `x11-terms/kitty-terminfo` | `partial` | `EAPI`, `SLOT`; `IUSE_MISSING` на `before` и `after` | `gentoo` e5e51449 → `gentoo` e5e51449 |
| `app-containers/docker-compose` | `unavailable` | `EAPI_UNSUPPORTED` на `before` и `after`, EAPI указан в строке 4 cache | `gentoo` e5e51449 → `gentoo` e5e51449 |
| `www-client/firefox-l10n` | `available` | `EAPI`, `SLOT`, `IUSE`; различий в сравниваемых полях нет | `gentoo` e5e51449 → `gentoo` e5e51449 |
| `dev-libs/expat` | `available` | `EAPI`, `SLOT`, `IUSE`; различий в сравниваемых полях нет | `gentoo` e5e51449 → `gentoo` e5e51449 |
| `net-misc/rclone` | `partial` | `EAPI`, `SLOT`; `IUSE_MISSING` на `before` и `after` | `gentoo` e5e51449 → `gentoo` e5e51449 |
| `dev-libs/libei` | `available` | `EAPI`, `SLOT`, `IUSE`; различий в сравниваемых полях нет | `gentoo` 88a930a9 → `gentoo` e5e51449 |
| `app-admin/chezmoi` | `partial` | `EAPI`, `SLOT`; `IUSE_MISSING` на `before` и `after` | `gentoo-zh` ed2af772 → `gentoo-zh` 16005b51 |
| `gui-libs/xdg-desktop-portal-umbriel` | `available` | `EAPI`, `SLOT`, `IUSE`; различий в сравниваемых полях нет | `umbriel-overlay` 1b7e362c → `umbriel-overlay` 7b73f8df |
| `gui-wm/umbriel` | `available` | `EAPI`, `SLOT`, `IUSE`; различий в сравниваемых полях нет | `umbriel-overlay` 1b7e362c → `umbriel-overlay` 7b73f8df |

Для `libei`, `chezmoi` и двух записей `umbriel-overlay` сторона `before` использовала историческую ревизию, указанную в provenance; остальные стороны использовали опубликованную ревизию соответствующего репозитория. Нагрузочный прогон остаётся отдельной проверкой 4.3.

## Этап 4.2: повторный анализ и повторное использование metadata

В том же работающем Compose-проекте `guit-acceptance-20261104` через UI повторно отправлен plan с SHA-256 `d79625731052a7761033d0289fd267c7e64eb97bd1a243a3d777a22246e49196`; поле локально патченных пакетов осталось пустым. Новый UUID: `853c15a7-9876-41b9-ae82-66a8b917fa2e`. Анализ завершился со статусом `completed`, Parser разобрал 10 записей, сохранено 10 отчётов.

Закреплённые ревизии совпали с первым прогоном:

| Repository | 4.1 | 4.2 | Результат |
|---|---|---|---|
| `gentoo` | `e5e514491d2c320d947d12da59d399094367a0b0` | `e5e514491d2c320d947d12da59d399094367a0b0` | без изменений |
| `gentoo-zh` | `16005b518245e2a5d63ed25c5f4338484689bbdc` | `16005b518245e2a5d63ed25c5f4338484689bbdc` | без изменений |
| `umbriel-overlay` | `7b73f8dfac5f5896b088def7e1670e7073437947` | `7b73f8dfac5f5896b088def7e1670e7073437947` | без изменений |

Во время анализа process trace зафиксировал `git fetch` в существующих mirror `/overlays/gentoo-zh.git` и `/overlays/umbriel-overlay.git`; вызовов `git clone --mirror` в trace не было. До и после запроса совпали inode каталогов (`8279624` и `8279677`), `refs/gentoo/published`, количество packed objects и по одному pack-файлу в каждом mirror. Backend не записывает успешное обновление mirror в application log: использование существующих mirror подтверждается командами `fetch` в process trace и состоянием volume до и после запроса. Ошибок refresh в Compose-логе не было.

Worker подтвердил попадание именно в файловый metadata cache. В его журнале за интервал `20:31:53–20:35:55 UTC` записано 20 строк `metadata cache hit`: 14 для `gentoo`, 2 для `gentoo-zh` и 4 для `umbriel-overlay`. Новых строк `metadata generated` за повторный прогон нет. В volume до и после оставалось 20 JSON-файлов cache объёмом 136 KiB. Первая обработка, напротив, оставила 20 событий `metadata generated` в журнале. Эти события соответствуют ветке успешного `Store.Load` в `backend/cmd/metadata-worker/main.go`; статус Lookup `found` отдельно для подтверждения cache hit не использовался.

Промахов metadata cache не было, поэтому изменений ревизий и вызванных ими промахов для разбора нет. Этап 4.3 с нагрузочным планом остаётся отдельной проверкой.

## Этап 4.3: повторы и общий лимит генерации

Проверка выполнена на source revision `e18090b`. Для большого задания взяты те же десять строк из 4.1; весь блок повторён 100 раз без перестановки строк. Вход был синтетическим, отдельный реальный план не использовался. SHA-256 текста из 1000 строк: `602435d595cd660b277b4f2f2c90e840e6f7418d1d4287812e9762da9ef7bf0d`.

```sh
awk '{ rows[NR] = $0 } END { for (i = 0; i < 100; i++) for (j = 1; j <= NR; j++) print rows[j] }' /tmp/guit-acceptance-4.1.plan > /tmp/guit-acceptance-4.3-plan
node /tmp/guit-acceptance-4.3-large.mjs
node /tmp/guit-acceptance-4.3-large-poll.mjs
```

UI принял задание `81eb752b-d5c4-4a19-b516-163b7721a28b`. Оно завершилось со статусом `completed`: Parser сохранил 1000 записей и создано 1000 отчётов. Проверка результата подтвердила, что каждая из десяти строк вошла 100 раз, порядок строк повторённого блока сохранён, а каждый отчёт ссылается на свой индекс записи. Получено 20 уникальных metadata-запросов (`Query`), прогресс завершился на `20/20`. Worker получил 20 запросов `/prepare`; в журнале за `20:54:14–20:58:23 UTC` есть ровно 20 `metadata cache hit` — по одному на каждый уникальный запрос, требующий worker. Ревизии совпали с 4.1/4.2: `gentoo` `e5e514491d2c320d947d12da59d399094367a0b0`, `gentoo-zh` `16005b518245e2a5d63ed25c5f4338484689bbdc`, `umbriel-overlay` `7b73f8dfac5f5896b088def7e1670e7073437947`.

Общий лимит проверен отдельно шестью одновременно отправленными однострочными анализами с разными Gentoo package/version keys. Использован Compose-проект `guit-acceptance-20261104-concurrency`: опубликованный Gentoo volume подключён только для чтения; backend закреплён на ревизии `e5e514491d2c320d947d12da59d399094367a0b0`. Для metadata cache создан отдельный volume `guit-acceptance-20261104-concurrency_metadata-cache`. Перед запуском в cache было 0 файлов. Настроенный `METADATA_WORKER_GENERATION_LIMIT` — `2`.

```sh
POSTGRES_PASSWORD=acceptance-only FRONTEND_PORT=18934 GENTOO_UPDATE_INTELLIGENCE_OVERLAY_CATALOG='[]' \
  docker compose --env-file /dev/null -p guit-acceptance-20261104-concurrency \
  -f compose.yaml -f /tmp/guit-acceptance-4.3-compose-override.yaml \
  up -d --no-build backend metadata-worker postgres
node /tmp/guit-acceptance-4.3-concurrent.mjs
docker exec guit-acceptance-20261104-concurrency-metadata-worker-1 \
  sh -c 'cat /test-metrics/events.log; cat /test-metrics/maximum'
docker logs guit-acceptance-20261104-concurrency-metadata-worker-1
```

Все шесть анализов завершились успешно, каждый сохранил одну запись и один отчёт; совокупно они запросили 12 различных metadata keys. UUID: `5d33262b-2743-4a0e-b5f6-f11bb332ec4d`, `fdab2fd8-71c9-4eaf-8122-8979f8a21107`, `828c8c22-3670-4d3c-9e7f-b84d1137d335`, `7eb51888-2ea6-4b30-824e-4bd42310d0d2`, `ed790131-c352-41ac-9758-72521f09d606`, `59c2bd7f-4aac-4a87-ae99-29800e3937fc`. Worker записал 12 `metadata generated` для этих разных keys, все с Gentoo revision `e5e514491d2c320d947d12da59d399094367a0b0`; каждый вызов `egencache` завершился с кодом 0. Тестовый cache после прогона содержал 12 файлов. Измеритель вокруг запуска `egencache` зафиксировал максимум `2` одновременных процесса, после завершения active count вернулся к `0`. Таким образом, лимит проверен на реальных промахах в общем worker при конкурирующих анализах; прогретый основной cache не использовался.
