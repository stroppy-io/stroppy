-- Миграция 002: Добавление ролей пользователей
-- Дата: 2024-01-02
-- Описание: Добавление системы ролей для пользователей

-- Добавляем колонку role в таблицу users
ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(50) DEFAULT 'user';

-- Создаем индекс для ролей
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);

-- Обновляем существующих пользователей (если есть)
UPDATE users SET role = 'user' WHERE role IS NULL;

-- Добавляем комментарий к колонке
COMMENT ON COLUMN users.role IS 'Роль пользователя (admin, user, guest)';
