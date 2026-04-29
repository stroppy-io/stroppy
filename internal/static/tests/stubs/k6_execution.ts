export const test = {
  fail(message?: string): never {
    throw new Error(message ?? "test.fail");
  },
  abort(message?: string): never {
    throw new Error(message ?? "test.abort");
  },
};
