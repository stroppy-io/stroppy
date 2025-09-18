import React, { createContext, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { apiClient } from '../services/api';
import type { User, LoginRequest, RegisterRequest } from '../services/api';

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  login: (credentials: LoginRequest) => Promise<void>;
  register: (data: RegisterRequest) => Promise<void>;
  logout: () => Promise<void>;
  checkAuth: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

interface AuthProviderProps {
  children: ReactNode;
}

export const AuthProvider: React.FC<AuthProviderProps> = ({ children }) => {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const isAuthenticated = !!user && !!apiClient.getToken();

  // Проверка авторизации при загрузке приложения
  const checkAuth = async () => {
    try {
      setIsLoading(true);
      const token = apiClient.getToken();
      
      if (!token) {
        setUser(null);
        return;
      }

      // Проверяем валидность токена, получая профиль пользователя
      const userProfile = await apiClient.getProfile();
      setUser(userProfile);
    } catch (error) {
      console.error('Ошибка проверки авторизации:', error);
      // Если токен недействителен, очищаем его
      apiClient.clearToken();
      setUser(null);
    } finally {
      setIsLoading(false);
    }
  };

  // Вход в систему
  const login = async (credentials: LoginRequest) => {
    try {
      setIsLoading(true);
      const authData = await apiClient.login(credentials);
      setUser(authData.user);
    } catch (error) {
      console.error('Ошибка входа:', error);
      throw error;
    } finally {
      setIsLoading(false);
    }
  };

  // Регистрация
  const register = async (data: RegisterRequest) => {
    try {
      setIsLoading(true);
      await apiClient.register(data);
      // После регистрации автоматически входим
      await login({ username: data.username, password: data.password });
    } catch (error) {
      console.error('Ошибка регистрации:', error);
      throw error;
    } finally {
      setIsLoading(false);
    }
  };

  // Выход из системы
  const logout = async () => {
    try {
      await apiClient.logout();
      setUser(null);
    } catch (error) {
      console.error('Ошибка выхода:', error);
    }
  };

  // Проверяем авторизацию при монтировании компонента
  useEffect(() => {
    checkAuth();
  }, []);

  const value: AuthContextType = {
    user,
    isLoading,
    isAuthenticated,
    login,
    register,
    logout,
    checkAuth,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

// Хук для использования контекста авторизации
export const useAuth = (): AuthContextType => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth должен использоваться внутри AuthProvider');
  }
  return context;
};
