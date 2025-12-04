import {
    WorkloadDescriptor,
    Generation_Rule,
    QueryParamDescriptor,
} from "./stroppy.pb.js";

interface GeneratorMatch {
    name: string;
    min?: number;
    max?: number;
    unique: boolean;
    fullMatch: string;
}

/**
 * Extracts generator syntax from SQL string
 * Matches patterns like ${generatorname{A:B}} or ${generatorname!{A:B}}
 */
function extractGeneratorSyntax(sql: string): GeneratorMatch[] {
    const matches: GeneratorMatch[] = [];
    // Pattern: ${generatorname{A:B}} or ${generatorname!{A:B}}
    // A and B are optional numbers
    // Match: ${name!{range}} or ${name{range}}
    // Use non-greedy match for range to stop at first }
    // Make sure we don't match across parameter boundaries by being more specific
    const pattern = /\$\{([^!{\s]+)(!?)\{([^}]*?)\}\}/g;
    
    let match;
    while ((match = pattern.exec(sql)) !== null) {
        const generatorName = match[1].trim();
        const uniqueFlag = match[2] === "!";
        const rangeStr = match[3].trim();
        
        let min: number | undefined;
        let max: number | undefined;
        
        if (rangeStr) {
            const parts = rangeStr.split(":");
            if (parts.length === 2) {
                const minStr = parts[0].trim();
                const maxStr = parts[1].trim();
                if (minStr) {
                    min = parseFloat(minStr);
                    if (isNaN(min)) {
                        console.warn(`Invalid min value in range: ${minStr}`);
                        continue;
                    }
                }
                if (maxStr) {
                    max = parseFloat(maxStr);
                    if (isNaN(max)) {
                        console.warn(`Invalid max value in range: ${maxStr}`);
                        continue;
                    }
                }
            } else if (parts.length === 1 && parts[0].trim()) {
                // Single value, treat as max
                max = parseFloat(parts[0].trim());
                if (isNaN(max)) {
                    console.warn(`Invalid max value in range: ${parts[0]}`);
                    continue;
                }
            }
        }
        
        matches.push({
            name: generatorName,
            min,
            max,
            unique: uniqueFlag,
            fullMatch: match[0],
        });
    }
    
    return matches;
}

/**
 * Checks if a Generation_Rule has a range type
 */
function hasRangeType(rule: Generation_Rule): boolean {
    if (!rule || !rule.kind) {
        return false;
    }
    
    const kind = rule.kind.oneofKind;
    return kind === "int32Range" ||
           kind === "int64Range" ||
           kind === "uint32Range" ||
           kind === "uint64Range" ||
           kind === "floatRange" ||
           kind === "doubleRange" ||
           kind === "decimalRange" ||
           kind === "stringRange" ||
           kind === "datetimeRange";
}

/**
 * Deep copies a Generation_Rule and updates its range
 */
