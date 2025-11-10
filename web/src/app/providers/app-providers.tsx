import type { PropsWithChildren } from 'react'
import { TransportProvider } from '@connectrpc/connect-query'
import { QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { appTransport } from '../../lib/connect/transport'
import { createQueryClient } from '../../lib/connect/query-client'
import { SessionProvider } from '../contexts/session-context'
import { AuthProvider } from '../contexts/auth-context'
import { ThemeProvider } from '../contexts/theme-context'

const queryClient = createQueryClient()

export const AppProviders = ({ children }: PropsWithChildren) => (
  <TransportProvider transport={appTransport}>
    <QueryClientProvider client={queryClient}>
      <SessionProvider>
        <AuthProvider>
          <ThemeProvider>{children}</ThemeProvider>
        </AuthProvider>
      </SessionProvider>
      <ReactQueryDevtools initialIsOpen={false} buttonPosition="bottom-right" position="bottom" />
    </QueryClientProvider>
  </TransportProvider>
)
