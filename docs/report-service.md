# Контракт внутреннего reportservice

`internal/reportservice` — внутренний слой координации между одной записью Parser, переданным Metadata Provider и существующим `changereport.Build`. Это не сетевой сервис. Пакет получает snapshots через Provider и собирает отчёт; собственного I/O в нём нет.

## API

Пакет предоставляет узкий интерфейс Provider и функцию сборки отчёта:

```go
type MetadataLookup interface {
	Lookup(metadataprovider.Query) (metadataprovider.LookupResult, error)
}

func Build(
	plan parser.Result,
	entryIndex int,
	provider MetadataLookup,
) (Result, error)

type Result struct {
	Report       changereport.Report
	BeforeLookup LookupOutcome
	AfterLookup  LookupOutcome
}

type SkipReason string

const SkipBindingRejected SkipReason = "BINDING_REJECTED"

type LookupOutcome struct {
	Query      *metadataprovider.Query
	Result     *metadataprovider.LookupResult
	Err        error
	SkipReason SkipReason
}
```

Ошибки `ErrProviderRequired` и `ErrInvalidLookupResult` распознаются через `errors.Is`. Некорректный индекс сохраняет `changereport.ErrInvalidEntryIndex`. `LookupOutcome` содержит точный `Query` выполненного запроса, исходный корректный `metadataprovider.LookupResult`, ошибку `Lookup` или причину пропуска.

## Outcome Lookup

Каждый исход сохраняет достаточно данных, чтобы отличить пропущенный запрос, штатный ответ Provider и ошибку чтения:

| Сценарий | Query | Result | Err | SkipReason |
|---|---|---|---|---|
| Lookup не выполнялся (`not_requested`) | отсутствует | отсутствует | отсутствует | `BINDING_REJECTED` |
| Lookup выполнен успешно | точный запрос | исходный результат Provider | отсутствует | пуст |
| Lookup завершился ошибкой | точный запрос | отсутствует | исходная ошибка с сохранённой цепочкой обёрток | пуст |

При успешном вызове сохраняется статус Provider без преобразования: `found`, `not_found` или `repository_unavailable`. Корректные сочетания — `found` с непустым `Snapshot`, а `not_found` и `repository_unavailable` с nil `Snapshot`. Неизвестный или пустой статус, `found` без снимка и снимок при статусе отсутствия дают ошибку `ErrInvalidLookupResult`; ошибочный ответ и снимок не используются. Ошибка Provider, включая `metadataprovider.ErrRead`, имеет приоритет над любым возвращённым `Result` и сохраняет исходную цепочку для `errors.Is`. Нельзя объединять `not_found`, `repository_unavailable` и ошибку чтения в один исход.

Для не выполненного вызова используется `not_requested`, выраженный заполненным `SkipReason`; отдельное дублирующее поле статуса не добавляется. При ошибке или некорректном ответе одной стороны её outcome сохраняет соответствующий запрос и ошибку, а обработка второй стороны продолжается.

## Алгоритм сборки

1. Сначала вызвать `changereport.Build(plan, entryIndex, nil, nil)`. Этот предварительный вызов проверяет индекс и применяет действующие проверки операции, установленного контекста, repository и другие правила связи. Без snapshots он не выполняет `metadatadiff.Compare`.
2. Если индекс некорректен, вернуть ошибку, распознаваемую как `changereport.ErrInvalidEntryIndex` через `errors.Is`. Не вызывать Provider, не допускать panic и не выбирать другую запись.
3. Для допустимого индекса требуется переданный Provider. Обычный nil и интерфейс с nil-указателем возвращают распознаваемую ошибку `ErrProviderRequired` и нулевой `Result`. Отсутствующий Provider проверяется до решения о пропуске запросов.
4. Разрешить Lookup только если предварительный отчёт содержит хотя бы одну диагностику и каждая имеет код `changereport.CodeSnapshotMissing` (`SNAPSHOT_MISSING`). При пустом списке, любой другой или неизвестной диагностике не вызывать Lookup ни для одной стороны, вернуть предварительный `Report` и установить обоим `SkipReason` значение `SkipBindingRejected`. Предварительная проверка закрыта по умолчанию. Не воспроизводить отдельно проверки операции, `InstalledContext`, repository или другие проверки `ChangeReport`.
5. Сформировать точные запросы из выбранной записи:
   - `before`: `Package` записи, версия и repository единственного установленного контекста;
   - `after`: `Package` записи, целевая версия и repository.
