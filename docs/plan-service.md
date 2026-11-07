# Контракт внутреннего planservice

`backend/internal/planservice` вычисляет единый результат для целого текстового плана. Он соединяет существующие Parser, Analyzer и reportservice; не вводит сохраняемую сущность `Analysis`, новые правила или общий статус плана.

## API

```go
func Analyze(
	input string,
	context analyzer.Context,
	provider reportservice.MetadataLookup,
) (Result, error)

type Result struct {
	Plan     parser.Result
	Analysis analyzer.Result
	Reports  []reportservice.Result
}
```

`Plan` — полный результат одного `parser.Parse(input)`. `Analysis` — полный результат одного отдельного `analyzer.Analyze(Plan, context)`. `Reports` сохраняет полные результаты последовательных вызовов `reportservice.Build(Plan, i, provider)` для каждого `Plan.Entries[i]`. В рамках `Analyze` результат или ошибка `Lookup` сохраняется по точному `metadataprovider.Query`; повторный запрос использует этот исход и не вызывает Provider снова. Обёртка кэша также передаёт `RepositoryFailure(string) error`, если исходный Provider поддерживает этот необязательный метод.

HTTP использует `AnalyzeParsedProgressive`: для Provider с поддержкой конкурентных Lookup он выполняет до двух уникальных запросов одновременно, для остальных — по одному. Готовые отчёты передаются через последовательный callback в порядке плана; результат и ошибка каждого запроса используются повторно. CLI и `Analyze` сохраняют последовательное поведение, описанное ниже.

## Алгоритм и связи

1. Один раз разобрать исходный текст через `parser.Parse`.
2. Один раз передать результат Parser и пользовательский `context` в `analyzer.Analyze`.
3. Подготовить `Reports` и кэш Lookup для этого вызова `Analyze`, затем последовательно вызвать `reportservice.Build` для каждого индекса `Plan.Entries`, начиная с нуля. Кэш сохраняет успешные ответы и ошибки по полному `Query`. Запросы с одинаковым `Query` в пределах вызова используют один и тот же исход.
4. При успехе вернуть исходные результаты Parser и Analyzer вместе со всеми отчётами.

Порядок записей Parser не меняется. На каждую запись приходится один отчёт; повторяющиеся пакеты не объединяются. При `nil error` выполняется `len(Reports) == len(Plan.Entries)`, а `Reports[i].Report.EntryIndex == i`. Findings Analyzer ссылаются на записи через `Finding.EntryIndex`; finding по blocker сохраняет `EntryIndex = nil` и `UnresolvedBlockerEvidence.BlockerIndex`.

`Blockers`, `Removals`, сводки и диагностики остаются в `Plan`. Они не становятся отдельными записями для reportservice. Finding категории `BLOCKER` не останавливает сбор отчётов.

## Отсутствие записей и Provider

Статусы Parser `partial` и `unsupported` — штатные результаты, а не внешние ошибки. При `partial` reportservice вызывается для всех извлечённых записей.

Если `Plan.Entries` пуст, функция возвращает `Plan` и `Analysis` без вызова Provider. Это относится к пустому или неподдерживаемому тексту, а также плану только с blockers и/или removals. Срез `Reports` пуст.

Если есть хотя бы одна запись, Provider требуется согласно контракту reportservice, даже если все записи — `NS` и Lookup будет пропущен. planservice не проверяет обычный или typed-nil Provider самостоятельно. Первый вызов `reportservice.Build` возвращает `ErrProviderRequired`; `errors.Is` сохраняет распознавание этой причины, а `Analyze` возвращает нулевой `Result`.

Ошибки Lookup, статусы `not_found` и `repository_unavailable`, `reportservice.ErrInvalidLookupResult` и причины пропуска хранятся в соответствующем `reportservice.Result`. Они не становятся внешней ошибкой и не отменяют обработку следующих записей.

