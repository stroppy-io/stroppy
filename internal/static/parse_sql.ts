import { Parser } from "node-sql-parser";

type QueryType = "CreateTable" | "Insert" | "Select" | "Other" | "Invalid";

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

type Sections = {
  (sectionName: string, queryName: string): ParsedQuery | undefined
  (sectionName: string): ParsedQuery[]
  (): Record<string, ParsedQuery[]>
};

type Queries = {
  (queryName: string): ParsedQuery | undefined
  (): ParsedQuery[]
}

const SectionNamePrefix = "--+";

export function parse_sql_with_sections(
  sqlContent: string,
  options: ParseOptions = {},
): Sections {
  const lines = sqlContent.split("\n");
  const sections: Record<string, ParsedQuery[]> = {};
  let section: string[] = [];
  let name: string = "";

  const f: Sections = function(sectionName?: string, queryName?: string): any {
    if (!sectionName) {
      return sections
    }
    if (!queryName) {
      return sections[sectionName]
    }
    return sections[sectionName]?.find((v) => v.name == queryName)
  }

  for (let line of lines) {
    let trimmed = line.trim();
    if (trimmed.startsWith(SectionNamePrefix)) {
      let parsed_section = parse_sql_internal(section, options);
      if (name == "" && parsed_section.length === 0) {
        name = trimmed.replace(SectionNamePrefix, "").trimStart();
        continue;
      }
      sections[name] = parsed_section;
      name = trimmed.replace(SectionNamePrefix, "").trimStart();
      section = [];
    } else {
      section.push(line);
    }
  }
  let parsed_section = parse_sql_internal(section, options);
  if (name == "" && parsed_section.length === 0) {
    return f;
  }
  sections[name] = parsed_section;
  return f;
}

const QueryNamePrefix = "--=";
const CommentLinePrefix = "--";

function parse_sql_internal(
  sqlContent: string | string[],
  options: ParseOptions = {},
): ParsedQuery[] {
  const lines = Array.isArray(sqlContent) ? sqlContent : sqlContent.split("\n");
  const parser = new Parser();

  let queries: ParsedQuery[] = [];

  let name: string | null = null;
  let sql: string[] = [];

  for (let line of lines) {
    line = line.trimEnd();
    let trimmed = line.trim();
    if (trimmed.startsWith(QueryNamePrefix)) {
      addQuery(queries, name, sql.join("\n").trim(), parser, options);
      name = line.replace(QueryNamePrefix, "").trimStart();
      sql.length = 0;
    } else if (trimmed.startsWith(CommentLinePrefix)) {
      continue; // skip comments
    } else {
      sql.push(line);
    }
  }
  // Add the last query if there is one
  addQuery(queries, name, sql.join("\n").trim(), parser, options);
  return queries;
}

export function parse_sql(sqlContent: string, options: ParseOptions = {}): Queries {
  const queries = parse_sql_internal(sqlContent, options)

  const f: Queries = function(queryName?: string): any {
    if (!queryName) {
      return queries
    }
    return queries.find((v) => v.name == queryName)
  }

  return f;
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

    const astType = astNode.type;

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

    if (astType === "select") {
      return "Select";
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
