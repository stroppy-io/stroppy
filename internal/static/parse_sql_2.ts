import { Parser } from "node-sql-parser";

type QueryType = "CreateTable" | "Insert" | "Other" | "Invalid";

// Options for SQL parsing
export interface ParseOptions {
  database?:
    | "MySQL"
    | "PostgreSQL"
    | "SQLite"
    | "TransactSQL"
    | "MariaDB"
    | "Hive"
    | "DB2"
    | "FlinkSQL"
    | "Redshift"
    | "Athena"
    | "BigQuery"
    | "Snowflake"
    | "Noql";
}

// Parsed structure for intermediate representation
export interface ParsedQuery {
  name: string;
  sql: string;
  type: QueryType;
  params: string[];
}

export function parse_sql(
  sqlContent: string,
  options: ParseOptions = {},
): ParsedQuery[] {
  const lines = sqlContent.split("\n");
  const parser = new Parser();

  let queries: ParsedQuery[] = [];

  let name: string | null = null;
  let sql: string | null = null;

  for (let line of lines) {
    if (line.startsWith("--= ")) {
      addQuery(queries, name, sql, parser, options);
      name = line.replace("--= ", "");
      sql = null;
    } else if (line.startsWith("--")) {
      continue; // skip comments
    } else {
      sql = (sql || "") + "\n" + line;
    }
  }
  // Add the last query if there is one
  addQuery(queries, name, sql, parser, options);
  return queries;
}

// Extract parameter names from SQL (format: :param where param ends with space or non-word character)
function extractParams(sql: string): string[] {
  const paramRegex: RegExp = /:([a-zA-Z_][a-zA-Z0-9_]*)(?=\W|$)/g;
  const params: string[] = [];
  let match: RegExpExecArray | null = null;

  while ((match = paramRegex.exec(sql)) !== null) {
    const paramName = match[1];
    if (!params.includes(paramName)) {
      params.push(paramName);
    }
  }

  return params;
}

// Determine query type from SQL using node-sql-parser
function determineQueryType(
  sql: string,
  parser: Parser,
  options: ParseOptions,
): QueryType {
  try {
    // Trim SQL and remove leading newlines for parsing
    const trimmedSql = sql.trim();
    if (!trimmedSql) {
      return "Invalid";
    }

    const parseOptions = options.database ? { database: options.database } : {};
    const ast = parser.astify(trimmedSql, parseOptions);

    // Handle array of statements (take first one)
    const astNode = Array.isArray(ast) ? ast[0] : ast;

    if (!astNode || typeof astNode !== "object" || !("type" in astNode)) {
      return "Invalid";
    }

    const astType = astNode.type as string;

    // Map node-sql-parser types to our QueryType
    if (astType === "create" && "keyword" in astNode) {
      const keyword = (astNode as { keyword?: string }).keyword;
      if (keyword === "table") {
        return "CreateTable";
      }
    }

    if (astType === "insert") {
      return "Insert";
    }

    return "Other";
  } catch (error) {
    // If parsing fails, the SQL is invalid
    return "Invalid";
  }
}

// Add a query to the queries array (single place for adding queries)
function addQuery(
  queries: ParsedQuery[],
  name: string | null,
  sql: string | null,
  parser: Parser,
  options: ParseOptions,
): void {
  if (name !== null && sql !== null) {
    const params = extractParams(sql);
    const type = determineQueryType(sql, parser, options);
    queries.push({ name: name, sql: sql, params: params, type: type });
  }
}
