import { useEffect, useState } from 'react'
import {
  enable as enableDarkMode,
  disable as disableDarkMode,
  auto as followSystemColorScheme,
  exportGeneratedCSS as collectCSS,
  isEnabled as isDarkReaderEnabled
} from 'darkreader'

export interface DarkReaderConfig {
  brightness: number
  contrast: number
  sepia: number
}

const defaultConfig: DarkReaderConfig = {
  brightness: 100,
  contrast: 90,
  sepia: 10
}

export const useDarkReader = () => {
  const [isEnabled, setIsEnabled] = useState(false)
  const [config, setConfig] = useState<DarkReaderConfig>(defaultConfig)

  const enable = (customConfig?: Partial<DarkReaderConfig>) => {
    const finalConfig = { ...config, ...customConfig }
    enableDarkMode(finalConfig)
    setIsEnabled(true)
    setConfig(finalConfig)
    localStorage.setItem('darkreader-enabled', 'true')
    localStorage.setItem('darkreader-config', JSON.stringify(finalConfig))
  }

  const disable = () => {
    disableDarkMode()
    setIsEnabled(false)
    localStorage.setItem('darkreader-enabled', 'false')
  }

  const toggle = () => {
    if (isEnabled) {
      disable()
    } else {
      enable()
    }
  }

  const updateConfig = (newConfig: Partial<DarkReaderConfig>) => {
    if (isEnabled) {
      const finalConfig = { ...config, ...newConfig }
      enableDarkMode(finalConfig)
      setConfig(finalConfig)
      localStorage.setItem('darkreader-config', JSON.stringify(finalConfig))
    }
  }

  const followSystem = () => {
    followSystemColorScheme({
      brightness: config.brightness,
      contrast: config.contrast,
      sepia: config.sepia
    })
    setIsEnabled(true)
    localStorage.setItem('darkreader-enabled', 'auto')
  }

  const stopFollowingSystem = () => {
    followSystemColorScheme(false)
    setIsEnabled(false)
    localStorage.setItem('darkreader-enabled', 'false')
  }

  const exportCSS = async () => {
    return await collectCSS()
  }

  const checkEnabled = () => {
    return isDarkReaderEnabled()
  }

  // Инициализация при загрузке
  useEffect(() => {
    const savedEnabled = localStorage.getItem('darkreader-enabled')
    const savedConfig = localStorage.getItem('darkreader-config')

    if (savedConfig) {
      try {
        const parsedConfig = JSON.parse(savedConfig)
        setConfig(parsedConfig)
      } catch (error) {
        console.warn('Failed to parse saved Dark Reader config:', error)
      }
    }

    if (savedEnabled === 'true') {
      enable()
    } else if (savedEnabled === 'auto') {
      followSystem()
    }
  }, [])

  return {
    isEnabled,
    config,
    enable,
    disable,
    toggle,
    updateConfig,
    followSystem,
    stopFollowingSystem,
    exportCSS,
    checkEnabled
  }
}
