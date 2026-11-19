import assert from 'node:assert/strict';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { digest, read, root, write } from './architecture-lab.mjs';
import { postgresRoleSQLFiles } from './architecture-postgres.mjs';

const assertionFile = 'packages/go/platform/resources/audit_assertions_test.go';
const frozenGrant = 'deploy/production/schema-55-audit-role-completion.sql';

export function auditRoleInputs(run, side, cwd) {
  if (side === 'baseline') {
    const release = read(join(run, 'baseline.json'));
    return {
      directory: cwd,
      releaseSHA256: release.source_archive_sha256,
      publication: 'frozen source archive',
      files: { [frozenGrant]: release.sourceFiles[frozenGrant] },
    };
  }
  const selected =
    process.env.INFRAPRO_ARCHITECTURE_REGRESSION_RELEASE ||
    process.env.INFRAPRO_ARCHITECTURE_HTTP_RELEASE ||
    (existsSync(join(run, 'candidate.evaluated.json')) &&
      read(join(run, 'candidate.evaluated.json')).directory) ||
    run;
  const releaseSHA256 = read(join(selected, 'candidate.json')).inputs.sourceSHA256;
  const record = join(run, 'deploy', 'offline-release.json');
  if (existsSync(record)) {
    const archived = read(record);
    if (archived.application_source_sha256 === releaseSHA256) {
      const path = join(run, archived.release);
      assert.equal(digest(readFileSync(path)), archived.manifest_sha256);
      return {
        directory: dirname(path),
        releaseSHA256,
        publication: archived.release,
        manifestSHA256: archived.manifest_sha256,
        files: archived.role_sql_sha256,
      };
    }
  }
  // Standalone original suites can precede offline packaging.
  return {
    directory: root,
    releaseSHA256,
    publication: 'release packaging inputs',
    files: Object.fromEntries(
      postgresRoleSQLFiles.map((name) => [name, digest(readFileSync(join(root, name)))]),
    ),
  };
}

export function installAuditRoleSetup(run, side, cwd, overlayDir, replacements) {
  const inputs = auditRoleInputs(run, side, cwd);
  const files = side === 'baseline' ? [frozenGrant] : postgresRoleSQLFiles;
  const evidence = {
    side,
    releaseSHA256: inputs.releaseSHA256,
    publication: inputs.publication,
    ...(inputs.manifestSHA256 && { manifestSHA256: inputs.manifestSHA256 }),
    files: {},
  };
  const directory = join(overlayDir, 'audit-source-grants', digest(JSON.stringify(inputs.files)));
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const plan = files.map((name, index) => {
    const bytes = readFileSync(join(inputs.directory, name)),
      sha256 = digest(bytes);
    assert.equal(sha256, inputs.files[name], 'Published audit role SQL changed: ' + name);
    evidence.files[name] = sha256;
    const path = join(directory, index + '.sql');
    writeFileSync(path, bytes, { mode: 0o600 });
    return { name, path, sha256 };
  });
  const planPath = join(directory, 'plan.json');
  write(planPath, plan);
  const source = join(cwd, assertionFile),
    original = readFileSync(source, 'utf8');
  const existing = replacements[source] ? readFileSync(replacements[source], 'utf8') : original;
  const before = '\tclient, e := minio.New(',
    identity = '\tdefer pool.Close()';
  assert.equal(existing.split(before).length, 2);
  assert.equal(existing.split(identity).length, 2);
  const helper = readFileSync(
    new URL('./fixtures/architecture-audit-role.go.inc', import.meta.url),
    'utf8',
  );
  const patched =
    existing
      .replace('import (\n', 'import (\n\t"crypto/sha256"\n\t"encoding/json"\n\t"fmt"\n')
      .replace(
        before,
        '\tinstallPublishedAuditSchemaRoles(t, f, ' + JSON.stringify(planPath) + ')\n' + before,
      )
      .replace(identity, identity + '\n\tverifyPublishedAuditIdentity(t, pool)') +
    '\n' +
    helper;
  const target = join(directory, 'audit-source-assertions.go');
  writeFileSync(target, patched, { mode: 0o600 });
  replacements[source] = target;
  return {
    ...evidence,
    fixture: assertionFile,
    originalSHA256: digest(original),
    setupSHA256: digest(helper),
    overlaySHA256: digest(patched),
  };
}
