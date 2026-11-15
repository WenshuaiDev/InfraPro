package testsupport

import (
	"context"
	"encoding/json"
	"fmt"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"time"
)

// Optional source-regression evidence: event types and counts only. No people,
// names, reasons, connection strings, payloads or content enter this artifact.
func auditCoverage(pool *pgxpool.Pool) error {
	path := os.Getenv("INFRAPRO_AUDIT_COVERAGE")
	if path == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, e := pool.Query(ctx, `SELECT event_type,count(*) FROM account_security_events WHERE schema_version=1 GROUP BY event_type ORDER BY event_type`)
	if e != nil {
		return e
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var event string
		var n int
		if e = rows.Scan(&event, &n); e != nil {
			return e
		}
		if _, known := ar.Lookup(event); !known {
			return fmt.Errorf("unregistered native audit producer: %s", event)
		}
		counts[event] = n
	}
	if e = rows.Err(); e != nil {
		return e
	}
	record, e := json.Marshal(map[string]any{"observed_at": time.Now().UTC(), "events": counts})
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write(append(record, '\n'))
	return e
}
