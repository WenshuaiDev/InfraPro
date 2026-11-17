// ingestion-lab-cleanup only deletes a completed synthetic load fixture in the
// fixed local acceptance services. It accepts no production endpoint or credentials.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "local load cleanup refused or incomplete:", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: ingestion-lab-cleanup .artifacts/ingestion/test-SESSION")
	}
	dir, e := filepath.Abs(os.Args[1])
	if e != nil {
		return e
	}
	root, e := filepath.Abs(".artifacts/ingestion")
	if e != nil {
		return e
	}
	if filepath.Dir(dir) != root || !regexp.MustCompile(`^test-[A-Za-z0-9]+$`).MatchString(filepath.Base(dir)) {
		return fmt.Errorf("an owned local ingestion test session is required")
	}
	read := func(name string, v any) error {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		return json.Unmarshal(b, v)
	}
	var done struct {
		Exit      *int `json:"exit"`
		Synthetic bool `json:"synthetic"`
	}
	if e = read("result.json", &done); e != nil {
		return e
	}
	if !done.Synthetic || done.Exit == nil || *done.Exit != 0 {
		return fmt.Errorf("test did not finish successfully")
	}
	var fixture struct {
		Synthetic bool   `json:"synthetic"`
		Schema    string `json:"schema"`
		Bucket    string `json:"bucket"`
		Container string `json:"business_container"`
	}
	if e = read("retained-fixture.json", &fixture); e != nil {
		return e
	}
	if !fixture.Synthetic || fixture.Container != "infrapro-ingestion-lab-business-1" || !regexp.MustCompile(`^identity_test_[a-z0-9]+$`).MatchString(fixture.Schema) || !regexp.MustCompile(`^parsing-[0-9a-f-]{36}$`).MatchString(fixture.Bucket) {
		return fmt.Errorf("fixture identity is invalid")
	}
	file, e := os.Open(filepath.Join(dir, "load.jsonl"))
	if e != nil {
		return e
	}
	defer file.Close()
	matched := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r map[string]any
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			continue
		}
		if r["event"] == "reconciled" {
			matched = r["receipts"] == r["expected_receipts"] && r["results"] == r["expected_results"] && r["failed"] == float64(0) && r["pending_ack"] == float64(0) && r["cross_tenant_units"] == float64(0) && r["duplicate_default_plans"] == float64(0)
		}
	}
	if e = scanner.Err(); e != nil {
		return e
	}
	if !matched {
		return fmt.Errorf("successful full reconciliation is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	pool, e := pgxpool.New(ctx, "postgres://ingestion_test:ingestion-business-test-only@127.0.0.1:56533/ingestion_test")
	if e != nil {
		return e
	}
	defer pool.Close()
	var active int
	e = pool.QueryRow(ctx, `SELECT count(DISTINCT l.pid) FROM pg_locks l JOIN pg_class c ON c.oid=l.relation JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND l.pid<>pg_backend_pid()`, fixture.Schema).Scan(&active)
	if e != nil {
		return e
	}
	if active != 0 {
		return fmt.Errorf("fixture still has active database users")
	}
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	if e != nil {
		return e
	}
	objects := client.ListObjects(ctx, fixture.Bucket, minio.ListObjectsOptions{Recursive: true})
	for failure := range client.RemoveObjects(ctx, fixture.Bucket, objects, minio.RemoveObjectsOptions{}) {
		if failure.Err != nil {
			return failure.Err
		}
	}
	if e = client.RemoveBucket(ctx, fixture.Bucket); e != nil {
		return e
	}
	if _, e = pool.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{fixture.Schema}.Sanitize()+" CASCADE"); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(map[string]any{"cleaned": true, "synthetic": true, "schema": fixture.Schema, "bucket": fixture.Bucket, "at": time.Now().UTC()}, "", "  ")
	if e = os.WriteFile(filepath.Join(dir, "cleanup.json"), b, 0600); e != nil {
		return e
	}
	fmt.Println("Removed the completed local load schema and private bucket; evidence files and cloud spool remain.")
	return nil
}
