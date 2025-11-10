import type { TFunction } from 'i18next'
import { getStatusLabel } from '@/app/features/runs/utils'
import type { RunSummary } from './types'
import { getDatabaseTypeLabel, getWorkloadTypeLabel } from './labels'

type ComparisonField = {
  key: string
  label: string
  getRaw: (run: RunSummary) => string | number | undefined
  format?: (run: RunSummary) => string
}

export type ComparisonSection = {
  key: string
  label: string
  fields: ComparisonField[]
}

export const buildComparisonSections = (t: TFunction<'runs'>): ComparisonSection[] => [
  {
    key: 'status',
    label: t('comparison.statusSection'),
    fields: [
      {
        key: 'status',
        label: t('table.columns.status'),
        getRaw: (run) => run.status,
        format: (run) => getStatusLabel(run.status, t),
      },
      {
        key: 'workloadType',
        label: t('filters.workloadType'),
        getRaw: (run) => run.workloadType ?? -1,
        format: (run) =>
          run.workloadType !== undefined ? getWorkloadTypeLabel(run.workloadType, t) : t('filters.workloadTypes.unspecified'),
      },
      {
        key: 'databaseType',
        label: t('filters.databaseType'),
        getRaw: (run) => run.databaseType ?? -1,
        format: (run) =>
          run.databaseType !== undefined ? getDatabaseTypeLabel(run.databaseType, t) : t('filters.databaseTypes.unspecified'),
      },
    ],
  },
  {
    key: 'cluster',
    label: t('comparison.cluster.title'),
    fields: [
      {
        key: 'runnerClusterNodes',
        label: t('comparison.cluster.runnerNodes'),
        getRaw: (run) => run.runnerClusterNodes ?? -1,
        format: (run) => (run.runnerClusterNodes ? run.runnerClusterNodes.toString() : '—'),
      },
      {
        key: 'databaseClusterNodes',
        label: t('comparison.cluster.databaseNodes'),
        getRaw: (run) => run.databaseClusterNodes ?? -1,
        format: (run) => (run.databaseClusterNodes ? run.databaseClusterNodes.toString() : '—'),
      },
    ],
  },
  {
    key: 'runnerMachine',
    label: t('comparison.machine.runnerTitle'),
    fields: [
      {
        key: 'runnerMachineSignature',
        label: t('comparison.machine.runnerSignature'),
        getRaw: (run) => run.runnerMachineSignature ?? '',
        format: (run) => run.runnerMachineSignature ?? '—',
      },
      {
        key: 'runnerMachineCores',
        label: t('comparison.machine.runnerCores'),
        getRaw: (run) => run.runnerMachineCores ?? -1,
        format: (run) => (run.runnerMachineCores ? `${run.runnerMachineCores} vCPU` : '—'),
      },
      {
        key: 'runnerMachineMemory',
        label: t('comparison.machine.runnerMemory'),
        getRaw: (run) => run.runnerMachineMemory ?? -1,
        format: (run) => (run.runnerMachineMemory ? `${run.runnerMachineMemory} GB` : '—'),
      },
      {
        key: 'runnerMachineDisk',
        label: t('comparison.machine.runnerDisk'),
        getRaw: (run) => run.runnerMachineDisk ?? -1,
        format: (run) => (run.runnerMachineDisk ? `${run.runnerMachineDisk} GB` : '—'),
      },
    ],
  },
  {
    key: 'databaseMachine',
    label: t('comparison.machine.databaseTitle'),
    fields: [
      {
        key: 'databaseMachineSignature',
        label: t('comparison.machine.databaseSignature'),
        getRaw: (run) => run.databaseMachineSignature ?? '',
        format: (run) => run.databaseMachineSignature ?? '—',
      },
      {
        key: 'databaseMachineCores',
        label: t('comparison.machine.databaseCores'),
        getRaw: (run) => run.databaseMachineCores ?? -1,
        format: (run) => (run.databaseMachineCores ? `${run.databaseMachineCores} vCPU` : '—'),
      },
      {
        key: 'databaseMachineMemory',
        label: t('comparison.machine.databaseMemory'),
        getRaw: (run) => run.databaseMachineMemory ?? -1,
        format: (run) => (run.databaseMachineMemory ? `${run.databaseMachineMemory} GB` : '—'),
      },
      {
        key: 'databaseMachineDisk',
        label: t('comparison.machine.databaseDisk'),
        getRaw: (run) => run.databaseMachineDisk ?? -1,
        format: (run) => (run.databaseMachineDisk ? `${run.databaseMachineDisk} GB` : '—'),
      },
    ],
  },
  {
    key: 'metrics',
    label: t('comparison.metrics.title'),
    fields: [
      {
        key: 'tpsAverage',
        label: t('table.columns.tps'),
        getRaw: (run) => run.tpsAverage ?? -1,
        format: (run) => (run.tpsAverage ? `${run.tpsAverage.toLocaleString()} TPS` : '—'),
      },
      {
        key: 'tpsP95',
        label: t('filters.tps.metrics.p95'),
        getRaw: (run) => run.tpsP95 ?? -1,
        format: (run) => (run.tpsP95 ? `${run.tpsP95.toLocaleString()} TPS` : '—'),
      },
    ],
  },
]

export const countDifferences = (sections: ComparisonSection[], runA: RunSummary, runB: RunSummary) =>
  sections.reduce((sectionCount, section) => {
    const fieldDiffs = section.fields.reduce((count, field) => {
      const rawA = field.getRaw(runA)
      const rawB = field.getRaw(runB)
      return rawA === rawB ? count : count + 1
    }, 0)
    return sectionCount + fieldDiffs
  }, 0)
