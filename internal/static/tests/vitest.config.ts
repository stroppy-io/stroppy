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
    },
  },
});

