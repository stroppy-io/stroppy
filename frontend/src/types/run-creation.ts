// Типы для формы создания запусков
export interface RunCreationFormData {
  // Основная информация о запуске
  runId: string
  name: string
  description?: string
  status: 'completed' | 'failed'
  
  // Тип нагрузки
  workloadType: WorkloadType
  
  // Тип базы данных
  databaseType: DatabaseType
  
  // Версия и сборка базы данных
  databaseVersion: DatabaseVersion
  
  // Конфигурация железа
  hardwareConfiguration: HardwareConfiguration
  
  // Схема развертывания
  deploymentLayout: DeploymentLayout
  
  // Сигнатура немезисов
  nemesisSignature: NemesisSignature
  
  // Свойства нагрузки
  workloadProperties: WorkloadProperties
}

// Типы нагрузки
export type WorkloadType = 'pgbench' | 'ycsb' | 'tpc-h' | 'tpc-c' | 'tpc-ds' | 'custom'

export interface WorkloadTypeOption {
  value: WorkloadType
  label: string
  description: string
}

// Типы баз данных
export type DatabaseType = 'postgres' | 'greenplum' | 'cloudberry' | 'clickhouse' | 'mysql'

export interface DatabaseTypeOption {
  value: DatabaseType
  label: string
  description: string
  // Доступные версии для данного типа БД
  availableVersions: DatabaseVersion[]
  // Доступные схемы развертывания для данного типа БД
  availableDeploymentLayouts: DeploymentLayoutType[]
}

// Версия и сборка базы данных
export interface DatabaseVersion {
  version: string
  build?: string
  description?: string
}

// Конфигурация железа
export interface HardwareConfiguration {
  // Уникальный идентификатор конфигурации
  id: string
  name: string
  description?: string
  
  // Характеристики
  cpu: CPUConfiguration
  memory: MemoryConfiguration
  storage: StorageConfiguration
  network: NetworkConfiguration
  
  // Количество узлов с данной конфигурацией
  nodeCount: number
}

export interface CPUConfiguration {
  cores: number
  model: string
  frequency?: string // например "2.4GHz"
  architecture?: string // например "x86_64", "arm64"
}

export interface MemoryConfiguration {
  totalGB: number
  type?: string // например "DDR4", "DDR5"
  speed?: string // например "3200MHz"
}

export interface StorageConfiguration {
  type: 'ssd' | 'nvme' | 'hdd' | 'mixed'
  capacityGB: number
  iops?: number
  throughputMBps?: number
  details?: string
}

export interface NetworkConfiguration {
  bandwidth?: string // например "10Gbps", "1Gbps"
  latency?: string // например "< 1ms"
  type?: string // например "InfiniBand", "Ethernet"
}

// Схема развертывания
export interface DeploymentLayout {
  type: DeploymentLayoutType
  signature: string // уникальная строка для группировки идентичных конфигураций
  configuration: DeploymentConfiguration
}

export type DeploymentLayoutType = 
  | 'single-node' 
  | 'master-replica' 
  | 'cluster' 
  | 'distributed'
  | 'sharded'

export interface DeploymentConfiguration {
  masters?: number
  replicas?: number
  segments?: number
  shards?: number
  witnesses?: number
  coordinators?: number
  // Дополнительные параметры специфичные для типа БД
  dbSpecific?: Record<string, any>
}

// Сигнатура немезисов
export interface NemesisSignature {
  signature: string // уникальная строка для группировки
  nemeses: NemesisConfig[]
}

export interface NemesisConfig {
  type: NemesisType
  enabled: boolean
  parameters?: Record<string, any>
  schedule?: NemesisSchedule
}

export type NemesisType = 
  | 'node-failure'
  | 'network-partition'
  | 'disk-failure'
  | 'cpu-stress'
  | 'memory-stress'
  | 'network-latency'
  | 'clock-skew'
  | 'custom'

export interface NemesisSchedule {
  startAfter?: string // например "5m"
  duration?: string // например "30s"
  interval?: string // например "10m"
}

// Свойства нагрузки
export interface WorkloadProperties {
  // Общие свойства
  runners: number
  duration?: string // например "10m", "1h"
  
  // Специфичные для типа нагрузки свойства
  pgbenchProperties?: PgbenchProperties
  ycsbProperties?: YcsbProperties
  tpchProperties?: TpchProperties
  tpccProperties?: TpccProperties
  customProperties?: Record<string, any>
}

export interface PgbenchProperties {
  scaleFactor: number
  clients: number
  threads?: number
  transactions?: number
  customScript?: string
}

export interface YcsbProperties {
  recordCount: number
  operationCount: number
  workload: 'workloada' | 'workloadb' | 'workloadc' | 'workloadd' | 'workloade' | 'workloadf'
  readProportion?: number
  updateProportion?: number
  insertProportion?: number
  scanProportion?: number
}

export interface TpchProperties {
  scaleFactor: number
  streams?: number
  queries?: string[] // список запросов для выполнения, например ["Q1", "Q2", "Q3"]
}

export interface TpccProperties {
  warehouses: number
  terminals?: number
  rampupTime?: string
  measureTime?: string
}

