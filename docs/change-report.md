# Ограниченный ChangeReport

Пакет `backend/internal/changereport` связывает выбранную запись Parser с подготовленными metadata-снимками, проверяет соответствие снимков записи, вызывает `metadatadiff.Compare` и формирует три раздела. Он не пересчитывает diff, не принимает и не меняет findings Analyzer, не реализует approval и не выполняет I/O.

## Go API

```go
func Build(
    plan parser.Result,
    entryIndex int,
    before, after *metadatadiff.Snapshot,
) (Report, error)
```

Функция детерминирована, не изменяет входы и не использует сеть, БД, Redis, постоянные ID или время. `nil` snapshot означает отсутствие снимка. Единственная ошибка функции — `ErrInvalidEntryIndex`, если индекс отрицательный или не меньше `len(plan.Entries)`. Ошибку можно проверить через `errors.Is`. При ней возвращается нулевой `Report`; другой индекс не подбирается. Все остальные ограничения представлены в `Report` с `nil`-ошибкой.

## Результат

`Report` содержит:

- `EntryIndex` — индекс записи Parser, начиная с нуля;
- `SourceLines` — копию `Entry.SourceLines`, номера исходных строк начинаются с единицы;
- `ParserStatus` и `ParserDiagnostics` — исходный результат Parser; вложенные `SourceLines` диагностик также копируются;
- `Gentoo` — результат metadata-связки или диагностики отказа;
- `EffectiveConfig` и `Upstream` — всегда `unavailable` с причиной `NOT_IMPLEMENTED`;
- `InterpretationLimitation` — отдельное ограничение: сравниваются выбранные repository metadata, соответствие установленной сборке не подтверждено.

Статус Parser независим от статуса Gentoo. Поэтому `parser.StatusPartial` и его диагностики остаются в отчёте, даже если Gentoo сравнение имеет статус `available`.

`GentooSection` содержит `Status`, `Comparison` и `Diagnostics`. После успешной проверки связи `Comparison` содержит полный результат `metadatadiff.Compare`, а `Status` равен `Comparison.Status`. Сохраняются snapshots, SHA-256, `ComparedFields`, `Changes`, evidence и `Limitations`. Если проверка связи отклонена, `Status` равен `unavailable`, `Comparison` равен `nil`, а `Diagnostics` объясняют отказ; семантические изменения не сообщаются.

`BindingDiagnostic` содержит `Code`, необязательные `Side` и `Field`, а также необязательные `Expected` и `Actual`. Необязательные значения представлены указателями: `nil` означает отсутствие значения, а указатель на пустую строку — известное пустое значение. Составные ссылки не кодируются строками для последующего разбора.

### Коды диагностики связи

| Код | Значение полей |
|---|---|
| `OPERATION_UNSUPPORTED` | Поле `operation`; `Expected` — `upgrade, downgrade or reinstall`, `Actual` — операция записи. |
| `INSTALLED_CONTEXT_MISSING` | Поле `installed_context`; в плане нет установленной версии. |
| `INSTALLED_CONTEXT_AMBIGUOUS` | Поле `installed_context`; `Expected` — ровно одна версия, `Actual` — фактическое число. |
| `SNAPSHOT_MISSING` | Сторона и поле `snapshot`; соответствующий снимок не передан. |
| `REPOSITORY_UNKNOWN` | Сторона и поле `repository`; repository плана неизвестен или пуст. |
| `PACKAGE_MISMATCH` | Сторона, поле `package`, ожидаемый `category/name` из записи и `category/name` снимка. |
| `VERSION_MISMATCH` | Сторона, поле `version`, ожидаемая версия из плана и версия снимка. |
| `REPOSITORY_MISMATCH` | Сторона, поле `repository`, repository плана и снимка. |
| `SLOT_MISMATCH` | Сторона, поле `SLOT`, явно показанный SLOT плана и нормализованный SLOT metadata. |
| `SUBSLOT_MISMATCH` | Сторона, поле `SUBSLOT`, явно показанный SUBSLOT плана и нормализованный SUBSLOT metadata. |

Диагностики собираются в фиксированном порядке. На первом этапе выполняются проверки операции и установленного контекста: сначала операция, затем отсутствие или неоднозначность контекста. На этапе снимков сначала проверяется наличие снимков (before, after), затем известность repository плана (before, after). На этапе идентичности для каждой стороны проверяются package, version и repository; стороны идут before, after. На этапе SLOT/SUBSLOT стороны также идут before, after, внутри стороны сначала SLOT, затем SUBSLOT. После этапа с одной или несколькими диагностиками дальнейшие проверки прекращаются.

## Проверка связи снимков

После проверки индекса для `R` / `reinstall` без установленного контекста действует отдельная ветка: сохраняется `INSTALLED_CONTEXT_MISSING`, затем проверяется только целевая сторона. Для неизвестного repository и отсутствующего after snapshot возвращаются диагностики стороны after. Если repository известен и snapshot доступен, проверяются package, version, repository и явно показанные SLOT/SUBSLOT. Before snapshot не используется и не восстанавливается. Даже корректный after оставляет `Gentoo.Status = unavailable` и `Comparison = nil`. Отсутствие installed само по себе не является ошибкой всего анализа.

