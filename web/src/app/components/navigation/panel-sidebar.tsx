import { Bot, Gauge, Layers3 } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { cn } from '@/lib/utils'

export const panelNavItems = [
  { to: '/app/dashboard', label: 'Дашборд', icon: Gauge },
    // TODO
  // { to: '/app/configurator', label: 'Конфигуратор', icon: Settings2 },
  { to: '/app/automations', label: 'Автоматизации', icon: Bot },
  { to: '/app/runs', label: 'Запуски', icon: Layers3 },
]

interface PanelSidebarProps {
  variant?: 'desktop' | 'mobile'
  onNavigate?: () => void
}

export const PanelSidebar = ({ variant = 'desktop', onNavigate }: PanelSidebarProps) => {
  const containerClasses =
    variant === 'desktop'
      ? 'hidden md:flex w-64 border-r border-sidebar-border bg-sidebar px-4 py-6 text-sidebar-foreground backdrop-blur'
      : 'flex w-full border-b border-sidebar-border bg-sidebar px-4 py-6 text-sidebar-foreground'

  return (
    <aside className={cn('flex-col text-sidebar-foreground', containerClasses)}>
      <NavLink to="/" className="mb-6">
        <p className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">Console</p>
        <p className="text-lg font-semibold text-sidebar-foreground">Stroppy Cloud</p>
      </NavLink>

      <nav className="flex flex-1 flex-col gap-2">
        {panelNavItems.map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            onClick={onNavigate}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition',
                isActive
                  ? 'bg-sidebar-primary text-sidebar-primary-foreground shadow-sm'
                  : 'text-sidebar-foreground/80 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'
              )
            }
          >
            <Icon className="h-4 w-4" />
            {label}
          </NavLink>
        ))}
      </nav>
    </aside>
  )
}
