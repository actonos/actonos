import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import tseslint from 'typescript-eslint';

export default [
  { ignores: ['dist', '../internal/server/dist', 'coverage', 'test-results', 'playwright-report', 'scripts', 'tests', '*.config.*'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.browser, ...globals.es2022 },
      parserOptions: { ecmaFeatures: { jsx: true }, sourceType: 'module' },
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'no-unused-vars': 'off',
      'no-undef': 'off',
      'no-empty': ['error', { allowEmptyCatch: true }],
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'react-hooks/exhaustive-deps': 'off',
      'react-refresh/only-export-components': 'off',
    },
  },
];
