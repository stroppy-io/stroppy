import type { PropsWithChildren } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { FullscreenLoader } from '../components/shell/fullscreen-loader'
import { useAuth } from '../contexts/auth-context'

export const ProtectedRoute = ({ children }: PropsWithChildren) => {
  const location = useLocation()
  const { status } = useAuth()

  if (status === 'checking' || status === 'loading') {
    return <FullscreenLoader label="Загружаем панель" />
  }

  if (status === 'unauthenticated') {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}

