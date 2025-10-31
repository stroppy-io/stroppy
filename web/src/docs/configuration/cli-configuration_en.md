# CLI Configuration

## Overview

Stroppy Cloud Panel provides a powerful CLI (Command Line Interface) for managing load tests. The CLI allows you to create, run, and analyze tests through the command line.

## CLI Installation

### Download Binary File

```bash
# Download latest version for Linux
curl -L https://github.com/arenadata/stroppy/releases/latest/download/stroppy-linux-amd64 -o stroppy
chmod +x stroppy
sudo mv stroppy /usr/local/bin/
```

### Verify Installation

```bash
stroppy --version
```

## Basic Commands

### Creating Configuration

```bash
# Create basic configuration
stroppy init my-benchmark

# Create configuration with template
stroppy init my-benchmark --template postgres
```

### Running Tests

```bash
# Run test with configuration
stroppy run config.yaml

# Run test with additional parameters
stroppy run config.yaml --seed 12345 --run-id "test-001"
```

### Analyzing Results

```bash
# Show results of last run
stroppy results

# Export results to JSON
stroppy results --format json --output results.json

# Show results of specific run
stroppy results --run-id "test-001"
```

## Configuration Files

### Configuration Structure

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
  url: "postgresql://user:password@localhost:5432/testdb"
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

benchmark:
  name: "My Benchmark"
  workloads:
    - name: "read-workload"
      async: false
      units:
        - descriptor:
            query:
              name: "select_users"
              sql: "SELECT * FROM users LIMIT 100"
          count: "1000"

steps:
  - name: "read-step"
    workload: "read-workload"
    executor: "k6-executor"
    exporter: "otlp-exporter"
```

## Advanced Configuration Options

### Environment Variables

```bash
# Set database URL via environment variable
export STROPPY_DB_URL="postgresql://user:pass@localhost:5432/db"
stroppy run config.yaml

# Set multiple environment variables
export STROPPY_LOG_LEVEL="DEBUG"
export STROPPY_RUN_ID="custom-run-001"
stroppy run config.yaml
```

### Configuration Overrides

```bash
# Override specific configuration values
stroppy run config.yaml --override "global.seed=54321"
stroppy run config.yaml --override "executors[0].k6.scenario.constant_vus.vus=100"
```

### Multiple Configuration Files

```bash
# Merge multiple configuration files
stroppy run base-config.yaml --merge override-config.yaml

# Use configuration from different directories
stroppy run configs/production/base.yaml --config-dir configs/production/
```

## Monitoring and Logging

### Log Levels

```yaml
logger:
  log_level: LOG_LEVEL_DEBUG    # DEBUG, INFO, WARN, ERROR, FATAL
  log_mode: LOG_MODE_DEVELOPMENT # DEVELOPMENT, PRODUCTION
```

### Real-time Monitoring

```bash
# Run with real-time metrics
stroppy run config.yaml --monitor

# Stream logs to file
stroppy run config.yaml --log-file test.log

# Enable verbose output
stroppy run config.yaml --verbose
```

## Integration with CI/CD

### GitHub Actions

```yaml
name: Performance Test
on:
  push:
    branches: [main]

jobs:
  performance-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Install Stroppy CLI
        run: |
          curl -L https://github.com/arenadata/stroppy/releases/latest/download/stroppy-linux-amd64 -o stroppy
          chmod +x stroppy
          sudo mv stroppy /usr/local/bin/
      
      - name: Run Performance Test
        run: |
          stroppy run configs/performance.yaml --run-id "ci-${{ github.run_id }}"
      
      - name: Upload Results
        uses: actions/upload-artifact@v2
        with:
          name: performance-results
          path: results/
```

### Jenkins Pipeline

```groovy
pipeline {
    agent any
    
    stages {
        stage('Performance Test') {
            steps {
                sh 'stroppy run configs/performance.yaml --run-id "jenkins-${BUILD_NUMBER}"'
            }
        }
        
        stage('Analyze Results') {
            steps {
                sh 'stroppy results --format json --output results.json'
                archiveArtifacts artifacts: 'results.json', fingerprint: true
            }
        }
    }
}
```

## Troubleshooting

### Common Issues

#### Connection Errors

```bash
# Test database connection
stroppy test-connection config.yaml

# Check configuration validity
stroppy validate config.yaml
```

#### Performance Issues

```bash
# Run with profiling
stroppy run config.yaml --profile

# Enable debug mode
stroppy run config.yaml --debug
```

#### Memory Issues

```bash
# Limit memory usage
stroppy run config.yaml --max-memory 2GB

# Use fewer virtual users
stroppy run config.yaml --override "executors[0].k6.scenario.constant_vus.vus=10"
```

### Debug Commands

```bash
# Show configuration after processing
stroppy run config.yaml --dry-run

# Validate configuration without running
stroppy validate config.yaml

# Show help for specific command
stroppy run --help
```

## Best Practices

### Configuration Management

1. **Use templates** for common configurations
2. **Version control** your configuration files
3. **Use environment variables** for sensitive data
4. **Validate configurations** before running tests

### Performance Optimization

1. **Start small** with low virtual user counts
2. **Gradually increase** load to find limits
3. **Monitor resources** during test execution
4. **Use appropriate timeouts** for your environment

### Security

1. **Never commit** passwords to version control
2. **Use environment variables** for sensitive data
3. **Restrict access** to test databases
4. **Use SSL connections** in production

## Next Steps

- [Advanced Configuration](./advanced-config.md)
- [Monitoring Setup](./monitoring.md)
- [Performance Tuning](./performance-tuning.md)