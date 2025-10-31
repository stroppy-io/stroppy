// Простые типы для конфигурации без использования proto файлов
export interface SimpleConfig {
  version: string
  run: SimpleRunConfig
  benchmark: SimpleBenchmarkDescriptor
}

export interface SimpleRunConfig {
  run_id: string
  seed: string
  driver: SimpleDriverConfig
  go_executor?: SimpleGoExecutor
  k6_executor?: SimpleK6Executor
  steps: SimpleRequestedStep[]
  logger?: SimpleLoggerConfig
  metadata: Record<string, string>
  plugins: SimplePlugin[]
}

export interface SimpleDriverConfig {
  driver_plugin_path: string
  driver_plugin_args: string[]
  url: string
  db_specific?: Record<string, any>
}

export interface SimpleGoExecutor {
  go_max_proc?: string
  cancel_on_error?: boolean
}

export interface SimpleK6Executor {
  k6_binary_path: string
  k6_binary_args: string[]
  k6_script_path: string
  k6_setup_timeout?: string
  k6_duration?: string
  k6_vus?: string
  k6_max_vus?: string
  k6_rate?: string
}

export interface SimpleRequestedStep {
  name: string
  executor?: 'GO_EXECUTOR' | 'K6_EXECUTOR'
}

export interface SimpleLoggerConfig {
  log_level: 'LOG_LEVEL_DEBUG' | 'LOG_LEVEL_INFO' | 'LOG_LEVEL_WARN' | 'LOG_LEVEL_ERROR'
  log_mode: 'LOG_MODE_DEVELOPMENT' | 'LOG_MODE_PRODUCTION'
}

export interface SimplePlugin {
  type: 'TYPE_UNSPECIFIED' | 'TYPE_SIDECAR'
  path: string
  settings?: Record<string, any>
}

export interface SimpleBenchmarkDescriptor {
  name: string
  steps: SimpleStepDescriptor[]
}

export interface SimpleStepDescriptor {
  name: string
  units: any[]
  async: boolean
}

// Типы для форм конфигуратора
export interface ConfigFormData {
  version: string
  run: RunConfigFormData
  benchmark: BenchmarkDescriptorFormData
}

export interface RunConfigFormData {
  runId: string
  seed: string
  driver: DriverConfigFormData
  goExecutor?: GoExecutorFormData
  k6Executor?: K6ExecutorFormData
  steps: RequestedStepFormData[]
  logger?: LoggerConfigFormData
  metadata: Record<string, string>
  plugins: PluginFormData[]
}

export interface DriverConfigFormData {
  driverPluginPath: string
  driverPluginArgs: string[]
  url: string
  dbSpecific?: Record<string, any>
}

export interface GoExecutorFormData {
  goMaxProc?: string
  cancelOnError?: boolean
}

export interface K6ExecutorFormData {
  k6BinaryPath: string
  k6BinaryArgs: string[]
  k6ScriptPath: string
  k6SetupTimeout?: string
  k6Duration?: string
  k6Vus?: string
  k6MaxVus?: string
  k6Rate?: string
}

export interface RequestedStepFormData {
  name: string
  executor?: 'GO_EXECUTOR' | 'K6_EXECUTOR'
}

export interface LoggerConfigFormData {
  logLevel: 'LOG_LEVEL_DEBUG' | 'LOG_LEVEL_INFO' | 'LOG_LEVEL_WARN' | 'LOG_LEVEL_ERROR'
  logMode: 'LOG_MODE_DEVELOPMENT' | 'LOG_MODE_PRODUCTION'
}

export interface PluginFormData {
  type: 'TYPE_UNSPECIFIED' | 'TYPE_SIDECAR'
  path: string
  settings?: Record<string, any>
}

export interface BenchmarkDescriptorFormData {
  name: string
  steps: StepDescriptorFormData[]
}

export interface StepDescriptorFormData {
  name: string
  units: StepUnitDescriptorFormData[]
  async: boolean
}

export interface StepUnitDescriptorFormData {
  id: string
  type: 'createTable' | 'query' | 'transaction'
  async: boolean
  createTable?: TableDescriptorFormData
  query?: QueryDescriptorFormData
  transaction?: TransactionDescriptorFormData
}

