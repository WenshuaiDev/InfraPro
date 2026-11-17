// Canonical console/API policy. Generated inline so native and Docker installs
// do not depend on an installation-specific include path.
import { readFileSync, writeFileSync } from 'node:fs';

const scope = '/api/v1/tenants/[^/]+(/projects/[^/]+)?';
const managed = '/api/v1/(platform|tenants/[^/]+(/projects/[^/]+)?)';
const uploads = ['proxy_http_version 1.1;', "proxy_set_header Connection '';", 'proxy_request_buffering off;', 'proxy_read_timeout 180s;', 'proxy_send_timeout 120s;', 'client_body_timeout 120s;'];
const stream = ['proxy_http_version 1.1;', 'proxy_buffering off;', 'proxy_cache off;', 'proxy_read_timeout 120s;', 'send_timeout 120s;'];
const rules = [
  [`${scope}/(files|spatial)/uploads/[^/]+/parts/[0-9]+$`, '8m', uploads],
  [`${scope}/monitoring/imports/[^/]+/upload/parts/[0-9]+$`, '32m', uploads],
  [`${scope}/displays/instances/[^/]+/uploads/[^/]+/parts/[0-9]+$`, '4m', uploads],
  [`${scope}/monitoring/imports/[^/]+/upload/complete$`, '256k', ['proxy_read_timeout 1810s;']],
  [`${scope}/spatial/uploads/[^/]+/complete$`, '256k', ['proxy_read_timeout 310s;']],
  [`${scope}/files/attachments/[^/]+/revisions/[^/]+/(content|preview)$`, '64k', stream],
  [`${scope}/engineering/resources/.*/attachments/[^/]+/attachment-(content|preview)$`, '256k', stream],
  [`${scope}/spatial/versions/[^/]+/(source|files/[^/]+)$`, '256k', stream],
  [`${scope}/monitoring/(source-files/[^/]+/download|exports/[^/]+/files/[^/]+/download|analysis/exports/[^/]+/files/[^/]+/download)$`, '256k', stream],
  [`${scope}/displays(/|$)`, '1m', stream],
  [`${managed}/device-classifications(/|$)`, '8m', []],
  [`${managed}/parsing/samples/[^/]+/content$`, '6m', uploads],
  [`${managed}/(engineering|device-definitions|device-catalog|devices|device-outputs|device-access)(/|$)`, '272k', []],
  [`${scope}/(monitoring|monitoring-access|spatial|spatial-access|sources|resource-access)(/|$)`, '256k', []],
  [`${managed}/(parsing|ingestion)(/|$)`, '256k', []],
  [`${scope}/files(/|$)`, '64k', []],
  ['/api/v1/tenants/[^/]+/projects(/|$)', '64k', []],
  ['/api/v1/(platform/tenants|tenants|access)(/|$)', '32k', []],
];
export function apiRoutes(upstream) {
  // A nested proxy_set_header would discard ALL inherited headers. Repeat the
  // complete trusted header set on streaming upload locations that set Connection.
  return rules.map(([pattern, limit, settings]) => `        location ~ ^${pattern} {\n            client_max_body_size ${limit};\n${settings.map(s => `            ${s}\n`).join('')}${settings.some(s => s.startsWith('proxy_set_header')) ? `            proxy_set_header Host $http_host;\n            proxy_set_header X-Forwarded-For $remote_addr;\n            proxy_set_header X-Forwarded-Proto $scheme;\n            proxy_set_header Forwarded '';\n            proxy_set_header X-Forwarded-Host '';\n` : ''}            proxy_pass http://${upstream};\n        }`).join('\n');
}

const begin = '        # BEGIN generated upload policy (node scripts/nginx-routes.mjs)';
const end = '        # END generated upload policy';
let failed = false;
for (const [file, upstream] of [['deploy/nginx/default.conf', 'api:8080'], ['deploy/display/nginx.conf', '127.0.0.1:8080'], ['deploy/display/https.nginx.conf', '127.0.0.1:18000']]) {
  const original = readFileSync(file, 'utf8');
  const a = original.indexOf(begin), b = original.indexOf(end, a);
  if (a < 0 || b < 0) throw Error(`Missing generated policy markers: ${file}`);
  const result = original.slice(0, a) + begin + '\n' + apiRoutes(upstream) + '\n' + original.slice(b);
  if (process.argv.includes('--check')) {
    if (result !== original) { console.error(`Stale Nginx upload policy: ${file}`); failed = true; }
  } else writeFileSync(file, result);
}
if (failed) process.exitCode = 1;
