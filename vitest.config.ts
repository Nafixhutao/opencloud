import { fileURLToPath } from 'node:url';

import { defineConfig } from 'vitest/config';

export default defineConfig({
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('.', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
    include: [
      'app/**/*.test.ts',
      'app/**/*.test.tsx',
      'components/**/*.test.tsx',
      'lib/**/*.ui.test.ts',
      'lib/api.test.ts',
      'lib/api-route.test.ts',
    ],
  },
});
