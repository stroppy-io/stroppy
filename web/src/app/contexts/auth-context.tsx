import type { PropsWithChildren } from 'react'
import { createContext, useContext, useEffect } from 'react'
import { useStore } from '@nanostores/react'
import {
  authStore,
  registerAccount,
  signInWithCredentials,
  signOut,
  checkAuthFromStorage,
  type AuthStatus,
  type AuthUser,
  type CredentialsPayload,
  type RegisterPayload,
} from '../../stores/auth'
import { sessionStore } from '../../stores/session'

interface AuthContextValue {
  user: AuthUser | null
  status: AuthStatus
  error: string | null
  token: string | null
  signIn: (payload: CredentialsPayload) => Promise<void>
  register: (payload: RegisterPayload) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined)

export const AuthProvider = ({ children }: PropsWithChildren) => {
  const auth = useStore(authStore)
  const session = useStore(sessionStore)
  useEffect(() => {
    checkAuthFromStorage().catch((error) => console.warn('auth bootstrap failed', error))
  }, [])

  const value: AuthContextValue = {
    user: auth.user,
    status: auth.status,
    error: auth.error,
    token: session.accessToken,
    signIn: signInWithCredentials,
    register: registerAccount,
    logout: signOut,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export const useAuth = () => {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth должен использоваться внутри AuthProvider')
  }
  return context
}
