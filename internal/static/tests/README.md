# TypeScript Framework Tests

This directory contains unit tests for the TypeScript framework code in `internal/static/`.

## Setup

Run once to install dependencies:

```bash
make ts-setup
```

## Running Tests

Run all tests once:

```bash
make ts-test
```

Or run tests in watch mode (auto-runs on file changes):

```bash
make ts-watch
```

## Writing Tests

- Test files should be named `*.test.ts`
- Import source files using relative paths: `import { function } from '../file.ts'`
- Use vitest's `describe`, `it`, and `expect` functions

Example:

```typescript
import { describe, it, expect } from 'vitest';
import { myFunction } from '../my_module.ts';

describe('myFunction', () => {
  it('should work correctly', () => {
    expect(myFunction()).toBe(true);
  });
});
```

