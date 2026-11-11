import { readFile, writeFile } from 'node:fs/promises';
import openapiTS, { astToString } from 'openapi-typescript';

const schema = new URL('../contracts/openapi/api.yaml', import.meta.url);
// Validate security references as well as generated types: generators tolerate unknown schemes.
const contract = JSON.parse(await readFile(schema, 'utf8'));
for (const [path, operations] of Object.entries(contract.paths)) {
  for (const operation of Object.values(operations)) {
    for (const requirement of operation.security ?? contract.security ?? []) {
      for (const name of Object.keys(requirement)) {
        if (!contract.components.securitySchemes[name]) throw new Error(`Unknown security scheme ${name} at ${path}`);
      }
    }
  }
}
const destination = new URL('../packages/ts/api-client/src/schema.d.ts', import.meta.url);
const output = '// Generated from contracts/openapi/api.yaml. Run pnpm contracts:generate.\n' + astToString(await openapiTS(schema));
if (process.argv.includes('--check')) {
  if (await readFile(destination, 'utf8') !== output) {
    throw new Error('API types are out of date. Run pnpm contracts:generate.');
  }
  console.log('OpenAPI types are up to date.');
} else {
  await writeFile(destination, output);
  console.log('Generated API types.');
}
