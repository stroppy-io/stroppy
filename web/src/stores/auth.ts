import { map } from 'nanostores'
import { ConnectError } from '@connectrpc/connect'
import { accountClient } from '@/lib/connect/clients'
import { clearSession, setSession, sessionStore } from './session'
import type { User } from '@/proto/panel/account_pb.ts'

export type AuthStatus = 'idle' | 'loading' | 'checking' | 'authenticated' | 'unauthenticated'

export interface AuthUser {
  id: string
  email: string
  name?: string
  admin: boolean
  avatarUrl?: string
  roles?: string[]
}

export interface AuthState {
  status: AuthStatus
  user: AuthUser | null
  error: string | null
}

const authStorageKey = 'stroppy:auth'
const defaultState: AuthState = {
  status: 'checking',
  user: null,
  error: null,
}

export const authStore = map<AuthState>(defaultState)

const persistAuth = (state: AuthState) => {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(authStorageKey, JSON.stringify(state))
  } catch (error) {
    console.warn('Unable to persist auth state', error)
  }
}

const updateAuthStore = (partial: Partial<AuthState>) => {
  const next = { ...authStore.get(), ...partial }
  authStore.set(next)
  persistAuth(next)
  return next
}

export const getAuthSnapshot = () => authStore.get()

export const setAuthStatus = (status: AuthStatus) => updateAuthStore({ status })

export const setAuthUser = (user: AuthUser | null) => {
  if (user) {
    updateAuthStore({ user, status: 'authenticated', error: null })
  } else {
    updateAuthStore({ user: null, status: 'unauthenticated', error: null })
  }
}

export const setAuthError = (error: string | null) => updateAuthStore({ error })

export interface CredentialsPayload {
  email: string
  password: string
}

export interface RegisterPayload extends CredentialsPayload {
  team?: string
}

const extractErrorMessage = (error: unknown): string => {
  if (error instanceof ConnectError) {
    return error.rawMessage
  }
  if (error instanceof Error) {
    return error.message
  }
  return 'unknown error'
}

const buildUserFromResponse = (user: User): AuthUser => ({
  id: user.id?.id ?? user.email,
  email: user.email,
  name: user.email?.split('@')[0],
  admin: user.admin ?? false,
})

const fetchCurrentUser = async (): Promise<AuthUser> => {
  const response = await accountClient.getMe({})
  return buildUserFromResponse(response)
}

export const signInWithCredentials = async (payload: CredentialsPayload) => {
  setAuthStatus('loading')
  setAuthError(null)
  try {
    const response = await accountClient.login({
      email: payload.email,
      password: payload.password,
    })

    setSession({
      accessToken: response.accessToken,
      refreshToken: response.refreshToken ?? null,
    })
    const user = await fetchCurrentUser()
    setAuthUser(user)
  } catch (error) {
    setAuthError(extractErrorMessage(error))
    clearSession()
    setAuthUser(null)
    throw error
  }
}

export const registerAccount = async (payload: RegisterPayload) => {
  setAuthStatus('loading')
  setAuthError(null)
  try {
    await accountClient.register({
      email: payload.email,
      password: payload.password,
    })
    await signInWithCredentials(payload)
  } catch (error) {
    setAuthError(extractErrorMessage(error))
    clearSession()
    setAuthUser(null)
    throw error
  }
}

export const refreshSession = async () => {
  const current = sessionStore.get()
  if (!current.refreshToken) {
    throw new Error('No refresh token available')
  }

  const response = await accountClient.refreshTokens({
    refreshToken: current.refreshToken,
  })

  setSession({
    accessToken: response.accessToken,
    refreshToken: current.refreshToken,
  })
}

export const signOut = async () => {
  clearSession()
  setAuthUser(null)
}

export const checkAuthFromStorage = async () => {
  setAuthStatus('checking')
  let session = sessionStore.get()

  if (!session.accessToken && session.refreshToken) {
    try {
      await refreshSession()
      session = sessionStore.get()
    } catch (error) {
      console.warn('Unable to refresh session', error)
      setAuthError('Session expired, please sign in again.')
      clearSession()
      setAuthUser(null)
      return
    }
  }

  if (session.accessToken) {
    try {
      const user = await fetchCurrentUser()
      setAuthUser(user)
      return
    } catch (error) {
      console.warn('Unable to load current user, signing out', error)
      setAuthError(extractErrorMessage(error))
      clearSession()
      setAuthUser(null)
      return
    }
  }

  setAuthUser(null)
}

const bootstrapAuth = () => {
  if (typeof window === 'undefined') return
  const saved = window.localStorage.getItem(authStorageKey)
  if (!saved) {
    authStore.set(defaultState)
    return
  }
  try {
    const parsed: AuthState = JSON.parse(saved)
    authStore.set(parsed)
  } catch (error) {
    console.warn('Auth bootstrap failed', error)
    authStore.set(defaultState)
    window.localStorage.removeItem(authStorageKey)
  }
}

bootstrapAuth()
