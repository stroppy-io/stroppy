import { map } from 'nanostores'

export type ThemeMode = 'light' | 'dark' | 'system'
export type ThemeState = {
  mode: ThemeMode
  resolved: Exclude<ThemeMode, 'system'>
}

const storageKey = 'stroppy:theme'
const defaultState: ThemeState = {
  mode: 'system',
  resolved: 'light',
}

export const themeStore = map<ThemeState>(defaultState)

const getSystemPreference = (): Exclude<ThemeMode, 'system'> => {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const applyResolvedTheme = (resolved: ThemeState['resolved']) => {
  if (typeof document === 'undefined') return
  document.documentElement.dataset.theme = resolved
  document.documentElement.classList.toggle('dark', resolved === 'dark')
}

const persistTheme = (state: ThemeState) => {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(storageKey, JSON.stringify(state))
}

export const setThemeMode = (mode: ThemeMode) => {
  const resolved = mode === 'system' ? getSystemPreference() : mode
  const nextState: ThemeState = { mode, resolved }
  themeStore.set(nextState)
  applyResolvedTheme(resolved)
  persistTheme(nextState)
}

export const getThemeSnapshot = () => themeStore.get()

const bootstrapTheme = () => {
  if (typeof window === 'undefined') return

  try {
    const saved = window.localStorage.getItem(storageKey)
    if (saved) {
      const parsed: ThemeState = JSON.parse(saved)
      themeStore.set(parsed)
      applyResolvedTheme(parsed.resolved)
    } else {
      const resolved = getSystemPreference()
      themeStore.set({ mode: 'system', resolved })
      applyResolvedTheme(resolved)
    }
  } catch (error) {
    console.warn('Theme bootstrap failed', error)
    const resolved = getSystemPreference()
    themeStore.set({ mode: 'system', resolved })
    applyResolvedTheme(resolved)
  }

  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const listener = () => {
    if (themeStore.get().mode === 'system') {
      setThemeMode('system')
    }
  }
  media.addEventListener('change', listener)
}

bootstrapTheme()

