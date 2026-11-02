# Происхождение снимков metadata

Оба файла — исходные записи `metadata/md5-cache` репозитория Gentoo из одного закреплённого commit. Содержимое получено напрямую и сохранено целиком.

| Поле | Значение |
|---|---|
| Имя репозитория | `gentoo` |
| URL зеркала | <https://github.com/gentoo-mirror/gentoo> |
| Commit | `ac2e2890e7702bcfa0dab44338707240894de8c7` |
| Способ получения | `curl` по HTTPS с `--fail --location` к `raw.githubusercontent.com`, по URL с полным commit SHA; файлы не были вычислены запуском ebuild или генератора metadata. |

## Соответствие файлов и хеши

| Версия | Исходный путь в репозитории | Ссылка на фиксированный commit | Локальный файл | SHA-256 содержимого |
|---|---|---|---|---|
| `2.13.9` | `metadata/md5-cache/dev-libs/libxml2-2.13.9` | [исходный файл](https://github.com/gentoo-mirror/gentoo/blob/ac2e2890e7702bcfa0dab44338707240894de8c7/metadata/md5-cache/dev-libs/libxml2-2.13.9) | [`libxml2-2.13.9.md5-cache`](libxml2-2.13.9.md5-cache) | `2e252a5dd5117202ed45b9ccdda024a61a9c7556eb224bde3321a8fedea3e2f3` |
| `2.14.6` | `metadata/md5-cache/dev-libs/libxml2-2.14.6` | [исходный файл](https://github.com/gentoo-mirror/gentoo/blob/ac2e2890e7702bcfa0dab44338707240894de8c7/metadata/md5-cache/dev-libs/libxml2-2.14.6) | [`libxml2-2.14.6.md5-cache`](libxml2-2.14.6.md5-cache) | `5345c9d0a4c0e7414a3a5bc41b34f77149dd5da3730f3a736d2189d8619da0b6` |

SHA-256 вычислен по полному содержимому каждого локального файла. Он не является Git blob SHA. Значение `_md5_` в последней строке cache-файла — поле исходной записи, а не SHA-256 snapshot.

Имя репозитория `gentoo` не является URL зеркала: источник в этом случае — зеркало `gentoo-mirror/gentoo`. Снимки фиксируют состояния repository metadata в указанном commit. Сравнение этих состояний не доказывает, что они соответствуют metadata пакета, установленного в системе пользователя.
