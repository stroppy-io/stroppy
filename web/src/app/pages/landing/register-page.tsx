import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate, type Location } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '../../contexts/auth-context'
import { FullscreenLoader } from '../../components/shell/fullscreen-loader'

export const LandingRegisterPage = () => {
  const { register, status, error } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [team, setTeam] = useState('')

  const redirectPath = (location.state as { from?: Location } | undefined)?.from?.pathname ?? '/app/dashboard'
  const isSubmitting = status === 'loading'

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    try {
      await register({ email, password, team })
      navigate(redirectPath, { replace: true })
    } catch (err) {
      console.error('registration failed', err)
    }
  }

  if (status === 'checking') {
    return <FullscreenLoader label="Проверяем сессию" />
  }

  if (status === 'authenticated') {
    return <Navigate to={redirectPath} replace />
  }

  return (
    <section className="mx-auto w-full max-w-xl space-y-6 px-4 py-12 sm:px-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.5em] text-muted-foreground">Регистрация</p>
        <h1 className="mt-2 text-3xl font-semibold text-foreground">Создайте аккаунт</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Новый аккаунт активируется сразу после успешной регистрации. RPC метод <span className="font-mono text-foreground/80">panel.AccountService.Register</span>{' '}
          выполняется против локального backend `localhost:8080`.
        </p>
        <p className="mt-4 text-xs text-muted-foreground">
          Уже есть аккаунт?{' '}
          <Link to="/login" className="font-semibold text-foreground underline-offset-4 hover:underline">
            Войдите
          </Link>
        </p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4 rounded-2xl border border-dashed border-border/80 bg-card p-6">
        <div className="space-y-2">
          <Label htmlFor="team">Команда / компания</Label>
          <Input
            id="team"
            value={team}
            onChange={(event) => setTeam(event.target.value)}
            placeholder="Stroppy team"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="register-email">Email</Label>
          <Input
            id="register-email"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="you@example.com"
            autoComplete="email"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="register-password">Пароль</Label>
          <Input
            id="register-password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            autoComplete="new-password"
            required
          />
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <Button type="submit" variant="outline" className="w-full uppercase tracking-wide" disabled={isSubmitting}>
          {isSubmitting ? 'Создаём…' : 'Создать аккаунт'}
        </Button>
      </form>
    </section>
  )
}
