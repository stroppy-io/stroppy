# Конфигурирование CLI

## Обзор

Stroppy Cloud Panel предоставляет мощный CLI (Command Line Interface) для управления нагрузочными тестами. CLI позволяет создавать, запускать и анализировать тесты через командную строку.

## Установка CLI

### Скачивание бинарного файла

```bash
# Скачать последнюю версию для Linux
curl -L https://github.com/arenadata/stroppy/releases/latest/download/stroppy-linux-amd64 -o stroppy
chmod +x stroppy
sudo mv stroppy /usr/local/bin/
```

### Проверка установки

```bash
stroppy --version
```

## Основные команды

### Создание конфигурации

```bash
# Создать базовую конфигурацию
stroppy init my-benchmark

# Создать конфигурацию с шаблоном
stroppy init my-benchmark --template postgres
```

### Запуск тестов

```bash
# Запустить тест с конфигурацией
stroppy run config.yaml

# Запустить тест с дополнительными параметрами
stroppy run config.yaml --seed 12345 --run-id "test-001"
```

### Анализ результатов

```bash
# Показать результаты последнего запуска
stroppy results

# Экспортировать результаты в JSON
stroppy results --format json --output results.json

# Показать результаты конкретного запуска
stroppy results --run-id "test-001"
```

## Конфигурационные файлы

### Структура конфигурации

```yaml
global:
  version: "1.0.0"
  run_id: "generate()"
  seed: 12345
  metadata:
    - key: "environment"
      value: "staging"
    - key: "test_type"
      value: "performance"

driver:
  driver_type: DRIVER_TYPE_POSTGRES
  url: "postgres://user:password@localhost:5432/testdb"
  db_specific:
    ssl_mode: "require"
    connection_timeout: "30s"

logger:
  log_level: LOG_LEVEL_INFO
  log_mode: LOG_MODE_PRODUCTION

executors:
  - name: "k6-executor"
    k6:
      k6_args: ["--http-debug"]
      setup_timeout: "60s"
      scenario:
        max_duration: "5m"
        constant_vus:
          vus: 50
          duration: "5m"

exporters:
  - name: "otlp-exporter"
    otlp_export:
      otlp_grpc_endpoint: "http://localhost:4317"
      otlp_metrics_prefix: "stroppy"

steps:
  - name: "setup"
    workload: "setup-workload"
    executor: "k6-executor"
    exporter: "otlp-exporter"

benchmark:
  name: "PostgreSQL Performance Test"
  workloads:
    - name: "setup-workload"
      async: false
      units:
        - descriptor:
            create_table:
              name: "users"
              columns:
                - name: "id"
                  sql_type: "SERIAL"
                  primary_key: true
                - name: "username"
                  sql_type: "VARCHAR(255)"
                  nullable: false
                - name: "email"
                  sql_type: "VARCHAR(255)"
                  nullable: false
                  unique: true
          count: 1
    - name: "test-workload"
      async: false
      units:
        - descriptor:
            query:
              name: "insert_user"
              sql: "INSERT INTO users (username, email) VALUES (${username}, ${email})"
              params:
                - name: "username"
                  generation_rule:
                    string_rules:
                      len_range:
                        min: 5
                        max: 20
                      alphabet:
                        ranges:
                          - min: 97
                            max: 122
                - name: "email"
                  generation_rule:
                    string_rules:
                      len_range:
                        min: 10
                        max: 50
                      alphabet:
                        ranges:
                          - min: 97
                            max: 122
          count: 1000
```

### Переменные окружения

```bash
# Настройка подключения к базе данных
export STROPPY_DB_URL="postgresql://user:password@localhost:5432/testdb"

# Настройка логирования
export STROPPY_LOG_LEVEL="INFO"

# Настройка экспорта метрик
export STROPPY_OTLP_ENDPOINT="http://localhost:4317"
```

## Параметры командной строки

### Глобальные флаги

```bash
# Уровень детализации вывода
--verbose, -v          # Подробный вывод
--quiet, -q           # Минимальный вывод
--debug               # Отладочная информация

# Конфигурация
--config, -c          # Путь к файлу конфигурации
--output, -o          # Путь для сохранения результатов
--format              # Формат вывода (json, yaml, table)
```

### Параметры запуска

```bash
# Управление тестами
--seed                # Семя для воспроизводимости
--run-id              # Идентификатор запуска
--duration            # Продолжительность теста
--vus                 # Количество виртуальных пользователей

# Настройки базы данных
--db-url              # URL подключения к БД
--db-driver           # Тип драйвера БД
--db-timeout          # Таймаут подключения
```