6. Вызвать один и тот же Provider последовательно, сначала для `before`, затем для `after`. Не выполнять повторные попытки или fallback. Ошибка первого вызова не отменяет второй.
7. Только структурно корректный snapshot из результата со статусом `found` передать в повторный `changereport.Build`. Для `not_found`, `repository_unavailable`, ошибки или некорректного ответа соответствующий snapshot отсутствует. Чужой, но структурно допустимый snapshot передаётся в Build, который проверяет его связь с записью.
8. Вернуть окончательный `Report` и оба `LookupOutcome`. Ошибки Lookup и нарушения контракта остаются в outcome; внешняя ошибка функции ограничена индексом, отсутствующим Provider и неожиданной ошибкой повторного `Build`.

Повторный `changereport.Build` остаётся владельцем проверки, что найденные snapshots соответствуют записи: `Package`, версия, repository и применимые SLOT/SUBSLOT. Reportservice не изменяет и не фильтрует `Comparison`, не заменяет диагностики Parser результатами Lookup и не дублирует правила связи.

Неизвестная диагностика предварительного отчёта блокирует запросы так же, как любая другая диагностика, отличная от `SNAPSHOT_MISSING`. Это правило должно быть покрыто тестом, чтобы добавление нового кода `BindingCode` не расширило разрешённый набор неявно.

## Отчёт и ограничения результата

Если предварительная проверка запрещает запросы, итогом служит предварительный `Report`; обе стороны имеют `not_requested` с объяснением. Если запросы выполнялись, результатом служит отчёт повторного `changereport.Build` с snapshots только для найденных сторон.

Диагностика `SNAPSHOT_MISSING` в `Report` показывает, что снимок отсутствует. `LookupOutcome` рядом объясняет причину: точная версия не найдена, repository не обслуживается или чтение завершилось ошибкой. Контракт `changereport.Build` при этом не меняется.

Отчёт возвращается без фильтрации `Comparison`, изменений и evidence; `ParserStatus` и `ParserDiagnostics` сохраняются. Ошибка получения не становится успешным пустым diff. Findings Analyzer остаются отдельно: reportservice не принимает, не создаёт и не изменяет их. Approval не реализован.

Ошибка получения не становится успешным пустым diff. Findings Analyzer остаются отдельными: reportservice не принимает, не создаёт и не изменяет их. Approval в этот контракт не входит.

## Границы и безопасность

- Вызывающий код заранее открывает и передаёт Provider.
- Reportservice не выбирает и не меняет commit и не открывает другие providers.
- Reportservice самостоятельно не выполняет сетевые запросы или Git-команды.
- Он не читает файлы и не выполняет ebuild.
- Контракт не вводит постоянные ID, время, API или хранение.
- Входные структуры не изменяются.

## Проверки

Тестовая реализация `MetadataLookup` с записью вызовов проверяет:

- точность двух запросов, порядок `before` → `after` и количество вызовов;
- `found` для обеих сторон;
- `not_found` и `repository_unavailable` по каждой стороне;
- ошибку `before` при успешном `after`, успешный `before` при ошибке `after` и ошибки обеих сторон;
- сохранение `errors.Is` и точных запросов в ошибочных outcomes;
- проверку статуса и наличия `Snapshot`, приоритет ошибки над совместно возвращённым результатом и продолжение второго запроса после некорректного ответа;
- пропуск Lookup для неподдерживаемой записи;
- запрет запросов для `NS` / `install`, неоднозначного установленного контекста и неизвестного repository, а также закрытое поведение predicate для неизвестного `BindingCode`;
- некорректный индекс, обычный и typed-nil Provider, а также реализацию Provider с value receiver;
- сохранение Parser partial и его диагностик;
- отказ повторного `Build` связывать полученные чужие snapshots;
- неизменность входных структур, включая вложенные slices;
- повторяемость результата при одинаковых ответах Provider.

Интеграционный тест проходит цепочку: временный Git-репозиторий с явно синтетическими cache-файлами → настоящий Metadata Provider → reportservice → `ChangeReport` для синтетической записи Parser. Он проверяет commit, версии, изменения и evidence без сети и клонирования Gentoo. Author/committer и отключение подписи задаются только для Git-команд теста.
