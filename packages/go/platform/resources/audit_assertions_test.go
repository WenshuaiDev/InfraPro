package resources_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Invoke after the real source journey, before its isolated schema is removed.
// No fake metadata adapters and no direct producer calls are used here.
func verifyAuditSources(t *testing.T, f resourceFixture) {
	t.Helper()
	if t.Failed() || os.Getenv("INFRAPRO_AUDIT_SOURCE_ASSERTIONS") != "1" {
		return
	}
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "audit-source-" + uuid.NewString()
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	defer func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err != nil {
				t.Error(o.Err)
				continue
			}
			if e := client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{}); e != nil {
				t.Error(e)
			}
		}
		if e := client.RemoveBucket(ctx, bucket); e != nil {
			t.Error(e)
		}
	}()
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "audit-exports")
	must(t, e)
	options := config.DefaultAudit()
	options.TempDirectory = t.TempDir()
	options.NodeID = "real-source-audit"
	s := runtime.Compose(f.env.Service.Access()).Audit.WithOptions(options).WithStorage(store)
	pc, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
	must(t, e)
	pc.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, e := c.Exec(ctx, "SET ROLE infrapro_audit_worker")
		return e
	}
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	must(t, e)
	defer pool.Close()
	worker := runtime.Compose(access.New(pool, slog.Default())).Audit.WithOptions(options).WithStorage(store)
	_, e = worker.Reconcile(ctx)
	must(t, e)
	rows, e := f.env.Pool.Query(ctx, `SELECT DISTINCT COALESCE(tenant_id::text,''),COALESCE(project_id::text,'') FROM account_security_events WHERE schema_version=1 ORDER BY 1,2`)
	must(t, e)
	scopes := []audit.Scope{}
	for rows.Next() {
		scope := audit.Scope{Kind: "platform"}
		must(t, rows.Scan(&scope.Tenant, &scope.Project))
		if scope.Tenant != "" {
			scope.Kind = "tenant"
		}
		if scope.Project != "" {
			scope.Kind = "project"
		}
		scopes = append(scopes, scope)
	}
	must(t, rows.Err())
	rows.Close()
	checked := 0
	for _, scope := range scopes {
		actor := f.fixture.actor(f.p)
		if scope.Kind != "platform" {
			actor = f.actor
		}
		if _, e = s.Capabilities(ctx, actor, scope); e != nil {
			// Try the actual recorded actors; qualification is still evaluated by the
			// public service against their current, unmodified membership and roles.
			ids, e2 := f.env.Pool.Query(ctx, `SELECT DISTINCT actor_id::text FROM account_security_events WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND actor_id IS NOT NULL`, scope.Tenant, scope.Project)
			must(t, e2)
			candidates := []string{}
			for ids.Next() {
				var id string
				must(t, ids.Scan(&id))
				candidates = append(candidates, id)
			}
			must(t, ids.Err())
			ids.Close()
			for _, id := range candidates {
				actor = access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
					var active bool
					e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active)
					if e == nil && !active {
						return access.ErrDenied
					}
					return e
				}}
				if _, e = s.Capabilities(ctx, actor, scope); e == nil {
					break
				}
			}
		}
		if e != nil {
			t.Logf("source scope retains no current audit reader (%s); current qualification correctly denied", scope.Kind)
			continue
		}
		filter := audit.Filter{From: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), To: time.Now().UTC().Format(time.RFC3339Nano), Limit: 100}
		for {
			page, e := s.Events(ctx, actor, scope, filter)
			if e != nil {
				t.Fatalf("audit source list in %+v: %+v", scope, e)
			}
			for _, event := range page.Items {
				detail, e := s.Event(ctx, actor, scope, event.ID)
				if e != nil {
					t.Fatalf("audit source detail %s in %+v: %+v", event.ID, scope, e)
				}
				for _, link := range detail.Links {
					if link.Available {
						_, e = s.Target(ctx, actor, scope, event.ID, link.ID)
						if e != nil {
							t.Fatalf("audit source target %s/%+v in %+v: %+v", event.ID, link, scope, e)
						}
					}
				}
				checked++
			}
			if page.NextCursor == "" {
				break
			}
			filter.Cursor = page.NextCursor
		}
		caps, e := s.Capabilities(ctx, actor, scope)
		must(t, e)
		workerActor := actor
		workerActor.Check = func(ctx context.Context, tx pgx.Tx, _ bool) error {
			var active bool
			err := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, actor.ID).Scan(&active)
			if err == nil && !active {
				return access.ErrDenied
			}
			return err
		}
		_, e = worker.Events(ctx, workerActor, scope, audit.Filter{Limit: 100})
		if e != nil {
			cause := e
			for errors.Unwrap(cause) != nil {
				cause = errors.Unwrap(cause)
			}
			t.Fatalf("restricted source %s metadata: %T %v", scope.Kind, cause, cause)
		}
		filter.Cursor = ""
		filter.Limit = 0
		ex, e := s.CreateExport(ctx, actor, scope, uuid.NewString(), audit.ExportInput{Filter: filter, FieldProfile: "technical", Reason: "真实来源审计与最小 SQL 角色验证", Epoch: caps.Epoch, AuthorizationRevision: caps.AuthorizationRevision, ProjectionVersion: caps.ProjectionVersion})
		must(t, e)
		_, e = worker.RunExportOnce(ctx)
		must(t, e)
		result, e := s.Export(ctx, actor, scope, ex.ID, "1")
		must(t, e)
		if result.State != "succeeded" {
			t.Fatalf("real %s source export failed: %+v", scope.Kind, result)
		}
		t.Logf("real source audit: %s events projected, exact links resolved, restricted worker CSV succeeded", scope.Kind)
	}
	if checked == 0 {
		t.Fatal("real source assertions observed no audit events")
	}
	var missing int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_receipts r WHERE r.operation NOT LIKE 'access.%' AND r.operation<>'task.control' AND NOT EXISTS(SELECT 1 FROM account_security_events e WHERE e.schema_version=1 AND e.tenant_id=r.tenant_id AND e.request_id=r.request_id::text AND e.event_type='monitoring.'||split_part(r.operation,':',1))`).Scan(&missing))
	if missing != 0 {
		t.Fatalf("%d real monitoring command receipts lack native audit facts", missing)
	}
	var events string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE(string_agg(DISTINCT event_type,','),'') FROM account_security_events WHERE schema_version=1`).Scan(&events))
	t.Logf("observed native source events: %s", strings.ReplaceAll(events, ",", ", "))
}
