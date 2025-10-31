# Running Your First Test

## Preparing for Testing

Before running your first test, make sure that:

1. ✅ Stroppy Cloud Panel is installed and running
2. ✅ You are registered and logged in
3. ✅ You have access to a test database
4. ✅ Configuration is set up (see [Basic Configuration](../configuration/basic-config.md))

## Creating a Simple Test

### 1. Accessing the Configurator

1. Log into the control panel
2. Click "Configurator" in the sidebar menu
3. Select "Create New Configuration"

### 2. Setting Basic Parameters

```yaml
# Example basic configuration
global:
  version: "1.0"
  run_id: "my-first-test"
  seed: "12345"
  driver:
    driver_type: "DRIVER_TYPE_POSTGRES"
    url: "postgres://user:password@localhost:5432/testdb"
```

### 3. Defining the Workload

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
              sql: "SELECT * FROM users LIMIT 100"
          count: "1000"
```

### 4. Configuring the Executor

```yaml
executors:
  - name: "simple_executor"
    k6:
      scenario:
        max_duration: "5m"
        constant_vus:
          vus: 10
          duration: "5m"
```

## Running the Test

### 1. Saving the Configuration

1. Click "Save Configuration"
2. Give the configuration a name (e.g., "My First Test")
3. Add a description

### 2. Launching

1. Go to the "Runs" section
2. Click "Create Run"
3. Select the saved configuration
4. Click "Run"

### 3. Monitoring

During test execution, you can:

- View current status
- See the number of operations performed
- Track performance metrics
- Receive error notifications

## Analyzing Results

After the test completes:

1. Go to the "Runs" section
2. Find your test in the list
3. Click "View Results"
4. Study the metrics:
   - TPS (Transactions Per Second)
   - Response time
   - Error count
   - Resource usage

## Common Issues

### Database Connection Error

```
Error: connection refused
```

**Solution**: Check the connection URL and ensure the database is accessible.

### Insufficient Permissions

```
Error: permission denied
```

**Solution**: Make sure the database user has permissions to execute queries.

### Execution Timeout

```
Error: context deadline exceeded
```

**Solution**: Increase test execution time or reduce the load.

## Next Steps

- [Analyzing Results](./analyzing-results.md)
- [Setting Up Advanced Configurations](../configuration/advanced-config.md)
- [Integrating with Monitoring](../configuration/monitoring.md)