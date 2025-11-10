import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate, type Location } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '../../contexts/auth-context'
import { FullscreenLoader } from '../../components/shell/fullscreen-loader'

export const LandingLoginPage = () => {
  const { signIn, status, error } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  const redirectPath = (location.state as { from?: Location } | undefined)?.from?.pathname ?? '/app/dashboard'
  const isSubmitting = status === 'loading'

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      await signIn({ email, password })
      navigate(redirectPath, { replace: true })
    } catch (err) {
      console.error('login failed', err)
    }
  }

  if (status === 'checking') {
    return <FullscreenLoader label="Проверяем сессию" />
  }

  if (status === 'authenticated') {
    return <Navigate to={redirectPath} replace />
  }

  return (
    <section className="mx-auto flex w-full max-w-xl flex-col gap-6 px-4 py-12 sm:px-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.5em] text-muted-foreground">Вход</p>
        <h1 className="mt-2 text-3xl font-semibold text-foreground">Добро пожаловать снова</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Используйте свой аккаунт Stroppy для доступа к панели. Авторизация выполняет прямой Connect RPC вызов к backend по адресу{' '}
          <span className="font-mono text-foreground/80">localhost:8080</span>.
        </p>
        <p className="mt-4 text-xs text-muted-foreground">
          Ещё нет аккаунта?{' '}
          <Link to="/register" className="font-semibold text-foreground underline-offset-4 hover:underline">
            Зарегистрируйтесь
          </Link>
        </p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4 rounded-2xl border border-border/80 bg-card p-6 shadow-sm">
        <div className="space-y-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="you@example.com"
            autoComplete="email"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="password">Пароль</Label>
          <Input
            id="password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <Button type="submit" className="w-full uppercase tracking-wide" disabled={isSubmitting}>
          {isSubmitting ? 'Подключаемся…' : 'Войти'}
        </Button>
      </form>
    </section>
  )
}