export interface TableDescriptorFormData {
  name: string
  columns: ColumnDescriptorFormData[]
  tableIndexes: IndexDescriptorFormData[]
  constraint: string
  dbSpecific?: Record<string, any>
}

export interface ColumnDescriptorFormData {
  name: string
  sqlType: string
  nullable: boolean
  primaryKey: boolean
  unique: boolean
  constraint: string
}

export interface IndexDescriptorFormData {
  name: string
  columns: string[]
  type: string
  unique: boolean
  dbSpecific?: Record<string, any>
}

export interface QueryDescriptorFormData {
  name: string
  sql: string
  params: QueryParamDescriptorFormData[]
  count: string
  dbSpecific?: Record<string, any>
}

export interface QueryParamDescriptorFormData {
  name: string
  replaceRegex: string
  generationRule?: GenerationRuleFormData
  dbSpecific?: Record<string, any>
}

export interface GenerationRuleFormData {
  type: 'string' | 'int32' | 'int64' | 'float' | 'double' | 'bool' | 'uuid' | 'datetime' | 'decimal'
  stringRules?: StringRuleFormData
  int32Rules?: Int32RuleFormData
  int64Rules?: Int64RuleFormData
  floatRules?: FloatRuleFormData
  doubleRules?: DoubleRuleFormData
  boolRules?: BoolRuleFormData
  uuidRules?: UuidRuleFormData
  datetimeRules?: DateTimeRuleFormData
  decimalRules?: DecimalRuleFormData
  distribution?: DistributionFormData
  nullPercentage?: number
  unique?: boolean
}

export interface StringRuleFormData {
  alphabet?: AlphabetFormData
  lenRange?: UInt64RangeFormData
  constant?: string
}

export interface Int32RuleFormData {
  range?: Int32RangeFormData
  constant?: number
}

export interface Int64RuleFormData {
  range?: Int64RangeFormData
  constant?: string
}

export interface FloatRuleFormData {
  range?: FloatRangeFormData
  constant?: number
}

export interface DoubleRuleFormData {
  range?: DoubleRangeFormData
  constant?: number
}

export interface BoolRuleFormData {
  constant?: boolean
}

export interface UuidRuleFormData {
  constant?: string
}

export interface DateTimeRuleFormData {
  range?: DateTimeRangeFormData
  constant?: string
}

export interface DecimalRuleFormData {
  range?: DecimalRangeFormData
  constant?: string
}

export interface AlphabetFormData {
  ranges: UInt32RangeFormData[]
}

export interface UInt32RangeFormData {
  min: number
  max: number
}

export interface UInt64RangeFormData {
  min: string
  max: string
}

export interface Int32RangeFormData {
  min: number
  max: number
}

export interface Int64RangeFormData {
  min: string
  max: string
}

export interface FloatRangeFormData {
  min: number
  max: number
}

export interface DoubleRangeFormData {
  min: number
  max: number
}

export interface DateTimeRangeFormData {
  type: 'default' | 'string' | 'timestampPb' | 'timestamp'
  default?: DateTimeRangeDefaultFormData
  string?: AnyStringRangeFormData
  timestampPb?: TimestampPbRangeFormData
  timestamp?: TimestampRangeFormData
}

export interface DateTimeRangeDefaultFormData {
  min?: string
  max?: string
}

export interface AnyStringRangeFormData {
  min: string
  max: string
}

export interface TimestampPbRangeFormData {
  min?: string
  max?: string
}

export interface TimestampRangeFormData {
  min: number
  max: number
}

export interface DecimalRangeFormData {
  type: 'default' | 'float' | 'double' | 'string'
  default?: DecimalRangeDefaultFormData
  float?: FloatRangeFormData
  double?: DoubleRangeFormData
  string?: AnyStringRangeFormData
}

export interface DecimalRangeDefaultFormData {
  min?: string
  max?: string
}

export interface DistributionFormData {
  type: 'NORMAL' | 'UNIFORM' | 'ZIPF'
  screw: number
}

export interface TransactionDescriptorFormData {
  name: string
  isolationLevel: 'TX_ISOLATION_LEVEL_UNSPECIFIED' | 'TX_ISOLATION_LEVEL_READ_UNCOMMITTED' | 'TX_ISOLATION_LEVEL_READ_COMMITTED' | 'TX_ISOLATION_LEVEL_REPEATABLE_READ' | 'TX_ISOLATION_LEVEL_SERIALIZABLE'
  queries: QueryDescriptorFormData[]
  count: string
  dbSpecific?: Record<string, any>
}

