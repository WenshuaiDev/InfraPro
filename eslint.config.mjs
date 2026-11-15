import base from '@infrapro/eslint-config';

export default [
  ...base,

  { ignores:['packages/ts/display-devtools/lib/**', 'services/diagnostics/internal/web/assets/**', 'apps/console/public/vendor/pdfjs/**'] },
  {
    files: ['packages/ts/ui/src/components/{button,badge,form,sidebar}.tsx'],
    rules: {
      // shadcn exposes each primitive together with its variants/context hook.
      'react-refresh/only-export-components': ['error', { allowConstantExport: true, allowExportNames: ['buttonVariants', 'badgeVariants', 'useFormField', 'useSidebar'] }],
    },
  },
  {
    files: ['packages/ts/{ui,layouts}/src/**/*.{ts,tsx}'],
    rules: { 'no-restricted-imports': ['error', { patterns: [{ group: ['@infrapro/api-client', '@infrapro/api-client/*', '@tanstack/react-router', '**/apps/**'], message: 'Shared UI/layout modules receive data and navigation through props.' }] }] },
  },
  {
    files: ['packages/ts/ui/src/**/*.{ts,tsx}'],
    rules: { 'no-restricted-imports': ['error', { patterns: [{ group: ['@infrapro/api-client', '@infrapro/api-client/*', '@infrapro/layouts', '@infrapro/layouts/*', '@tanstack/react-router', '**/apps/**'], message: 'UI primitives must not depend on application, layout or API modules.' }] }] },
  },
  {
    files: ['apps/*/src/**/*.{ts,tsx}'],
    rules: { 'no-restricted-imports': ['error', { patterns: [{ group: ['@radix-ui/*', '@infrapro/ui/src/*', '@infrapro/layouts/src/*', '**/apps/screen/**'], message: 'Use the public shared UI/layout exports.' }] }] },
  },
  { files: ['apps/diagnostics/src/**/*.{ts,tsx}', 'packages/ts/diagnostics-client/src/**/*.{ts,tsx}'], rules: { 'no-restricted-imports': ['error', { patterns: [{ group: ['@infrapro/api-client', '@infrapro/api-client/*', '**/apps/console/**', '@radix-ui/*'], message: 'Diagnostics has its own identity/client and uses shared public UI.' }] }] } },
];
