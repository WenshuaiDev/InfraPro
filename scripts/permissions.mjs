import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync, readdirSync } from 'node:fs';
const check = process.argv.includes('--check');
const catalog = JSON.parse(execFileSync('go', ['run', './tools/infrapro/cmd/infrapro', 'access', 'catalog'], { encoding: 'utf8' }));
const codes = catalog.map((p) => p.code);
if (new Set(codes).size !== codes.length || catalog.some((p) => !p.name || !p.description || !p.scope || !p.risk || !p.group)) throw new Error('Invalid permission catalog');
const name = (code) => code.split(/[._]/).map((part) => part[0].toUpperCase() + part.slice(1)).join('');
const ts = '// Generated from platform/access/catalog.go. Run pnpm permissions:generate.\nexport const Permissions = {\n' + codes.map((code) => `  ${name(code)}: '${code}',`).join('\n') + '\n} as const;\nexport type PermissionCode = typeof Permissions[keyof typeof Permissions];\n';
const tsPath = 'packages/ts/api-client/src/permissions.generated.ts';
const apiPath = 'contracts/openapi/api.yaml';
const api = JSON.parse(readFileSync(apiPath, 'utf8'));
api.components.schemas.PermissionCode.enum = codes;
const yaml = JSON.stringify(api, null, 2) + '\n';
for (const [path, content] of [[tsPath, ts], [apiPath, yaml]]) {
  if (check) { if (readFileSync(path, 'utf8') !== content) throw new Error(`${path} is out of date; run pnpm permissions:generate`); }
  else writeFileSync(path, content);
}
function files(dir) { return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => entry.isDirectory() ? files(`${dir}/${entry.name}`) : [`${dir}/${entry.name}`]); }
for (const dir of ['packages/go/platform', 'services/api', 'tools/infrapro', 'apps/console/src', 'packages/ts/api-client/src']) {
  for (const path of files(dir).filter((p) => /\.(go|tsx?)$/.test(p) && !p.endsWith('_test.go') && !p.endsWith('.generated.ts') && !p.endsWith('schema.d.ts') && !p.endsWith('/catalog.go') && !p.endsWith('/project_catalog.go') && !p.endsWith('/engineering_catalog.go') && !p.endsWith('/resource_catalog.go') && !p.endsWith('/project_resource_catalog.go'))) {
    const text = readFileSync(path, 'utf8');
    if (codes.some((code) => text.includes(`"${code}"`) || text.includes(`'${code}'`))) throw new Error(`Permission literal outside catalog: ${path}`);
    if (/(?:===|==|!=|!==)\s*["'](?:platform_admin|member)["']/.test(text)) throw new Error(`Role identity used as condition: ${path}`);
  }
}
if (check) {
  execFileSync('go', ['test', './packages/go/platform/access', '-run', '^TestCatalogPolicy$'], { stdio: 'inherit' });
  execFileSync('go', ['test', './services/api/internal/transport/http', '-run', '^TestRoutePolicyCoverage$'], { stdio: 'inherit' });
}
console.log(check ? 'Permission catalog and generated files are synchronized.' : 'Generated permission constants and OpenAPI enum.');
