import type { Interceptor } from '@connectrpc/connect'
import { Code, ConnectError } from '@connectrpc/connect'
import { getSessionSnapshot } from '../../../stores/session'

type RefreshSessionFn = () => Promise<void>
type SignOutFn = () => Promise<void>

const attachAuthHeader = (header: Headers, token: string | null) => {
  if (!token) return
  header.set('Authorization', `Bearer ${token}`)
}

const loadAuthActions = async (): Promise<{ refreshSession?: RefreshSessionFn; signOut?: SignOutFn }> => {
  try {
    const module = await import('../../../stores/auth')
    return {
      refreshSession: module.refreshSession,
      signOut: module.signOut,
    }
  } catch (error) {
    console.error('Unable to load auth helpers', error)
    return {}
  }
}

const tryRefreshAccessToken = async () => {
  const session = getSessionSnapshot()
  const actions = await loadAuthActions()
  if (!session.refreshToken) {
    await actions.signOut?.()
    return false
  }
  try {
    if (!actions.refreshSession) {
      await actions.signOut?.()
      return false
    }
    await actions.refreshSession()
    return true
  } catch (error) {
    console.warn('refresh token flow failed', error)
    await actions.signOut?.()
    return false
  }
}

export const authInterceptor: Interceptor = (next) => async (request) => {
  const token = getSessionSnapshot().accessToken
  attachAuthHeader(request.header, token)

  try {
    return await next(request)
  } catch (error) {
    if (error instanceof ConnectError && error.code === Code.Unauthenticated) {
      const refreshed = await tryRefreshAccessToken()
      if (refreshed) {
        const updatedToken = getSessionSnapshot().accessToken
        if (updatedToken) {
          const retryRequest = request.clone()
          attachAuthHeader(retryRequest.header, updatedToken)
          return await next(retryRequest)
        }
      }
    }
    throw error
  }
}
