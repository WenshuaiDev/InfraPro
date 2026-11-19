import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const catalogAliases = { diagnostics: 'diagnostics-server', observer: 'diagnostics-observer', infrapro: 'maintenance-cli' };

export function checkCoverage({ root = resolve('.'), additionalEntries = [] } = {}) {
  const read = (name) => JSON.parse(readFileSync(join(root, name), 'utf8'));
  const catalog = read('contracts/diagnostics/runtime-catalog.json').entries;
  const { roles } = read('deploy/architecture/roles.json');
  const entries = [
    ...readdirSync(join(root, 'services/api/cmd'), { withFileTypes: true })
      .filter((dir) => dir.isDirectory()).map((dir) => `services/api/cmd/${dir.name}/main.go`),
    'services/parser/cmd/parser/main.go', 'services/ingest-relay/cmd/ingest-relay/main.go', ...additionalEntries,
  ];
  for (const entry of entries) {
    const matches = catalog.filter((item) => item.entry_point === entry);
    if (matches.length !== 1 || !matches[0].reason || !matches[0].lanes.length) throw Error(`Missing runtime catalog ownership: ${entry}`);
  }
  const go = (args) => execFileSync('go', args, { cwd: root, encoding: 'utf8', maxBuffer: 8 << 20 }).trim();
  // Thin command mains delegate to owning bootstrap functions. Check the linked
  // packages here; the architecture probe test independently starts the binaries.
  const format = '{{printf "%s|" .Dir}}{{range .Deps}}{{if eq . "github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"}}reporter{{end}}{{end}}';
  for (const line of go(['list', '-f', format, ...entries.map((path) => './' + dirname(path))]).split('\n')) {
    const [dir, reporter] = line.split('|');
    if (reporter !== 'reporter') throw Error('Missing process reporter: ' + relative(root, dir));
  }
  const profiles = ['native', 'docker', 'mixed'].map((profile) => {
    const directory = 'deploy/diagnostics/' + profile;
    const manifest = read(directory + '/split-manifest.json');
    const bindings = readdirSync(resolve(root, directory))
      .filter((name) => name.startsWith('split-') && name.endsWith('bindings.json'))
      .flatMap((name) => read(directory + '/' + name).bindings);
    return { profile, manifest, bindings };
  });
  for (const [role, spec] of Object.entries(roles)) {
    const id = catalogAliases[role] ?? role;
    const c = catalog.find((entry) => entry.id === id);
    if (!c?.reason) throw Error('Missing deployment role in runtime catalog: ' + id);
    if (spec.mode === 'conditional' && spec.entrypoint) {
      if (c.entry_point !== spec.entrypoint || !existsSync(join(root, c.entry_point))) throw Error('Missing conditional runtime: '+id);
      continue;
    }
    if (c.entry_point && !['manual', 'child'].includes(spec.mode) && !entries.includes(c.entry_point))
      throw Error('Missing executable for runtime catalog: ' + c.entry_point);
    if ((id !== 'nats-server' && !c.entry_point) || !['split', 'both'].includes(spec.mode)) continue;
    for (const { profile, manifest, bindings } of profiles) {
      const slots = manifest.replicas.filter((r) => r.component_id === id);
      const expected = id === 'nats-server' && profile === 'docker' ? 3 : 1;
      if (slots.length !== expected) throw Error(`Missing replicas: ${profile}/${id}`);
      if (id === 'nats-server' && manifest.components.find(c=>c.id===id)?.min_ready_replicas !== Math.floor(expected/2)+1)
        throw Error('Missing required broker quorum: '+profile);
      for (const slot of slots) {
      if (slot?.expectation !== 'continuous') throw Error(`Missing expected replica: ${profile}/${id}`);
      const required = slot.required_probe_ids.map((key) => manifest.probes.find((p) => p.id === key));
      const evidence = required.map((p) => bindings.find((b) => b.id === p?.binding_id && b.adapter === p.adapter));
      if (required.length < 2 || evidence.some((b) => !b) || !required.some((p) => p.facet === 'presence') ||
          !evidence.some((b) => id === 'nats-server' ? ['docker_container','jetstream_health'].includes(b.adapter) : b.adapter === 'self_report' && b.expected_replica_id === slot.id))
        throw Error(`Missing required independent probes: ${profile}/${slot.id}`);
      }
    }
  }
  for (const id of ['web','postgres','redis','clickhouse','keeper','s3','python-converter']) {
    if (!catalog.some((c) => c.id === id && c.reason)) throw Error('Missing dependency catalog: ' + id);
  }
  if (go(['list','-deps','./services/diagnostics/...']).split('\n').some((line) => /InfraPro\/(packages\/go\/platform|services\/api)/.test(line)))
    throw Error('Diagnostics imports business runtime');
  return { owning_processes: entries.length, catalog_entries: catalog.length, deployment_roles: Object.keys(roles).length,
    binding_profiles: profiles.map(({profile}) => profile), isolated_dependency_graph: true };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) console.log(JSON.stringify(checkCoverage()));
