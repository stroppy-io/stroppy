import type { PropsWithChildren } from 'react'
import { createContext, useContext } from 'react'
import { useStore } from '@nanostores/react'
import { clearSession, sessionStore, setSession } from '../../stores/session'

interface SessionContextValue {
  token: string | null
  refreshToken: string | null
  setToken: (token: string | null, options?: { refreshToken?: string | null; expiresAt?: number | null }) => void
  clear: () => void
}

const SessionContext = createContext<SessionContextValue | undefined>(undefined)

export const SessionProvider = ({ children }: PropsWithChildren) => {
  const session = useStore(sessionStore)

  const value: SessionContextValue = {
    token: session.accessToken,
    refreshToken: session.refreshToken,
    setToken: (token, options = {}) =>
      setSession({
        accessToken: token,
        refreshToken: options.refreshToken ?? session.refreshToken,
        expiresAt: options.expiresAt ?? session.expiresAt,
      }),
    clear: clearSession,
  }

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}

export const useSession = () => {
  const context = useContext(SessionContext)
  if (!context) {
    throw new Error('useSession должен использоваться внутри SessionProvider')
  }
  return context
}

