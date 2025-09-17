# Stroppy Cloud Panel

Современная панель управления облачными ресурсами для Stroppy.

## Структура проекта

```
stroppy-cloud-panel/
├── frontend/              # React frontend приложение
│   ├── src/              # Исходный код
│   ├── public/           # Статические файлы
│   ├── package.json      # Зависимости frontend
│   └── README.md         # Документация frontend
└── README.md             # Этот файл
```

## Frontend

Frontend приложение построено на современном стеке:

- **React 19** - библиотека для создания пользовательских интерфейсов
- **TypeScript** - типизированный JavaScript
- **Vite** - быстрый инструмент сборки и dev-сервер
- **Tailwind CSS** - utility-first CSS фреймворк
- **Ant Design 5.0** - библиотека React компонентов
- **Yarn** - менеджер пакетов

### Быстрый старт

1. Перейдите в папку frontend:
   ```bash
   cd frontend
   ```

2. Установите зависимости:
   ```bash
   yarn install
   ```

3. Запустите dev-сервер:
   ```bash
   yarn dev
   ```

4. Откройте браузер по адресу: http://localhost:5173

### Доступные команды

- `yarn dev` - запуск в режиме разработки
- `yarn build` - сборка для продакшена
- `yarn preview` - предварительный просмотр сборки
- `yarn type-check` - проверка типов TypeScript
- `yarn lint` - линтинг кода
- `yarn clean` - очистка папки сборки

## Разработка

Проект использует современные инструменты разработки:

- **Hot Module Replacement (HMR)** - мгновенная перезагрузка при изменениях
- **TypeScript** - полная типизация для надежности
- **ESLint** - проверка качества кода
- **PostCSS** - обработка CSS с автопрефиксером
- **Vite** - быстрая сборка и оптимизация

## Лицензия

© 2024 Stroppy Cloud Panel. Все права защищены.
