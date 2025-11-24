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
