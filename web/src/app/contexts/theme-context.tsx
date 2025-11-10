import type { PropsWithChildren } from 'react'
import { createContext, useContext } from 'react'
import { useStore } from '@nanostores/react'
import { themeStore, setThemeMode, type ThemeMode } from '../../stores/theme'

interface ThemeContextValue {
  mode: ThemeMode
  resolvedMode: 'light' | 'dark'
  setMode: (mode: ThemeMode) => void
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined)

export const ThemeProvider = ({ children }: PropsWithChildren) => {
  const theme = useStore(themeStore)

  const value: ThemeContextValue = {
    mode: theme.mode,
    resolvedMode: theme.resolved,
    setMode: setThemeMode,
  }

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export const useTheme = () => {
  const context = useContext(ThemeContext)
  if (!context) {
    throw new Error('useTheme должен использоваться внутри ThemeProvider')
  }
  return context
}

