import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';

// Exercise the original inbox and published-dictionary queries as their
// owners. Keep invite tokens, draft contents and publishing metadata private.
export function verifyRestrictedLookupPermissions(ownerQuery, accountID) {
  assert.match(accountID, /^[a-f0-9-]{36}$/);
  const invitation = randomUUID();
  const inviteID = ownerQuery(`BEGIN;
    INSERT INTO account_invitations(id,inviter_id,token_hash,expires_at)
      VALUES('${invitation}','${accountID}','${randomUUID()}',now()+interval '1 hour');
    SET LOCAL ROLE infrapro_control_api;
    SELECT id::text FROM account_invitations WHERE inviter_id='${accountID}' AND id::text=ANY(ARRAY['${invitation}']);
    ROLLBACK;`);
  assert.equal(inviteID, invitation);
  const releaseID = randomUUID(), category = randomUUID(), code = 'ARC-' + category;
  const content = { dictionary_code: 'device_category', sensor_root_id: category,
    nodes: [{ id: category, code, name: 'Sensor', description: '', order: 0,
      selectable: true, status: 'active', path: [category], name_path: ['Sensor'], sensor: true }] };
  const encoded = JSON.stringify(content), contentHash = createHash('sha256').update(encoded).digest('hex');
  const published = JSON.parse(ownerQuery(`BEGIN;
    INSERT INTO device_category_identities(id,code) VALUES('${category}','${code}');
    INSERT INTO device_category_releases(id,number,content,content_hash,created_by,reason)
      SELECT '${releaseID}',COALESCE(max(number),0)+1,'${encoded}'::jsonb,'${contentHash}','${accountID}','ARC-032'
      FROM device_category_releases;
    INSERT INTO device_category_release_nodes(release_id,category_id,name,sort_order,selectable,status,path,name_path,sensor)
      VALUES('${releaseID}','${category}','Sensor',0,true,'active',ARRAY['${category}'::uuid],ARRAY['Sensor'],true);
    SET CONSTRAINTS device_category_publication IMMEDIATE;
    SET LOCAL ROLE infrapro_audit_worker;
    SELECT row_to_json(r) FROM (SELECT id::text,number,content,content_hash,created_at,reason
      FROM device_category_releases WHERE id='${releaseID}') r;
    ROLLBACK;`));
  assert.deepEqual(published.content, content);
  assert.equal(published.id, releaseID);
  assert(published.number > 0 && Date.parse(published.created_at) > 0);
  assert.equal(published.reason, 'ARC-032');
  assert.equal(published.content_hash, contentHash);
  const sample = ownerQuery("SELECT tenant_id||'/'||facility_id FROM engineering_resource_records WHERE kind='facility' AND data->>'deleted_at' IS NULL ORDER BY id LIMIT 1");
  const [tenant, facility] = sample ? sample.split('/') : [randomUUID(), randomUUID()];
  const graphQuery = `SELECT data FROM engineering_resource_records WHERE tenant_id='${tenant}' AND facility_id='${facility}'
      AND (false OR data->>'deleted_at' IS NULL) ORDER BY kind,id LIMIT 100001`;
  const projection = (query, prefix) => JSON.parse(ownerQuery(`BEGIN; ${prefix}
      SELECT json_build_object('rows',count(*),'sha256',encode(sha256(convert_to(
        COALESCE(string_agg(data::text,chr(10)),''),'UTF8')),'hex')) FROM (${query}) g;
      ROLLBACK;`));
  const queries = {
    engineeringGraph: graphQuery,
    boreholes: `SELECT to_jsonb(b) AS data FROM engineering_boreholes b WHERE tenant_id='${tenant}' ORDER BY id LIMIT 100`,
    sections: `SELECT to_jsonb(s) AS data FROM engineering_monitoring_sections s WHERE tenant_id='${tenant}' ORDER BY id LIMIT 100`,
    deviceCategories: `SELECT to_jsonb(c)||jsonb_build_object('name',n.name,'path',n.path,'name_path',n.name_path,
        'sensor',n.sensor,'status',n.status) AS data
      FROM device_effective_classifications c LEFT JOIN device_category_release_nodes n ON n.release_id=c.dictionary_release_id
        AND n.category_id=c.category_id WHERE c.tenant_id='${tenant}' ORDER BY c.device_id LIMIT 100`,
    productCategories: `SELECT jsonb_build_array(s.current_release_id,r.id,r.product_version_id,r.revision,r.category_id,r.selected_release_id) AS data
      FROM device_classification_state s LEFT JOIN product_classification_records r ON r.ended_at IS NULL
      WHERE s.id=1 ORDER BY r.product_version_id,r.revision LIMIT 100`,
  };
  const matched = {};
  for (const [name, query] of Object.entries(queries)) {
    matched[name] = projection(query, '');
    assert.deepEqual(projection(query, 'SET LOCAL ROLE infrapro_audit_worker;'), matched[name], name);
  }
  const denied = {
    infrapro_control_api: ['SELECT token_hash FROM account_invitations', 'SELECT note FROM account_invitations'],
    infrapro_audit_worker: ['SELECT created_by FROM device_category_releases', 'SELECT created_txid FROM device_category_releases',
      'SELECT draft FROM device_classification_state', 'UPDATE device_category_releases SET content=content WHERE false',
      'SELECT description FROM device_category_release_nodes', 'SELECT reason FROM product_classification_records',
      'UPDATE product_classification_records SET revision=revision WHERE false', 'UPDATE engineering_boreholes SET name=name WHERE false',
      'UPDATE engineering_monitoring_sections SET name=name WHERE false'],
  };
  for (const [role, statements] of Object.entries(denied)) {
    for (const statement of statements) {
      assert.throws(() => ownerQuery(`\\set VERBOSITY verbose
        BEGIN; SET LOCAL ROLE ${role}; ${statement}; ROLLBACK;`),
      (error) => /ERROR:\s+42501:/.test(error.stderr?.toString()));
    }
  }
  return { inboxInvitationLookup: { returnsOwnInvitation: true, deniedStatements: denied.infrapro_control_api },
    auditPublishedDictionary: { returnedFields: Object.keys(published), publicationConstraintChecked: true,
      deniedStatements: denied.infrapro_audit_worker },
    auditEngineeringGraph: { scopedQuery: true, ...matched.engineeringGraph }, sourceMetadata: matched };
}

