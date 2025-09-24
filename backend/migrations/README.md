# Миграция с SQLite на PostgreSQL

Этот каталог содержит скрипты для миграции базы данных с SQLite на PostgreSQL.

## Файлы

- `001_initial_schema.sql` - SQL скрипт для создания схемы PostgreSQL
- `migrate_from_sqlite.py` - Python скрипт для миграции данных из SQLite в PostgreSQL
- `README.md` - Этот файл с инструкциями

## Подготовка к миграции

### 1. Установка зависимостей

Для работы скрипта миграции необходимо установить Python библиотеки:

```bash
pip install psycopg2-binary
```

### 2. Создание базы данных PostgreSQL

Создайте базу данных PostgreSQL:

```sql
CREATE DATABASE stroppy;
CREATE USER stroppy WITH PASSWORD 'stroppy';
GRANT ALL PRIVILEGES ON DATABASE stroppy TO stroppy;
```

## Выполнение миграции

### 1. Создание схемы

Сначала создайте схему в PostgreSQL:

```bash
psql -h localhost -U stroppy -d stroppy -f 001_initial_schema.sql
```

### 2. Миграция данных

Запустите скрипт миграции данных:

```bash
python3 migrate_from_sqlite.py ./stroppy.db "host=localhost port=5432 user=stroppy password=stroppy dbname=stroppy"
```

## Проверка миграции

После миграции проверьте данные:

```sql
-- Подключитесь к PostgreSQL
psql -h localhost -U stroppy -d stroppy

-- Проверьте количество записей
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM runs;

-- Проверьте несколько записей
SELECT * FROM users LIMIT 5;
SELECT * FROM runs LIMIT 5;
```

## Docker миграция

Если вы используете Docker, выполните миграцию следующим образом:

### 1. Запустите PostgreSQL контейнер

```bash
docker-compose up -d postgres
```

### 2. Скопируйте SQLite файл в контейнер (если нужно)

```bash
docker cp ./stroppy.db stroppy-postgres:/tmp/stroppy.db
```

### 3. Выполните миграцию

```bash
# Создание схемы
docker exec -i stroppy-postgres psql -U stroppy -d stroppy < migrations/001_initial_schema.sql

# Миграция данных (если есть SQLite файл)
docker exec -i stroppy-postgres python3 /path/to/migrate_from_sqlite.py /tmp/stroppy.db "host=localhost port=5432 user=stroppy password=stroppy dbname=stroppy"
```

## Откат миграции

Если нужно вернуться к SQLite:

1. Остановите PostgreSQL контейнер
2. Удалите PostgreSQL volume
3. Обновите переменные окружения обратно на SQLite
4. Перезапустите приложение

```bash
docker-compose down
docker volume rm stroppy-cloud-panel_postgres_data
```

## Примечания

- Скрипт миграции использует `ON CONFLICT DO NOTHING` для избежания дублирования данных
- Последовательности PostgreSQL обновляются автоматически после миграции
- Все временные метки сохраняются как есть
- JSON поля (config) мигрируются без изменений

## Устранение проблем

### Ошибка подключения к PostgreSQL

Убедитесь, что:
- PostgreSQL запущен
- Пользователь и база данных созданы
- Пароль указан правильно
- Порт 5432 доступен

### Ошибка подключения к SQLite

Убедитесь, что:
- Файл SQLite существует
- У вас есть права на чтение файла
- Путь к файлу указан правильно

### Ошибки при миграции данных

- Проверьте логи скрипта миграции
- Убедитесь, что схема PostgreSQL создана
- Проверьте совместимость типов данных
