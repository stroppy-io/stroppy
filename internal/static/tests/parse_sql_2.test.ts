import { describe, it, expect } from "vitest";
import {
  parse_sql,
  parse_sql_with_groups,
  ParsedQuery,
} from "../parse_sql_2.ts";

describe("parse_sql_with_groups", () => {
  it("should parse groups of queries", () => {
    const sqlContent = `--+ group one
--= some query
Select 1;
--= some other
Select 2;
--+ group two
--+ group three
-- its empty`;
    const result = parse_sql_with_groups(sqlContent);
    expect(result).toEqual({
      "group one": [
        {
          name: "some query",
          params: [],
          sql: "Select 1;",
          type: "Other",
        },
        {
          name: "some other",
          params: [],
          sql: "Select 2;",
          type: "Other",
        },
      ],
      "group three": [],
      "group two": [],
    });
  });
});

describe("parse_sql", () => {
  it("should parse a single query with name and SQL", () => {
    const sqlContent = `--= create_table
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  name TEXT
);`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "create_table",
        sql: "CREATE TABLE users (\n  id INTEGER PRIMARY KEY,\n  name TEXT\n);",
        params: [],
        type: "CreateTable",
      },
    ]);
  });

  it("should parse multiple queries", () => {
    const sqlContent = `--= create_table
CREATE TABLE users (
  id INTEGER PRIMARY KEY
);

--= insert_data
INSERT INTO users (id, name) VALUES (1, 'Alice');

--= select_data
SELECT * FROM users;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "create_table",
        sql: "CREATE TABLE users (\n  id INTEGER PRIMARY KEY\n);",
        params: [],
        type: "CreateTable",
      },
      {
        name: "insert_data",
        sql: "INSERT INTO users (id, name) VALUES (1, 'Alice');",
        params: [],
        type: "Insert",
      },
      {
        name: "select_data",
        sql: "SELECT * FROM users;",
        params: [],
        type: "Other",
      },
    ]);
  });

  it("should skip comment lines starting with --", () => {
    const sqlContent = `--= create_table
-- This is a comment
CREATE TABLE users (
  id INTEGER PRIMARY KEY
);
-- Another comment
--= insert_data
-- Yet another comment
INSERT INTO users VALUES (1);`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "create_table",
        sql: "CREATE TABLE users (\n  id INTEGER PRIMARY KEY\n);",
        params: [],
        type: "CreateTable",
      },
      {
        name: "insert_data",
        sql: "INSERT INTO users VALUES (1);",
        params: [],
        type: "Insert",
      },
    ]);
  });

  it("should handle empty SQL content", () => {
    const result = parse_sql("");
    expect(result).toEqual([]);
  });

  it("should handle SQL content with only comments", () => {
    const sqlContent = `-- This is just a comment
-- Another comment`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([]);
  });

  it("should handle query name without SQL", () => {
    const sqlContent = `--= empty_query
--= another_query
SELECT 1;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "empty_query",
        sql: "",
        params: [],
        type: "Invalid",
      },
      {
        name: "another_query",
        sql: "SELECT 1;",
        params: [],
        type: "Other",
      },
    ]);
  });

  it("should handle multiline SQL statements", () => {
    const sqlContent = `--= complex_query
SELECT 
  u.id,
  u.name,
  COUNT(o.id) as order_count
FROM users u
LEFT JOIN orders o ON u.id = o.user_id
GROUP BY u.id, u.name
HAVING COUNT(o.id) > 5;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "complex_query",
        sql: "SELECT\n  u.id,\n  u.name,\n  COUNT(o.id) as order_count\nFROM users u\nLEFT JOIN orders o ON u.id = o.user_id\nGROUP BY u.id, u.name\nHAVING COUNT(o.id) > 5;",
        params: [],
        type: "Other",
      },
    ]);
  });

  it("should handle query name with trailing spaces", () => {
    const sqlContent = `--= query_name   
SELECT 1;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "query_name",
        sql: "SELECT 1;",
        params: [],
        type: "Other",
      },
    ]);
  });

  it("should handle empty lines in SQL", () => {
    const sqlContent = `--= create_table
CREATE TABLE users (
  id INTEGER PRIMARY KEY
);


--= insert_data
INSERT INTO users VALUES (1);`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "create_table",
        sql: "CREATE TABLE users (\n  id INTEGER PRIMARY KEY\n);",
        params: [],
        type: "CreateTable",
      },
      {
        name: "insert_data",
        sql: "INSERT INTO users VALUES (1);",
        params: [],
        type: "Insert",
      },
    ]);
  });

  it("should extract parameters from SQL queries", () => {
    const sqlContent = `--= insert_with_params
INSERT INTO users (id, name, email) VALUES (:id, :name, :email);

--= select_with_params
SELECT * FROM users WHERE id = :id AND status = :status;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "insert_with_params",
        sql: "INSERT INTO users (id, name, email) VALUES (:id, :name, :email);",
        params: ["id", "name", "email"],
        type: "Insert",
      },
      {
        name: "select_with_params",
        sql: "SELECT * FROM users WHERE id = :id AND status = :status;",
        params: ["id", "status"],
        type: "Other",
      },
    ]);
  });

  it("should handle duplicate parameters (only include once)", () => {
    const sqlContent = `--= query_with_duplicates
SELECT * FROM users WHERE id = :id OR parent_id = :id;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "query_with_duplicates",
        sql: "SELECT * FROM users WHERE id = :id OR parent_id = :id;",
        params: ["id"],
        type: "Other",
      },
    ]);
  });

  it("should handle parameters at end of line", () => {
    const sqlContent = `--= query_end_param
SELECT * FROM users WHERE id = :id`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "query_end_param",
        sql: "SELECT * FROM users WHERE id = :id",
        params: ["id"],
        type: "Other",
      },
    ]);
  });

  it("should handle parameters with underscores", () => {
    const sqlContent = `--= query_underscore
SELECT * FROM users WHERE user_id = :user_id AND account_name = :account_name;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "query_underscore",
        sql: "SELECT * FROM users WHERE user_id = :user_id AND account_name = :account_name;",
        params: ["user_id", "account_name"],
        type: "Other",
      },
    ]);
  });

  it("should detect invalid SQL", () => {
    const sqlContent = `--= invalid_query
SELECT * FROM WHERE id = 1;`;

    const result = parse_sql(sqlContent);
    expect(result).toEqual([
      {
        name: "invalid_query",
        sql: "SELECT * FROM WHERE id = 1;",
        params: [],
        type: "Invalid",
      },
    ]);
  });

  it("should accept database option", () => {
    const sqlContent = `--= create_table
CREATE TABLE users (id INTEGER PRIMARY KEY);`;

    const result = parse_sql(sqlContent, { database: "PostgreSQL" });
    expect(result).toEqual([
      {
        name: "create_table",
        sql: "CREATE TABLE users (id INTEGER PRIMARY KEY);",
        params: [],
        type: "CreateTable",
      },
    ]);
  });
});
