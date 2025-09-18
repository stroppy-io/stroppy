import React, { createContext, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { theme } from 'antd';
import type { ThemeConfig } from 'antd';

export type ThemeMode = 'light' | 'dark';

interface ThemeContextType {
  mode: ThemeMode;
  toggleTheme: () => void;
  antdTheme: ThemeConfig;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

interface ThemeProviderProps {
  children: ReactNode;
}

// Светлая тема
const lightTheme: ThemeConfig = {
  algorithm: theme.defaultAlgorithm,
  token: {
    colorPrimary: '#1890ff',
    borderRadius: 6,
    colorBgContainer: '#ffffff',
    colorBgLayout: '#f5f5f5',
    colorText: '#000000',
    colorTextSecondary: '#666666',
    colorBorder: '#d9d9d9',
    colorBgElevated: '#ffffff',
  },
  components: {
    Layout: {
      headerBg: '#ffffff',
      siderBg: '#ffffff',
      bodyBg: '#f5f5f5',
    },
    Menu: {
      itemBg: 'transparent',
      itemSelectedBg: '#e6f7ff',
      itemHoverBg: '#f0f0f0',
      colorText: '#000000',
    },
    Card: {
      colorBgContainer: '#ffffff',
      colorBorder: '#d9d9d9',
    },
    Table: {
      colorBgContainer: '#ffffff',
      colorText: '#000000',
      headerBg: '#fafafa',
    },
    Button: {
      colorBgContainer: '#ffffff',
      colorText: '#000000',
    },
    Input: {
      colorBgContainer: '#ffffff',
      colorText: '#000000',
      colorBorder: '#d9d9d9',
    },
  },
};

// Тёмная тема
const darkTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    colorPrimary: '#1890ff',
    borderRadius: 6,
    colorBgContainer: '#141414',
    colorBgLayout: '#000000',
    colorText: '#ffffff',
    colorTextSecondary: '#a6a6a6',
    colorBorder: '#424242',
    colorBgElevated: '#1f1f1f',
  },
  components: {
    Layout: {
      headerBg: '#141414',
      siderBg: '#141414',
      bodyBg: '#000000',
    },
    Menu: {
      itemBg: 'transparent',
      itemSelectedBg: '#1890ff1a',
      itemHoverBg: '#262626',
      colorText: '#ffffff',
    },
    Card: {
      colorBgContainer: '#141414',
      colorBorder: '#424242',
    },
    Table: {
      colorBgContainer: '#141414',
      colorText: '#ffffff',
      headerBg: '#1f1f1f',
      colorBorder: 'transparent',
      borderColor: 'transparent',
      headerBorderRadius: 0,
    },
    Button: {
      colorBgContainer: '#141414',
      colorText: '#ffffff',
      colorBorder: '#424242',
    },
    Input: {
      colorBgContainer: '#1f1f1f',
      colorText: '#ffffff',
      colorBorder: '#424242',
    },
    Statistic: {
      colorText: '#ffffff',
      colorTextDescription: '#a6a6a6',
    },
    Typography: {
      colorText: '#ffffff',
      colorTextSecondary: '#a6a6a6',
    },
  },
};

export const ThemeProvider: React.FC<ThemeProviderProps> = ({ children }) => {
  const [mode, setMode] = useState<ThemeMode>(() => {
    // Получаем сохранённую тему из localStorage или используем светлую по умолчанию
    const savedTheme = localStorage.getItem('theme') as ThemeMode;
    return savedTheme || 'light';
  });

  // Сохраняем тему в localStorage при изменении
  useEffect(() => {
    localStorage.setItem('theme', mode);
    
    // Обновляем класс на body для дополнительных стилей
    document.body.className = mode === 'dark' ? 'dark-theme' : 'light-theme';
  }, [mode]);

  const toggleTheme = () => {
    setMode(prev => prev === 'light' ? 'dark' : 'light');
  };

  const antdTheme = mode === 'light' ? lightTheme : darkTheme;

  const value: ThemeContextType = {
    mode,
    toggleTheme,
    antdTheme,
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