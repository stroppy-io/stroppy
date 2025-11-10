import { useState } from 'react'
import { LogOut, Menu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import { ThemeToggle } from './theme-toggle'
import { PanelSidebar } from './panel-sidebar'
import { useAuth } from '../../contexts/auth-context'

export const PanelTopbar = () => {
  const { user, logout } = useAuth()
  const [navOpen, setNavOpen] = useState(false)
  const [isLoggingOut, setIsLoggingOut] = useState(false)

  const handleLogout = async () => {
    setIsLoggingOut(true)
    try {
      await logout()
    } catch (error) {
      console.error('logout failed', error)
    } finally {
      setIsLoggingOut(false)
    }
  }

  return (
    <header className="flex items-center gap-4 border-b border-border bg-card/80 px-4 py-3 backdrop-blur">
      <Sheet open={navOpen} onOpenChange={setNavOpen}>
        <SheetTrigger asChild>
          <Button variant="outline" size="icon" className="md:hidden">
            <Menu className="h-4 w-4" />
            <span className="sr-only">Открыть меню</span>
          </Button>
        </SheetTrigger>
        <SheetContent side="left" className="w-72 p-0">
          <PanelSidebar variant="mobile" onNavigate={() => setNavOpen(false)} />
        </SheetContent>
      </Sheet>

      <div className="flex flex-1 flex-col text-left">
        <p className="text-xs uppercase tracking-[0.3em] text-muted-foreground">Control plane</p>
        <p className="text-sm font-semibold text-foreground">Оркестрация экспериментов</p>
      </div>

      <div className="flex items-center gap-3">
        <ThemeToggle />
        <div className="flex items-center gap-2 rounded-full border border-border/80 bg-card/80 px-4 py-1 text-sm font-semibold">
          <span className="text-foreground">{user?.email ?? 'anonymous@stroppy.io'}</span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="gap-1 text-muted-foreground hover:text-foreground"
            onClick={handleLogout}
            disabled={isLoggingOut}
          >
            <LogOut className="h-4 w-4" />
            {isLoggingOut ? 'Выходим…' : 'Выйти'}
          </Button>
        </div>
      </div>
    </header>
  )
}
