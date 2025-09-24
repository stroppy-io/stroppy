-- Миграция 003: Добавление метаданных для запусков
-- Дата: 2024-01-03
-- Описание: Добавление дополнительных полей для метаданных запусков

-- Добавляем колонки для метаданных
ALTER TABLE runs ADD COLUMN IF NOT EXISTS tags TEXT;
ALTER TABLE runs ADD COLUMN IF NOT EXISTS environment VARCHAR(100);
ALTER TABLE runs ADD COLUMN IF NOT EXISTS created_by INTEGER;

-- Создаем внешний ключ для created_by (если нужно)
-- ALTER TABLE runs ADD CONSTRAINT fk_runs_created_by FOREIGN KEY (created_by) REFERENCES users(id);

-- Создаем индексы для новых полей
CREATE INDEX IF NOT EXISTS idx_runs_environment ON runs(environment);
CREATE INDEX IF NOT EXISTS idx_runs_created_by ON runs(created_by);

-- Добавляем комментарии
COMMENT ON COLUMN runs.tags IS 'Теги для категоризации запусков (JSON массив)';
COMMENT ON COLUMN runs.environment IS 'Окружение выполнения (dev, staging, prod)';
COMMENT ON COLUMN runs.created_by IS 'ID пользователя, создавшего запуск';
