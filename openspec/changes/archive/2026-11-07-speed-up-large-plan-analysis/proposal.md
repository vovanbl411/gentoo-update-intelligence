# Proposal

## Why

Последовательные запросы metadata увеличивают время анализа больших планов, даже когда worker способен выполнять несколько генераций.

## What Changes

- Ограниченно распараллелить уникальные запросы metadata одного HTTP-анализа и сохранить порядок отчётов и прогресс.
- Ограничить одновременно выполняемую подготовку metadata в общем worker.
- Сравнить последовательную и параллельную обработку и проверить возможность раннего чтения generated-cache.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `automatic-metadata-preparation`: параллелизм запросов и предел общей подготовки metadata.

## Impact

Backend, metadata-worker и документация; формат HTTP API не меняется.
