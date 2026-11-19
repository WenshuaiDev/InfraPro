package resources_test

import (
	"context"
	"errors"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPublishedAnalysisInputChannelQualification(t *testing.T) {
	f := resourcesSetup(t)
	project := f.project(t, "AnalysisInputRead")
	scope := res.ProjectScope(f.tenant, project.id)
	facility := f.facility(t, "ARTIFACT")
	point, err := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"name": "测点", "code": "P"})
	must(t, err)
	owner := res.Target{Kind: res.Channel, Facility: facility.ID, Point: point.ID}
	createChannel := func(code string) string {
		t.Helper()
		channel, err := f.write(t, owner, "create", map[string]any{"code": code, "name": code, "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
		must(t, err)
		return channel.ID
	}
	channel, hidden := createChannel("VISIBLE"), createChannel("HIDDEN")
	grant := f.accessInput(t, project, facility.ID, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility.ID, grant)

	tx, err := f.env.Pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	var schema string
	must(t, tx.QueryRow(ctx, "SELECT current_schema()").Scan(&schema))
	role := pgx.Identifier{"input_proof_" + uuid.NewString()}.Sanitize()
	_, err = tx.Exec(ctx, "CREATE ROLE "+role)
	must(t, err)
	_, err = tx.Exec(ctx, "GRANT USAGE ON SCHEMA "+pgx.Identifier{schema}.Sanitize()+" TO "+role)
	must(t, err)
	for _, permission := range []string{
		"SELECT(tenant_id,project_id,revision,allow_facility_create) ON project_resource_policies",
		"SELECT(id,version,status) ON projects",
		"SELECT(id,tenant_id), UPDATE(id) ON engineering_facilities",
		"SELECT(tenant_id,facility_id,kind,id,parent_kind,parent_id,deleted_at) ON engineering_resource_nodes",
		"SELECT(tenant_id,project_id,facility_id,link_id,resource_kind,channel_id,ended_at) ON project_resource_grants",
		"SELECT(id,status) ON project_facility_links",
	} {
		_, err = tx.Exec(ctx, "GRANT "+permission+" TO "+role)
		must(t, err)
	}
	subject := &access.ReadSubject{ID: uuid.NewString(), Tenant: f.tenant, Project: project.id}
	subject.Authorize = func(_ context.Context, _ pgx.Tx, op access.Operation) (access.ProjectAuthorization, error) {
		if op != access.ProjectResourceOperation("channel", "detail") {
			return access.ProjectAuthorization{}, access.ErrDenied
		}
		policy, _ := access.PolicyFor(op)
		return access.ProjectAuthorization{Permissions: policy.Permissions}, nil
	}
	actor := subject.ReadActor()
	restricted := func(t *testing.T, sub pgx.Tx) {
		t.Helper()
		_, err := sub.Exec(ctx, "SET LOCAL ROLE "+role)
		must(t, err)
	}
	t.Run("project and tenant authorization", func(t *testing.T) {
		must(t, f.srv.CheckCalculationInputChannelsTx(ctx, tx, project.actor, scope, []string{channel}))
		must(t, f.srv.CheckCalculationInputChannelsTx(ctx, tx, f.actor, res.TenantScope(f.tenant), []string{hidden, channel}))
		if err := f.srv.CheckCalculationInputChannelsTx(ctx, tx, actor, res.ProjectScope(f.tenant, uuid.NewString()), []string{channel}); !errors.Is(err, access.ErrDenied) {
			t.Fatalf("foreign project claimed the subject: %v", err)
		}
	})
	t.Run("only engineering identity columns", func(t *testing.T) {
		sub, err := tx.Begin(ctx)
		must(t, err)
		defer sub.Rollback(ctx)
		restricted(t, sub)
		must(t, f.srv.CheckCalculationInputChannelsTx(ctx, sub, actor, scope, []string{channel, channel}))
		code(t, f.srv.CheckCalculationInputChannelsTx(ctx, sub, actor, scope, []string{channel, hidden}), "resource_not_found")
		_, err = f.srv.CalculationTargetsTx(ctx, sub, actor, scope, []res.Target{{Kind: res.Channel, ID: channel}}, false)
		var denial *pgconn.PgError
		if !errors.As(err, &denial) || denial.Code != "42501" {
			t.Fatalf("full engineering metadata must remain unreadable, got %v", err)
		}
	})
	for _, tc := range []struct {
		name, mutate string
		args         []any
		allowed      bool
	}{
		{"retired ancestor", "UPDATE engineering_points SET status='retired' WHERE id=$1", []any{point.ID}, true},
		{"retired channel", "UPDATE engineering_channels SET status='retired' WHERE id=$1", []any{channel}, true},
		{"ended direct grant", "UPDATE project_resource_grants SET ended_at=now(),ended_by=$2,end_reason='资格回归' WHERE channel_id=$1", []any{channel, f.actor.ID}, false},
		{"revoked facility link", "UPDATE project_facility_links SET status='revoked',revoked_at=now(),revoked_by=$2,revoke_reason='资格回归' WHERE project_id=$1", []any{project.id, f.actor.ID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := tx.Begin(ctx)
			must(t, err)
			defer sub.Rollback(ctx)
			_, err = sub.Exec(ctx, tc.mutate, tc.args...)
			must(t, err)
			restricted(t, sub)
			err = f.srv.CheckCalculationInputChannelsTx(ctx, sub, actor, scope, []string{channel})
			if tc.allowed {
				must(t, err)
			} else {
				code(t, err, "resource_not_found")
			}
		})
	}
	t.Run("depth, cycle and same facility", func(t *testing.T) {
		sub, err := tx.Begin(ctx)
		must(t, err)
		defer sub.Rollback(ctx)
		// These malformed edge cases cannot be installed through the guarded
		// structure writer. Copy only this fixture's real keys to a temporary
		// projection, keeping its real authorization, facility lock and grant.
		_, err = sub.Exec(ctx, `CREATE TEMP TABLE engineering_resource_nodes ON COMMIT DROP AS
			SELECT tenant_id,facility_id,kind,id,parent_kind,parent_id,deleted_at FROM `+pgx.Identifier{schema, "engineering_resource_nodes"}.Sanitize())
		must(t, err)
		_, err = sub.Exec(ctx, "CREATE INDEX ON engineering_resource_nodes (tenant_id,facility_id,kind,id)")
		must(t, err)
		_, err = sub.Exec(ctx, `INSERT INTO engineering_resource_nodes (tenant_id,facility_id,kind,id,parent_kind,parent_id)
			SELECT $1,$2,'component',md5(n::text)::uuid,CASE WHEN n=1 THEN 'facility' ELSE 'component' END,
			CASE WHEN n=1 THEN $2::uuid ELSE md5((n-1)::text)::uuid END FROM generate_series(1,512) n`, f.tenant, facility.ID)
		must(t, err)
		_, err = sub.Exec(ctx, "GRANT SELECT ON pg_temp.engineering_resource_nodes TO "+role)
		must(t, err)
		for _, tc := range []struct {
			name, sql string
			args      []any
			allowed   bool
		}{
			{"512 structural levels plus channel", "UPDATE pg_temp.engineering_resource_nodes SET parent_kind='component',parent_id=md5('511')::uuid WHERE id=$1", []any{point.ID}, true},
			{"depth exceeds budget", "UPDATE pg_temp.engineering_resource_nodes SET parent_kind='component',parent_id=md5('512')::uuid WHERE id=$1", []any{point.ID}, false},
			{"cyclic point", "UPDATE pg_temp.engineering_resource_nodes SET parent_kind='point',parent_id=id WHERE id=$1", []any{point.ID}, false},
			{"missing ancestor", "UPDATE pg_temp.engineering_resource_nodes SET parent_kind='component',parent_id=gen_random_uuid() WHERE id=$1", []any{point.ID}, false},
			{"ancestor belongs to another facility", "UPDATE pg_temp.engineering_resource_nodes SET facility_id=gen_random_uuid() WHERE kind='point' AND id=$1", []any{point.ID}, false},
			{"deleted ancestor", "UPDATE pg_temp.engineering_resource_nodes SET deleted_at=now() WHERE id=$1", []any{point.ID}, false},
			{"deleted channel", "UPDATE pg_temp.engineering_resource_nodes SET deleted_at=now() WHERE id=$1", []any{channel}, false},
			{"deleted facility", "UPDATE pg_temp.engineering_resource_nodes SET deleted_at=now() WHERE id=$1", []any{facility.ID}, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				check, err := sub.Begin(ctx)
				must(t, err)
				defer check.Rollback(ctx)
				_, err = check.Exec(ctx, tc.sql, tc.args...)
				must(t, err)
				restricted(t, check)
				err = f.srv.CheckCalculationInputChannelsTx(ctx, check, actor, scope, []string{channel})
				if tc.allowed {
					must(t, err)
				} else {
					code(t, err, "resource_not_found")
				}
			})
		}
	})
	t.Run("facility remains locked until caller ends transaction", func(t *testing.T) {
		must(t, f.srv.CheckCalculationInputChannelsTx(ctx, tx, project.actor, scope, []string{channel}))
		writer, err := f.env.Pool.Begin(ctx)
		must(t, err)
		defer writer.Rollback(ctx)
		var id string
		err = writer.QueryRow(ctx, "SELECT id::text FROM engineering_facilities WHERE id=$1 FOR UPDATE NOWAIT", facility.ID).Scan(&id)
		var busy *pgconn.PgError
		if !errors.As(err, &busy) || busy.Code != "55P03" {
			t.Fatalf("concurrent facility writer should be locked out: %v", err)
		}
	})
}