function updateRange(
    rule: Generation_Rule,
    min?: number,
    max?: number
): Generation_Rule {
    // Deep copy the rule
    const newRule: Generation_Rule = JSON.parse(JSON.stringify(rule));
    
    if (!newRule.kind) {
        return newRule;
    }
    
    const kind = newRule.kind.oneofKind;
    
    switch (kind) {
        case "int32Range": {
            if (newRule.kind.int32Range) {
                if (min !== undefined) {
                    newRule.kind.int32Range.min = Math.round(min);
                }
                if (max !== undefined) {
                    newRule.kind.int32Range.max = Math.round(max);
                }
            }
            break;
        }
        case "int64Range": {
            if (newRule.kind.int64Range) {
                if (min !== undefined) {
                    newRule.kind.int64Range.min = Math.round(min).toString();
                }
                if (max !== undefined) {
                    newRule.kind.int64Range.max = Math.round(max).toString();
                }
            }
            break;
        }
        case "uint32Range": {
            if (newRule.kind.uint32Range) {
                if (min !== undefined) {
                    newRule.kind.uint32Range.min = Math.max(0, Math.round(min));
                }
                if (max !== undefined) {
                    newRule.kind.uint32Range.max = Math.max(0, Math.round(max));
                }
            }
            break;
        }
        case "uint64Range": {
            if (newRule.kind.uint64Range) {
                if (min !== undefined) {
                    newRule.kind.uint64Range.min = Math.max(0, Math.round(min)).toString();
                }
                if (max !== undefined) {
                    newRule.kind.uint64Range.max = Math.max(0, Math.round(max)).toString();
                }
            }
            break;
        }
        case "floatRange": {
            if (newRule.kind.floatRange) {
                if (min !== undefined) {
                    newRule.kind.floatRange.min = min;
                }
                if (max !== undefined) {
                    newRule.kind.floatRange.max = max;
                }
            }
            break;
        }
        case "doubleRange": {
            if (newRule.kind.doubleRange) {
                if (min !== undefined) {
                    newRule.kind.doubleRange.min = min;
                }
                if (max !== undefined) {
                    newRule.kind.doubleRange.max = max;
                }
            }
            break;
        }
        case "stringRange": {
            if (newRule.kind.stringRange) {
                if (min !== undefined) {
                    newRule.kind.stringRange.minLen = Math.max(0, Math.round(min)).toString();
                }
                if (max !== undefined) {
                    newRule.kind.stringRange.maxLen = Math.max(0, Math.round(max)).toString();
                }
            }
            break;
        }
        case "decimalRange": {
            // decimalRange has nested types, handle the common cases
            if (newRule.kind.decimalRange && newRule.kind.decimalRange.type) {
                const decimalType = newRule.kind.decimalRange.type;
                if (decimalType.oneofKind === "float") {
                    const floatRange = (decimalType as { oneofKind: "float"; float: any }).float;
                    if (floatRange) {
                        if (min !== undefined) {
                            floatRange.min = min;
                        }
                        if (max !== undefined) {
                            floatRange.max = max;
                        }
                    }
                } else if (decimalType.oneofKind === "double") {
                    const doubleRange = (decimalType as { oneofKind: "double"; double: any }).double;
                    if (doubleRange) {
                        if (min !== undefined) {
                            doubleRange.min = min;
                        }
                        if (max !== undefined) {
                            doubleRange.max = max;
                        }
                    }
                } else if (decimalType.oneofKind === "string") {
                    const stringRange = (decimalType as { oneofKind: "string"; string: any }).string;
                    if (stringRange) {
                        if (min !== undefined) {
                            stringRange.min = min.toString();
                        }
                        if (max !== undefined) {
                            stringRange.max = max.toString();
                        }
                    }
                }
            }
            break;
        }
        case "datetimeRange": {
            // datetimeRange has nested types, handle timestamp case
            if (newRule.kind.datetimeRange && newRule.kind.datetimeRange.type) {
                const datetimeType = newRule.kind.datetimeRange.type;
                if (datetimeType.oneofKind === "timestamp") {
                    const timestampRange = (datetimeType as { oneofKind: "timestamp"; timestamp: any }).timestamp;
                    if (timestampRange) {
                        if (min !== undefined) {
                            timestampRange.min = Math.max(0, Math.round(min));
                        }
                        if (max !== undefined) {
                            timestampRange.max = Math.max(0, Math.round(max));
                        }
                    }
                } else if (datetimeType.oneofKind === "string") {
                    const stringRange = (datetimeType as { oneofKind: "string"; string: any }).string;
                    if (stringRange) {
                        if (min !== undefined) {
                            stringRange.min = min.toString();
                        }
                        if (max !== undefined) {
                            stringRange.max = max.toString();
                        }
                    }
                }
            }
            break;
        }
    }
    
    return newRule;
}

/**
 * Creates a new param name from generator match
 */
function createParamName(match: GeneratorMatch): string {
    let name = match.name;
    if (match.unique) {
        name += "!";
    }
    if (match.min !== undefined || match.max !== undefined) {
        const minStr = match.min !== undefined ? match.min.toString() : "";
        const maxStr = match.max !== undefined ? match.max.toString() : "";
        name += `{${minStr}:${maxStr}}`;
    }
    return name;
}

/**
 * Applies generator ranges from SQL syntax to workload params
 * Modifies workloads in place
 */
