import type { HTMLAttributes } from 'react'
import { createContext, forwardRef, useContext } from 'react'
import { cn } from '@/lib/utils'

export type ChartConfig = Record<
  string,
  {
    label: string
    color: string
  }
>

const ChartContext = createContext<ChartConfig>({})
const useChartConfig = () => useContext(ChartContext)

export interface ChartContainerProps extends HTMLAttributes<HTMLDivElement> {
  config: ChartConfig
}

export const ChartContainer = forwardRef<HTMLDivElement, ChartContainerProps>(({ className, config, style, children, ...props }, ref) => {
  const cssVariables = Object.entries(config).reduce<Record<string, string>>((vars, [key, value]) => {
    vars[`--chart-${key}`] = value.color
    return vars
  }, {})

  return (
    <ChartContext.Provider value={config}>
      <div ref={ref} className={cn('flex min-h-[220px] w-full flex-1', className)} style={{ ...cssVariables, ...style }} {...props}>
        {children}
      </div>
    </ChartContext.Provider>
  )
})
ChartContainer.displayName = 'ChartContainer'

type TooltipPayload = {
  color?: string
  name?: string
  value?: number
  dataKey?: string | number
}

export interface ChartTooltipContentProps {
  active?: boolean
  label?: string | number
  payload?: TooltipPayload[]
  indicator?: 'dot' | 'line'
}

export const ChartTooltipContent = ({ active, payload, label, indicator = 'dot' }: ChartTooltipContentProps) => {
  const config = useChartConfig()
  if (!active || !payload || payload.length === 0) {
    return null
  }

  return (
    <div className="rounded-xl border border-border/80 bg-background/80 px-3 py-2 text-xs shadow-lg backdrop-blur">
      {label ? <p className="mb-1 font-semibold text-foreground">{label}</p> : null}
      <div className="space-y-1">
        {(payload ?? []).map((item, index) => {
          if (!item) return null
          const color = item.color ?? config[item.name ?? '']?.color ?? 'hsl(var(--foreground))'
          const text = config[item.name ?? '']?.label ?? item.name
          return (
            <div key={item.dataKey?.toString() ?? index} className="flex items-center gap-2 text-foreground/80">
              <span
                className={cn('inline-flex h-2.5 w-2.5 rounded-full', indicator === 'line' && 'h-0.5 w-3 rounded-sm')}
                style={{ backgroundColor: color, borderColor: color }}
              />
              <span className="capitalize">{text}</span>
              <span className="font-semibold text-foreground">{item.value?.toLocaleString('ru-RU')}</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
