-- Миграция 001: Создание начальной схемы базы данных
-- Дата: 2024-01-01
-- Описание: Создание таблиц users и runs для PostgreSQL

-- Создание таблицы пользователей
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Создание таблицы запусков
CREATE TABLE IF NOT EXISTS runs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    config TEXT NOT NULL,
    result TEXT,
    tps_max DOUBLE PRECISION,
    tps_min DOUBLE PRECISION,
    tps_average DOUBLE PRECISION,
    tps_95p DOUBLE PRECISION,
    tps_99p DOUBLE PRECISION,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

-- Создание индексов для оптимизации запросов
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_runs_created_at ON runs(created_at);
CREATE INDEX IF NOT EXISTS idx_runs_name ON runs(name);

-- Комментарии к таблицам
COMMENT ON TABLE users IS 'Таблица пользователей системы';
COMMENT ON TABLE runs IS 'Таблица запусков тестов';

-- Комментарии к полям
COMMENT ON COLUMN users.id IS 'Уникальный идентификатор пользователя';
COMMENT ON COLUMN users.username IS 'Имя пользователя (уникальное)';
COMMENT ON COLUMN users.password_hash IS 'Хеш пароля пользователя';
COMMENT ON COLUMN users.created_at IS 'Дата и время создания пользователя';
COMMENT ON COLUMN users.updated_at IS 'Дата и время последнего обновления пользователя';

COMMENT ON COLUMN runs.id IS 'Уникальный идентификатор запуска';
COMMENT ON COLUMN runs.name IS 'Название запуска';
COMMENT ON COLUMN runs.description IS 'Описание запуска';
COMMENT ON COLUMN runs.status IS 'Статус запуска (pending, running, completed, failed)';
COMMENT ON COLUMN runs.config IS 'Конфигурация запуска в формате JSON';
COMMENT ON COLUMN runs.result IS 'Результат выполнения запуска';
COMMENT ON COLUMN runs.tps_max IS 'Максимальное значение TPS';
COMMENT ON COLUMN runs.tps_min IS 'Минимальное значение TPS';
COMMENT ON COLUMN runs.tps_average IS 'Среднее значение TPS';
COMMENT ON COLUMN runs.tps_95p IS '95-й процентиль TPS';
COMMENT ON COLUMN runs.tps_99p IS '99-й процентиль TPS';
COMMENT ON COLUMN runs.created_at IS 'Дата и время создания запуска';
COMMENT ON COLUMN runs.updated_at IS 'Дата и время последнего обновления запуска';
COMMENT ON COLUMN runs.started_at IS 'Дата и время начала выполнения';
COMMENT ON COLUMN runs.completed_at IS 'Дата и время завершения выполнения';
