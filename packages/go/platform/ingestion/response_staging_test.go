package ingestion

import (
	"context"
	"errors"
	"io"
	"testing"

	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestResponseStagingFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cause     error
		code      string
		retryable bool
	}{
		{"verified", nil, "", false},
		{"statement timeout", &pgconn.PgError{Code: "57014"}, "storage_unavailable", true},
		{"lock timeout", &pgconn.PgError{Code: "55P03"}, "storage_unavailable", true},
		{"serialization", &pgconn.PgError{Code: "40001"}, "storage_unavailable", true},
		{"lost session", &pgconn.PgError{Code: "08006"}, "storage_unavailable", true},
		{"unknown commit", &auditrecord.CommitUnknown{Cause: io.EOF}, "storage_unavailable", true},
		{"client timeout", context.DeadlineExceeded, "storage_unavailable", true},
		{"object dependency", c.Fail("storage_unavailable"), "storage_unavailable", true},
		{"missing grant", &pgconn.PgError{Code: "42501"}, "dependency_unavailable", false},
		{"constraint violation", &pgconn.PgError{Code: "23514"}, "dependency_unavailable", false},
		{"budget", c.Fail("capacity_exceeded"), "capacity_exceeded", false},
		{"invalid immutable bytes", c.Fail("integrity_failed"), "integrity_failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := responseStagingError(tc.cause)
			if code(err) != tc.code || retryableAttempt(err) != tc.retryable || !errors.Is(err, tc.cause) {
				t.Fatalf("cause=%v code=%s retryable=%v", err, code(err), retryableAttempt(err))
			}
		})
	}
}
