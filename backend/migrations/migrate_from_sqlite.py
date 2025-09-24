#!/usr/bin/env python3
"""
Скрипт для миграции данных из SQLite в PostgreSQL
Использование: python3 migrate_from_sqlite.py <sqlite_db_path> <postgres_dsn>
"""

import sys
import sqlite3
import psycopg2
from psycopg2.extras import RealDictCursor
import json
from datetime import datetime

def connect_sqlite(db_path):
    """Подключение к SQLite базе данных"""
    try:
        conn = sqlite3.connect(db_path)
        conn.row_factory = sqlite3.Row
        return conn
    except sqlite3.Error as e:
        print(f"Ошибка подключения к SQLite: {e}")
        sys.exit(1)

def connect_postgres(dsn):
    """Подключение к PostgreSQL базе данных"""
    try:
        conn = psycopg2.connect(dsn)
        return conn
    except psycopg2.Error as e:
        print(f"Ошибка подключения к PostgreSQL: {e}")
        sys.exit(1)

def migrate_users(sqlite_conn, postgres_conn):
    """Миграция пользователей"""
    print("Миграция пользователей...")
    
    # Получаем пользователей из SQLite
    sqlite_cursor = sqlite_conn.cursor()
    sqlite_cursor.execute("SELECT * FROM users")
    users = sqlite_cursor.fetchall()
    
    if not users:
        print("Пользователи не найдены в SQLite")
        return
    
    # Вставляем пользователей в PostgreSQL
    postgres_cursor = postgres_conn.cursor()
    
    for user in users:
        try:
            postgres_cursor.execute("""
                INSERT INTO users (id, username, password_hash, created_at, updated_at)
                VALUES (%s, %s, %s, %s, %s)
                ON CONFLICT (id) DO NOTHING
            """, (
                user['id'],
                user['username'],
                user['password_hash'],
                user['created_at'],
                user['updated_at']
            ))
        except Exception as e:
            print(f"Ошибка при миграции пользователя {user['username']}: {e}")
    
    postgres_conn.commit()
    print(f"Мигрировано пользователей: {len(users)}")

def migrate_runs(sqlite_conn, postgres_conn):
    """Миграция запусков"""
    print("Миграция запусков...")
    
    # Получаем запуски из SQLite
    sqlite_cursor = sqlite_conn.cursor()
    sqlite_cursor.execute("SELECT * FROM runs")
    runs = sqlite_cursor.fetchall()
    
    if not runs:
        print("Запуски не найдены в SQLite")
        return
    
    # Вставляем запуски в PostgreSQL
    postgres_cursor = postgres_conn.cursor()
    
    for run in runs:
        try:
            postgres_cursor.execute("""
                INSERT INTO runs (
                    id, name, description, status, config, result,
                    tps_max, tps_min, tps_average, tps_95p, tps_99p,
                    created_at, updated_at, started_at, completed_at
                ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                ON CONFLICT (id) DO NOTHING
            """, (
                run['id'],
                run['name'],
                run['description'],
                run['status'],
                run['config'],
                run['result'],
                run['tps_max'],
                run['tps_min'],
                run['tps_average'],
                run['tps_95p'],
                run['tps_99p'],
                run['created_at'],
                run['updated_at'],
                run['started_at'],
                run['completed_at']
            ))
        except Exception as e:
            print(f"Ошибка при миграции запуска {run['name']}: {e}")
    
    postgres_conn.commit()
    print(f"Мигрировано запусков: {len(runs)}")

def update_sequences(postgres_conn):
    """Обновление последовательностей PostgreSQL"""
    print("Обновление последовательностей...")
    
    postgres_cursor = postgres_conn.cursor()
    
    # Обновляем последовательность для users
    postgres_cursor.execute("SELECT MAX(id) FROM users")
    max_user_id = postgres_cursor.fetchone()[0]
    if max_user_id:
        postgres_cursor.execute(f"SELECT setval('users_id_seq', {max_user_id})")
    
    # Обновляем последовательность для runs
    postgres_cursor.execute("SELECT MAX(id) FROM runs")
    max_run_id = postgres_cursor.fetchone()[0]
    if max_run_id:
        postgres_cursor.execute(f"SELECT setval('runs_id_seq', {max_run_id})")
    
    postgres_conn.commit()
    print("Последовательности обновлены")

def main():
    if len(sys.argv) != 3:
        print("Использование: python3 migrate_from_sqlite.py <sqlite_db_path> <postgres_dsn>")
        print("Пример: python3 migrate_from_sqlite.py ./stroppy.db 'host=localhost port=5432 user=stroppy password=stroppy dbname=stroppy'")
        sys.exit(1)
    
    sqlite_path = sys.argv[1]
    postgres_dsn = sys.argv[2]
    
    print(f"Миграция из SQLite: {sqlite_path}")
    print(f"Миграция в PostgreSQL: {postgres_dsn}")
    
    # Подключение к базам данных
    sqlite_conn = connect_sqlite(sqlite_path)
    postgres_conn = connect_postgres(postgres_dsn)
    
    try:
        # Выполняем миграцию
        migrate_users(sqlite_conn, postgres_conn)
        migrate_runs(sqlite_conn, postgres_conn)
        update_sequences(postgres_conn)
        
        print("Миграция завершена успешно!")
        
    except Exception as e:
        print(f"Ошибка во время миграции: {e}")
        postgres_conn.rollback()
        sys.exit(1)
    
    finally:
        sqlite_conn.close()
        postgres_conn.close()

if __name__ == "__main__":
    main()