// Предустановленные конфигурации
export const WORKLOAD_TYPES: WorkloadTypeOption[] = [
  {
    value: 'pgbench',
    label: 'pgbench',
    description: 'Стандартный тест производительности PostgreSQL'
  },
  {
    value: 'ycsb',
    label: 'YCSB',
    description: 'Yahoo! Cloud Serving Benchmark'
  },
  {
    value: 'tpc-h',
    label: 'TPC-H',
    description: 'TPC Benchmark H (Decision Support)'
  },
  {
    value: 'tpc-c',
    label: 'TPC-C',
    description: 'TPC Benchmark C (Online Transaction Processing)'
  },
  {
    value: 'tpc-ds',
    label: 'TPC-DS',
    description: 'TPC Benchmark DS (Decision Support)'
  },
  {
    value: 'custom',
    label: 'Пользовательский',
    description: 'Пользовательский тест производительности'
  }
]

export const DATABASE_TYPES: DatabaseTypeOption[] = [
  {
    value: 'postgres',
    label: 'PostgreSQL',
    description: 'Реляционная СУБД PostgreSQL',
    availableVersions: [
      { version: '16.1', description: 'PostgreSQL 16.1' },
      { version: '15.5', description: 'PostgreSQL 15.5' },
      { version: '14.10', description: 'PostgreSQL 14.10' },
      { version: '13.13', description: 'PostgreSQL 13.13' }
    ],
    availableDeploymentLayouts: ['single-node', 'master-replica', 'cluster']
  },
  {
    value: 'greenplum',
    label: 'Greenplum',
    description: 'MPP база данных Greenplum',
    availableVersions: [
      { version: '7.1.0', description: 'Greenplum 7.1.0' },
      { version: '6.25.3', description: 'Greenplum 6.25.3' }
    ],
    availableDeploymentLayouts: ['distributed', 'cluster']
  },
  {
    value: 'cloudberry',
    label: 'Cloudberry',
    description: 'MPP база данных Cloudberry',
    availableVersions: [
      { version: '1.5.0', description: 'Cloudberry 1.5.0' },
      { version: '1.4.1', description: 'Cloudberry 1.4.1' }
    ],
    availableDeploymentLayouts: ['distributed', 'cluster']
  },
  {
    value: 'clickhouse',
    label: 'ClickHouse',
    description: 'Колоночная СУБД ClickHouse',
    availableVersions: [
      { version: '23.12', description: 'ClickHouse 23.12' },
      { version: '23.11', description: 'ClickHouse 23.11' }
    ],
    availableDeploymentLayouts: ['single-node', 'cluster', 'sharded']
  },
  {
    value: 'mysql',
    label: 'MySQL',
    description: 'Реляционная СУБД MySQL',
    availableVersions: [
      { version: '8.0.35', description: 'MySQL 8.0.35' },
      { version: '5.7.44', description: 'MySQL 5.7.44' }
    ],
    availableDeploymentLayouts: ['single-node', 'master-replica', 'cluster']
  }
]

export const NEMESIS_TYPES: Array<{value: NemesisType, label: string, description: string}> = [
  {
    value: 'node-failure',
    label: 'Отказ узла',
    description: 'Симуляция отказа узла кластера'
  },
  {
    value: 'network-partition',
    label: 'Разделение сети',
    description: 'Симуляция разделения сети между узлами'
  },
  {
    value: 'disk-failure',
    label: 'Отказ диска',
    description: 'Симуляция отказа дискового накопителя'
  },
  {
    value: 'cpu-stress',
    label: 'Нагрузка на CPU',
    description: 'Создание высокой нагрузки на процессор'
  },
  {
    value: 'memory-stress',
    label: 'Нагрузка на память',
    description: 'Создание высокой нагрузки на оперативную память'
  },
  {
    value: 'network-latency',
    label: 'Задержка сети',
    description: 'Увеличение задержки сетевых пакетов'
  },
  {
    value: 'clock-skew',
    label: 'Расхождение времени',
    description: 'Симуляция расхождения системного времени'
  },
  {
    value: 'custom',
    label: 'Пользовательский',
    description: 'Пользовательский тип немезиса'
  }
]

// Утилитарные функции
export const generateHardwareSignature = (config: HardwareConfiguration): string => {
  return `${config.cpu.cores}c-${config.memory.totalGB}gb-${config.storage.type}-${config.storage.capacityGB}gb-${config.nodeCount}n`
}

export const generateDeploymentSignature = (layout: DeploymentConfiguration, type: DeploymentLayoutType): string => {
  const parts: string[] = [type]
  if (layout.masters) parts.push(`m${layout.masters}`)
  if (layout.replicas) parts.push(`r${layout.replicas}`)
  if (layout.segments) parts.push(`s${layout.segments}`)
  if (layout.shards) parts.push(`sh${layout.shards}`)
  if (layout.witnesses) parts.push(`w${layout.witnesses}`)
  if (layout.coordinators) parts.push(`c${layout.coordinators}`)
  return parts.join('-')
}

export const generateNemesisSignature = (nemeses: NemesisConfig[]): string => {
  const enabledNemeses = nemeses.filter(n => n.enabled)
  if (enabledNemeses.length === 0) return 'none'
  return enabledNemeses.map(n => n.type).sort().join('+')
}
