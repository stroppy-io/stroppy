import { Outlet } from 'react-router-dom'
import { LandingNav } from '../components/navigation/landing-nav'
import { AppFooter } from '../components/shell/app-footer'

export const LandingLayout = () => {
  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <LandingNav />
      <main className="flex-1">
        <Outlet />
      </main>
      <AppFooter />
    </div>
  )
}
