import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync, readdirSync, existsSync, mkdirSync, rmSync, cpSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../', import.meta.url));
const run = (cmd, args, options = {}) => execFileSync(cmd, args, { cwd: root, stdio: 'inherit', ...options });
if (!process.argv.includes('--skip-ui')) run('pnpm', ['--filter', '@infrapro/diagnostics', 'build']);
const dist = join(root, 'apps/diagnostics/dist');
const manifest = JSON.parse(readFileSync(join(dist, '.vite/manifest.json'), 'utf8'));
const html = readFileSync(join(dist, 'index.html'), 'utf8');
if (html.includes('ui-not-built') || /(?:src|href)=["']https?:\/\//i.test(html) || /@vite\/client/.test(html)) throw Error('Invalid standalone production UI');
for (const entry of Object.values(manifest)) for (const file of [entry.file, ...(entry.css ?? []), ...(entry.assets ?? [])]) {
  if (!file || file.includes('..') || !existsSync(join(dist, file))) throw Error('Incomplete diagnostics asset manifest');
}
const files = readdirSync(dist, { recursive: true, withFileTypes: true }).filter((file) => file.isFile()).map((file) => join(file.parentPath, file.name)).sort();
const hash = createHash('sha256'); for (const file of files) hash.update(file.slice(dist.length)).update(readFileSync(file));
const build = `diag-${hash.digest('hex').slice(0, 20)}`;
const assets = join(root, 'services/diagnostics/internal/web/assets');
mkdirSync(assets, { recursive: true });
// Only this script's exact embed directory is refreshed. The source placeholder
// remains for go test on a fresh checkout; production binaries exclude it below.
for (const name of readdirSync(assets)) if (name !== 'ui-not-built.html') rmSync(join(assets, name), { recursive: true, force: true });
cpSync(join(dist, 'assets'), join(assets, 'assets'), { recursive: true });
writeFileSync(join(assets, 'index.html'), html);
writeFileSync(join(assets, 'build.json'), JSON.stringify({ build_id: build, asset_manifest: manifest }));
if (process.argv.includes('--embed-only')) { console.log(`Embedded verified diagnostics UI ${build}`); process.exit(0); }
mkdirSync(join(root, 'build/bin'), { recursive: true });
const target = process.argv.includes('--linux') ? 'infrapro-diagnostics-linux' : 'infrapro-diagnostics';
run('go', ['build', '-trimpath', '-tags', 'diagnostics_release', '-ldflags', `-X github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/web.BuildID=${build}`, '-o', join(root, 'build/bin', target), './services/diagnostics/cmd/infrapro-diagnostics'], { env: { ...process.env, ...(process.argv.includes('--linux') ? { GOOS: 'linux', CGO_ENABLED: '0' } : {}) } });
writeFileSync(join(root, 'build/bin', `${target}.build.json`), JSON.stringify({ build_id: build, ui_sha256: createHash('sha256').update(html).digest('hex'), output: target }, null, 2) + '\n');
if (!process.argv.includes('--linux')) run(join(root,'build/bin',target),['version','--verify-assets']);
console.log(`Built standalone diagnostics: ${target} (${build}).`);