export function apply_generators_ranges(workloads: WorkloadDescriptor[]): void {
    for (const workload of workloads) {
        for (const unit of workload.units) {
            if (!unit.descriptor) {
                continue;
            }
            
            const descriptor = unit.descriptor;
            let sql: string | undefined;
            let params: QueryParamDescriptor[] | undefined;
            
            if (descriptor.type.oneofKind === "query") {
                const queryDesc = descriptor.type.query;
                sql = queryDesc.sql;
                params = queryDesc.params;
            } else if (descriptor.type.oneofKind === "transaction") {
                // For transactions, process each query in the transaction
                const tx = descriptor.type.transaction;
                if (!tx) {
                    continue;
                }
                
                // Process queries within transaction
                for (const query of tx.queries) {
                    if (!query.sql) {
                        continue;
                    }
                    
                    const matches = extractGeneratorSyntax(query.sql);
                    if (matches.length === 0) {
                        continue;
                    }
                    
                    let sqlModified = query.sql;
                    
                    for (const match of matches) {
                        // First check transaction-level params, then query-level params
                        let baseParam = tx.params?.find(p => p.name === match.name);
                        let targetParams = tx.params;
                        
                        if (!baseParam && query.params) {
                            baseParam = query.params.find(p => p.name === match.name);
                            targetParams = query.params;
                        }
                        
                        if (!baseParam) {
                            console.warn(
                                `Param '${match.name}' not found in transaction '${tx.name}' or query '${query.name}' params for match '${match.fullMatch}'`
                            );
                            continue;
                        }
                        
                        if (!baseParam.generationRule || !hasRangeType(baseParam.generationRule)) {
                            console.warn(
                                `Param '${match.name}' does not have a range type for match '${match.fullMatch}'`
                            );
                            continue;
                        }
                        
                        const newParamName = createParamName(match);
                        
                        // Check if param with this name already exists in target params
                        if (targetParams && targetParams.find(p => p.name === newParamName)) {
                            // Param already exists, just replace the SQL syntax (replace all occurrences)
                            sqlModified = sqlModified.split(match.fullMatch).join(`\${${newParamName}}`);
                            continue;
                        }
                        
                        const newRule = updateRange(
                            baseParam.generationRule,
                            match.min,
                            match.max
                        );
                        
                        if (match.unique) {
                            newRule.unique = true;
                        }
                        
                        const newParam: QueryParamDescriptor = {
                            name: newParamName,
                            generationRule: newRule,
                            replaceRegex: baseParam.replaceRegex,
                            dbSpecific: baseParam.dbSpecific,
                        };
                        
                        // Add param to the same location where base param was found
                        if (!targetParams) {
                            // If no params array exists, create one
                            if (tx.params === undefined) {
                                tx.params = [];
                            }
                            targetParams = tx.params;
                        }
                        targetParams.push(newParam);
                        
                        // Replace the syntax in SQL with the new param name (replace all occurrences)
                        sqlModified = sqlModified.split(match.fullMatch).join(`\${${newParamName}}`);
                    }
                    
                    // Update the SQL with replaced syntax
                    query.sql = sqlModified;
                }
                continue;
            }
            
            if (!sql || !params) {
                continue;
            }
            
            const matches = extractGeneratorSyntax(sql);
            if (matches.length === 0) {
                continue;
            }
            
            let sqlModified = sql;
            
            for (const match of matches) {
                const baseParam = params.find(p => p.name === match.name);
                if (!baseParam) {
                    console.warn(
                        `Param '${match.name}' not found in unit params for match '${match.fullMatch}'`
                    );
                    continue;
                }
                
                if (!baseParam.generationRule || !hasRangeType(baseParam.generationRule)) {
                    console.warn(
                        `Param '${match.name}' does not have a range type for match '${match.fullMatch}'`
                    );
                    continue;
                }
                
                const newParamName = createParamName(match);
                
                // Check if param with this name already exists
                if (params.find(p => p.name === newParamName)) {
                    // Param already exists, just replace the SQL syntax (replace all occurrences)
                    sqlModified = sqlModified.split(match.fullMatch).join(`\${${newParamName}}`);
                    continue;
                }
                
                const newRule = updateRange(
                    baseParam.generationRule,
                    match.min,
                    match.max
                );
                
                if (match.unique) {
                    newRule.unique = true;
                }
                
                const newParam: QueryParamDescriptor = {
                    name: newParamName,
                    generationRule: newRule,
                    replaceRegex: baseParam.replaceRegex,
                    dbSpecific: baseParam.dbSpecific,
                };
                
                params.push(newParam);
                
                // Replace the syntax in SQL with the new param name (replace all occurrences)
                sqlModified = sqlModified.split(match.fullMatch).join(`\${${newParamName}}`);
            }
            
            // Update the SQL with replaced syntax
            if (descriptor.type.oneofKind === "query") {
                descriptor.type.query.sql = sqlModified;
            }
        }
    }
}

