import {
  UnitDescriptor,
  WorkloadDescriptor,
  Status,
  DriverTransactionStat,
} from "./stroppy.pb.js";

// Minimal driver interface for what helpers need
export interface Driver {
  runUnit(unit: Uint8Array): Uint8Array;
  notifyStep(name: string, status: Status): void;
  teardown(): any;
}

// Run a single unit descriptor
export function RunUnit(driver: Driver, unit: UnitDescriptor): void {
  driver.runUnit(UnitDescriptor.toBinary(unit));
}

// Run all units in a workload
export function RunWorkload(driver: Driver, wl: WorkloadDescriptor): void {
  wl.units
    .map((wu) => wu.descriptor)
    .filter((d) => d !== undefined)
    .forEach((d) => RunUnit(driver, d));
}

// Find workload by name
export function getWorkload(
  workloads: WorkloadDescriptor[],
  name: string
): WorkloadDescriptor | undefined {
  return workloads.find((w) => w.name === name);
}

// Execute a workload step with notifications
export function runWorkloadStep(
  driver: Driver,
  workloads: WorkloadDescriptor[],
  stepName: string
): void {
  const workload = getWorkload(workloads, stepName);
  if (workload) {
    driver.notifyStep(stepName, Status.STATUS_RUNNING);
    RunWorkload(driver, workload);
    driver.notifyStep(stepName, Status.STATUS_COMPLETED);
  }
}

export function lookup(workloads: WorkloadDescriptor[], ...args: string[]): any {
  if (args.length === 0) return workloads;
  const [wlName, kind, unitName, nestedName] = args;

  const wl = workloads.find((w) => w.name === wlName);
  if (!wl) throw new Error(`Workload '${wlName}' not found`);
  if (!kind) return wl;

  if (kind !== "query" && kind !== "transaction") {
    throw new Error(`Invalid kind '${kind}'. Must be 'query' or 'transaction'.`);
  }

  const units = wl.units.filter((u: any) => u.descriptor?.type.oneofKind === kind);
  if (!unitName) return units;

  const unit = units.find((u: any) => {
    const t = u.descriptor?.type;
    if (t?.oneofKind === "query") return t.query.name === unitName;
    if (t?.oneofKind === "transaction") return t.transaction.name === unitName;
    return false;
  });

  if (!unit)
    throw new Error(
      `Unit '${unitName}' of kind '${kind}' not found in workload '${wlName}'`,
    );

  const type = unit.descriptor?.type;
  // @ts-ignore
  const obj = type[kind];

  if (!nestedName) return obj;

  if (kind === "transaction") {
    const nested = obj.queries.find((q: any) => q.name === nestedName);
    if (!nested)
      throw new Error(
        `Nested query '${nestedName}' not found in transaction '${unitName}'`,
      );
    return nested;
  }

  throw new Error(
    `Cannot search for nested '${nestedName}' in non-transaction unit '${unitName}'`,
  );
}