# Анализ результатов

## Обзор

После завершения нагрузочного теста Stroppy Cloud Panel предоставляет детальную аналитику результатов, включая метрики производительности, графики и рекомендации по оптимизации.

## Основные метрики

### TPS (Transactions Per Second)

**TPS** - это ключевая метрика, показывающая количество транзакций, выполняемых в секунду.

```yaml
# Пример конфигурации для измерения TPS
executors:
  - name: "tps_test"
    k6:
      scenario:
        max_duration: "5m"
        constant_vus:
          vus: 50
          duration: "5m"
```

**Интерпретация результатов:**
- **Высокий TPS** (>1000) - отличная производительность
- **Средний TPS** (100-1000) - приемлемая производительность
- **Низкий TPS** (<100) - требует оптимизации

### Время отклика

Время отклика измеряется в миллисекундах и включает:

- **P50** - медианное время отклика
- **P95** - 95-й процентиль
- **P99** - 99-й процентиль
- **Максимальное** время отклика

```sql
-- Пример запроса для анализа времени отклика
SELECT 
  AVG(response_time) as avg_response_time,
  PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY response_time) as p50,
  PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time) as p95,
  PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY response_time) as p99
FROM test_results
WHERE test_id = 'your-test-id';
```

### Количество ошибок

Отслеживание ошибок критически важно для понимания стабильности системы:

- **HTTP ошибки** (4xx, 5xx)
- **Ошибки подключения к БД**
- **Таймауты**
- **Ошибки валидации**

## Графики и визуализация

### График TPS по времени

```javascript
// Пример создания графика TPS
const tpsChart = {
  type: 'line',
  data: {
    labels: timeLabels,
    datasets: [{
      label: 'TPS',
      data: tpsData,
      borderColor: '#1890ff',
      backgroundColor: 'rgba(24, 144, 255, 0.1)'
    }]
  }
};
```

### График времени отклика

```javascript
// График времени отклика по процентилям
const responseTimeChart = {
  type: 'line',
  data: {
    labels: timeLabels,
    datasets: [
      {
        label: 'P50',
        data: p50Data,
        borderColor: '#52c41a'
      },
      {
        label: 'P95',
        data: p95Data,
        borderColor: '#faad14'
      },
      {
        label: 'P99',
        data: p99Data,
        borderColor: '#f5222d'
      }
    ]
  }
};
```

## Анализ производительности

### Сравнение с базовой линией

```yaml
# Конфигурация для базового теста
global:
  metadata:
    test_type: "baseline"
    environment: "staging"

benchmark:
  name: "Baseline Performance Test"
  workloads:
    - name: "baseline_workload"
      units:
        - descriptor:
            query:
              name: "simple_select"
              sql: "SELECT 1"
          count: "1000"
```

### Анализ узких мест

1. **База данных**
   - Медленные запросы
   - Блокировки
   - Недостаток индексов

2. **Сеть**
   - Задержки соединения
   - Пропускная способность
   - Потеря пакетов

3. **Приложение**
   - Неэффективные алгоритмы
   - Утечки памяти
   - Блокирующие операции

## Рекомендации по оптимизации

### Оптимизация запросов

```sql
-- Плохой запрос
SELECT * FROM users WHERE name LIKE '%john%';

-- Оптимизированный запрос
SELECT id, name, email FROM users 
WHERE name ILIKE 'john%' 
ORDER BY name 
LIMIT 100;
```

### Настройка индексов

```sql
-- Создание индекса для оптимизации поиска
CREATE INDEX CONCURRENTLY idx_users_name_gin 
ON users USING gin(to_tsvector('english', name));

-- Анализ использования индексов
EXPLAIN (ANALYZE, BUFFERS) 
SELECT * FROM users WHERE name ILIKE 'john%';
```

### Настройка пула соединений

```yaml
# Конфигурация пула соединений
driver:
  driver_type: "DRIVER_TYPE_POSTGRES"
  url: "postgres://user:pass@host:5432/db"
  db_specific:
    max_connections: 100
    min_connections: 10
    connection_timeout: "30s"
    idle_timeout: "300s"
```

## Экспорт результатов

### JSON формат

```json
{
  "test_id": "test-001",
  "start_time": "2024-01-15T10:00:00Z",
  "end_time": "2024-01-15T10:05:00Z",
  "metrics": {
    "tps": {
      "avg": 1250.5,
      "max": 1500.0,
      "min": 800.0
    },
    "response_time": {
      "p50": 45.2,
      "p95": 120.5,
      "p99": 250.8
    },
    "errors": {
      "total": 5,
      "rate": 0.001
    }
  }
}
```

### CSV формат

```csv
timestamp,tps,response_time_p50,response_time_p95,response_time_p99,errors
2024-01-15T10:00:00Z,1200,45.2,120.5,250.8,0
2024-01-15T10:01:00Z,1250,44.8,118.2,245.6,1
2024-01-15T10:02:00Z,1300,43.5,115.8,240.2,0
```

## Автоматизация анализа

### CI/CD интеграция

```yaml
# .github/workflows/performance-test.yml
name: Performance Test
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  performance-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Run Performance Test
        run: |
          stroppy-cloud-panel run --config performance-config.yaml
      - name: Analyze Results
        run: |
          stroppy-cloud-panel analyze --output results.json
      - name: Upload Results
        uses: actions/upload-artifact@v2
        with:
          name: performance-results
          path: results.json
```

### Алерты и уведомления

```yaml
# Конфигурация алертов
alerts:
  - name: "High Response Time"
    condition: "response_time_p95 > 200ms"
    action: "slack_notification"
    webhook: "https://hooks.slack.com/services/..."
  
  - name: "Low TPS"
    condition: "tps < 500"
    action: "email_notification"
    recipients: ["team@company.com"]
```

## Следующие шаги

- [Продвинутая конфигурация](../configuration/advanced-config.md)
- [Мониторинг в реальном времени](../configuration/monitoring.md)
- [Интеграция с Grafana](../configuration/grafana-integration.md)
