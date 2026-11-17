package resources_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type displayReadPlanSample struct {
	SQL               string          `json:"sql"`
	Count             int             `json:"calls"`
	Plan              json.RawMessage `json:"plan"`
	TotalMilliseconds int64           `json:"total_ms"`
	MaxMilliseconds   int64           `json:"max_ms"`
	args              []any
}

type displayReadPlanQueryKey struct{}
type displayReadPlanQuery struct {
	sql     string
	started time.Time
}

type displayReadPlanTracer struct {
	sync.Mutex
	total   int
	samples map[string]*displayReadPlanSample
}

func (c *displayReadPlanTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.Lock()
	defer c.Unlock()
	c.total++
	upper := strings.ToUpper(strings.TrimSpace(data.SQL))
	if !strings.HasPrefix(upper, "SELECT ") && !strings.HasPrefix(upper, "WITH ") {
		return ctx
	}
	if strings.HasPrefix(upper, "WITH ") {
		for _, write := range []string{"INSERT INTO ", "UPDATE ", "DELETE FROM "} {
			if strings.Contains(upper, write) {
				return ctx
			}
		}
	}
	for _, relation := range []string{"project_resource_visibility", "project_device_grants", "monitoring_source_bindings", "project_spatial_grants", "project_spatial_views", "display_read_query_rows", "display_read_scope_members", "display_read_queries", "display_read_jobs"} {
		if !strings.Contains(data.SQL, relation) {
			continue
		}
		if old := c.samples[data.SQL]; old != nil {
			old.Count++
		} else {
			c.samples[data.SQL] = &displayReadPlanSample{SQL: data.SQL, Count: 1, args: append([]any{}, data.Args...)}
		}
		break
	}
	return context.WithValue(ctx, displayReadPlanQueryKey{}, displayReadPlanQuery{sql: data.SQL, started: time.Now()})
}

func (c *displayReadPlanTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	query, ok := ctx.Value(displayReadPlanQueryKey{}).(displayReadPlanQuery)
	if !ok {
		return
	}
	elapsed := time.Since(query.started).Milliseconds()
	c.Lock()
	defer c.Unlock()
	if sample := c.samples[query.sql]; sample != nil {
		sample.TotalMilliseconds += elapsed
		sample.MaxMilliseconds = max(sample.MaxMilliseconds, elapsed)
	}
}

// Optional evidence collection uses the actual SQL and parameters sent by the
// owning services, never a hand-copied approximation of their queries. EXPLAIN
// does not execute the statement; load timings remain a separate measurement.
func displayReadPlanAccess(t *testing.T, f resourceFixture, queryBudgets ...int) *access.Service {
	t.Helper()
	directory := os.Getenv("INFRAPRO_DISPLAY_READ_PLAN_REPORT")
	if directory == "" && len(queryBudgets) == 0 {
		return f.env.Service.Access()
	}
	tracer := &displayReadPlanTracer{samples: map[string]*displayReadPlanSample{}}
	config := f.env.Pool.Config()
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, config)
	must(t, err)
	t.Cleanup(pool.Close)
	started := time.Now()
	t.Cleanup(func() {
		tracer.Lock()
		total := tracer.total
		tracer.Unlock()
		for _, budget := range queryBudgets {
			if total > budget {
				t.Errorf("display qualification repeated too many SQL calls: %d > %d", total, budget)
			}
		}
		if directory == "" {
			t.Logf("display read SQL calls: %d", total)
			return
		}
		// This cleanup precedes fixture/database disposal. Use the original
		// untraced pool so evidence generation cannot capture its own queries.
		var schema string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
		rows, err := f.env.Pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=$1 ORDER BY tablename`, schema)
		must(t, err)
		tables := []string{}
		for rows.Next() {
			var table string
			must(t, rows.Scan(&table))
			tables = append(tables, table)
		}
		must(t, rows.Err())
		rows.Close()
		for _, table := range tables {
			_, err = f.env.Pool.Exec(ctx, "ANALYZE "+pgx.Identifier{schema, table}.Sanitize())
			must(t, err)
		}
		tracer.Lock()
		samples := make([]*displayReadPlanSample, 0, len(tracer.samples))
		for _, sample := range tracer.samples {
			copy := *sample
			samples = append(samples, &copy)
		}
		tracer.Unlock()
		sort.Slice(samples, func(i, j int) bool { return samples[i].SQL < samples[j].SQL })
		if len(samples) == 0 {
			t.Fatal("no production read SQL captured")
		}
		ids := regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
		for _, sample := range samples {
			var raw []byte
			err = f.env.Pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON, COSTS TRUE) "+sample.SQL, sample.args...).Scan(&raw)
			must(t, err)
			sample.Plan = json.RawMessage(ids.ReplaceAll(raw, []byte("<synthetic-id>")))
		}
		data, err := json.MarshalIndent(map[string]any{
			"passed": !t.Failed(), "test": t.Name(), "startedAt": started.UTC().Format(time.RFC3339Nano), "completedAt": time.Now().UTC().Format(time.RFC3339Nano),
			"scope":     "Actual service SQL and client-observed timings from isolated synthetic contract fixtures; ANALYZE before EXPLAIN after each scenario; plans are estimates. Parameters are not exported and fixture UUIDs are redacted.",
			"sql_calls": total, "distinct_explained_queries": len(samples), "queries": samples,
		}, "", "  ")
		must(t, err)
		must(t, os.MkdirAll(directory, 0700))
		must(t, os.WriteFile(filepath.Join(directory, t.Name()+".json"), data, 0600))
		digest := sha256.Sum256(data)
		t.Logf("query plans: %d actual SQL calls; %d explained shapes; SHA256 %s", total, len(samples), hex.EncodeToString(digest[:]))
	})
	return access.New(pool, slog.Default())
}
