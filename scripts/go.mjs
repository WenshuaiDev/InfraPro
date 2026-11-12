import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { resolve, join, basename } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const action = process.argv[2];
const allowed = ['download', 'tidy', 'fmt', 'check', 'test', 'build'];
if (!allowed.includes(action)) throw new Error(`Usage: node scripts/go.mjs ${allowed.join('|')}`);

const go = (args, cwd = root, capture = false) => execFileSync('go', args, {
  cwd, stdio: capture ? 'pipe' : 'inherit', encoding: 'utf8',
});
const workspace = JSON.parse(go(['work', 'edit', '-json'], root, true));
const modules = workspace.Use.map(({ DiskPath }) => resolve(root, DiskPath));

function sourceFiles(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return entry.name.endsWith('.go') ? [path] : [];
  });
}

for (const module of modules) {
  console.log(`[go:${action}] ${module.slice(root.length)}`);
  if (action === 'fmt' || action === 'check') {
    const files = sourceFiles(module);
    if (files.length) {
      const output = execFileSync('gofmt', [action === 'fmt' ? '-w' : '-l', ...files], { encoding: 'utf8' });
      if (output.trim()) throw new Error(`Run make fmt:\n${output}`);
    }
    if (action === 'check') go(['vet', './...'], module);
  } else if (action === 'test') {
    // Tenant lifecycle tests exercise real password hashing and database fixtures.
    // The complete race suite exceeds Go's default ten-minute package limit.
    go(['test', '-race', '-count=1', '-timeout=20m', './...'], module);
  } else if (action === 'download' || action === 'tidy') {
    go(['mod', action], module);
  } else {
    go(['build', './...'], module);
  }
}

if (action === 'build') {
  const outputDir = join(root, 'build', 'bin');
  mkdirSync(outputDir, { recursive: true });
  const names = new Set();
  for (const module of modules) {
    const commandDir = join(module, 'cmd');
    if (!existsSync(commandDir)) continue;
    for (const command of readdirSync(commandDir, { withFileTypes: true }).filter((entry) => entry.isDirectory())) {
      const name = basename(command.name);
      if (names.has(name)) throw new Error(`Duplicate command output: ${name}`);
      names.add(name);
      go(['build', '-trimpath', '-o', join(outputDir, name), `./cmd/${name}`], module);
    }
  }
}
