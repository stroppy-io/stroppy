import axios, { AxiosError } from 'axios';
import type { AxiosInstance } from 'axios';

// Типы для API
export interface User {
  id: number;
  username: string;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface RegisterRequest {
  username: string;
  password: string;
}

export interface AuthResponse {
  user: User;
  token: string;
}

export interface Run {
  id: number;
  user_id: number;
  name: string;
  description: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  config: string;
  result?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface CreateRunRequest {
  name: string;
  description: string;
  config: string;
}

export interface UpdateRunRequest {
  name: string;
  description: string;
  config: string;
}

export interface UpdateStatusRequest {
  status: Run['status'];
  result?: string;
}

export interface RunListResponse {
  runs: Run[];
  total: number;
  page: number;
  limit: number;
}

export interface ApiError {
  error: string;
  details?: string;
}

// Конфигурация API
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080/api/v1';

class ApiClient {
  private client: AxiosInstance;
  private token: string | null = null;

  constructor() {
    this.client = axios.create({
      baseURL: API_BASE_URL,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Перехватчик для добавления токена
    this.client.interceptors.request.use((config) => {
      if (this.token) {
        config.headers.Authorization = `Bearer ${this.token}`;
      }
      return config;
    });

    // Перехватчик для обработки ошибок
    this.client.interceptors.response.use(
      (response) => response,
      (error: AxiosError<ApiError>) => {
        if (error.response?.status === 401) {
          // Токен истек или недействителен
          this.clearToken();
          window.location.href = '/login';
        }
        return Promise.reject(error);
      }
    );

    // Загружаем токен из localStorage при инициализации
    this.loadToken();
  }

  // Управление токеном
  setToken(token: string) {
    this.token = token;
    localStorage.setItem('auth_token', token);
  }

  clearToken() {
    this.token = null;
    localStorage.removeItem('auth_token');
  }

  private loadToken() {
    const token = localStorage.getItem('auth_token');
    if (token) {
      this.token = token;
    }
  }

  getToken(): string | null {
    return this.token;
  }

  // Методы аутентификации
  async register(data: RegisterRequest): Promise<{ message: string; user: User }> {
    const response = await this.client.post('/auth/register', data);
    return response.data;
  }

  async login(data: LoginRequest): Promise<AuthResponse> {
    const response = await this.client.post('/auth/login', data);
    const authData: AuthResponse = response.data;
    this.setToken(authData.token);
    return authData;
  }

  async logout(): Promise<void> {
    this.clearToken();
  }

  async getProfile(): Promise<User> {
    const response = await this.client.get('/profile');
    return response.data;
  }

  // Методы для работы с запусками
  async createRun(data: CreateRunRequest): Promise<Run> {
    const response = await this.client.post('/runs', data);
    return response.data;
  }

  async getRuns(
    page: number = 1, 
    limit: number = 20, 
    search?: string,
    status?: string,
    dateFrom?: string,
    dateTo?: string
  ): Promise<RunListResponse> {
    const params: any = { page, limit };
    
    if (search) params.search = search;
    if (status) params.status = status;
    if (dateFrom) params.date_from = dateFrom;
    if (dateTo) params.date_to = dateTo;

    const response = await this.client.get('/runs', { params });
    return response.data;
  }

  async getRun(id: number): Promise<Run> {
    const response = await this.client.get(`/runs/${id}`);
    return response.data;
  }

  async updateRun(id: number, data: UpdateRunRequest): Promise<Run> {
    const response = await this.client.put(`/runs/${id}`, data);
    return response.data;
  }

  async updateRunStatus(id: number, data: UpdateStatusRequest): Promise<Run> {
    const response = await this.client.put(`/runs/${id}/status`, data);
    return response.data;
  }

  async deleteRun(id: number): Promise<void> {
    await this.client.delete(`/runs/${id}`);
  }

  // Проверка состояния сервера
  async healthCheck(): Promise<{ status: string; timestamp: number }> {
    const response = await axios.get(`${API_BASE_URL.replace('/api/v1', '')}/health`);
    return response.data;
  }
}

// Создаем единственный экземпляр API клиента
export const apiClient = new ApiClient();

// Утилитарные функции для обработки ошибок
export const getErrorMessage = (error: unknown): string => {
  if (axios.isAxiosError(error)) {
    const apiError = error.response?.data as ApiError;
    return apiError?.error || error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return 'Произошла неизвестная ошибка';
};
