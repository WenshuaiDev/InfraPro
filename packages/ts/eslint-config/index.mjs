import js from '@eslint/js';
import globals from 'globals';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

export default [
  { ignores: ['**/dist/**', '**/node_modules/**', '**/*.d.ts', 'build/**', 'test-results/**', 'playwright-report/**', '.artifacts/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  { languageOptions: { globals: { ...globals.browser, ...globals.node } } },
  { ...reactHooks.configs.flat.recommended, files: ['**/*.{ts,tsx}'] },
  { ...reactRefresh.configs.vite, files: ['**/*.tsx'] },
];
