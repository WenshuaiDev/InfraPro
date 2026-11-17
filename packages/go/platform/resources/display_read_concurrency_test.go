//go:build monitoringintegration

package resources_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Named channel eligibility recursively enters the raw input authorization
// seam after personnel authorization. Concurrent readers must retain shared
// identity locks, while account changes still wait for the read transaction.
func TestDisplayReadConcurrentPersonnelInputQualification(t *testing.T) {
	f := monitoringSetup(t)
	p := f.project(t, "ConcurrentDisplayPersonnel")
	var facility, grant, login string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	g := f.accessInput(t, p, facility, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, p, facility, g)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, f.channel).Scan(&grant))
	_, e := f.service.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: grant, Range: &md.Range{}, Capabilities: []string{"read"}})
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL LIMIT 1`, p.actor.ID).Scan(&login))
	delegator := access.Actor{ID: p.actor.ID, Check: func(c context.Context, tx pgx.Tx, _ bool) error {
		_, err := access.RequireProjectTx(c, tx, p.actor.ID, access.MonitoringOperation("project", "details"), f.tenant, p.id)
		return err
	}}
	work, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ready, start, results := make(chan struct{}, 2), make(chan struct{}), make(chan error, 2)
	for range 2 {
		go func() {
			results <- f.env.Service.Access().Transact(work, func(tx pgx.Tx) error {
				if _, err := tx.Exec(work, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`); err != nil {
					return err
				}
				if _, err := access.Lock(work, tx, false); err != nil {
					return err
				}
				if err := f.env.Service.CheckDisplaySession(work, tx, p.actor.ID, login); err != nil {
					return err
				}
				ready <- struct{}{}
				select {
				case <-start:
				case <-work.Done():
					return work.Err()
				}
				return f.service.AuthorizeRawInputTx(work, tx, delegator, md.ProjectScope(f.tenant, p.id), []md.RawNeed{{Channel: f.channel}}, false)
			})
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case err := <-results:
			t.Fatal("personnel authorization failed before barrier", err)
		case <-work.Done():
			t.Fatal(work.Err())
		}
	}
	writer, e := f.env.Pool.Begin(work)
	must(t, e)
	_, e = writer.Exec(work, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE NOWAIT`, p.actor.ID)
	var locked *pgconn.PgError
	if !errors.As(e, &locked) || locked.Code != "55P03" {
		t.Errorf("account mutation must wait for active read authorization: %v", e)
	}
	must(t, writer.Rollback(ctx))
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Errorf("concurrent input qualification: %v", err)
		}
	}
}
