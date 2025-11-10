import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { z } from 'zod'
import { create } from '@bufbuild/protobuf'
import { useTranslation } from '@/i18n/use-translation'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { useMutation, useTransport } from '@connectrpc/connect-query'
import { runAutomation } from '@/proto/panel/automate-AutomateService_connectquery.ts'
import { RunAutomationRequestSchema } from '@/proto/panel/automate_pb.ts'
import {
  ClusterSchema,
  DatabaseSchema,
  Database_Type,
  MachineInfoSchema,
  WorkloadSchema,
  Workload_Type,
} from '@/proto/panel/types_pb.ts'
import { getDatabaseTypeLabel, getWorkloadTypeLabel } from '@/app/features/runs/labels'
import {SupportedCloud} from "@/proto/crossplane/types_pb.ts";

type MachineFormValues = {
  cores: string
  memory: string
  disk: string
  publicIp: boolean
}

type FormValues = {
  workloadName: string
  workloadType: Workload_Type
  workloadParameters: string
  databaseName: string
  databaseType: Database_Type
  databaseParameters: string
  cloudProvider: SupportedCloud
  workloadMachine: MachineFormValues
  databaseMachine: MachineFormValues
}

const formSchema = z.object({
  workloadName: z.string().min(3),
  workloadType: z.nativeEnum(Workload_Type),
  workloadParameters: z.string().optional(),
  databaseName: z.string().min(3),
  databaseType: z.nativeEnum(Database_Type),
  databaseParameters: z.string().optional(),
  cloudProvider: z.nativeEnum(SupportedCloud),
  workloadCores: z.coerce.number().int().positive().default(2),
  workloadMemory: z.coerce.number().int().positive().default(2),
  workloadDisk: z.coerce.number().int().positive().default(15),
  workloadPublicIp: z.boolean().optional(),
  databaseCores: z.coerce.number().int().positive().default(2),
  databaseMemory: z.coerce.number().int().positive().default(2),
  databaseDisk: z.coerce.number().int().positive().default(15),
  databasePublicIp: z.boolean().optional(),
})

const SUPPORTED_WORKLOAD_TYPES: Workload_Type[] = [Workload_Type.TPCC]
const SUPPORTED_DATABASE_TYPES: Database_Type[] = [Database_Type.POSTGRES_ORIOLE]
const SUPPORTED_CLOUDS: SupportedCloud[] = [SupportedCloud.YANDEX]

const defaultMachine: MachineFormValues = {
  cores: '2',
  memory: '2',
  disk: '15',
  publicIp: false,
}

const parseParameterString = (value: string): Record<string, string> | undefined | null => {
  if (!value || !value.trim()) {
    return undefined
  }
  const pairs = value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  if (!pairs.length) {
    return undefined
  }
  const result: Record<string, string> = {}
  for (const pair of pairs) {
    const [key, ...rest] = pair.split('=')
    if (!key || rest.length === 0) {
      return null
    }
    const parsedKey = key.trim()
    const parsedValue = rest.join('=').trim()
    if (!parsedKey || !parsedValue) {
      return null
    }
    result[parsedKey] = parsedValue
  }
  return Object.keys(result).length ? result : undefined
}

