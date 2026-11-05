# Tasks

## 1. Загрузчик и backend

- [x] 1.1 Реализовать YAML, defaults, env и валидацию; проверить приоритет, пустые env, ноль, типы, ошибки и отсутствие секретов в диагностике тестами.
- [x] 1.2 Подключить --config и передавать worker settings явно; проверить fail-fast до БД, timeout и регрессию API/очереди/overlays.

## 2. Compose и документация

- [x] 2.1 Подключить read-only файл, убрать принудительные defaults; проверить рендер, override и прежний общий URL Gentoo.
- [x] 2.2 Обновить инструкции, поля, env и restart/recreate; выполнить humanize-docs.

## 3. Итоговая проверка

- [x] 3.1 Выполнить gofmt, Go tests, race tests, vet, сборку backend, diff/whitespace и OpenSpec --strict; записать результаты и ограничения.

## Workflow follow-up

- После review архивировать change и проверить итоговые спецификации.
