# Ожидаемые факты ebuild-updates-3.txt

[Исходный план](ebuild-updates-3.txt) сохранён без изменения текста. Команда и версия Portage неизвестны.

Ожидаются статус `parsed`, 43 записи в исходном порядке, 36 `upgrade` и 7 `reinstall`, без диагностик. Во входе нет сводки Portage: `Summary = nil`. Число 43 здесь — число строк пакетов, а не восстановленная сводка.

## Записи

Номер записи совпадает с номером исходной строки. «—» означает отсутствие явно показанной установленной версии. Размеры приведены в KiB после проверки и удаления разделителей тысяч.

| Строка | Пакет | Целевая версия | Установленная версия | Операция | Repository | KiB |
|---:|---|---|---|---|---|---:|
| 1 | `dev-lang/go` | `1.27.3` | `1.27.2` | `upgrade` | `gentoo` | 34311 |
| 2 | `app-containers/docker-cli` | `29.9.3` | `29.9.0` | `upgrade` | `gentoo` | 6984 |
| 3 | `app-admin/terraform` | `1.17.1` | `1.16.4` | `upgrade` | `gentoo` | 16807 |
| 4 | `dev-util/gitlab-cli` | `1.128.0` | `1.123.0` | `upgrade` | `gentoo` | 29573 |
| 5 | `app-containers/docker-buildx` | `0.38.2` | `0.37.1` | `upgrade` | `gentoo` | 15318 |
| 6 | `sys-libs/libxcrypt` | `4.5.2-r1` | `4.5.2` | `upgrade` | `gentoo` | 656 |
| 7 | `dev-libs/nss` | `3.133` | `3.132` | `upgrade` | `gentoo` | 85786 |
| 8 | `dev-python/uv` | `0.13.4-r1` | `0.12.7` | `upgrade` | `gentoo` | 51693 |
| 9 | `dev-lang/python` | `3.14.10_p1` | `3.14.10` | `upgrade` | `gentoo` | 23726 |
| 10 | `app-misc/ca-certificates` | `20260816.3.131.1-r1` | `20260816.3.131.1` | `upgrade` | `gentoo` | 86260 |
| 11 | `dev-util/bpftool` | `7.7.0-r2` | — | `reinstall` | `gentoo` | 0 |
| 12 | `sys-libs/libcap-ng` | `0.9.6` | `0.9.3` | `upgrade` | `gentoo` | 182 |
| 13 | `dev-python/pyjwt` | `2.15.1` | `2.13.0` | `upgrade` | `gentoo` | 129 |
| 14 | `dev-python/rfc3161-client` | `1.0.9` | `1.0.8` | `upgrade` | `gentoo` | 57272 |
| 15 | `dev-python/uv-build` | `0.13.4` | `0.12.7` | `upgrade` | `gentoo` | 454 |
| 16 | `dev-python/pyasn1` | `0.6.5` | `0.6.4` | `upgrade` | `gentoo` | 152 |
| 17 | `dev-python/flit-core` | `4.1.0` | `4.0.2` | `upgrade` | `gentoo` | 63 |
| 18 | `dev-python/platformdirs` | `4.12.3` | `4.12.1` | `upgrade` | `gentoo` | 68 |
| 19 | `dev-python/sigstore` | `4.5.1` | `4.5.0` | `upgrade` | `gentoo` | 414 |
| 20 | `dev-util/bindgen` | `0.73.2` | `0.72.1` | `upgrade` | `gentoo` | 7357 |
| 21 | `dev-python/pyclip` | `0.7.0-r2` | — | `reinstall` | `guru` | 0 |
| 22 | `dev-python/markupsafe` | `3.0.4` | `3.0.3` | `upgrade` | `gentoo` | 160 |
| 23 | `dev-util/github-cli` | `2.103.3` | `2.102.0` | `upgrade` | `gentoo` | 78051 |
| 24 | `dev-python/wcwidth` | `0.9.2` | `0.9.1` | `upgrade` | `gentoo` | 933 |
| 25 | `dev-python/setuptools-scm` | `10.3.2` | `10.2.3` | `upgrade` | `gentoo` | 83 |
| 26 | `dev-python/mako` | `1.4.3` | `1.4.1` | `upgrade` | `gentoo` | 404 |
| 27 | `app-containers/slirp4netns` | `1.3.5` | `1.3.4` | `upgrade` | `gentoo` | 64 |
| 28 | `dev-libs/blake3` | `1.8.7` | `1.8.5` | `upgrade` | `gentoo` | 263 |
| 29 | `sys-apps/portage` | `3.0.83-r1` | `3.0.82.2-r1` | `upgrade` | `gentoo` | 1327 |
| 30 | `dev-util/mesa_clc` | `26.2.6` | — | `reinstall` | `gentoo` | 67094 |
| 31 | `net-vpn/openvpn` | `2.7.6` | `2.7.5` | `upgrade` | `gentoo` | 2058 |
| 32 | `dev-python/gbinder` | `1.3.1` | — | `reinstall` | `guru` | 0 |
| 33 | `net-misc/openssh` | `10.6_p2` | `10.5_p1` | `upgrade` | `gentoo` | 2301 |
| 34 | `app-containers/docker` | `29.9.3` | `29.9.0` | `upgrade` | `gentoo` | 21915 |
| 35 | `media-libs/libva-intel-media-driver` | `26.3.5-r2` | `26.3.5-r1` | `upgrade` | `gentoo` | 14 |
| 36 | `net-vpn/networkmanager-openvpn` | `1.12.5` | `1.12.3` | `upgrade` | `gentoo` | 714 |
| 37 | `virtual/linux-sources` | `3-r13` | `3-r12` | `upgrade` | `gentoo-zh` | 0 |
| 38 | `app-containers/waydroid` | `1.6.2` | — | `reinstall` | `guru` | 0 |
| 39 | `media-libs/mesa` | `26.2.6` | — | `reinstall` | `gentoo` | 0 |
| 40 | `dev-util/perf` | `7.2` | — | `reinstall` | `gentoo` | 0 |
| 41 | `app-editors/vscode` | `1.145.0` | `1.144.0-r1` | `upgrade` | `gentoo` | 369924 |
| 42 | `app-editors/zed` | `1.27.0` | `1.26.2` | `upgrade` | `zed-overlay` | 15166 |
| 43 | `gui-wm/umbriel` | `0.1.0_pre20261104132027` | `0.1.0_pre20261103013457` | `upgrade` | `umbriel-overlay` | 2072 |

