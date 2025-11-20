import type { PropsWithChildren } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { FullscreenLoader } from '../components/shell/fullscreen-loader'
import { useAuth } from '../contexts/auth-context'

interface ProtectedRouteProps extends PropsWithChildren {
  requireAdmin?: boolean
}

export const ProtectedRoute = ({ children, requireAdmin = false }: ProtectedRouteProps) => {
  const location = useLocation()
  const { status, user } = useAuth()

  if (status === 'checking' || status === 'loading') {
    return <FullscreenLoader label="Загружаем панель" />
  }

  if (status === 'unauthenticated' || !user) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  if (requireAdmin && !user.admin) {
    return <Navigate to="/app/dashboard" state={{ from: location, reason: 'forbidden' }} replace />
  }

  return <>{children}</>
}
