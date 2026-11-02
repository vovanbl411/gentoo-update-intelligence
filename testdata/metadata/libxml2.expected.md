# Ожидаемый результат: `dev-libs/libxml2`

Сравниваются [snapshot `2.13.9`](libxml2-2.13.9.md5-cache) и [snapshot `2.14.6`](libxml2-2.14.6.md5-cache) из commit `ac2e2890e7702bcfa0dab44338707240894de8c7` репозитория `gentoo` на зеркале `gentoo-mirror/gentoo`. Происхождение и SHA-256 указаны в [`provenance.md`](provenance.md).

## Поля

| Поле | Before: 2.13.9 | After: 2.14.6 | Evidence |
|---|---|---|---|
| EAPI | `8` | `8` | [`2.13.9`, строка 5](libxml2-2.13.9.md5-cache#L5); [`2.14.6`, строка 5](libxml2-2.14.6.md5-cache#L5) |
| Исходный SLOT | `2` | `2/16` | [`2.13.9`, строка 14](libxml2-2.13.9.md5-cache#L14); [`2.14.6`, строка 14](libxml2-2.14.6.md5-cache#L14) |
| Нормализованный SLOT | `2` | `2` | Выделен из исходных SLOT в строках 14 обеих сторон. |
| Нормализованный SUBSLOT | `2`, неявный | `16`, явный | Для EAPI 8 отсутствие `/SUBSLOT` означает SUBSLOT, равный SLOT. Исходные значения — строка 14 обеих сторон. |
| IUSE | `examples icu lzma +python readline static-libs test python_targets_python3_12 python_targets_python3_13 python_targets_python3_14 abi_x86_32 abi_x86_64 abi_x86_x32 abi_mips_n32 abi_mips_n64 abi_mips_o32 abi_s390_32 abi_s390_64` | `icu +python readline static-libs test python_targets_python3_12 python_targets_python3_13 python_targets_python3_14 abi_x86_32 abi_x86_64 abi_x86_x32 abi_mips_n32 abi_mips_n64 abi_mips_o32 abi_s390_32 abi_s390_64` | [`2.13.9`, строка 8](libxml2-2.13.9.md5-cache#L8); [`2.14.6`, строка 8](libxml2-2.14.6.md5-cache#L8) |

## Изменения

- `EAPI`: `8` → `8`, различий нет.
- Нормализованный `SLOT`: `2` → `2`, различий нет.
- Нормализованный `SUBSLOT`: `2` → `16`.
- IUSE removed: `examples`, `lzma`.
- IUSE added: пусто.
- Изменения объявленных defaults у общих флагов: отсутствуют. `+python` указан с обеих сторон.

Ожидаемый статус — `available`; `compared_fields` содержит `EAPI`, `SLOT/SUBSLOT` и `IUSE`. Для сравнения этих трёх групп полей `limitations` пуст. Пустые изменения по EAPI и SLOT означают только отсутствие различий в этих сравнимых полях.

Удаление `examples` или `lzma` из IUSE не доказывает, что любой из этих флагов был включён у пользователя. Изменение SUBSLOT само по себе не определяет конкретный список пакетов для пересборки. Эти строки относятся к выбранным состояниям repository metadata, а не подтверждают metadata установленной пользовательской сборки.
