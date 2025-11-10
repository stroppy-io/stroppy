import type {
  Status,
  Workload_Type,
  Database_Type,
  Tps_Filter_Type,
  MachineInfo_Filter_Type,
  NumberFilterOperator,
} from '@/proto/panel/types_pb.ts'

export interface NumericFilter<TParameter> {
  parameterType: TParameter
  operator: NumberFilterOperator
  value: number
}

export interface RunsFilters {
  page: number
  pageSize: number
  status?: Status
  workloadType?: Workload_Type
  databaseType?: Database_Type
  workloadName?: string
  databaseName?: string
  tpsFilters?: Array<NumericFilter<Tps_Filter_Type>>
  orderByTps?: {
    parameterType: Tps_Filter_Type
    descending: boolean
  }
  machineFilters?: Array<NumericFilter<MachineInfo_Filter_Type>>
}

export interface RunSummary {
  id: string
  status: Status
  workloadName: string
  workloadType?: Workload_Type
  databaseName?: string
  databaseType?: Database_Type
  tpsAverage?: number
  tpsP95?: number
  createdAt?: Date
  updatedAt?: Date
  runnerClusterNodes?: number
  runnerMachineSignature?: string
  runnerMachineCores?: number
  runnerMachineMemory?: number
  runnerMachineDisk?: number
  databaseClusterNodes?: number
  databaseMachineSignature?: string
  databaseMachineCores?: number
  databaseMachineMemory?: number
  databaseMachineDisk?: number
}
