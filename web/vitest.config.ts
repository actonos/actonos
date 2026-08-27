import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'path';

export default defineConfig({
  root: __dirname,
  cacheDir: path.resolve(__dirname, 'node_modules/.vite'),
  plugins: [react()],
  resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  test: {
    root: __dirname,
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}'],
    exclude: ['tests/**', 'node_modules/**'],
    setupFiles: [path.resolve(__dirname, './src/test/setup.ts')],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
      thresholds: { lines: 20, functions: 20, branches: 20, statements: 20 },
    },
  },
});
