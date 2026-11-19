package ingestion

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Recheck one retained, fully selected plan, including stale claims, without
// writing to the original evidence or depending on a live parser or S3.
func TestArchitectureRetainedWorkerAuthority(t *testing.T) {
	database := os.Getenv("INFRAPRO_ARCH_AUTHORITY_FIXTURE")
	if database == "" {
		t.Skip("retained acceptance fixture required")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(database)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a := access.New(pool, slog.Default())
	service := New(a, parsing.New(a)).WithOptions(Options{Enabled: true})
	var plan Plan
	var receipt Receipt
	var decision Decision
	err = pool.QueryRow(ctx, `SELECT `+planJSON+`,`+receiptJSON+`,`+decisionJSON+`
		FROM ingestion_parse_plans p JOIN ingestion_identity_decisions d ON p.decision_id=d.id 
		JOIN ingestion_receipts r ON r.id=d.receipt_id JOIN ingestion_results z ON z.plan_id=p.id LIMIT 1`).Scan(&plan, &receipt, &decision)
	if err != nil {
		t.Fatal(err)
	}
	err = service.machineTx(ctx, plan.TenantID, false, func(tx pgx.Tx) error {
		state, err := service.identityAuthority(ctx, tx, receipt, decision)
		if err != nil {
			return err
		}
		if state.source.ID != receipt.SourceID || state.grant.ID != *decision.GrantID || state.revision.Snapshot.ProtocolID != state.protocol.ID {
			t.Fatal("selected authority references changed")
		}
		if err = service.planAllowed(ctx, tx, plan); err != nil {
			return err
		}
		stale := plan
		stale.Snapshot.Grant.Generation = "0"
		if err = service.planAllowed(ctx, tx, stale); code(err) != "authorization_revoked" {
			t.Fatalf("stale terminal grant: %v", err)
		}
		stale = plan
		stale.Snapshot.SourceGrant.Generation = "0"
		if err = service.planAllowed(ctx, tx, stale); code(err) != "authorization_revoked" {
			t.Fatalf("stale source grant: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
