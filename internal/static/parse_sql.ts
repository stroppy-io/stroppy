import { WorkloadDescriptor } from "./stroppy.pb.js";

// Parsed structure for intermediate representation
interface ParsedQuery {
  name: string;
  sql: string;
}

interface ParsedTransaction {
  name: string;
  queries: ParsedQuery[];
}

export interface ParsedWorkload {
  name: string;
  queries: ParsedQuery[];
  transactions: ParsedTransaction[];
}

/**
 * Parses SQL file content and extracts workloads with queries and transactions
 */
export function parse_sql(sqlContent: string): ParsedWorkload[] {
  const lines = sqlContent.split("\n");
  const workloads: ParsedWorkload[] = [];

  let currentWorkload: ParsedWorkload | null = null;
  let currentTransaction: ParsedTransaction | null = null;
  let currentQuery: ParsedQuery | null = null;
  let sqlBuffer: string[] = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();

    // Workload start
    if (trimmed.startsWith("-- workload ") && !trimmed.endsWith(" end")) {
      const workloadName = trimmed.substring("-- workload ".length).trim();
      if (!workloadName) {
        throw new Error(`Line ${i + 1}: Workload must have a name`);
      }
      currentWorkload = {
        name: workloadName,
        queries: [],
        transactions: [],
      };
      workloads.push(currentWorkload);
      continue;
    }

    // Workload end
    if (trimmed === "-- workload end") {
      if (!currentWorkload) {
        throw new Error(`Line ${i + 1}: Workload end without workload start`);
      }
      currentWorkload = null;
      continue;
    }

    if (!currentWorkload) {
      // Skip lines outside workloads (comments, empty lines)
      continue;
    }

    // Transaction start
    if (trimmed.startsWith("-- transaction ") && !trimmed.endsWith(" end")) {
      const txName = trimmed.substring("-- transaction ".length).trim();
      if (!txName) {
        throw new Error(`Line ${i + 1}: Transaction must have a name`);
      }
      if (currentTransaction) {
        throw new Error(`Line ${i + 1}: Nested transactions not supported`);
      }
      currentTransaction = {
        name: txName,
        queries: [],
      };
      continue;
    }

    // Transaction end
    if (trimmed === "-- transaction end") {
      if (!currentTransaction) {
        throw new Error(
          `Line ${i + 1}: Transaction end without transaction start`,
        );
      }
      if (currentQuery) {
        throw new Error(`Line ${i + 1}: Unclosed query in transaction`);
      }
      currentWorkload.transactions.push(currentTransaction);
      currentTransaction = null;
      continue;
    }

    // Query start
    if (trimmed.startsWith("-- query ") && !trimmed.endsWith(" end")) {
      const queryName = trimmed.substring("-- query ".length).trim();
      if (!queryName) {
        throw new Error(`Line ${i + 1}: Query must have a name`);
      }
      if (currentQuery) {
        throw new Error(`Line ${i + 1}: Nested queries not supported`);
      }
      currentQuery = {
        name: queryName,
        sql: "",
      };
      sqlBuffer = [];
      continue;
    }

    // Query end
    if (trimmed === "-- query end") {
      if (!currentQuery) {
        throw new Error(`Line ${i + 1}: Query end without query start`);
      }

      // Trim and join SQL lines
      currentQuery.sql = sqlBuffer
        .map((l) => l.trimEnd())
        .join("\n")
        .trim();

      if (!currentQuery.sql) {
        throw new Error(
          `Line ${i + 1}: Query '${currentQuery.name}' has no SQL content`,
        );
      }

      // Add query to transaction or workload
      if (currentTransaction) {
        currentTransaction.queries.push(currentQuery);
      } else {
        currentWorkload.queries.push(currentQuery);
      }

      currentQuery = null;
      sqlBuffer = [];
      continue;
    }

    // Collect SQL content
    if (currentQuery) {
      sqlBuffer.push(line);
    }
  }

  // Validation: check for unclosed blocks
  if (currentWorkload) {
    throw new Error(`Unclosed workload: ${currentWorkload.name}`);
  }
  if (currentTransaction) {
    throw new Error(`Unclosed transaction: ${currentTransaction.name}`);
  }
  if (currentQuery) {
    throw new Error(`Unclosed query: ${currentQuery.name}`);
  }

  return workloads;
}

/**
 * Updates WorkloadDescriptors with SQL from parsed workloads (modifies in place)
 */
export function update_with_sql(
  workloads: WorkloadDescriptor[],
  parsedWorkloads: ParsedWorkload[],
): void {
  // Create a map for quick lookup
  const parsedMap = new Map<string, ParsedWorkload>();
  for (const parsed of parsedWorkloads) {
    parsedMap.set(parsed.name, parsed);
  }

  for (const workload of workloads) {
    const parsed = parsedMap.get(workload.name);
    if (!parsed) {
      console.warn(
        `Workload '${workload.name}' not found in SQL file, skipping`,
      );
      continue;
    }

    // Create maps for parsed queries and transactions
    const parsedQueriesMap = new Map<string, ParsedQuery>();
    for (const query of parsed.queries) {
      parsedQueriesMap.set(query.name, query);
    }

    const parsedTransactionsMap = new Map<string, ParsedTransaction>();
    for (const tx of parsed.transactions) {
      parsedTransactionsMap.set(tx.name, tx);
    }

    // Update units
    for (const unit of workload.units) {
      if (!unit.descriptor) {
        continue;
      }

      const descriptor = unit.descriptor;
      if (descriptor.type.oneofKind === "query") {
        const queryDescriptor = descriptor.type.query;
        const parsedQuery = parsedQueriesMap.get(queryDescriptor.name);

        if (!parsedQuery) {
          console.warn(
            `Query '${queryDescriptor.name}' in workload '${workload.name}' not found in SQL file, skipping`,
          );
          continue;
        }

        queryDescriptor.sql = parsedQuery.sql;
      } else if (descriptor.type.oneofKind === "transaction") {
        const txDescriptor = descriptor.type.transaction;
        const parsedTx = parsedTransactionsMap.get(txDescriptor.name);

        if (!parsedTx) {
          console.warn(
            `Transaction '${txDescriptor.name}' in workload '${workload.name}' not found in SQL file, skipping`,
          );
          continue;
        }

        // Create map for parsed transaction queries
        const parsedTxQueriesMap = new Map<string, ParsedQuery>();
        for (const query of parsedTx.queries) {
          parsedTxQueriesMap.set(query.name, query);
        }

        // Update queries within transaction
        for (const query of txDescriptor.queries) {
          const parsedQuery = parsedTxQueriesMap.get(query.name);

          if (!parsedQuery) {
            console.warn(
              `Query '${query.name}' in transaction '${txDescriptor.name}' of workload '${workload.name}' not found in SQL file, skipping`,
            );
            continue;
          }

          query.sql = parsedQuery.sql;
        }
      }
    }
  }
}
