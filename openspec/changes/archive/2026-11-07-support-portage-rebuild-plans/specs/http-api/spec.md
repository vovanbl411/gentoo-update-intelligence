## ADDED Requirements

### Requirement: Пересборки в DTO v1
API SHALL передавать операцию `reinstall` и целочисленное поле `summary.reinstalls`, сохраняя вложенность результата и маршруты. Отсутствующее в прежнем сохранённом результате поле SHALL читаться как 0 без переразбора и миграции БД.

#### Scenario: Новый план с пересборками
- **WHEN** результат содержит R и сводку с reinstalls
- **THEN** DTO содержит reinstall и число пересборок; installed содержит только явно показанные сведения

#### Scenario: Старый сохранённый результат
- **WHEN** GET читает результат без Reinstalls
- **THEN** summary.reinstalls равно 0 и Provider не вызывается
