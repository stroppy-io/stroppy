import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { z } from 'zod'
import { create } from '@bufbuild/protobuf'
import { useTranslation } from '@/i18n/use-translation'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { addRun } from '@/proto/panel/run-RunService_connectquery.ts'
import { RunRecordSchema } from '@/proto/panel/run_pb.ts'
import { Database_Type, MachineInfoSchema, Status, Workload_Type } from '@/proto/panel/types_pb.ts'
import { useMutation, useTransport } from '@connectrpc/connect-query'
import { getStatusLabel } from '@/app/features/runs/utils'

type FormValues = {
  workloadName: string
  workloadType: Workload_Type
  databaseName: string
  databaseType: Database_Type
  status: Status
  description: string
  tpsAverage: string
  tpsP95: string
  runnerNodes: string
  runnerCores: string
  runnerMemory: string
  runnerDisk: string
  databaseNodes: string
  databaseCores: string
  databaseMemory: string
  databaseDisk: string
}

const formSchema = z.object({
  workloadName: z.string().min(3),
  workloadType: z.nativeEnum(Workload_Type),
  databaseName: z.string().min(3),
  databaseType: z.nativeEnum(Database_Type),
  status: z.nativeEnum(Status),
  description: z.string().optional(),
  tpsAverage: z.coerce.number().positive().optional(),
  tpsP95: z.coerce.number().positive().optional(),
  runnerNodes: z.coerce.number().int().positive().optional(),
  runnerCores: z.coerce.number().int().positive().optional(),
  runnerMemory: z.coerce.number().int().positive().optional(),
  runnerDisk: z.coerce.number().int().positive().optional(),
  databaseNodes: z.coerce.number().int().positive().optional(),
  databaseCores: z.coerce.number().int().positive().optional(),
  databaseMemory: z.coerce.number().int().positive().optional(),
  databaseDisk: z.coerce.number().int().positive().optional(),
})

