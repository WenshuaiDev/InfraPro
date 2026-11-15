import { cpSync, mkdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(resolve(root, 'apps/console/package.json'));
const source = dirname(require.resolve('pdfjs-dist/package.json'));
const target = resolve(root, 'apps/console/public/vendor/pdfjs');
mkdirSync(target, { recursive: true });
for (const directory of ['cmaps', 'standard_fonts', 'wasm', 'iccs']) cpSync(resolve(source, directory), resolve(target, directory), { recursive: true });
cpSync(resolve(source, 'LICENSE'), resolve(target, 'LICENSE'));
