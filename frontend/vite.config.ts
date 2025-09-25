import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    // Обеспечиваем, что переменные окружения доступны во время сборки
    'import.meta.env.VITE_API_BASE_URL': JSON.stringify(process.env.VITE_API_BASE_URL || '/api/v1'),
  },
  build: {
    // Настройки для продакшена
    outDir: 'dist',
    assetsDir: 'assets',
    sourcemap: false,
    minify: 'terser',
  },
})
