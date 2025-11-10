import { createConnectTransport } from '@connectrpc/connect-web'
import { authInterceptor } from './interceptors/auth-interceptor'

const normalizeBaseUrl = (value: string | undefined) => {
  if (!value || value.trim().length === 0) {
    return undefined
  }
  return value.endsWith('/') ? value.slice(0, -1) : value
}

const resolveBaseUrl = () => {
  const envUrl = normalizeBaseUrl(import.meta.env.VITE_API_BASE_URL)
  if (envUrl) {
    return envUrl
  }
  if (typeof window !== 'undefined') {
    return window.location.origin
  }
  return ''
}

export const appTransport = createConnectTransport({
  baseUrl: resolveBaseUrl(),
  useBinaryFormat: true,
  interceptors: [authInterceptor],
})