Для остальных случаев порядок проверок в `Build`:

1. Проверить индекс записи. Некорректный индекс возвращает `ErrInvalidEntryIndex` и нулевой `Report`.
2. Проверить, что операция — `upgrade`, `downgrade` или `reinstall` и в `Installed.Versions` ровно одна версия. `NS` / `install` не поддерживается: показанная старая версия не считается заменяемой. Для обеих проверок текущего этапа диагностики можно вернуть вместе.
3. Проверить наличие снимков и непустые явно известные repository обеих сторон.
4. Для `upgrade`, `downgrade` и `reinstall` с единственной установленной версией `before` — эта показанная версия, а `after` — целевая версия. Проверить для before, затем after, что снимок соответствует `Entry.Package`, точной версии плана и repository этой стороны. Версии сравниваются буквально, включая `-rN`; направление `Compare` следует переданным сторонам. Автоматической сортировки, перестановки или подбора версий нет.
5. Проверить явно известные SLOT/SUBSLOT плана только при `Snapshot.Slot.State == valid`. SLOT сопоставляется с `Snapshot.Slot.Name`, SUBSLOT — с `Snapshot.Slot.Subslot`.
6. Если предыдущие проверки прошли, вызвать `metadatadiff.Compare(before, after)` и сохранить весь результат.

Проверки этапа 3 собирают все применимые диагностики: сначала отсутствие снимков, затем неизвестные repository плана. Проверки идентичности и SLOT/SUBSLOT также собирают все несовпадения текущего этапа в описанном порядке. После этапа с одной или несколькими диагностиками дальнейшие проверки прекращаются.

Неизвестные SLOT/SUBSLOT плана не заполняются догадками. Неявный SUBSLOT metadata, равный SLOT, не делает неизвестный SUBSLOT плана известным. Если SLOT metadata имеет состояние `missing` или `invalid`, сборщик не объявляет несовпадение: ограничения обрабатывает `Compare`. После проверки связи Compare самостоятельно проверяет происхождение снимков, EAPI и доступность сравниваемых полей.

## Разделы без реализации

`EffectiveConfig` и `Upstream` всегда присутствуют и имеют статус `unavailable` и причину `NOT_IMPLEMENTED`. Это обозначает отсутствие реализации, а не сетевую ошибку, недоступность источника или проверенное отсутствие изменений. Эти разделы не содержат придуманных изменений или evidence.

Сравнение относится к выбранным repository metadata. Совпадение версии и repository с планом не подтверждает, что before metadata использовались при установленной сборке. Это ограничение области интерпретации не понижает корректный результат `available` до `partial`.

## Примеры и тесты

Положительный тест использует синтетическую запись upgrade `dev-libs/libxml2` `2.13.9` → `2.14.6` из `gentoo` и реальные metadata fixtures: [snapshot 2.13.9](../testdata/metadata/libxml2-2.13.9.md5-cache), [snapshot 2.14.6](../testdata/metadata/libxml2-2.14.6.md5-cache) и их [provenance](../testdata/metadata/provenance.md). Снимки нормализуются через `metadatadiff.Normalize`. Текущий metadata diff сравнивает EAPI 8, нормализованные SLOT/SUBSLOT и IUSE. Ожидаемый `Gentoo.Status` — `available`; `Comparison` равен результату `Compare`, SUBSLOT меняется `2` → `16`, IUSE удаляет `examples` и `lzma`. Тест проверяет evidence по строкам `14` для SLOT/SUBSLOT и `8` для IUSE. Два остальных раздела имеют причину `NOT_IMPLEMENTED`. Синтетическая запись не является реальным выводом emerge.

Синтетический downgrade тест использует те же metadata snapshots в обратном относительно версий направлении: установленная `2.14.6` становится `before`, целевая `2.13.9` — `after`. Результат равен `Compare(before, after)` без перестановки снимков; IUSE additions и removals относятся к целевому состоянию. Реальных downgrade fixtures пока нет.

Отрицательный тест связывает реальную запись `app-emulation/qemu` с индексом `2` из [Parser fixture](../testdata/parser/ebuild-updates.txt) со снимками libxml2. Он проверяет отказ по package и отсутствие `Comparison`, а не изменения QEMU.

Тесты также проверяют несовпадения package, версий и repository по обеим сторонам, переставленные snapshots, совпадающие и несовпадающие SLOT/SUBSLOT, неизвестный SLOT/SUBSLOT плана, неявный SUBSLOT metadata, отсутствующие snapshots, `install`, пустой и неоднозначный установленный контекст, неизвестный/пустой repository, статусы `partial` и `unavailable` от Compare, `parser.StatusPartial`, ошибки индекса, evidence и строки, неизменность вложенных входных срезов и детерминированность результата.
