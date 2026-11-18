package runtime_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

func TestMFAWorkersDoNotRequirePersonnelKeys(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("explicit test database required")
	}
	ctx := context.Background()
	env, err := testsupport.New(ctx, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	cfg := env.Config
	cfg.MFAKeyringFile = ""
	cfg.Display.PoolSize = 2
	worker, err := runtime.OpenWorker(ctx, cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if worker.Identity != nil || worker.Access == nil {
		t.Fatal("worker should only compose authorization")
	}
	display, err := runtime.OpenDisplay(ctx, cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer display.Close()
	if display.Identity != nil || display.Access == nil {
		t.Fatal("display runtime should verify proof without decrypting factors")
	}
	api, err := runtime.Open(ctx, cfg, slog.Default(), true)
	if err == nil {
		api.Close()
		t.Fatal("personnel API started without keys")
	}
}
