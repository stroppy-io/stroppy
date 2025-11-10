import { Link } from 'react-router-dom'
import { NavigationMenu, NavigationMenuList, NavigationMenuItem, NavigationMenuLink } from '@/components/ui/navigation-menu'
import { Button } from '@/components/ui/button'
import { ThemeToggle } from './theme-toggle'
import { useTranslation } from '@/i18n/use-translation'
import { useAuth } from '../../contexts/auth-context'

export const LandingNav = () => {
  const { t } = useTranslation('landing')
  const { status } = useAuth()
  const isAuthenticated = status === 'authenticated'

  const navLinks = [
    { path: '/#features', label: t('navigation.features') },
    { path: '/#tops', label: t('navigation.tops') },
    { path: '/docs', label: t('navigation.about') },
  ]

  return (
    <header className="sticky top-0 z-40 border-b border-border bg-background/80 backdrop-blur">
      <div className="mx-auto flex h-16 w-full max-w-6xl items-center justify-between px-4 sm:px-6">
        <Link to="/" className="flex items-center gap-2 font-semibold tracking-tight">
          <span className="rounded-md bg-primary px-2 py-1 text-xs uppercase text-primary-foreground">Stroppy</span>
          <span className="text-sm text-muted-foreground">Cloud Panel</span>
        </Link>

        <NavigationMenu className="hidden md:flex">
          <NavigationMenuList className="gap-1">
            {navLinks.map((link) => (
              <NavigationMenuItem key={link.path}>
                <NavigationMenuLink asChild>
                  <Link to={link.path} className="text-sm text-muted-foreground transition hover:text-foreground">
                    {link.label}
                  </Link>
                </NavigationMenuLink>
              </NavigationMenuItem>
            ))}
          </NavigationMenuList>
        </NavigationMenu>

        <div className="flex items-center gap-2">
          <ThemeToggle />
          <Button variant="outline" asChild className="uppercase tracking-wide">
            <Link to={isAuthenticated ? '/app/dashboard' : '/login'}>
              {isAuthenticated ? t('navigation.console') : t('navigation.login')}
            </Link>
          </Button>
        </div>
      </div>
    </header>
  )
}