export const AutomationsCreatePage = () => {
  const { t } = useTranslation('automations')
  const { t: tRuns } = useTranslation('runs')
  const navigate = useNavigate()
  const transport = useTransport()
  const mutation = useMutation(runAutomation, { transport })

  const [formValues, setFormValues] = useState<FormValues>({
    workloadName: '',
    workloadType: Workload_Type.TPCC,
    workloadParameters: '',
    databaseName: '',
    databaseType: Database_Type.POSTGRES_ORIOLE,
    databaseParameters: '',
    cloudProvider: SupportedCloud.YANDEX,
    workloadMachine: { ...defaultMachine, publicIp: true },
    databaseMachine: { ...defaultMachine },
  })
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitError, setSubmitError] = useState<string | null>(null)

  const getCloudProviderLabel = (provider: SupportedCloud) => {
    switch (provider) {
      case SupportedCloud.YANDEX:
        return t('form.cloudProviders.yandex')
      default:
        return t('form.cloudProviders.unspecified')
    }
  }

  const handleInputChange = (name: keyof FormValues, value: string | SupportedCloud | Workload_Type | Database_Type) => {
    setFormValues((prev) => ({ ...prev, [name]: value }))
    setErrors((prev) => ({ ...prev, [name as string]: '' }))
  }

  const machineErrorPrefix: Record<'databaseMachine' | 'workloadMachine', 'database' | 'workload'> = {
    databaseMachine: 'database',
    workloadMachine: 'workload',
  }

  const handleMachineChange = (
    target: 'databaseMachine' | 'workloadMachine',
    field: keyof MachineFormValues,
    value: string | boolean,
  ) => {
    setFormValues((prev) => ({
      ...prev,
      [target]: {
        ...prev[target],
        [field]: value,
      },
    }))
    const prefix = machineErrorPrefix[target]
    const capitalizedField = field.charAt(0).toUpperCase() + field.slice(1)
    setErrors((prev) => ({ ...prev, [`${prefix}${capitalizedField}`]: '' }))
  }

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSubmitError(null)
    setErrors({})

    const parsed = formSchema.safeParse({
      workloadName: formValues.workloadName,
      workloadType: formValues.workloadType,
      workloadParameters: formValues.workloadParameters || undefined,
      databaseName: formValues.databaseName,
      databaseType: formValues.databaseType,
      databaseParameters: formValues.databaseParameters || undefined,
      cloudProvider: formValues.cloudProvider,
      workloadCores: formValues.workloadMachine.cores,
      workloadMemory: formValues.workloadMachine.memory,
      workloadDisk: formValues.workloadMachine.disk,
      workloadPublicIp: formValues.workloadMachine.publicIp,
      databaseCores: formValues.databaseMachine.cores,
      databaseMemory: formValues.databaseMachine.memory,
      databaseDisk: formValues.databaseMachine.disk,
      databasePublicIp: formValues.databaseMachine.publicIp,
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
    const databaseParameters = parseParameterString(values.databaseParameters ?? '')
    if (databaseParameters === null) {
      setErrors((prev) => ({
        ...prev,
        databaseParameters: t('form.errors.invalidParameters', { target: t('form.fields.databaseParameters') }),
      }))
      return
    }
    const workloadParameters = parseParameterString(values.workloadParameters ?? '')
    if (workloadParameters === null) {
      setErrors((prev) => ({
        ...prev,
        workloadParameters: t('form.errors.invalidParameters', { target: t('form.fields.workloadParameters') }),
      }))
      return
    }

    const buildCluster = (machine: { cores: number; memory: number; disk: number; publicIp?: boolean }) =>
      create(ClusterSchema, {
        isSingleMachineMode: true,
        machines: [
          create(MachineInfoSchema, {
            cores: machine.cores,
            memory: machine.memory,
            disk: machine.disk,
            publicIp: machine.publicIp ?? false,
          }),
        ],
      })

    const request = create(RunAutomationRequestSchema, {
      usingCloudProvider: values.cloudProvider,
      database: create(DatabaseSchema, {
        name: values.databaseName,
        databaseType: values.databaseType,
        parameters: databaseParameters ?? undefined,
        runnerCluster: buildCluster({
          cores: values.databaseCores,
          memory: values.databaseMemory,
          disk: values.databaseDisk,
          publicIp: values.databasePublicIp,
        }),
      }),
      workload: create(WorkloadSchema, {
        name: values.workloadName,
        workloadType: values.workloadType,
        parameters: workloadParameters ?? undefined,
        runnerCluster: buildCluster({
          cores: values.workloadCores,
          memory: values.workloadMemory,
          disk: values.workloadDisk,
          publicIp: values.workloadPublicIp,
        }),
      }),
    })

    try {
      await mutation.mutateAsync(request)
      navigate('/app/automations')
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : 'Failed to start automation')
    }
  }

  const machineField = (
    target: 'databaseMachine' | 'workloadMachine',
    label: string,
    machineValues: MachineFormValues,
    errorPrefix: string,
  ) => (
    <div className="space-y-4 rounded-xl border border-border/60 bg-muted/20 p-4">
      <h3 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">{label}</h3>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="space-y-1">
          <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
            {t('form.fields.machineCores')}
          </label>
          <Input
            type="number"
            min={1}
            disabled={true}
            value={machineValues.cores}
            onChange={(event) => handleMachineChange(target, 'cores', event.target.value)}
          />
          {errors[`${errorPrefix}Cores`] && <p className="text-xs text-destructive">{errors[`${errorPrefix}Cores`]}</p>}
        </div>
        <div className="space-y-1">
          <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
            {t('form.fields.machineMemory')}
          </label>
          <Input
            type="number"
            min={1}
            disabled={true}
            value={machineValues.memory}
            onChange={(event) => handleMachineChange(target, 'memory', event.target.value)}
          />
          {errors[`${errorPrefix}Memory`] && (
            <p className="text-xs text-destructive">{errors[`${errorPrefix}Memory`]}</p>
          )}
        </div>
        <div className="space-y-1">
          <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
            {t('form.fields.machineDisk')}
          </label>
          <Input
            type="number"
            min={1}
            disabled={true}
            value={machineValues.disk}
            onChange={(event) => handleMachineChange(target, 'disk', event.target.value)}
          />
          {errors[`${errorPrefix}Disk`] && <p className="text-xs text-destructive">{errors[`${errorPrefix}Disk`]}</p>}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Checkbox
          id={`${target}-public-ip`}
          disabled={true}
          checked={machineValues.publicIp}
          onCheckedChange={(checked) => handleMachineChange(target, 'publicIp', checked === true)}
        />
        <label htmlFor={`${target}-public-ip`} className="text-sm text-foreground">
          {t('form.fields.machinePublicIp')}
        </label>
      </div>
    </div>
  )

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs uppercase tracking-[0.5em] text-muted-foreground">{t('page.title')}</p>
          <h1 className="text-3xl font-semibold text-foreground">{t('form.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('form.description')}</p>
        </div>
        <Button variant="outline" onClick={() => navigate('/app/automations')} disabled={mutation.isPending}>
          {t('actions.backToList')}
        </Button>
      </div>

      <form className="space-y-6" onSubmit={handleSubmit}>
        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">
            {t('form.sections.cloud')}
          </h2>
          <div className="space-y-1">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
              {t('form.fields.cloudProvider')}
            </label>
            <Select
              value={formValues.cloudProvider.toString()}
              onValueChange={(value) => handleInputChange('cloudProvider', Number(value) as SupportedCloud)}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('form.fields.cloudProvider')} />
              </SelectTrigger>
              <SelectContent>
                {SUPPORTED_CLOUDS.map((cloud) => (
                  <SelectItem key={cloud} value={cloud.toString()}>
                    {getCloudProviderLabel(cloud)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">
            {t('form.sections.database')}
          </h2>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
                {t('form.fields.databaseName')}
              </label>
              <Input
                type="text"
                value={formValues.databaseName}
                onChange={(event) => handleInputChange('databaseName', event.target.value)}
              />
              {errors.databaseName && <p className="text-xs text-destructive">{errors.databaseName}</p>}
            </div>
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
                {t('form.fields.databaseType')}
              </label>
              <Select
                value={formValues.databaseType.toString()}
                onValueChange={(value) => handleInputChange('databaseType', Number(value) as Database_Type)}
              >
                <SelectTrigger>
                  <SelectValue placeholder={t('form.fields.databaseType')} />
                </SelectTrigger>
                <SelectContent>
                  {SUPPORTED_DATABASE_TYPES.map((type) => (
                    <SelectItem key={type} value={type.toString()}>
                      {getDatabaseTypeLabel(type, tRuns)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.databaseType && <p className="text-xs text-destructive">{errors.databaseType}</p>}
            </div>
          </div>
          <div className="space-y-1">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
              {t('form.fields.databaseParameters')}
            </label>
            <Textarea
              value={formValues.databaseParameters}
              onChange={(event) => handleInputChange('databaseParameters', event.target.value)}
              rows={4}
              placeholder="key=value"
            />
            <p className="text-xs text-muted-foreground">{t('form.parametersHint')}</p>
            {errors.databaseParameters && <p className="text-xs text-destructive">{errors.databaseParameters}</p>}
          </div>
          {machineField('databaseMachine', t('form.sections.databaseMachine'), formValues.databaseMachine, 'database')}
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.3em] text-muted-foreground">
            {t('form.sections.workload')}
          </h2>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
                {t('form.fields.workloadName')}
              </label>
              <Input
                type="text"
                value={formValues.workloadName}
                onChange={(event) => handleInputChange('workloadName', event.target.value)}
              />
              {errors.workloadName && <p className="text-xs text-destructive">{errors.workloadName}</p>}
            </div>
            <div className="space-y-1">
              <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
                {t('form.fields.workloadType')}
              </label>
              <Select
                value={formValues.workloadType.toString()}
                onValueChange={(value) => handleInputChange('workloadType', Number(value) as Workload_Type)}
              >
                <SelectTrigger>
                  <SelectValue placeholder={t('form.fields.workloadType')} />
                </SelectTrigger>
                <SelectContent>
                  {SUPPORTED_WORKLOAD_TYPES.map((type) => (
                    <SelectItem key={type} value={type.toString()}>
                      {getWorkloadTypeLabel(type, tRuns)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.workloadType && <p className="text-xs text-destructive">{errors.workloadType}</p>}
            </div>
          </div>
          <div className="space-y-1">
            <label className="text-xs uppercase tracking-[0.3em] text-muted-foreground">
              {t('form.fields.workloadParameters')}
            </label>
            <Textarea
              value={formValues.workloadParameters}
              onChange={(event) => handleInputChange('workloadParameters', event.target.value)}
              rows={4}
              placeholder="key=value"
            />
            <p className="text-xs text-muted-foreground">{t('form.parametersHint')}</p>
            {errors.workloadParameters && <p className="text-xs text-destructive">{errors.workloadParameters}</p>}
          </div>
          {machineField('workloadMachine', t('form.sections.workloadMachine'), formValues.workloadMachine, 'workload')}
        </Card>

        {submitError && <p className="text-sm text-destructive">{submitError}</p>}

        <div className="flex flex-wrap gap-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => navigate('/app/automations')}
            disabled={mutation.isPending}
          >
            {t('form.actions.cancel')}
          </Button>
          <Button type="submit" disabled={mutation.isPending}>
            {t('form.actions.submit')}
          </Button>
        </div>
      </form>
    </div>
  )
}
