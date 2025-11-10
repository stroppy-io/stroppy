import { map } from 'nanostores'

export interface SessionState {
  accessToken: string | null
  refreshToken: string | null
  expiresAt: number | null
}

const storageKey = 'stroppy:session'
const defaultState: SessionState = {
  accessToken: null,
  refreshToken: null,
  expiresAt: null,
}

export const sessionStore = map<SessionState>(defaultState)

const persistSession = (state: SessionState) => {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(storageKey, JSON.stringify(state))
}

export const setSession = (partial: Partial<SessionState>) => {
  const next = { ...sessionStore.get(), ...partial }
  sessionStore.set(next)
  persistSession(next)
}

export const clearSession = () => {
  sessionStore.set(defaultState)
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(storageKey)
  }
}

export const getSessionSnapshot = () => sessionStore.get()

const bootstrapSession = () => {
  if (typeof window === 'undefined') return
  const saved = window.localStorage.getItem(storageKey)
  if (!saved) return
  try {
    const parsed: SessionState = JSON.parse(saved)
    sessionStore.set({ ...defaultState, ...parsed })
  } catch (error) {
    console.warn('Session bootstrap failed', error)
    clearSession()
  }
}

bootstrapSession()