export const RunsCreatePage = () => {
  const { t } = useTranslation('runs')
  const navigate = useNavigate()
  const transport = useTransport()
  const mutation = useMutation(addRun, { transport })

  const initialValues: FormValues = {
    workloadName: '',
    workloadType: Workload_Type.TPCC,
    databaseName: '',
    databaseType: Database_Type.POSTGRES_ORIOLE,
    status: Status.RUNNING,
    description: '',
    tpsAverage: '',
    tpsP95: '',
    runnerNodes: '',
    runnerCores: '',
    runnerMemory: '',
    runnerDisk: '',
    databaseNodes: '',
    databaseCores: '',
    databaseMemory: '',
    databaseDisk: '',
  }
  const [formValues, setFormValues] = useState<FormValues>(initialValues)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitError, setSubmitError] = useState<string | null>(null)

  const handleTextChange = (name: keyof FormValues, value: string) => {
    setFormValues((prev) => ({ ...prev, [name]: value }))
    setErrors((prev) => ({ ...prev, [name]: '' }))
  }

  const handleSelectChange = (name: keyof FormValues, value: string) => {
    setFormValues((prev) => ({ ...prev, [name]: Number(value) as FormValues[keyof FormValues] }))
    setErrors((prev) => ({ ...prev, [name]: '' }))
  }

  const buildClusterMachines = (count?: number, cores?: number, memory?: number, disk?: number) => {
    if (!count && !cores && !memory && !disk) return []
    const machineCount = count && count > 0 ? count : 1
    return Array.from({ length: machineCount }, () => create(MachineInfoSchema, { cores, memory, disk }))
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSubmitError(null)
    const parsed = formSchema.safeParse({
      ...formValues,
      tpsAverage: formValues.tpsAverage || undefined,
      tpsP95: formValues.tpsP95 || undefined,
      runnerNodes: formValues.runnerNodes || undefined,
      runnerCores: formValues.runnerCores || undefined,
      runnerMemory: formValues.runnerMemory || undefined,
      runnerDisk: formValues.runnerDisk || undefined,
      databaseNodes: formValues.databaseNodes || undefined,
      databaseCores: formValues.databaseCores || undefined,
      databaseMemory: formValues.databaseMemory || undefined,
      databaseDisk: formValues.databaseDisk || undefined,
    })

    if (!parsed.success) {
      const fieldErrors: Record<string, string> = {}
      parsed.error.issues.forEach((issue) => {
        if (issue.path[0]) {
          fieldErrors[String(issue.path[0])] = issue.message
        }
      })
      setErrors(fieldErrors)
      return
    }

    const values = parsed.data

    const runnerMachines = buildClusterMachines(values.runnerNodes, values.runnerCores, values.runnerMemory, values.runnerDisk)
    const databaseMachines = buildClusterMachines(values.databaseNodes, values.databaseCores, values.databaseMemory, values.databaseDisk)

    try {
      await mutation.mutateAsync(
        create(RunRecordSchema, {
          status: values.status,
          workload: {
            name: values.workloadName,
            workloadType: values.workloadType,
            runnerCluster: runnerMachines.length
              ? {
                  machines: runnerMachines,
                }
              : undefined,
          },
          database: {
            name: values.databaseName,
            databaseType: values.databaseType,
            runnerCluster: databaseMachines.length
              ? {
                  machines: databaseMachines,
                }
              : undefined,
          },
          tps: {
            average: values.tpsAverage ? BigInt(values.tpsAverage) : undefined,
            p95th: values.tpsP95 ? BigInt(values.tpsP95) : undefined,
          },
        }),
      )
      navigate('/app/runs')
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : 'Failed to create run')
    }
  }

  const textField = (
    name: keyof FormValues,
    label: string,
    type: string = 'text',
  ) => (
    <div className="space-y-1">
      <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground" htmlFor={name}>
        {label}
      </label>
      <Input
        id={name}
        type={type}
        value={formValues[name]}
        onChange={(event) => handleTextChange(name, event.target.value)}
      />
      {errors[name as string] && <p className="text-xs text-destructive">{errors[name as string]}</p>}
    </div>
  )

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('actions.createRun')}</p>
          <h1 className="text-3xl font-semibold text-foreground">{t('page.title')}</h1>
        </div>
        <Button variant="outline" onClick={() => navigate('/app/runs')}>
          {t('actions.backToList')}
        </Button>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">{t('comparison.statusSection')}</h2>
          <div className="grid gap-4 md:grid-cols-2">
            {textField('workloadName', t('filters.workloadName'))}
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.workloadType')}</label>
              <Select value={formValues.workloadType.toString()} onValueChange={(value) => handleSelectChange('workloadType', value)}>
                <SelectTrigger>
                  <SelectValue placeholder={t('filters.workloadType')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={Workload_Type.TPCC.toString()}>{t('filters.workloadTypes.tpcc')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {textField('databaseName', t('filters.databaseName'))}
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('filters.databaseType')}</label>
              <Select value={formValues.databaseType.toString()} onValueChange={(value) => handleSelectChange('databaseType', value)}>
                <SelectTrigger>
                  <SelectValue placeholder={t('filters.databaseType')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={Database_Type.POSTGRES_ORIOLE.toString()}>{t('filters.databaseTypes.postgresOriole')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('table.columns.status')}</label>
              <Select value={formValues.status.toString()} onValueChange={(value) => handleSelectChange('status', value)}>
                <SelectTrigger>
                  <SelectValue placeholder={t('table.columns.status')} />
                </SelectTrigger>
                <SelectContent>
                  {[Status.RUNNING, Status.COMPLETED, Status.FAILED, Status.CANCELED, Status.IDLE].map((status) => (
                    <SelectItem key={status} value={status.toString()}>
                      {getStatusLabel(status, t)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">{t('details.runDescription')}</label>
            <Textarea value={formValues.description} onChange={(event) => handleTextChange('description', event.target.value)} />
          </div>
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">{t('comparison.metrics.title')}</h2>
          <div className="grid gap-4 md:grid-cols-2">
            {textField('tpsAverage', 'TPS AVG', 'number')}
            {textField('tpsP95', 'TPS P95', 'number')}
          </div>
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">{t('comparison.machine.runnerTitle')}</h2>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            {textField('runnerNodes', t('comparison.cluster.runnerNodes'), 'number')}
            {textField('runnerCores', t('comparison.machine.runnerCores'), 'number')}
            {textField('runnerMemory', t('comparison.machine.runnerMemory'), 'number')}
            {textField('runnerDisk', t('comparison.machine.runnerDisk'), 'number')}
          </div>
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">{t('comparison.machine.databaseTitle')}</h2>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            {textField('databaseNodes', t('comparison.cluster.databaseNodes'), 'number')}
            {textField('databaseCores', t('comparison.machine.databaseCores'), 'number')}
            {textField('databaseMemory', t('comparison.machine.databaseMemory'), 'number')}
            {textField('databaseDisk', t('comparison.machine.databaseDisk'), 'number')}
          </div>
        </Card>

        {submitError && <p className="text-sm text-destructive">{submitError}</p>}
        <Button type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? t('list.loading') : t('actions.createRun')}
        </Button>
      </form>
    </div>
  )
}
