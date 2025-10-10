# Analyzing Results

## Overview

After completing a load test, Stroppy Cloud Panel provides detailed analytics of results, including performance metrics, charts, and optimization recommendations.

## Key Metrics

### TPS (Transactions Per Second)

**TPS** is a key metric showing the number of transactions executed per second.

```yaml
# Example configuration for measuring TPS
executors:
  - name: "tps_test"
    k6:
      scenario:
        max_duration: "5m"
        constant_vus:
          vus: 50
          duration: "5m"
```

**Result interpretation:**
- **High TPS** (>1000) - excellent performance
- **Medium TPS** (100-1000) - acceptable performance
- **Low TPS** (<100) - requires optimization

### Response Time

Response time is measured in milliseconds and includes:

- **P50** - median response time
- **P95** - 95th percentile
- **P99** - 99th percentile
- **Maximum** response time

```sql
-- Example query for analyzing response time
SELECT 
  AVG(response_time) as avg_response_time,
  PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY response_time) as p50,
  PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time) as p95,
  PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY response_time) as p99
FROM test_results
WHERE test_id = 'your-test-id';
```

### Error Count

Tracking errors is critical for understanding system stability:

- **HTTP errors** (4xx, 5xx)
- **Database connection errors**
- **Timeouts**
- **Validation errors**

## Charts and Visualization

### TPS Over Time Chart

```javascript
// Example TPS chart creation
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

### Response Time Chart

```javascript
// Response time chart by percentiles
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

## Performance Analysis

### Baseline Comparison

```yaml
# Configuration for baseline test
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

### Bottleneck Analysis

1. **Database**
   - Slow queries
   - Locks
   - Missing indexes

2. **Network**
   - Connection delays
   - Bandwidth
   - Packet loss

3. **Application**
   - Inefficient algorithms
   - Memory leaks
   - Blocking operations

## Optimization Recommendations

### Query Optimization

```sql
-- Bad query
SELECT * FROM users WHERE name LIKE '%john%';

-- Optimized query
SELECT id, name, email FROM users 
WHERE name ILIKE 'john%' 
ORDER BY name 
LIMIT 100;
```

### Index Configuration

```sql
-- Creating index for search optimization
CREATE INDEX CONCURRENTLY idx_users_name_gin 
ON users USING gin(to_tsvector('english', name));

-- Analyzing index usage
EXPLAIN (ANALYZE, BUFFERS) 
SELECT * FROM users WHERE name ILIKE 'john%';
```

### Connection Pool Configuration

```yaml
# Connection pool configuration
driver:
  driver_type: "DRIVER_TYPE_POSTGRES"
  url: "postgres://user:pass@host:5432/db"
  db_specific:
    max_connections: 100
    min_connections: 10
    connection_timeout: "30s"
    idle_timeout: "300s"
```

## Exporting Results

### JSON Format

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

### CSV Format

```csv
timestamp,tps,response_time_p50,response_time_p95,response_time_p99,errors
2024-01-15T10:00:00Z,1200,45.2,120.5,250.8,0
2024-01-15T10:01:00Z,1250,44.8,118.2,245.6,1
2024-01-15T10:02:00Z,1300,43.5,115.8,240.2,0
```

## Analysis Automation

### CI/CD Integration

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

### Alerts and Notifications

```yaml
# Alert configuration
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

## Next Steps

- [Advanced Configuration](../configuration/advanced-config.md)
- [Real-time Monitoring](../configuration/monitoring.md)
- [Grafana Integration](../configuration/grafana-integration.md)