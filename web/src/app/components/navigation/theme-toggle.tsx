import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from '../../contexts/theme-context'

const cycleOrder = ['light', 'dark', 'system'] as const

export const ThemeToggle = () => {
  const { mode, setMode } = useTheme()
  const nextMode = cycleOrder[(cycleOrder.indexOf(mode) + 1) % cycleOrder.length] ?? 'light'
  const icon =
    mode === 'light' ? <Sun className="h-4 w-4" /> : mode === 'dark' ? <Moon className="h-4 w-4" /> : <Monitor className="h-4 w-4" />

  return (
    <button
      type="button"
      onClick={() => setMode(nextMode)}
      className="inline-flex items-center gap-2 rounded-full border border-border bg-card/80 px-3 py-1 text-xs font-medium text-muted-foreground transition hover:bg-muted"
      aria-label="Переключить тему"
    >
      {icon}
      <span className="hidden sm:inline">{mode}</span>
    </button>
  )
}
