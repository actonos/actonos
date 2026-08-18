import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import fs from 'node:fs';
import path from 'path';

// Single source of truth for the version is the repository VERSION file, which
// also feeds the Go binary via ldflags. package.json is kept in sync by
// scripts/version-bump.sh; the file read below is the authoritative fallback so
// the bundle can never disagree with the daemon it ships alongside.
function resolveVersion(): string {
  const versionFile = path.resolve(__dirname, '../VERSION');
  try {
    const value = fs.readFileSync(versionFile, 'utf8').trim();
    if (value) return value;
  } catch {
    // Fall through to package.json when building outside the repo tree.
  }
  try {
    const pkg = JSON.parse(fs.readFileSync(path.resolve(__dirname, 'package.json'), 'utf8'));
    if (typeof pkg.version === 'string' && pkg.version) return pkg.version;
  } catch {
    // Ignore and report an explicit dev version below.
  }
  return '0.0.0-dev';
}

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(resolveVersion()),
  },
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../internal/server/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks: {
          editor: [
            '@tiptap/react',
            '@tiptap/starter-kit',
            '@tiptap/extension-code-block',
            '@tiptap/extension-highlight',
            '@tiptap/extension-link',
            '@tiptap/extension-typography',
          ],
          i18n: ['i18next', 'react-i18next', 'i18next-browser-languagedetector'],
          icons: ['lucide-react'],
        },
      },
    },
  },
});
