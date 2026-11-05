# Proposal

## Why

Эксперимент musl + GNU подтвердил уменьшение размера и побайтовое совпадение пяти metadata-результатов с glibc. Пользователь выбрал musl как штатную базу, сохранив GNU-инструменты вместо экспериментальной подмены BusyBox.

## What Changes

- Перевести Dockerfile.base на закреплённую amd64 musl stage3, без установки BusyBox.
- Использовать musl loader в экспорте базы и проверке sandbox стандартного workflow.
- Сохранить glibc-базу/runtime под отдельными локальными тегами для сравнения и отката.
- Проверить стандартную сборку, GNU-инструменты, отсутствие BusyBox, HTTPS Git, worker/sync и сравнение metadata с glibc.
- Обновить текущее описание базы; исторические результаты экспериментов сохранить как историю проверок.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

Нет: контракты API, metadata, сети и volumes не меняются. Для изменения сборки используется `skip_specs: true`.

## Impact

`gentoo/Dockerfile.base`, скрипт сборки, инструкции и новые результаты приёмки. Compose сохраняет имена сервисов и образов. BusyBox остаётся opt-in экспериментом. Go-код, существующие volumes, commit, push и публикация в registry вне задачи.