## Примеры использования

### Простой тест производительности

```bash
# 1. Создать конфигурацию
stroppy init simple-test --template postgres

# 2. Запустить тест
stroppy run simple-test/config.yaml

# 3. Посмотреть результаты
stroppy results
```

### Тест с кастомными параметрами

```bash
# Запуск с переопределением параметров
stroppy run config.yaml \
  --seed 54321 \
  --run-id "custom-test" \
  --db-url "postgresql://user:pass@db:5432/test" \
  --duration "10m" \
  --vus 100
```

### Экспорт результатов

```bash
# Экспорт в JSON для дальнейшего анализа
stroppy results --format json --output results.json

# Экспорт в CSV для Excel
stroppy results --format csv --output results.csv
```

## Интеграция с CI/CD

### GitHub Actions

```yaml
name: Performance Tests
on: [push, pull_request]

jobs:
  performance-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Stroppy
        run: |
          curl -L https://github.com/arenadata/stroppy/releases/latest/download/stroppy-linux-amd64 -o stroppy
          chmod +x stroppy
          sudo mv stroppy /usr/local/bin/
      
      - name: Run Performance Test
        run: |
          stroppy run config.yaml \
            --run-id "${{ github.run_id }}" \
            --seed "${{ github.run_number }}"
      
      - name: Upload Results
        uses: actions/upload-artifact@v3
        with:
          name: performance-results
          path: results.json
```

### GitLab CI

```yaml
performance-test:
  stage: test
  image: ubuntu:latest
  before_script:
    - apt-get update && apt-get install -y curl
    - curl -L https://github.com/arenadata/stroppy/releases/latest/download/stroppy-linux-amd64 -o stroppy
    - chmod +x stroppy
    - mv stroppy /usr/local/bin/
  script:
    - stroppy run config.yaml --run-id "$CI_PIPELINE_ID"
  artifacts:
    reports:
      junit: results.xml
    paths:
      - results.json
```

## Отладка и мониторинг

### Логирование

```bash
# Включить отладочные логи
stroppy run config.yaml --debug

# Сохранить логи в файл
stroppy run config.yaml --verbose 2>&1 | tee test.log
```

### Мониторинг в реальном времени

```bash
# Запуск с мониторингом метрик
stroppy run config.yaml --monitor

# Просмотр метрик через веб-интерфейс
stroppy dashboard --port 8080
```

## Лучшие практики

### Организация конфигураций

```
project/
├── configs/
│   ├── development.yaml
│   ├── staging.yaml
│   └── production.yaml
├── scripts/
│   ├── run-tests.sh
│   └── analyze-results.sh
└── results/
    ├── latest/
    └── archive/
```

### Скрипт для автоматизации

```bash
#!/bin/bash
# run-tests.sh

set -e

CONFIG_DIR="configs"
RESULTS_DIR="results/$(date +%Y%m%d_%H%M%S)"

# Создать директорию для результатов
mkdir -p "$RESULTS_DIR"

# Запустить тесты для каждого окружения
for config in "$CONFIG_DIR"/*.yaml; do
    env=$(basename "$config" .yaml)
    echo "Running tests for environment: $env"
    
    stroppy run "$config" \
        --run-id "${env}-$(date +%Y%m%d_%H%M%S)" \
        --output "$RESULTS_DIR/${env}-results.json"
done

echo "All tests completed. Results saved to: $RESULTS_DIR"
```

## Устранение неполадок

### Частые проблемы

1. **Ошибка подключения к БД**
   ```bash
   # Проверить доступность БД
   stroppy test-connection --db-url "postgresql://user:pass@host:5432/db"
   ```

2. **Недостаточно ресурсов**
   ```bash
   # Запуск с ограниченными ресурсами
   stroppy run config.yaml --vus 10 --duration "2m"
   ```

3. **Проблемы с экспортом метрик**
   ```bash
   # Проверить доступность OTLP endpoint
   curl -X POST http://localhost:4317/v1/metrics
   ```

### Получение помощи

```bash
# Справка по команде
stroppy --help

# Справка по конкретной команде
stroppy run --help

# Документация по конфигурации
stroppy config --help
```

## Следующие шаги

- [Продвинутая конфигурация](advanced-config.md)
- [Мониторинг и метрики](monitoring.md)
- [Интеграция с Grafana](grafana-integration.md)
