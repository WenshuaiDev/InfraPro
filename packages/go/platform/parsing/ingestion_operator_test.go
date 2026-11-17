package parsing_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

// Exercise the shipped command, including config loading, JSON reports and exit
// status. Every invocation uses a private test config, never inherited runtime
// overrides or the local acceptance environment.
type ingestionOperatorCLI struct {
	binary string
	env    []string
}

func newIngestionOperatorCLI(t *testing.T, databaseURL, bucket string) ingestionOperatorCLI {
	t.Helper()
	directory := t.TempDir()
	binary := filepath.Join(directory, "ingestion-operator")
	build := exec.Command("go", "build", "-o", binary, "../../../../tools/infrapro/cmd/ingestion-operator")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build ingestion-operator: %v %s", err, output)
	}
	config := map[string]any{
		"database_url":    databaseURL,
		"redis_url":       os.Getenv("INFRAPRO_TEST_REDIS_URL"),
		"support_contact": "isolated operator verification",
		"csrf_auth_key":   hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
	}
	if bucket != "" {
		config["parsing"] = map[string]string{
			"object_endpoint":   "http://127.0.0.1:58333",
			"object_access_key": "contract-admin",
			"object_secret_key": "contract-admin-test-only",
			"bucket":            bucket,
		}
	}
	raw, err := json.Marshal(config)
	must(t, err)
	path := filepath.Join(directory, "runtime.json")
	must(t, os.WriteFile(path, raw, 0600))
	cli := ingestionOperatorCLI{binary: binary}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "INFRAPRO_") {
			cli.env = append(cli.env, value)
		}
	}
	cli.env = append(cli.env, "INFRAPRO_CONFIG="+path)
	return cli
}

func (cli ingestionOperatorCLI) run(t *testing.T, action, target string, success bool) []byte {
	t.Helper()
	command := exec.CommandContext(t.Context(), cli.binary, action, target)
	command.Env = cli.env
	output, err := command.CombinedOutput()
	if success {
		if err != nil {
			t.Fatalf("ingestion-operator %s: %v %s", action, err, output)
		}
	} else {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(output), "ingestion verification rejected") {
			t.Fatalf("ingestion-operator %s must exit 1 with a rejection: %v %s", action, err, output)
		}
	}
	return output
}

func operatorReport(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	must(t, err)
	must(t, json.Unmarshal(raw, target))
	info, err := os.Stat(path)
	must(t, err)
	if info.Mode().Perm() != 0600 {
		t.Fatal("operator report is not private")
	}
}

func (cli ingestionOperatorCLI) verify(t *testing.T, pkg string, objects, receipts int) {
	t.Helper()
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "manifest.json")
	cli.run(t, "manifest", manifestPath, true)
	var manifest ingestion.OperatorManifest
	operatorReport(t, manifestPath, &manifest)
	if int64(manifest.Schema) != migrations.Version || len(manifest.Objects) != objects || len(manifest.Receipts) != receipts {
		t.Fatal("CLI manifest differs from restored/current database")
	}
	var release map[string]int
	must(t, json.Unmarshal(cli.run(t, "release", pkg, true), &release))
	if release["schema"] != manifest.Schema || release["measurement_versions"] != len(manifest.Methods) || release["identification_versions"] != len(manifest.Capabilities.IdentificationMethods) {
		t.Fatal("CLI release does not describe its verified manifest", release)
	}
	objectsPath := filepath.Join(directory, "objects.json")
	cli.run(t, "objects", objectsPath, true)
	var report map[string]int
	operatorReport(t, objectsPath, &report)
	if report["objects"] != objects || report["verified"] != objects || report["missing_or_corrupt"] != 0 || report["receipts"] != receipts {
		t.Fatal("CLI object verification incomplete", report)
	}
	t.Logf("operator CLI: manifest/release/objects passed at schema %d (%d objects, %d receipts)", manifest.Schema, objects, receipts)
}

func TestIngestionOperatorEmptyInstall(t *testing.T) {
	pkg := os.Getenv("INFRAPRO_PARSER_TEST_PACKAGE")
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" || pkg == "" {
		t.Skip("isolated test database and packaged parser required")
	}
	env, err := testsupport.New(ctx, slog.Default())
	must(t, err)
	t.Cleanup(env.Close)
	var accounts int
	must(t, env.Pool.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&accounts))
	if accounts != 0 {
		t.Fatal("empty-install test must precede bootstrap")
	}
	cli := newIngestionOperatorCLI(t, env.Config.DatabaseURL, "")
	cli.verify(t, pkg, 0, 0)
	for _, version := range []int64{45, migrations.Version - 1, migrations.Version + 1} {
		t.Run(fmt.Sprintf("unsupported_%d", version), func(t *testing.T) {
			_, err := env.Pool.Exec(ctx, `UPDATE goose_db_version SET is_applied=(version_id<=$1)`, version)
			must(t, err)
			if version > migrations.Version {
				_, err = env.Pool.Exec(ctx, `INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,true)`, version)
				must(t, err)
			}
			t.Cleanup(func() {
				_, err := env.Pool.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id>$1`, migrations.Version)
				must(t, err)
				_, err = env.Pool.Exec(ctx, `UPDATE goose_db_version SET is_applied=true`)
				must(t, err)
			})
			for _, action := range []string{"manifest", "release", "objects"} {
				target := filepath.Join(t.TempDir(), "report.json")
				if action == "release" {
					target = pkg
				}
				output := cli.run(t, action, target, false)
				if !strings.Contains(string(output), "schema_incompatible") || !strings.Contains(string(output), fmt.Sprintf("expected schema %d", migrations.Version)) {
					t.Fatal("CLI omitted safe schema rejection details")
				}
				if action != "release" {
					if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("incompatible schema produced a success report")
					}
				}
			}
		})
	}
}
