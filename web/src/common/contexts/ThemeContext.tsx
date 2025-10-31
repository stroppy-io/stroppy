import React, { createContext, useContext } from 'react';
import type { ReactNode } from 'react';
import { useDarkReader } from '../hooks/useDarkReader';

interface ThemeContextType {
  // Dark Reader integration
  darkReaderEnabled: boolean;
  toggleDarkReader: () => void;
  updateDarkReaderConfig: (config: Partial<{ brightness: number; contrast: number; sepia: number }>) => void;
  followSystemTheme: () => void;
  stopFollowingSystem: () => void;
  exportCSS: () => Promise<string>;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

interface ThemeProviderProps {
  children: ReactNode;
}


export const ThemeProvider: React.FC<ThemeProviderProps> = ({ children }) => {
  // Dark Reader integration
  const {
    isEnabled: darkReaderEnabled,
    toggle: toggleDarkReader,
    updateConfig: updateDarkReaderConfig,
    followSystem: followSystemTheme,
    stopFollowingSystem,
    exportCSS
  } = useDarkReader();

  const value: ThemeContextType = {
    // Dark Reader integration
    darkReaderEnabled,
    toggleDarkReader,
    updateDarkReaderConfig,
    followSystemTheme,
    stopFollowingSystem,
    exportCSS
  };

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  );
};

// Хук для использования контекста тем
export const useTheme = (): ThemeContextType => {
  const context = useContext(ThemeContext);
  if (context === undefined) {
    throw new Error('useTheme должен использоваться внутри ThemeProvider');
  }
  return context;
};