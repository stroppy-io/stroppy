import { defineConfig } from 'vitest/config';
import path from 'path';

export default defineConfig({
  test: {
    globals: true,
    environment: 'node',
    include: ['**/*.test.ts'],
  },
  resolve: {
    alias: {
      // Ensure proper resolution of .js imports from .ts files
      './stroppy.pb.js': path.resolve(__dirname, '../stroppy.pb.ts'),
      'k6/metrics': path.resolve(__dirname, 'stubs/k6_metrics.ts'),
      'k6/execution': path.resolve(__dirname, 'stubs/k6_execution.ts'),
      'k6/x/encoding': path.resolve(__dirname, 'stubs/k6_x_encoding.ts'),
      'k6/x/stroppy': path.resolve(__dirname, 'stubs/k6_x_stroppy.ts'),
    },
  },
});
