import { Outlet } from 'react-router-dom'
import { PanelSidebar } from '../components/navigation/panel-sidebar'
import { PanelTopbar } from '../components/navigation/panel-topbar'

export const PanelLayout = () => {
  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <PanelSidebar />
      <div className="flex flex-1 flex-col">
        <PanelTopbar />
        <main className="flex-1 overflow-y-auto px-4 py-6 md:px-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