// Утилиты для конвертации
export const convertFormDataToConfig = (formData: ConfigFormData): SimpleConfig => {
  return {
    version: formData.version,
    run: convertRunConfigFormData(formData.run),
    benchmark: convertBenchmarkDescriptorFormData(formData.benchmark)
  }
}

const convertRunConfigFormData = (formData: RunConfigFormData): SimpleRunConfig => {
  return {
    run_id: formData.runId,
    seed: formData.seed,
    driver: convertDriverConfigFormData(formData.driver),
    go_executor: formData.goExecutor ? convertGoExecutorFormData(formData.goExecutor) : undefined,
    k6_executor: formData.k6Executor ? convertK6ExecutorFormData(formData.k6Executor) : undefined,
    steps: formData.steps.map(convertRequestedStepFormData),
    logger: formData.logger ? convertLoggerConfigFormData(formData.logger) : undefined,
    metadata: formData.metadata,
    plugins: formData.plugins.map(convertPluginFormData)
  }
}

const convertDriverConfigFormData = (formData: DriverConfigFormData): SimpleDriverConfig => {
  return {
    driver_plugin_path: formData.driverPluginPath,
    driver_plugin_args: formData.driverPluginArgs,
    url: formData.url,
    db_specific: formData.dbSpecific
  }
}

const convertGoExecutorFormData = (formData: GoExecutorFormData): SimpleGoExecutor => {
  return {
    go_max_proc: formData.goMaxProc,
    cancel_on_error: formData.cancelOnError
  }
}

const convertK6ExecutorFormData = (formData: K6ExecutorFormData): SimpleK6Executor => {
  return {
    k6_binary_path: formData.k6BinaryPath,
    k6_binary_args: formData.k6BinaryArgs,
    k6_script_path: formData.k6ScriptPath,
    k6_setup_timeout: formData.k6SetupTimeout,
    k6_duration: formData.k6Duration,
    k6_vus: formData.k6Vus,
    k6_max_vus: formData.k6MaxVus,
    k6_rate: formData.k6Rate
  }
}

const convertRequestedStepFormData = (formData: RequestedStepFormData): SimpleRequestedStep => {
  return {
    name: formData.name,
    executor: formData.executor
  }
}

const convertLoggerConfigFormData = (formData: LoggerConfigFormData): SimpleLoggerConfig => {
  return {
    log_level: formData.logLevel,
    log_mode: formData.logMode
  }
}

const convertPluginFormData = (formData: PluginFormData): SimplePlugin => {
  return {
    type: formData.type,
    path: formData.path,
    settings: formData.settings
  }
}

const convertBenchmarkDescriptorFormData = (formData: BenchmarkDescriptorFormData): SimpleBenchmarkDescriptor => {
  return {
    name: formData.name,
    steps: formData.steps.map(convertStepDescriptorFormData)
  }
}

const convertStepDescriptorFormData = (formData: StepDescriptorFormData): SimpleStepDescriptor => {
  return {
    name: formData.name,
    units: formData.units,
    async: formData.async
  }
}

// Утилиты для генерации YAML
const removeUndefinedValues = (obj: any): any => {
  if (typeof obj !== 'object' || obj === null) {
    return obj
  }

  if (Array.isArray(obj)) {
    return obj
      .map(removeUndefinedValues)
      .filter(value => value !== undefined)
  }

  const newObj: { [key: string]: any } = {}
  for (const key in obj) {
    if (Object.prototype.hasOwnProperty.call(obj, key)) {
      const value = obj[key]
      if (value !== undefined && value !== null && value !== '') {
        if (typeof value === 'object' && Object.keys(value).length === 0) {
          // Skip empty objects
          continue
        }
        newObj[key] = removeUndefinedValues(value)
      }
    }
  }
  return newObj
}

export const generateYAML = (config: SimpleConfig): string => {
  // Удаляем undefined значения
  const cleanConfig = removeUndefinedValues(config)

  // Используем простую генерацию YAML без внешних библиотек
  return JSON.stringify(cleanConfig, null, 2)
}
