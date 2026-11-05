## ADDED Requirements

### Requirement: YAML backend в Compose
Compose SHALL подключать BACKEND_CONFIG_PATH, по умолчанию `./config/backend.yaml`, read-only как `/etc/gentoo-update-intelligence/backend.yaml` и передавать --config. Отсутствие файла SHALL NOT создавать каталог. Обычные env SHALL передаваться как явные overrides без принудительных defaults. SOURCE_URL SHALL иметь приоритет над общим GENTOO_REPOSITORY_URL; DSN SHALL оставаться в env.

#### Scenario: Настройка через файл
- **WHEN** оператор меняет YAML и перезапускает backend
- **THEN** применяются настройки файла без пересборки, если они не перекрыты непустыми env

#### Scenario: Изменение env или источника файла
- **WHEN** оператор меняет env либо BACKEND_CONFIG_PATH
- **THEN** новое значение применяется через пересоздание backend; простой restart не объявляется способом изменения окружения или bind source
