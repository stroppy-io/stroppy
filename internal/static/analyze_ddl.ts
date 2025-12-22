import { Parser } from "node-sql-parser";
import {
    WorkloadDescriptor,
    Generation_Rule,
} from "./stroppy.pb.js";

export function params_by_ddl(
    workloads: WorkloadDescriptor[],
    workloadName: string,
    tableName: string,
): any[] {
    const workload = workloads.find((w) => w.name === workloadName);
    if (!workload) {
        throw new Error(`Workload '${workloadName}' not found`);
    }

    let ddlSql = "";

    for (const unit of workload.units) {
        if (unit.descriptor?.type.oneofKind === 'query') {
            const query = unit.descriptor.type.query;
            // Basic check if this query creates the table
            if (query.sql.toLowerCase().includes(`create table`) && query.sql.includes(tableName)) {
                ddlSql = query.sql;
                break;
            }
        }
    }

    if (!ddlSql) {
        console.warn(`CREATE TABLE statement for '${tableName}' not found in workload '${workloadName}'`);
        return [];
    }

    const parser = new Parser();
    let ast;
    try {
        ast = parser.astify(ddlSql, { database: "postgresql" });
    } catch (e) {
        console.error("Failed to parse SQL:", e);
        return [];
    }

    const statements = Array.isArray(ast) ? ast : [ast];

    const createTableStmt = statements.find((stmt: any) =>
        stmt.type === 'create' && stmt.keyword === 'table' &&
        (stmt.table && stmt.table.some((t: any) => t.table === tableName))
    );

    if (!createTableStmt) {
        console.warn(`Parsed SQL does not contain CREATE TABLE for '${tableName}'`);
        return [];
    }

    const columns = (createTableStmt as any).create_definitions;
    const params: any[] = [];

    for (const col of columns) {
        if (col.resource === 'column') {
            let colName = col.column;
            // Handle node-sql-parser AST variations
            if (typeof colName === 'object' && colName !== null) {
                if ('column' in colName) {
                    colName = colName.column;
                }
            }
            if (typeof colName === 'object' && colName !== null) {
                if ('expr' in colName) {
                    colName = colName.expr;
                }
            }
            if (typeof colName === 'object' && colName !== null) {
                if ('value' in colName) {
                    colName = colName.value;
                }
            }
            const definition = col.definition;
            const dataType = definition.dataType.toUpperCase();
            const rule = mapTypeToGenerator(dataType, definition);

            if (rule) {
                params.push({
                    name: `${tableName}.${colName}`,
                    generationRule: rule
                });
            }
        }
    }

    return params;
}

function mapTypeToGenerator(dataType: string, definition: any): Generation_Rule | null {
    // Check for primary key or unique constraint in definition
    // node-sql-parser structure for constraints varies, checking common paths
    let isUnique = false;
    if (definition.constraint) {
        // definition.constraint might be an object or null
        // e.g. { type: 'primary key', ... }
        // or it might be in column definition flags
    }

    // Simple heuristic for now: if it says PRIMARY KEY in suffix or constraint
    // We might need to inspect the AST more closely for robust constraint handling
    // For tpcb_mini.sql: "id INTEGER NOT NULL PRIMARY KEY"

    // In node-sql-parser, inline constraints are often in definition
    // Let's assume unique if it's a key

    // Helper to create rule
    const createRule = (kind: any, unique: boolean = false) => ({
        unique,
        kind
    });

    if (dataType === 'INTEGER' || dataType === 'INT' || dataType === 'INT4') {
        return createRule({
            oneofKind: "int32Range",
            int32Range: { min: 1, max: 2147483647 }
        }, isUnique);
    }

    if (dataType === 'BIGINT' || dataType === 'INT8') {
        return createRule({
            oneofKind: "int64Range",
            int64Range: { min: "1", max: "9223372036854775807" }
        }, isUnique);
    }

    if (dataType === 'SERIAL') {
        return createRule({
            oneofKind: "int32Range",
            int32Range: { min: 1, max: 2147483647 }
        }, true);
    }

    if (dataType === 'VARCHAR' || dataType === 'TEXT' || dataType === 'CHAR') {
        const maxLen = definition.length ? parseInt(definition.length) : 100;
        return createRule({
            oneofKind: "stringRange",
            stringRange: {
                maxLen: maxLen.toString(),
                minLen: "1",
                alphabet: { ranges: [{ min: 97, max: 122 }] } // a-z
            }
        }, isUnique);
    }

    if (dataType === 'TIMESTAMP' || dataType === 'DATE') {
        return createRule({
            oneofKind: "datetimeRange",
            datetimeRange: {
                type: {
                    oneofKind: "timestamp",
                    timestamp: { min: 0, max: 2147483647 }
                }
            }
        }, isUnique);
    }

    return null;
}
