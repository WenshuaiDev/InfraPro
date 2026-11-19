// Verify trusted-terminal display scopes using two real deployment logins on a
// disposable clone of the paired HTTP fixtures. The source databases stay online.
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { basename, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { read, root, runDirectory, write } from './architecture-lab.mjs';
import { provision } from './architecture-postgres.mjs';
import { checkoutApplicationDigest } from './architecture-runtime.mjs';

const roles = ['display-runtime', 'display-assets'];
const domains = ['resources', 'devices', 'monitoring', 'spatial'];
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const tables = ['spatial_blobs', 'spatial_assets', 'spatial_asset_versions', 'project_spatial_grants'];
function query(container, uri, sql) {
  const result = execFileSync(
    'docker',
    [
      'exec',
      '-i',
      '-e',
      'PGPASSWORD',
      container,
      'psql',
      '-XqAt',
      '-v',
      'ON_ERROR_STOP=1',
      '-h',
      '127.0.0.1',
      '-U',
      decodeURIComponent(uri.username),
      '-d',
      uri.pathname.slice(1),
    ],
    {
      input: sql,
      env: { ...process.env, PGPASSWORD: decodeURIComponent(uri.password) },
      stdio: 'pipe',
      encoding: 'utf8',
      maxBuffer: 8 << 20,
    },
  );
  return result.trim();
}

function fixture(base, kind, choice) {
  const archived = (name) => join(base, name, 'storage-archive.json');
  const name =
    choice ||
    readdirSync(base)
      .filter(
        (name) =>
          name.startsWith(kind + '-') &&
          existsSync(archived(name)) &&
          existsSync(join(base, name, 'snapshot.json')) &&
          read(archived(name)).status === 'detached',
      )
      .sort((left, right) => statSync(archived(right)).mtimeMs - statSync(archived(left)).mtimeMs)[0];
  assert(name, 'A completed ' + kind + ' HTTP fixture is required');
  assert.match(name, new RegExp('^' + kind + '-[a-f0-9]{12}$'));
  const dir = join(base, name),
    lab = read(join(dir, 'private/lab.json')),
    snapshot = read(join(dir, 'snapshot.json'));
  assert.match(lab.container, /^arc-http-pg-[a-f0-9]{12}$/);
  const url = new URL(read(lab.b.config).database_url);
  assert.equal(url.hostname, '127.0.0.1');
  assert.equal(url.pathname, '/' + lab.b.database);
  assert.equal(lab.b.database, 'arc_http_' + name.split('-').at(-1) + '_b');
  return { name, dir, lab, snapshot, url };
}

function copySpatialProjections(fixture, target) {
  // Both stages originate from the same S0 snapshot. Preserve the published
  // version and grant IDs, their real bytes' digest, and ordinary FK checks.
  const src = (table) => `COALESCE((SELECT jsonb_agg(to_jsonb(t)) FROM public.${table} t),'[]')`;
  const data = JSON.parse(
    query(
      fixture.lab.container,
      fixture.url,
      'SELECT jsonb_build_object(' + tables.map((t) => `'${t}',${src(t)}`).join(',') + ')',
    ),
  );
  assert(data.project_spatial_grants.some((g) => !g.ended_at));
  function records(table, rows) {
    assert(rows.length && rows.length <= 1000);
    const encoded = Buffer.from(JSON.stringify(rows)).toString('base64');
    return `jsonb_populate_recordset(NULL::public.${table},convert_from(decode('${encoded}','base64'),'UTF8')::jsonb)`;
  }
  const contents = tables.map((t) => {
    const rows = t === 'spatial_assets' ? data[t].map((a) => ({ ...a, current_version_id: null })) : data[t];
    return `INSERT INTO public.${t} SELECT * FROM ${records(t, rows)};`;
  });
  contents.push(
    `UPDATE spatial_assets a SET current_version_id=s.current_version_id FROM ${records('spatial_assets', data.spatial_assets)} s WHERE a.id=s.id AND s.current_version_id IS NOT NULL;`,
  );
  assert.equal(query(fixture.lab.container, target, 'SELECT count(*) FROM spatial_assets'), '0');
  query(fixture.lab.container, target, 'BEGIN; ' + contents.join('\n') + ' COMMIT;');
  return Object.fromEntries(
    tables.map((t) => [
      t,
      {
        rows: data[t].length,
        sha256: hash(JSON.stringify(data[t].sort((a, b) => a.id.localeCompare(b.id)))),
      },
    ]),
  );
}

function confirmPrivacy(container, uri, role) {
  const denied = {
    deviceSerial: 'SELECT serial_number FROM devices',
    deviceNotes: 'SELECT description FROM devices',
    deviceOutputNotes: 'SELECT description FROM device_outputs',
    rawEnvelope: 'SELECT envelope FROM ingestion_receipts',
    rawObjectKey: 'SELECT object_key FROM ingestion_objects',
    fileObjectKey: 'SELECT object_key FROM file_objects',
    spatialObjectKey: 'SELECT object_key FROM spatial_blobs',
    spatialSource: 'SELECT source_blob_id FROM spatial_asset_versions',
    replaceSpatialVersion: "UPDATE spatial_asset_versions SET state='withdrawn' WHERE false",
    replaceDeviceGrant: 'DELETE FROM project_device_grants WHERE false',
    replacePackagedAsset: "UPDATE display_package_files SET object_key='forbidden' WHERE false",
  };
  Object.assign(denied, {
    facilityPrivateDescription: 'SELECT description FROM engineering_facilities',
    privateFixedBaselineEvidence: 'SELECT evidence FROM derived_fixed_baselines',
  });
  if (role === 'display-assets')
    Object.assign(denied, {
      derivedBindings: 'SELECT input_bindings FROM derived_channel_configs',
      replacePackage: 'DELETE FROM display_packages WHERE false',
    });
  const statements = Object.entries(denied).map(
    ([label, sql]) => `DO $denied$ BEGIN BEGIN ${sql};
    RAISE EXCEPTION 'Unexpectedly permitted ${label}'; EXCEPTION WHEN insufficient_privilege THEN NULL; END; END $denied$;
    SELECT '${label}';`,
  );
  const got = query(container, uri, `BEGIN; ${statements.join('\n')} ROLLBACK;`).split('\n');
  assert.deepEqual(got, Object.keys(denied));
  return Object.fromEntries(got.map((label) => [label, '42501']));
}

function validateScenes(scenes) {
  const owner = scenes.owner,
    runtime = scenes['display-runtime'],
    assets = scenes['display-assets'];
  function choose(xs, domain, selector) {
    return xs.filter((r) => r.domain === domain && r.selector === selector);
  }
  for (const domain of domains) {
    for (const which of ['all', 'empty', 'explicit', 'unauthorized']) {
      const expected = choose(owner, domain, which),
        actual = choose(runtime, domain, which);
      assert.deepEqual(actual, expected, `Runtime ${domain} ${which}`);
      if (which === 'all' || which === 'explicit')
        assert(
          expected.some((x) => x.result.count > 0),
          `Positive ${domain} ${which}`,
        );
      if (which === 'empty')
        assert(
          expected.every((x) => x.result.count === 0 && !x.result.code),
          `Empty ${domain}`,
        );
      if (which === 'unauthorized')
        assert(
          expected.every((x) => x.result.code === 'project_not_found'),
          `Revoked actor ${domain}`,
        );
    }
    assert(
      choose(assets, domain, 'all').every((x) => x.result.code === '42501'),
      `Asset origin must not resolve ${domain}`,
    );
    assert(choose(assets, domain, 'unauthorized').every((x) => x.result.code === 'project_not_found'));
  }
  const defaults = choose(runtime, 'spatial', 'defaults');
  assert.deepEqual(defaults, choose(owner, 'spatial', 'defaults'));
  assert(defaults.some((x) => x.result.count > 0));
  const eligibility = choose(runtime, 'monitoring', 'derived-qualification');
  assert.deepEqual(
    eligibility,
    choose(owner, 'monitoring', 'derived-qualification'),
    'Derived recursive input qualification',
  );
  assert(
    eligibility.some((x) => x.result.kinds?.visibleDerived > 0 && x.result.count > 0),
    'Expected an authorized derived channel',
  );
}

function selectSoftware(dir, work, workingSource) {
  const pointer = read(join(dir, 'candidate.evaluated.json'));
  const release = read(join(pointer.directory, 'candidate.json'));
  const actual = checkoutApplicationDigest(root);
  let cli = join(release.bin, 'infrapro');
  if (actual !== release.inputs.sourceSHA256) {
    assert(workingSource, 'Selected release differs from current domain source');
    cli = join(work, 'infrapro');
    execFileSync('go', ['build', '-p=2', '-o', cli, './cmd/infrapro'], {
      cwd: join(root, 'tools/infrapro'),
      env: { ...process.env, GOMAXPROCS: '2' },
      stdio: 'pipe',
    });
  }
  return {
    cli,
    cliSHA256: hash(readFileSync(cli)),
    sourceSHA256: actual,
    sealed: actual === release.inputs.sourceSHA256,
    selectedReleaseSHA256: release.inputs.sourceSHA256,
  };
}

function migrateClone(software, source, target, work) {
  const path = join(work, 'service.json');
  const original = read(source.lab.b.config);
  write(path, {
    ...original,
    database_url: target.toString(),
    redis_key_prefix: 'infrapro:' + basename(work) + ':identity:v1:',
  });
  execFileSync(software.cli, ['database', 'migrate'], {
    cwd: root,
    env: {
      ...process.env,
      INFRAPRO_CONFIG: path,
      INFRAPRO_DATABASE_URL: target.toString(),
    },
    stdio: 'pipe',
  });
  return path;
}

export function verifyDisplayScopePermissions(run, { derived, spatial, workingSource = false } = {}) {
  const dir = runDirectory(run),
    source = join(dir, 'http/sandboxes');
  const d = fixture(source, 'derived', derived),
    s = fixture(source, 'spatial', spatial);
  assert.equal(d.lab.container, s.lab.container);
  assert.equal(d.snapshot.sourceDumpSHA256, s.snapshot.sourceDumpSHA256);
  assert.equal(d.snapshot.sourceDumpSHA256, read(join(dir, 'snapshot/manifest.json')).pg.seed);
  const session = randomBytes(6).toString('hex'),
    db = 'arc_scope_' + session,
    container = d.lab.container;
  const target = new URL(d.url);
  target.pathname = '/' + db;
  const system = new URL(d.url);
  system.pathname = '/postgres';
  const work = join(dir, 'private', 'display-scope-' + session),
    cred = join(work, 'credentials');
  for (const role of roles) mkdirSync(join(cred, 'roles', role), { recursive: true, mode: 0o700 });
  write(join(cred, 'layout.json'), { mode: 'local' });
  const software = selectSoftware(dir, work, workingSource);
  const probe = join(work, 'scope-probe');
  execFileSync(
    'go',
    ['build', '-p=2', '-o', probe, join(root, 'scripts/fixtures/architecture-display-scope-probe.go')],
    {
      cwd: join(root, 'packages/go/platform'),
      env: { ...process.env, GOMAXPROCS: '2' },
      stdio: 'pipe',
    },
  );
  query(container, system, `CREATE DATABASE ${db} TEMPLATE ${d.lab.b.database}`);
  const logins = {};
  const report = {
    startedAt: new Date().toISOString(),
    releaseSHA256: software.selectedReleaseSHA256,
    sealed: software.sealed,
    migrationCLI: software.cliSHA256,
    applicationSourceSHA256: software.sourceSHA256,
    sourceDumpSHA256: d.snapshot.sourceDumpSHA256,
    sources: [d.name, s.name],
    clone: db,
    probeSHA256: hash(readFileSync(probe)),
    projects: [],
    scenes: {},
    accounts: {},
  };
  try {
    migrateClone(software, d, target, work);
    report.spatialRows = copySpatialProjections(s, target);
    report.appliedRoleSQL = provision({
      container,
      url: target.toString(),
      directory: cred,
      localLogins: true,
    });
    const targets = { owner: target };
    for (const role of roles) {
      const uri = new URL(read(join(cred, 'roles', role, 'environment.json')).INFRAPRO_DATABASE_URL);
      const user = decodeURIComponent(uri.username);
      assert.match(user, new RegExp('^arc_' + role.replaceAll('-', '_') + '_[a-f0-9]{12}$'));
      logins[role] = user;
      targets[role] = uri;
      report.accounts[role] = JSON.parse(
        query(
          container,
          uri,
          `SELECT json_build_object('user',session_user,'role',current_user,'superuser',rolsuper,
        'bypassRLS',rolbypassrls,'canCreateRole',rolcreaterole,'connections',rolconnlimit,'group',pg_has_role(current_user,'infrapro_${role.replaceAll('-', '_')}','USAGE'))
        FROM pg_roles WHERE rolname=current_user`,
        ),
      );
      const account = report.accounts[role];
      assert(account.group && !account.superuser && !account.bypassRLS && !account.canCreateRole);
      assert.equal(account.user, user);
      assert.equal(account.role, user);
    }
    report.projects = JSON.parse(
      query(
        container,
        target,
        `SELECT json_agg(p ORDER BY p."Project") FROM (SELECT t.tenant_id::text AS "Tenant",t.id::text AS "Project",m.account_id::text AS "Account"
      FROM projects t JOIN project_memberships m ON m.id=t.administrator_membership_id AND m.status='active' WHERE t.status='active') p`,
      ),
    );
    assert(report.projects.length);
    for (const [role, uri] of Object.entries(targets)) {
      const rows = JSON.parse(
        execFileSync(probe, [], {
          input: JSON.stringify({
            URL: uri.toString(),
            Role: role,
            Projects: report.projects,
          }),
          encoding: 'utf8',
          stdio: 'pipe',
        }),
      );
      report.scenes[role] = rows;
      if (role !== 'owner') {
        report.accounts[role].denied = confirmPrivacy(container, uri, role);
        // This is the narrowed asset revalidation metadata used at delivery.
        const q = `SELECT json_build_object('rows',count(*),'digest',encode(sha256(convert_to(COALESCE(string_agg(v.id::text,',' ORDER BY v.id),''),'UTF8')),'hex'))
          FROM project_spatial_grants g JOIN spatial_asset_versions v ON v.tenant_id=g.tenant_id AND v.id=g.version_id
          WHERE g.ended_at IS NULL AND v.state='published' AND v.content_available`;
        const permit = JSON.parse(query(container, uri, q));
        assert(permit.rows > 0);
        assert.deepEqual(permit, JSON.parse(query(container, target, q)));
        report.accounts[role].publishedSpatialPermit = permit;
      }
    }
    validateScenes(report.scenes);
    report.complete = true;
    report.endedAt = new Date().toISOString();
    write(
      join(dir, 'deploy', software.sealed ? 'display-scope-permissions.json' : 'display-scope-working.json'),
      report,
    );
    return report;
  } finally {
    query(container, system, `DROP DATABASE ${db}`);
    for (const user of Object.values(logins)) query(container, system, `DROP ROLE ${user}`);
    if (!report.complete) write(join(dir, 'deploy/display-scope-attempts', session + '.json'), report);
  }
}

function physicalDenial(container, uri) {
  assert.match(container, /^arc-http-storage-[a-f0-9]{12}-ch1-1$/);
  const owner = (statement) =>
    execFileSync('docker', ['exec', container, 'clickhouse-client', '--query', statement], {
      encoding: 'utf8',
      stdio: 'pipe',
    }).trim();
  // Read an attached synthetic table. Completed source tables are detached by
  // the HTTP runner and can remain archived for this authentication test.
  const database = owner(
    "SELECT database FROM system.tables WHERE match(database,'^arc_(http_)?[a-f0-9]{12}_(a|b|seed|self)$') AND name='observations' AND total_rows>0 ORDER BY startsWith(database,'arc_http_') DESC,database LIMIT 1",
  );
  assert.match(database, /^arc_(http_)?[a-f0-9]{12}_(a|b|seed|self)$/);
  const relation = database + '.observations';
  const rows = Number(owner('SELECT count() FROM ' + relation));
  assert(rows > 0);
  const denied = spawnSync(
    'docker',
    [
      'exec',
      '-i',
      '-e',
      'CLICKHOUSE_PASSWORD',
      container,
      'clickhouse-client',
      '--user',
      uri.username,
      '--query',
      'SELECT * FROM ' + relation + ' LIMIT 1',
    ],
    {
      env: {
        ...process.env,
        CLICKHOUSE_PASSWORD: decodeURIComponent(uri.password),
      },
      stdio: 'pipe',
      encoding: 'utf8',
    },
  );
  assert.notEqual(denied.status, 0);
  assert.match(denied.stderr, /Code: (516|497)\b/);
  return {
    relation,
    ownerRows: rows,
    issuedClickHouseIdentity: false,
    code: Number(denied.stderr.match(/Code: (516|497)\b/)[1]),
  };
}

export function verifyFixedArtifactPermissions(run, { analysis, workingSource = false } = {}) {
  const dir = runDirectory(run),
    source = fixture(join(dir, 'http/sandboxes'), 'analysis', analysis);
  const session = randomBytes(6).toString('hex'),
    db = 'arc_artifact_' + session,
    container = source.lab.container;
  const target = new URL(source.url);
  target.pathname = '/' + db;
  const admin = new URL(source.url);
  admin.pathname = '/postgres';
  const work = join(dir, 'private', 'display-artifact-' + session),
    cred = join(work, 'credentials');
  const role = 'display-runtime';
  mkdirSync(join(cred, 'roles', role), { recursive: true, mode: 0o700 });
  write(join(cred, 'layout.json'), { mode: 'local' });
  const software = selectSoftware(dir, work, workingSource);
  assert.equal(source.snapshot.sourceDumpSHA256, read(join(dir, 'snapshot/manifest.json')).pg.seed);
  const sealed = software.sealed && source.snapshot.candidateSHA256 === software.selectedReleaseSHA256;
  assert(workingSource || sealed, 'Rebuild the analysis fixture with the evaluated candidate first');
  const report = {
    startedAt: new Date().toISOString(),
    fixture: source.name,
    sourceDumpSHA256: source.snapshot.sourceDumpSHA256,
    sourceReleaseSHA256: source.snapshot.candidateSHA256,
    applicationSourceSHA256: software.sourceSHA256,
    sealed,
    selectedReleaseSHA256: software.selectedReleaseSHA256,
    cliSHA256: software.cliSHA256,
    clone: db,
  };
  query(container, admin, `CREATE DATABASE ${db} TEMPLATE ${source.lab.b.database}`);
  let login;
  try {
    const signature = "to_regprocedure('architecture_fixed_baseline_metadata(uuid,uuid,text,text)')";
    report.beforeMigrate = query(container, target, `SELECT ${signature} IS NOT NULL`) === 't';
    const config = migrateClone(software, source, target, work);
    report.afterMigrate = query(container, target, `SELECT ${signature} IS NOT NULL`) === 't';
    assert(report.afterMigrate);
    report.appliedRoleSQL = provision({
      container,
      url: target.toString(),
      directory: cred,
      localLogins: true,
    });
    const uri = new URL(read(join(cred, 'roles', role, 'environment.json')).INFRAPRO_DATABASE_URL);
    login = uri.username;
    assert.match(login, /^arc_display_runtime_[a-f0-9]{12}$/);
    const membership = JSON.parse(
      query(
        container,
        target,
        `SELECT row_to_json(q) FROM (
      SELECT a.tenant_id::text AS "Tenant",a.project_id::text AS "Project",a.id::text AS "Artifact",v.id::text AS "Version",
             v.version_no::text AS "Generation",v.raw_snapshot_id::text AS "Snapshot",v.output_rows AS "OutputRows",ac.username AS "Account",
             ARRAY(SELECT DISTINCT r.channel_id::text FROM analysis_artifact_input_ranges r WHERE r.version_id=v.id ORDER BY 1) AS "Channels",
             g.id::text AS "Grant" FROM analysis_artifacts a JOIN analysis_artifact_versions v ON v.artifact_id=a.id
             JOIN projects p ON p.id=a.project_id JOIN project_memberships m ON m.id=p.administrator_membership_id JOIN accounts ac ON ac.id=m.account_id
             JOIN analysis_artifact_input_ranges r ON r.version_id=v.id JOIN monitoring_data_grants g ON g.tenant_id=p.tenant_id AND g.project_id=p.id
             AND g.channel_id=r.channel_id AND g.revoked_at IS NULL AND 'read'=ANY(g.capabilities)
      WHERE a.deleted_at IS NULL AND a.status='active' AND v.output_rows>0 AND a.scope_key=p.id::text AND p.status='active' AND ac.status='active'
      ORDER BY v.version_no DESC,v.id LIMIT 1) q`,
      ),
    );
    assert(membership.OutputRows > 0 && membership.Channels.length);
    const credentials = Object.values(read(join(source.dir, 'private/accounts.json'))).find(
      (a) => a.username === membership.Account,
    );
    assert(credentials, 'Fixture project administrator must have an isolated personnel login');
    const output = join(work, 'result.json'),
      spec = join(work, 'input.json');
    write(spec, {
      ...membership,
      Password: credentials.password,
      RuntimeURL: uri.toString(),
      Config: config,
      Output: output,
    });
    report.runtime = login;
    report.baselineProjection = JSON.parse(
      query(
        container,
        uri,
        `SELECT json_build_object(
      'signature',pg_get_function_result(p.oid),'definer',p.prosecdef,
      'mayExecute',has_function_privilege(current_user,p.oid,'EXECUTE'),
      'definitionSHA256',encode(sha256(convert_to(pg_get_functiondef(p.oid),'UTF8')),'hex'))
      FROM pg_proc p WHERE p.oid=${signature}`,
      ),
    );
    assert.equal(report.baselineProjection.signature, 'TABLE(snapshot jsonb, input_ranges jsonb)');
    assert(report.baselineProjection.definer && report.baselineProjection.mayExecute);
    report.privateSQL = confirmPrivacy(container, uri, role);
    report.physicalObservations = physicalDenial(source.lab.chContainer, uri);
    const probe = join(root, 'scripts/fixtures/architecture-display-artifact-test.go.txt');
    const test = join(work, 'architecture_display_artifact_test.go');
    writeFileSync(test, readFileSync(probe), { mode: 0o600 });
    const overlay = join(work, 'overlay.json');
    write(overlay, {
      Replace: {
        [join(root, 'packages/go/platform/resources/architecture_display_artifact_test.go')]: test,
      },
    });
    const processResult = spawnSync(
      'go',
      [
        'test',
        '-p=2',
        '-vet=off',
        '-tags=monitoringintegration',
        '-overlay',
        overlay,
        '-count=1',
        '-run',
        '^TestArchitectureDisplayArtifactPermissions$',
        '-v',
        './resources',
      ],
      {
        cwd: join(root, 'packages/go/platform'),
        env: {
          ...process.env,
          GOMAXPROCS: '2',
          INFRAPRO_DATABASE_URL: target.toString(),
          INFRAPRO_CONFIG: config,
          INFRAPRO_ARCHITECTURE_ARTIFACT_INPUT: spec,
        },
        encoding: 'utf8',
        stdio: 'pipe',
        timeout: 90_000,
        maxBuffer: 8 << 20,
      },
    );
    const log = join(dir, 'deploy/display-scope-attempts', session + '.log');
    mkdirSync(join(dir, 'deploy/display-scope-attempts'), {
      recursive: true,
      mode: 0o700,
    });
    writeFileSync(log, processResult.stdout + '\n' + processResult.stderr, {
      mode: 0o600,
    });
    assert.equal(processResult.status, 0, 'See ' + log);
    report.outcome = read(output);
    assert(report.outcome.complete);
    report.outputRows = membership.OutputRows;
    report.complete = true;
    report.endedAt = new Date().toISOString();
    write(
      join(dir, 'deploy', sealed ? 'display-artifact-permissions.json' : 'display-artifact-working.json'),
      report,
    );
    return report;
  } finally {
    query(container, admin, `DROP DATABASE ${db}`);
    if (login) query(container, admin, `DROP ROLE ${login}`);
    if (!report.complete) write(join(dir, 'deploy/display-scope-attempts', session + '.json'), report);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const args = process.argv.slice(2),
    workingSource = args.includes('--working-source');
  const [run, derived, spatial, analysis] = args.filter((arg) => !arg.startsWith('--'));
  if (!args.includes('--artifacts-only')) {
    const value = verifyDisplayScopePermissions(run, {
      derived,
      spatial,
      workingSource,
    });
    console.log(
      JSON.stringify({
        complete: value.complete,
        sources: value.sources,
        scenes: value.scenes['display-runtime'].length,
        appliedRoleSQL: value.appliedRoleSQL,
      }),
    );
  }
  if (!args.includes('--scopes-only')) {
    const value = verifyFixedArtifactPermissions(run, {
      analysis,
      workingSource,
    });
    console.log(
      JSON.stringify({
        complete: value.complete,
        source: value.fixture,
        physical: value.physicalObservations,
        revoked: value.outcome.revoked,
      }),
    );
  }
}
