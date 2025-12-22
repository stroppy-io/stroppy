import { describe, it, expect } from 'vitest';
import { WorkloadDescriptor, TxIsolationLevel } from '../stroppy.pb.js';
import { apply_generators_ranges } from '../apply_generators.ts';

describe('apply_generators_ranges', () => {
  it('should process transaction queries with generator syntax', () => {
    // Reproduce the exact workload structure from execute_sql.ts
    const workloads: WorkloadDescriptor[] = [
      WorkloadDescriptor.create({
        name: "create_schema",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_accounts_table",
                  sql: "CREATE TABLE accounts ( id INTEGER NOT NULL PRIMARY KEY, balance INTEGER );",
                  params: [],
                  groups: [],
                },
              },
            },
          },
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "create_history_table",
                  sql: "CREATE TABLE history ( account_id INTEGER, amount INTEGER, created_at TIMESTAMP );",
                  params: [],
                  groups: [],
                },
              },
            },
          },
        ],
      }),
      WorkloadDescriptor.create({
        name: "insert",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "insert_accounts",
                  sql: "INSERT INTO accounts (id, balance)\nVALUES (${accounts.id}, ${accounts.balance});",
                  groups: [],
                  params: [],
                },
              },
            },
          },
        ],
      }),
      WorkloadDescriptor.create({
        name: "workload",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "transaction",
                transaction: {
                  name: "update_and_log",
                  isolationLevel: TxIsolationLevel.UNSPECIFIED,
                  queries: [
                    {
                      name: "update_balance",
                      sql: "UPDATE accounts\nSET balance = balance + ${amount}\nWHERE id = ${accounts.id{1:100}};",
                      params: [],
                      groups: [],
                    },
                    {
                      name: "insert_history",
                      sql: "INSERT INTO history (account_id, amount, created_at)\nVALUES (${accounts.id!{1:50}}, ${amount}, CURRENT_TIMESTAMP);",
                      params: [],
                      groups: [],
                    },
                  ],
                  groups: [],
                  params: [
                    {
                      name: "amount",
                      generationRule: {
                        unique: false,
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: { min: -1000, max: 1000 },
                        },
                      },
                    },
                    {
                      name: "accounts.id",
                      generationRule: {
                        unique: false,
                        kind: {
                          oneofKind: "int32Range",
                          int32Range: { min: 1, max: 2147483647 },
                        },
                      },
                    },
                  ],
                },
              },
            },
          },
        ],
      }),
      WorkloadDescriptor.create({
        name: "cleanup",
        units: [
          {
            count: "1",
            descriptor: {
              type: {
                oneofKind: "query",
                query: {
                  name: "drop_tables",
                  sql: "DROP TABLE IF EXISTS accounts, history CASCADE;",
                  params: [],
                  groups: [],
                },
              },
            },
          },
        ],
      }),
    ];

    const txBefore = workloads[2].units[0].descriptor?.type.oneofKind === 'transaction' 
      ? workloads[2].units[0].descriptor.type.transaction 
      : undefined;
    expect(txBefore).toBeDefined();
    expect(txBefore?.name).toBe("update_and_log");
    expect(txBefore?.params).toHaveLength(2);

    // Apply generator ranges
    apply_generators_ranges(workloads);

    const txAfter = workloads[2].units[0].descriptor?.type.oneofKind === 'transaction'
      ? workloads[2].units[0].descriptor.type.transaction
      : undefined;
    expect(txAfter).toBeDefined();

    // Verify params
    const paramNames = txAfter?.params.map((p: any) => p.name) || [];
    expect(paramNames).toContain("amount");
    expect(paramNames).toContain("accounts.id");
    expect(paramNames).toContain("accounts.id{1:100}");
    expect(paramNames).toContain("accounts.id!{1:50}");

    // Verify SQL replacement
    const query1Sql = txAfter?.queries[0].sql || "";
    const query2Sql = txAfter?.queries[1].sql || "";

    expect(query1Sql).toContain("${accounts.id{1:100}}");
    expect(query2Sql).toContain("${accounts.id!{1:50}}");

    // Verify SQL doesn't contain the original syntax pattern (should be replaced)
    const sql1Matches = (query1Sql.match(/\$\{accounts\.id\{1:100\}\}/g) || []).length;
    const sql2Matches = (query2Sql.match(/\$\{accounts\.id!\{1:50\}\}/g) || []).length;
    expect(sql1Matches).toBe(1);
    expect(sql2Matches).toBe(1);

    // Verify param ranges
    const param1 = txAfter?.params.find((p: any) => p.name === "accounts.id{1:100}");
    expect(param1).toBeDefined();
    const range1 = param1?.generationRule?.kind.int32Range;
    expect(range1?.min).toBe(1);
    expect(range1?.max).toBe(100);
    expect(param1?.generationRule?.unique).toBe(false);

    const param2 = txAfter?.params.find((p: any) => p.name === "accounts.id!{1:50}");
    expect(param2).toBeDefined();
    const range2 = param2?.generationRule?.kind.int32Range;
    expect(range2?.min).toBe(1);
    expect(range2?.max).toBe(50);
    expect(param2?.generationRule?.unique).toBe(true);
  });
});

