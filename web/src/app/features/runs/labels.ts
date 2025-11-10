import {
  Workload_Type,
  Database_Type,
  Tps_Filter_Type,
  MachineInfo_Filter_Type,
  NumberFilterOperator,
} from '@/proto/panel/types_pb.ts'

export const getWorkloadTypeLabel = (type: Workload_Type, t: (key: string) => string) => {
  switch (type) {
    case Workload_Type.TPCC:
      return t('filters.workloadTypes.tpcc')
    default:
      return t('filters.workloadTypes.unspecified')
  }
}

export const getDatabaseTypeLabel = (type: Database_Type, t: (key: string) => string) => {
  switch (type) {
    case Database_Type.POSTGRES_ORIOLE:
      return t('filters.databaseTypes.postgresOriole')
    default:
      return t('filters.databaseTypes.unspecified')
  }
}

export const getTpsMetricLabel = (metric: Tps_Filter_Type, t: (key: string) => string) => {
  switch (metric) {
    case Tps_Filter_Type.AVERAGE:
      return t('filters.tps.metrics.average')
    case Tps_Filter_Type.MAX:
      return t('filters.tps.metrics.max')
    case Tps_Filter_Type.MIN:
      return t('filters.tps.metrics.min')
    case Tps_Filter_Type.P95TH:
      return t('filters.tps.metrics.p95')
    case Tps_Filter_Type.P99TH:
      return t('filters.tps.metrics.p99')
    default:
      return t('filters.tps.metrics.unspecified')
  }
}

export const getMachineParameterLabel = (param: MachineInfo_Filter_Type, t: (key: string) => string) => {
  switch (param) {
    case MachineInfo_Filter_Type.CORES:
      return t('filters.machine.parameters.cores')
    case MachineInfo_Filter_Type.MEMORY:
      return t('filters.machine.parameters.memory')
    case MachineInfo_Filter_Type.DISK:
      return t('filters.machine.parameters.disk')
    default:
      return t('filters.machine.parameters.unspecified')
  }
}

export const getNumberOperatorLabel = (operator: NumberFilterOperator, t: (key: string) => string) => {
  switch (operator) {
    case NumberFilterOperator.TYPE_GREATER_THAN:
      return t('filters.operators.gt')
    case NumberFilterOperator.TYPE_GREATER_THAN_OR_EQUAL:
      return t('filters.operators.gte')
    case NumberFilterOperator.TYPE_LESS_THAN:
      return t('filters.operators.lt')
    case NumberFilterOperator.TYPE_LESS_THAN_OR_EQUAL:
      return t('filters.operators.lte')
    case NumberFilterOperator.TYPE_EQUAL:
      return t('filters.operators.eq')
    case NumberFilterOperator.TYPE_NOT_EQUAL:
      return t('filters.operators.neq')
    default:
      return t('filters.operators.default')
  }
}
