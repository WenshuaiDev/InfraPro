import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { digest } from './architecture-lab.mjs';

export function membershipPlanner(profile = 'default') {
  assert(['default', 'matched'].includes(profile), 'Unknown membership planner profile');
  return profile === 'matched'
    ? { profile, pgOptions: '-c enable_nestloop=off -c jit=off', enableNestloop: 'off', jit: 'off' }
    : { profile, pgOptions: '', enableNestloop: 'on', jit: 'on' };
}

// A newly bulk-created test schema has no normal table statistics yet. Keep
// every original grant/foreign key trigger, both original service calls, and
// the production query deadline. Only the fixture loads rows in transactions.
export function installDisplayBoundarySetup(cwd, overlayDir, replacements, traces, profile = 'default') {
  const source = join(cwd, 'packages/go/platform/resources/display_read_load_test.go');
  const text = readFileSync(replacements[source] || source,'utf8');
  const start = text.indexOf('func displayBulkPoints(');
  const end = text.indexOf('func TestDisplayReadFiftyThousandMemberBoundary(',start);
  assert(start >= 0 && end > start);
  const helper = text.slice(start,end);
  const insert = /f\.env\.Pool\.Exec\(ctx, (`WITH inserted AS [^`]+`), f\.tenant, facility, f\.actor\.ID, offset\+1, offset\+count, p\.id\)/.exec(helper)?.[1];
  assert(insert && insert.includes('INSERT INTO project_resource_grants'));
  const readSource = readFileSync(join(cwd,'packages/go/platform/resources/display_scope.go'),'utf8');
  const preview = /tx\.Query\(ctx, `(WITH visible AS MATERIALIZED [^`]+LIMIT 50001)`/.exec(readSource)?.[1];
  assert(preview,'Original display scope query required');
  const directorySource = readFileSync(join(cwd,'packages/go/platform/resources/display_reads.go'),'utf8');
  const directory = /query := strings\.ReplaceAll\(`(WITH RECURSIVE nodes AS MATERIALIZED[^`]+)`, "engineering_resource_nodes", nodes\)/.exec(directorySource)?.[1];
  assert(directory, 'Original display preview metadata query required');
  const planner = membershipPlanner(profile);
  const next = helper.slice(0,helper.indexOf('\n')) + `
	t.Helper()
	end := offset + count
	batches := 0
	for first := offset; first < end; first += 500 {
		_, err := f.env.Pool.Exec(ctx, ${insert}, f.tenant, facility, f.actor.ID, first+1, min(first+500, end), p.id)
		must(t, err)
		batches++
	}
	checkPlan := func(stage string) {
		t.Helper()
		err := f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			var nestloop, jit, statement, lock string
			if err := tx.QueryRow(ctx, \`SELECT current_setting('enable_nestloop'),current_setting('jit'),current_setting('statement_timeout'),current_setting('lock_timeout')\`).Scan(&nestloop, &jit, &statement, &lock); err != nil { return err }
			if nestloop != "${planner.enableNestloop}" || jit != "${planner.jit}" || statement != "10s" || lock != "5s" {
				return fmt.Errorf("unexpected planning settings nestloop=%s jit=%s statement_timeout=%s lock_timeout=%s",nestloop,jit,statement,lock)
			}
			store := func(prefix string, data []byte) error {
				target := os.Getenv("ARC_DISPLAY_BOUNDARY_TRACE")
				if target == "" { return nil }
				return os.WriteFile(fmt.Sprintf("%s/%d-%s%s.json", target, end, prefix, stage), data, 0600)
			}
			settings, err := json.Marshal(map[string]string{"enable_nestloop":nestloop, "jit":jit, "statement_timeout":statement, "lock_timeout":lock})
			if err != nil { return err }
			if err = store("settings-",settings); err != nil { return err }
			var plan []byte
			if err = tx.QueryRow(ctx, \`EXPLAIN (FORMAT JSON) ${preview}\`, f.tenant, p.id, nil, nil, nil, nil, nil).Scan(&plan); err != nil { return err }
			if err = store("",plan); err != nil { return err }
			// The original preview samples up to ten facilities. This project has one.
			sample := dr.Bytes([]dr.Member{{Kind:"facility", ID:facility}})
			if err = tx.QueryRow(ctx, \`EXPLAIN (FORMAT JSON) ${directory}\`, f.tenant, p.id, sample, false).Scan(&plan); err != nil { return err }
			return store("directory-",plan)
		})
		must(t,err)
	}
	checkPlan("before-analyze")
	_, err := f.env.Pool.Exec(ctx, \`ANALYZE engineering_facilities, engineering_components, engineering_points,
	 engineering_channels, engineering_monitoring_sections, engineering_boreholes, engineering_structure_nodes,
	 engineering_versions, project_resource_grants, project_facility_links\`)
	must(t, err)
	checkPlan("after-analyze")
	var total, roots, points, codes, firstCode, lastCode int
	err = f.env.Pool.QueryRow(ctx, \`SELECT count(*),count(*) FILTER (WHERE g.resource_kind='facility'),
	 count(p.id),count(DISTINCT p.code),min(substring(p.code from '^LOAD-P-([0-9]+)$')::integer),
	 max(substring(p.code from '^LOAD-P-([0-9]+)$')::integer)
	 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active'
	 LEFT JOIN engineering_points p ON g.resource_kind='point' AND p.id=g.point_id AND p.tenant_id=g.tenant_id
	 AND p.facility_id=g.facility_id AND p.deleted_at IS NULL
	 WHERE g.project_id=$1 AND g.tenant_id=$2 AND g.facility_id=$3 AND g.ended_at IS NULL\`, p.id, f.tenant, facility).
		Scan(&total, &roots, &points, &codes, &firstCode, &lastCode)
	must(t,err)
	if total != end+1 || roots != 1 || points != end || codes != end || firstCode != 1 || lastCode != end {
		t.Fatalf("invalid visible fixture memberships total=%d roots=%d points=%d codes=%d first=%d last=%d", total, roots, points, codes, firstCode, lastCode)
	}
	t.Logf("constraint-checked membership fixture total=%d roots=%d points=%d committed_batches=%d", total, roots, points, batches)
}
`;
  const replacement = text.slice(0,start) + next + text.slice(end);
  assert.equal(replacement.slice(replacement.indexOf('func TestDisplayReadFiftyThousandMemberBoundary(')),text.slice(end));
  mkdirSync(traces,{recursive:true,mode:0o700});
  const target = join(overlayDir,'display-membership-fixture.go');
  writeFileSync(target,replacement,{mode:0o600});
  replacements[source] = target;
  return { originalHelperSHA256:digest(helper), installedHelperSHA256:digest(next), originalAssertionsSHA256:digest(text.slice(end)),
    insertSHA256:digest(insert), previewSQLSHA256:digest(preview), directorySQLSHA256:digest(directory),
    planner, committedBatchSize:500, constraints:'original schema', traces };
}