Если `reportservice` отклоняет binding и задаёт `SkipBindingRejected`, Lookup не выполняется, а `BeforeLookup` и `AfterLookup` сохраняют этот `SkipReason` без Query и Result. Если исходный Provider поддерживает `RepositoryFailure`, planservice передаёт этот вызов через свою обёртку: ошибка подготовки установленных repository добавляется к `BeforeLookup.Err`, а ошибка целевого repository остаётся в `AfterLookup.Err`. Несколько ошибок установленных repository объединяются через `errors.Join`. Поэтому причина подготовки остаётся на стороне записи, даже когда binding не допускает metadata lookup. Provider без `RepositoryFailure` сохраняет причины пропуска без дополнительной ошибки.

Любая внешняя ошибка `reportservice.Build` останавливает цикл. `Analyze` возвращает нулевой `Result`, не выдавая частичный список отчётов за полный. Ошибка оборачивается с индексом записи через `%w`, поэтому исходная причина доступна через `errors.Is`. При штатном обходе некорректный индекс не ожидается; planservice не подменяет Build и не добавляет отдельную проверку этого случая.

## Границы

planservice не повторяет проверки допустимости записей и snapshots, не строит metadata Query и не меняет порядок before/after. Он возвращает результаты Parser, Analyzer и reportservice целиком: без фильтрации findings, диагностик, Compare или evidence. Порядок и семантика каждого компонента остаются его собственным контрактом.

Компонент не открывает Provider, не выбирает commit и не выполняет Git-команды, файловый ввод-вывод или сетевые запросы. Он не объединяет записи плана и не добавляет retry или fallback; кэш Lookup действует только в пределах одного вызова `Analyze`. planservice не генерирует ID или время, не создаёт approval и не изменяет переданный `Context`.

Общий статус `successful`, `safe` или `approved` не вводится. Статус Parser, findings Analyzer и доступность metadata остаются независимыми частями результата. Approval не реализуется. Предупреждения о пропущенных обновлениях из-за dependency conflict остаются ограничением Parser и отдельным отложенным этапом. Поддержка blockers принята после review `a38074f`.

## Проверки

Тесты через публичную `Analyze` используют небольшой fake `MetadataLookup` и проверяют:

- полный Parser и Analyzer результат сравнением с прямыми вызовами на том же входе;
- один отчёт на запись, включая повторяющиеся пакеты, сохранение исходного порядка и последовательность Lookup `before` → `after` для каждой записи;
- пропуск Lookup для `NS` и отклонённых связей;
- продолжение после ошибок Lookup и сохранение `not_found`, `repository_unavailable`, некорректного ответа Provider и `errors.Is`;
- обычный и typed-nil Provider при наличии записей, включая план только с `NS`, и отсутствие требований к Provider при пустом `Entries`;
- пустой, неподдерживаемый, blocker-only, removal-only и смешанный partial-планы;
- ссылки findings на запись или blocker, неизменность `Context` с вложенными slices и повторяемость результата;
- сохранение полных `reportservice.Result`.

Сквозной тест создаёт временный локальный Git-репозиторий с синтетическими cache-файлами и использует настоящий Metadata Provider. План включает upgrade, downgrade, отсутствующие metadata, `NS`, неразрешённый blocker, удаление и неподдерживаемую значимую строку. Проверяются Parser partial и исходные строки, факты плана, порядок Reports, точный commit, изменения и evidence, направление downgrade, причина отсутствия metadata и отсутствие Lookup для `NS`. Сеть не используется. Git author/committer и отключение подписи задаются только для команд подготовки тестового репозитория.

Проверки реализации (из каталога `backend/`): `gofmt` для изменённых Go-файлов, `go test -count=1 ./...`, `go vet ./...`, `git diff --check` и whitespace-проверка новых файлов.

## Состояние

planservice принят после review `013523e`. Компонент используется в [CLI](cli.md) и [HTTP API](http-api.md). HTTP-обработчик также использует разбор заранее подготовленного плана и callback прогресса; статусы задания и хранение относятся к HTTP-слою.

## Планы с пересборками

Запросы планируются через `reportservice.LookupQueries` по тем же правилам, что и фактический `Build`. Для `R` без installed выполняется только after. Дедупликация и прогресс учитывают уникальные фактические запросы. Для неизменённого `ebuild-updates-3.txt` тесты подтверждают 43 отчёта и 79 запросов: 36 пар обновлений и 7 целевых сторон пересборок. При найденных корректных snapshots доступны 36 сравнений; семь пересборок остаются без сравнения.
