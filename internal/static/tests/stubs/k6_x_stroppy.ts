const stats = {
  elapsed: {
    milliseconds: () => 0,
    seconds: () => 0,
    microseconds: () => 0,
    nanoseconds: () => 0,
    string: () => "0s",
  },
};

const result = {
  stats,
  rows: {
    columns: () => [],
    close: () => null,
    err: () => null,
    next: () => false,
    values: () => [],
    readAll: () => [],
  },
};

export function NewDriver() {
  return {
    setup(): void {},
    runQuery: () => result,
    insertSpecBin: () => stats,
    begin: () => ({
      runQuery: () => result,
      commit(): void {},
      rollback(): void {},
    }),
  };
}

export function NotifyStep(_name: string, _status?: number): void {}
export function DeclareEnv(_names: string[], _defaultValue: string, _description: string): void {}
export function Once<F extends (...args: any[]) => any>(fn?: F): F {
  return (fn ?? ((() => undefined) as F));
}
