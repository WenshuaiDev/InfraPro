import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { resolve, relative, join } from 'node:path';
export function checkCoverage({ root = resolve('.'), additionalEntries = [] } = {}) {
  const catalog = JSON.parse(readFileSync(join(root, 'contracts/diagnostics/runtime-catalog.json'), 'utf8')).entries;
  const entries = [...readdirSync(join(root, 'services/api/cmd')).map((name) => `services/api/cmd/${name}/main.go`), 'services/parser/cmd/parser/main.go', ...additionalEntries];
  for (const entry of entries) {
    const matches = catalog.filter((item) => item.entry_point === entry);
    if (matches.length !== 1 || !matches[0].reason || !matches[0].lanes.length) throw Error(`Missing runtime catalog ownership: ${entry}`);
    if (!existsSync(join(root, entry)) || !readFileSync(join(root, entry), 'utf8').includes('diagnosticreport.FromEnv')) throw Error(`Missing process reporter: ${entry}`);
  }
  for (const id of ['web', 'postgres', 'redis', 'clickhouse', 'keeper', 's3', 'diagnostics-server', 'diagnostics-observer', 'python-converter', 'maintenance-cli']) if (!catalog.some((c) => c.id === id && c.reason)) throw Error(`Missing dependency catalog: ${id}`);
  const graph = execFileSync('go', ['list', '-deps', './services/diagnostics/...'], { cwd: root, encoding: 'utf8' });
  if (graph.split('\n').some((line) => /InfraPro\/(packages\/go\/platform|services\/api)/.test(line))) throw Error('Diagnostics imports business runtime');
  return { owning_processes: entries.length, catalog_entries: catalog.length, isolated_dependency_graph: true };
}
if (process.argv[1] && relative(resolve('.'), process.argv[1]).replaceAll('\\', '/') === 'scripts/diagnostics-coverage.mjs') console.log(JSON.stringify(checkCoverage()));
