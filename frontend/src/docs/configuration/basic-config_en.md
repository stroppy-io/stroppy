# Basic Configuration

## Overview

Stroppy Cloud Panel configuration is described in YAML format and contains all necessary parameters for conducting load testing.

## Configuration Structure

```yaml
global:          # Global settings
benchmark:       # Test description
executors:       # Executor settings
exporters:       # Metrics export settings
steps:          # Step to executor mapping
sideCars:       # Plugin settings
```

## Global Settings (global)

### Required Parameters

```yaml
global:
  version: "1.0"                    # Configuration format version
  run_id: "my-test-001"            # Unique run identifier
  seed: "12345"                    # Seed for reproducible tests
  driver:                          # Database driver settings
    driver_type: "DRIVER_TYPE_POSTGRES"
    url: "postgres://user:pass@host:5432/db"
  logger:                          # Logging settings
    log_level: "LOG_LEVEL_INFO"
    log_mode: "LOG_MODE_PRODUCTION"
```

### Additional Parameters

```yaml
global:
  metadata:                        # Arbitrary metadata
    environment: "staging"
    team: "performance"
    project: "user-service"
```

## Database Driver Configuration

### PostgreSQL

```yaml
driver:
  driver_type: "DRIVER_TYPE_POSTGRES"
  url: "postgres://username:password@localhost:5432/database_name"
  db_specific:
    ssl_mode: "require"
    connect_timeout: "30s"
    statement_timeout: "60s"
```

### MySQL

```yaml
driver:
  driver_type: "DRIVER_TYPE_MYSQL"
  url: "mysql://username:password@localhost:3306/database_name"
  db_specific:
    charset: "utf8mb4"
    parse_time: true
    timeout: "30s"
```

## Test Description (benchmark)

### Simple Test

```yaml
benchmark:
  name: "Simple Read Test"
  workloads:
    - name: "read_workload"
      async: false
      units:
        - descriptor:
            query:
              name: "select_users"
              sql: "SELECT * FROM users WHERE active = true"
          count: "1000"
```

### Multi-Operation Test

```yaml
benchmark:
  name: "CRUD Operations Test"
  workloads:
    - name: "read_workload"
      async: false
      units:
        - descriptor:
            query:
              name: "select_users"
              sql: "SELECT * FROM users LIMIT 100"
          count: "500"
    - name: "write_workload"
      async: false
      units:
        - descriptor:
            query:
              name: "insert_user"
              sql: "INSERT INTO users (name, email) VALUES (${name}, ${email})"
              params:
                - name: "name"
                  generation_rule:
                    string_rules:
                      len_range:
                        min: "5"
                        max: "20"
                - name: "email"
                  generation_rule:
                    string_rules:
                      len_range:
                        min: "10"
                        max: "50"
          count: "200"
```

## Executor Configuration (executors)

### Constant Load

```yaml
executors:
  - name: "constant_load"
    k6:
      scenario:
        max_duration: "10m"
        constant_vus:
          vus: 50
          duration: "10m"
```

### Ramping Load

```yaml
executors:
  - name: "ramping_load"
    k6:
      scenario:
        max_duration: "15m"
        ramping_vus:
          start_vus: 0
          stages:
            - duration: "2m"
              target: 10
            - duration: "5m"
              target: 50
            - duration: "3m"
              target: 100
            - duration: "5m"
              target: 0
          pre_allocated_vus: 20
          max_vus: 150
```

### Arrival Rate Load

```yaml
executors:
  - name: "arrival_rate"
    k6:
      scenario:
        max_duration: "10m"
        constant_arrival_rate:
          rate: 10
          time_unit: "1s"
          duration: "10m"
          pre_allocated_vus: 5
          max_vus: 50
```

## Step to Executor Mapping (steps)

```yaml
steps:
  - name: "read_step"
    workload: "read_workload"
    executor: "constant_load"
  - name: "write_step"
    workload: "write_workload"
    executor: "ramping_load"
```

## Metrics Export (exporters)

### OpenTelemetry

```yaml
exporters:
  - name: "otlp_exporter"
    otlp_export:
      otlp_http_endpoint: "http://localhost:4318"
      otlp_metrics_prefix: "stroppy"
      otlp_headers: "Authorization=Bearer token123"
```

## Complete Configuration Example

```yaml
global:
  version: "1.0"
  run_id: "full-test-001"
  seed: "12345"
  driver:
    driver_type: "DRIVER_TYPE_POSTGRES"
    url: "postgres://test:test@localhost:5432/testdb"
  logger:
    log_level: "LOG_LEVEL_INFO"
    log_mode: "LOG_MODE_PRODUCTION"
  metadata:
    environment: "staging"
    team: "performance"

benchmark:
  name: "Full CRUD Test"
  workloads:
    - name: "read_workload"
      async: false
      units:
        - descriptor:
            query:
              name: "select_users"
              sql: "SELECT * FROM users WHERE created_at > NOW() - INTERVAL '1 day'"
          count: "1000"
    - name: "write_workload"
      async: false
      units:
        - descriptor:
            query:
              name: "insert_user"
              sql: "INSERT INTO users (name, email, created_at) VALUES (${name}, ${email}, NOW())"
              params:
                - name: "name"
                  generation_rule:
                    string_rules:
                      len_range:
                        min: "5"
                        max: "20"
                - name: "email"
                  generation_rule:
                    string_rules:
                      len_range:
                        min: "10"
                        max: "50"
          count: "500"

executors:
  - name: "read_executor"
    k6:
      scenario:
        max_duration: "5m"
        constant_vus:
          vus: 20
          duration: "5m"
  - name: "write_executor"
    k6:
      scenario:
        max_duration: "5m"
        ramping_vus:
          start_vus: 0
          stages:
            - duration: "1m"
              target: 5
            - duration: "3m"
              target: 20
            - duration: "1m"
              target: 0
          pre_allocated_vus: 10
          max_vus: 30

steps:
  - name: "read_step"
    workload: "read_workload"
    executor: "read_executor"
  - name: "write_step"
    workload: "write_workload"
    executor: "write_executor"

exporters:
  - name: "metrics_exporter"
    otlp_export:
      otlp_http_endpoint: "http://localhost:4318"
      otlp_metrics_prefix: "stroppy_test"
```

## Configuration Validation

Before running a test, the configuration is automatically validated:

- Required fields check
- Database connection URL validation
- SQL query syntax validation
- Data generation parameters validation

## Next Steps

- [Advanced Configuration](./advanced-config.md)
- [Monitoring Setup](./monitoring.md)
- [Performance Tuning](./performance-tuning.md)