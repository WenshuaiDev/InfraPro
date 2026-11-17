package http_test

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	displayhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/display"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func browserDisplay(t *testing.T, ctx context.Context, env *testsupport.Environment, storage *monitoring.Storage) func() {
	if os.Getenv("INFRAPRO_DISPLAY_E2E") != "1" {
		return func() {}
	}
	root, e := filepath.Abs("../../../../..")
	if e != nil {
		t.Fatal(e)
	}
	env.Config.Display = config.Display{RuntimeOrigin: "http://display-runtime.127.0.0.1.nip.io:15880", AssetOrigin: "http://display-assets.127.0.0.1.nip.io:15881", RuntimeAddr: "127.0.0.1:15880", AssetAddr: "127.0.0.1:15881", ShellDirectory: filepath.Join(root, "apps/display-runtime/dist"), Lease: 60 * time.Second, SwitchGrace: 15 * time.Second, MaxSessions: 100, MaxSubscriptions: 16, MaxQueries: 8, MaxInstances: 100, MaxPackageJobs: 8, MaxResultBytes: 8 << 20, MaxFileBytes: 4 << 20, PoolSize: 8}
	if os.Getenv("INFRAPRO_NOTIFICATIONS_E2E") == "1" {
		offset := 0
		if value := os.Getenv("INFRAPRO_NOTIFICATIONS_E2E_PORT_OFFSET"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 || offset > 40000 {
				t.Fatal("invalid notification test port offset")
			}
		}
		env.Config.Display.RuntimeOrigin = fmt.Sprintf("http://display-runtime.127.0.0.1.nip.io:%d", 15890+offset)
		env.Config.Display.AssetOrigin = fmt.Sprintf("http://display-assets.127.0.0.1.nip.io:%d", 15891+offset)
		env.Config.Display.RuntimeAddr = fmt.Sprintf("127.0.0.1:%d", 15890+offset)
		env.Config.Display.AssetAddr = fmt.Sprintf("127.0.0.1:%d", 15891+offset)
	}
	if os.Getenv("INFRAPRO_TASKS_E2E") == "1" {
		env.Config.Display.RuntimeOrigin = "http://display-runtime.127.0.0.1.nip.io:15870"
		env.Config.Display.AssetOrigin = "http://display-assets.127.0.0.1.nip.io:15871"
		env.Config.Display.RuntimeAddr = "127.0.0.1:15870"
		env.Config.Display.AssetAddr = "127.0.0.1:15871"
	}
	if os.Getenv("INFRAPRO_AUDIT_E2E") == "1" {
		env.Config.Display.RuntimeOrigin = "http://display-runtime.127.0.0.1.nip.io:15860"
		env.Config.Display.AssetOrigin = "http://display-assets.127.0.0.1.nip.io:15861"
		env.Config.Display.RuntimeAddr = "127.0.0.1:15860"
		env.Config.Display.AssetAddr = "127.0.0.1:15861"
	}
	for key, dest := range map[string]*string{
		"INFRAPRO_E2E_RUNTIME_ORIGIN": &env.Config.Display.RuntimeOrigin,
		"INFRAPRO_E2E_ASSET_ORIGIN":   &env.Config.Display.AssetOrigin,
		"INFRAPRO_E2E_RUNTIME_ADDR":   &env.Config.Display.RuntimeAddr,
		"INFRAPRO_E2E_ASSET_ADDR":     &env.Config.Display.AssetAddr,
	} {
		if value := os.Getenv(key); value != "" {
			*dest = value
		}
	}
	env.Config.Monitoring.ObjectEndpoint = "http://127.0.0.1:58333"
	env.Config.Monitoring.ObjectAccessKey = "contract-admin"
	env.Config.Monitoring.ObjectSecretKey = "contract-admin-test-only"
	env.Config.Monitoring.Bucket = "contract-source"
	business := runtime.Compose(env.Service.Access(), storage).Configure(env.Config)
	service := business.Displays
	servers := []*http.Server{{Addr: env.Config.Display.RuntimeAddr, Handler: displayhttp.RuntimeHandler(service), ReadHeaderTimeout: 5 * time.Second}, {Addr: env.Config.Display.AssetAddr, Handler: displayhttp.AssetHandler(service), ReadHeaderTimeout: 5 * time.Second}}
	for _, s := range servers {
		go func() {
			if e := s.ListenAndServe(); e != nil && e != http.ErrServerClosed {
				t.Error(e)
			}
		}()
	}
	worker, cancel := context.WithCancel(ctx)
	go service.RunWorker(worker, func(e error) { t.Log("display worker", e) })
	go func() { _ = business.DisplayReads.RunWorker(worker, "display-browser") }()
	return func() {
		cancel()
		stop, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		for _, s := range servers {
			_ = s.Shutdown(stop)
		}
	}
}