// This fixture runs immediately after migration, before the isolated database
// records events. Its completed export never enters the archive or backup set.
export function verifyAuditDownloadPermissions({ ownerQuery, transferURL, container }) {
  const uri = new URL(transferURL);
  const login = decodeURIComponent(uri.username);
  assert.match(login, /^arc_transfer_api_(?:[a-f0-9]{8}|[a-f0-9]{12})$/);
  const exportID = randomUUID();
  const objectID = randomUUID();
  const payload = Buffer.from('audit\n');
  const checksum = createHash('sha256').update(payload).digest('hex');
  const root = `'${exportID}'::uuid`;
  const item = `'${objectID}'::uuid`;
  const rootKey = 'id=' + root;
  const objectKey = 'id=' + item;
  const generationKey = 'export_id=' + root + ' AND generation=1';
  const reservationID=randomUUID(), reservationKey=`id='${reservationID}'`;
  const node='arc-transfer-'+reservationID;
  ownerQuery(`BEGIN;
    DO $$ BEGIN
      IF (SELECT enabled FROM event_protocol WHERE id=1 FOR SHARE) IS DISTINCT FROM false THEN
        RAISE EXCEPTION 'Prepare audit download fixture before enabling event recording';
      END IF;
    END $$;
    INSERT INTO audit_exports(id,scope_kind,scope_key,owner_id,membership_key,filter,field_profile,reason,operation_id)
    VALUES(${root},'platform','platform',(SELECT id FROM accounts ORDER BY id LIMIT 1),'fixture','{}','standard',
           'ARC-032 download',(SELECT id FROM audit_operations ORDER BY id LIMIT 1));
    INSERT INTO audit_export_generations(export_id,generation,requested_by,operation_id,authorization_vector,
        projection_version,restore_epoch,state,phase,fence,object_id,finished_at)
    SELECT x.id,1,x.owner_id,x.operation_id,'fixture',1,p.restore_epoch,'succeeded','succeeded',1,${item},now()
      FROM audit_exports x CROSS JOIN audit_protocol p WHERE x.id=${root} AND p.id=1;
    INSERT INTO audit_export_objects(id,export_id,generation,object_key,fence,restore_epoch,state,size_bytes,sha256,expires_at)
    SELECT ${item},g.export_id,g.generation,'audit-exports/v1/'||g.export_id::text,g.fence,g.restore_epoch,
           'available',${payload.length},'${checksum}',now()+interval '1 day'
      FROM audit_export_generations g WHERE ${generationKey};
    COMMIT;`);
  try {
    // The same password-bearing slot used by the real service authenticates
    // over TCP. Only the parent database owner creates and removes test rows.
    const read = (statement) => {
      const env = { ...process.env, PGPASSWORD: decodeURIComponent(uri.password) };
      return execFileSync('docker', ['exec', '-i', '-e', 'PGPASSWORD', container,
        'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1', '-h', '127.0.0.1', '-U', login, '-d', uri.pathname.slice(1)],
      { input: statement, env, encoding: 'utf8', stdio: 'pipe' }).trim();
    };
    const refused = new Map([
      ['changeExportID', `UPDATE audit_exports SET id=gen_random_uuid() WHERE ${rootKey}`],
      ['advanceGeneration', `UPDATE audit_exports SET current_generation=2 WHERE ${rootKey}`],
      ['changeGenerationRoot', `UPDATE audit_export_generations SET export_id=gen_random_uuid() WHERE ${generationKey}`],
      ['changeGenerationState', `UPDATE audit_export_generations SET state='failed' WHERE ${generationKey}`],
      ['changeGenerationFence', `UPDATE audit_export_generations SET fence=fence+1 WHERE ${generationKey}`],
      ['replaceDigest', `UPDATE audit_export_objects SET sha256=repeat('0',64) WHERE ${objectKey}`],
      ['replaceObjectKey', `UPDATE audit_export_objects SET object_key=object_key||'.new' WHERE ${objectKey}`],
      ['changeObjectEpoch', `UPDATE audit_export_objects SET restore_epoch=gen_random_uuid() WHERE ${objectKey}`],
      ['expireAvailable', `UPDATE audit_export_objects SET state='expired' WHERE ${objectKey}`],
      ['readPrivateReservationTime', `SELECT created_at FROM audit_temp_reservations WHERE ${reservationKey}`],
      ['changeReservationBytes', `UPDATE audit_temp_reservations SET reserved_bytes=99 WHERE ${reservationKey}`],
      ['extendReservation', `UPDATE audit_temp_reservations SET expires_at=now()+interval '2 days' WHERE ${reservationKey}`],
      ['moveReservation', `UPDATE audit_temp_reservations SET node_id='another' WHERE ${reservationKey}`],
      ['eraseReservation', `DELETE FROM audit_temp_reservations WHERE ${reservationKey}`],
      ['resurrectReservation', `UPDATE audit_temp_reservations SET released_at=NULL WHERE ${reservationKey}`],
      ['rewriteReleasedTimestamp', `UPDATE audit_temp_reservations SET released_at=clock_timestamp() WHERE ${reservationKey}`],
      ['resurrectMissing', `UPDATE audit_export_objects SET state='available' WHERE ${objectKey}`],
    ]);
    const expectDenied = (name) => `DO $denial$ BEGIN
      BEGIN
        ${refused.get(name)};
        RAISE EXCEPTION 'Transfer unexpectedly permitted ${name}';
      EXCEPTION WHEN insufficient_privilege THEN NULL;
      END;
    END $denial$;
    SELECT json_build_object('check','${name}','sqlstate','42501');`;
    const rootLock = `SELECT x.id,g.generation,o.sha256,o.size_bytes FROM audit_exports x
      JOIN audit_export_generations g ON g.export_id=x.id AND g.generation=COALESCE(NULLIF('1','')::bigint,x.current_generation)
      LEFT JOIN audit_export_objects o ON o.id=g.object_id`;
    const rows = read(`BEGIN;
      SELECT json_build_object('current',current_user,'session',session_user,'superuser',rolsuper,'bypassrls',rolbypassrls,
          'transfer',pg_has_role(current_user,'infrapro_transfer_api','USAGE'))
        FROM pg_roles WHERE rolname=current_user;
      SELECT json_build_object('root',r.id,'generation',r.generation,'digest',r.sha256,'bytes',r.size_bytes)
        FROM (${rootLock} WHERE x.id=${root} FOR UPDATE OF x,g) r;
      SELECT json_build_object('missingExportRows',count(*))
        FROM (${rootLock} WHERE x.id='${randomUUID()}' FOR UPDATE OF x,g) r;
      SELECT json_build_object('lockedObject',id,'state',state)
        FROM audit_export_objects WHERE ${objectKey} FOR UPDATE;
      DO $$ BEGIN PERFORM pg_advisory_xact_lock(hashtextextended('${node}',834327123)); END $$;
      INSERT INTO audit_temp_reservations(id,node_id,reserved_bytes,expires_at)
        VALUES('${reservationID}','${node}',${payload.length},clock_timestamp()+interval '2 minutes');
      SELECT json_build_object('reserved',COALESCE(sum(reserved_bytes),0)::bigint) FROM audit_temp_reservations
        WHERE node_id='${node}' AND released_at IS NULL AND expires_at>clock_timestamp();
      ${[...refused.keys()].filter((name) => !['resurrectReservation','rewriteReleasedTimestamp','resurrectMissing'].includes(name)).map(expectDenied).join('\n')}
      UPDATE audit_temp_reservations SET released_at=clock_timestamp() WHERE ${reservationKey} AND released_at IS NULL
        RETURNING json_build_object('released',released_at IS NOT NULL);
      WITH second AS (UPDATE audit_temp_reservations SET released_at=clock_timestamp() WHERE ${reservationKey}
        AND released_at IS NULL RETURNING id) SELECT json_build_object('releasedAgain',count(*)) FROM second;
      ${expectDenied('resurrectReservation')}
      ${expectDenied('rewriteReleasedTimestamp')}
      SELECT json_build_object('remainingBudgetUse',COALESCE(sum(reserved_bytes),0)::bigint) FROM audit_temp_reservations
        WHERE node_id='${node}' AND released_at IS NULL AND expires_at>clock_timestamp();
      UPDATE audit_export_objects SET state='missing' WHERE ${objectKey} AND state='available'
        RETURNING json_build_object('marked',state);
      ${expectDenied('resurrectMissing')}
      ROLLBACK;`).split('\n').map((line) => JSON.parse(line));
    const [identity, locked, absent, lockedObject] = rows;
    assert.deepEqual(identity, { current: login, session: login, superuser: false, bypassrls: false, transfer: true });
    assert.deepEqual(locked, { root: exportID, generation: 1, digest: checksum, bytes: payload.length });
    assert.deepEqual(absent, { missingExportRows: 0 });
    assert.deepEqual(lockedObject, { lockedObject: objectID, state: 'available' });
    assert.deepEqual(rows.filter((row) => row.marked), [{ marked: 'missing' }]);
    assert.deepEqual(rows.filter((row) => Object.hasOwn(row,'reserved')), [{reserved:payload.length}]);
    assert.deepEqual(rows.filter((row) => Object.hasOwn(row,'released')), [{released:true}]);
    assert.deepEqual(rows.filter((row) => Object.hasOwn(row,'releasedAgain')), [{releasedAgain:0}]);
    assert.deepEqual(rows.filter((row) => Object.hasOwn(row,'remainingBudgetUse')), [{remainingBudgetUse:0}]);
    const denied = rows.filter((row) => row.check);
    assert.deepEqual(denied, [...refused.keys()].map((check) => ({ check, sqlstate: '42501' })));
    const afterRollback = ownerQuery(`SELECT state||'/'||generation||'/'||sha256 FROM audit_export_objects WHERE ${objectKey}`);
    assert.equal(afterRollback, 'available/1/' + checksum);
    return { authenticatedRole: identity.current, tablesLocked: ['audit_exports', 'audit_export_generations', 'audit_export_objects'],
      absentRows: absent.missingExportRows, stateTransition: [lockedObject.state, 'missing'], rollbackState: 'available',
      temporaryReservation:{reservedBytes:payload.length,released:true,repeatReleaseRows:0,remainingBudgetUse:0}, denied };
  } finally {
    ownerQuery(`BEGIN;
      DELETE FROM audit_export_objects WHERE ${objectKey};
      DELETE FROM audit_export_generations WHERE ${generationKey};
      DELETE FROM audit_exports WHERE ${rootKey};
      COMMIT;`);
  }
}
