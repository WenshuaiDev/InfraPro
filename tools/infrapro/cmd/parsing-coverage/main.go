// Command parsing-coverage verifies a proposed release before deployment or rollback.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "parsing coverage rejected:", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: parsing-coverage RELEASE_DIRECTORY (INFRAPRO_CONFIG selects the owning database)")
	}
	cfg, e := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return c.Fail("invalid_configuration")
	}
	pool, e := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if e != nil {
		return c.Fail("database_unavailable")
	}
	defer pool.Close()
	rows, e := pool.Query(context.Background(), `SELECT entry FROM parsing_method_versions ORDER BY method_id,method_version`)
	if e != nil {
		return c.Fail("database_unavailable")
	}
	defer rows.Close()
	required := []c.Entry{}
	for rows.Next() {
		var entry c.Entry
		if e = rows.Scan(&entry); e != nil {
			return c.Fail("invalid_registered_manifest")
		}
		required = append(required, entry)
	}
	if e = rows.Err(); e != nil {
		return c.Fail("database_unavailable")
	}
	catalog, e := c.VerifyPackage(os.Args[1], required)
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"compatible": true, "required_versions": len(required), "package_versions": len(catalog.Entries)})
}
