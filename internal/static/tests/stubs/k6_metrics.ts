export class Counter {
  constructor(_name: string) {}
  add(_value: number, _tags?: Record<string, string>): void {}
}

export class Rate {
  constructor(_name: string) {}
  add(_value: number, _tags?: Record<string, string>): void {}
}

export class Trend {
  constructor(_name: string, _isTime?: boolean) {}
  add(_value: number, _tags?: Record<string, string>): void {}
}