Repository установленной стороны совпадает с указанным в таблице там, где эта сторона показана. Для пересборок `Installed.Versions` пуст; target не копируется в installed.

## SLOT, SUBSLOT и маркеры

- Строка 1: target `0/1.27.3`, installed `0/1.27.2`.
- Строка 6: target и installed `0/1`.
- Строка 9: target и installed SLOT `3.14`; SUBSLOT не указан.
- В остальных строках SLOT/SUBSLOT не указаны и остаются неизвестными.
- Отдельный `~` показан в строках 2, 3, 4, 7, 21, 23, 30, 32, 34, 35, 38, 39, 40, 42 и 43; он сохраняется независимо от операции.

## Группы флагов пересборок

Группы и значения сохраняются в исходном порядке, включая скобки, регистр, знаки и маркеры изменения:

- Строка 11, bpftool: `USE`, `LLVM_SLOT`; `LLVM_SLOT="22 -19 -20 -21 (-23) (-18%)"`.
- Строки 21 и 32, pyclip и gbinder: `USE`, `PYTHON_TARGETS`; `PYTHON_TARGETS="python3_14 -python3_13 (-python3_12%)"`.
- Строка 30, mesa_clc: `USE`, `LLVM_SLOT`, `VIDEO_CARDS`.
- Строка 38, waydroid: `USE`, `PYTHON_SINGLE_TARGET`; `PYTHON_SINGLE_TARGET="python3_14 -python3_13 (-python3_12%)"`.
- Строка 39, mesa: `USE`, `CPU_FLAGS_X86`, `LLVM_SLOT`, `VIDEO_CARDS`; сохраняются все показанные VIDEO_CARDS, включая `(-freedreno)`, `(-imagination)`, `(-v3d)`.
- Строка 40, perf: `USE`, `LLVM_SLOT`, `PYTHON_SINGLE_TARGET`.

Полное содержимое всех семи наборов групп проверяет `TestRebuildFlagGroups` в [тестах Parser](../../backend/internal/parser/reinstall_test.go). Для `(-18%)` проверяются имя `18`, состояние `disabled`, исходный токен и маркеры `(`, `)`, `-`, `%`.

## Сервисы и UI

Управляемый Provider получает 79 уникальных запросов: 36 пар before/after для обновлений и 7 after для пересборок. Возвращаются 43 отчёта в исходном порядке; итоговый прогресс — 79/79. При корректных найденных snapshots доступны 36 сравнений. Для семи пересборок before Lookup пропущен с `BINDING_REJECTED`, сохраняется `INSTALLED_CONTEXT_MISSING`, сравнение недоступно.

Без пользовательского контекста этот fixture не создаёт findings: показанные маркеры `%` относятся к другим именованным группам, а не к USE. Это не объясняет причины пересборок и не подтверждает безопасность плана. UI показывает 43 карточки и семь подписей «Пересборка», без предупреждения о неполном разборе и без выдуманного общего счётчика Portage.
