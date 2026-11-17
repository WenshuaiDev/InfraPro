import { execFileSync, spawn } from 'node:child_process';
import { resolve } from 'node:path';

const preview = process.env.INFRAPRO_E2E_PREVIEW;
const directory = process.env.INFRAPRO_E2E_PREVIEW_DIR;
if (preview && !directory) throw Error('Preview tests require an isolated build. Run node scripts/e2e.mjs.');
if (!preview) execFileSync(process.execPath, ['scripts/prepare-document-preview.mjs'], { stdio: 'inherit' });
const child = spawn(process.execPath, [resolve('apps/console/node_modules/vite/bin/vite.js'), preview ? 'preview' : 'dev', ...(preview ? ['--outDir', directory] : [])], {
  cwd: resolve('apps/console'), stdio: 'inherit',
});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.once('error', error => { console.error(error.message); process.exitCode = 1; });
child.once('exit', code => { process.exitCode = code ?? 1; });
