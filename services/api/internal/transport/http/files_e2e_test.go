package http_test

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Each real-browser run owns a dedicated bucket, schema and checker directory.
func browserFiles(t *testing.T, ctx context.Context, env *testsupport.Environment) func() {
	t.Helper()
	if os.Getenv("INFRAPRO_FILES_E2E") != "1" {
		return func() {}
	}
	directory, e := os.MkdirTemp("", "fa-http-")
	if e != nil {
		t.Fatal(e)
	}
	o := env.Config.Files
	if o.MaxFileBytes == 0 {
		o = config.DefaultFiles()
	}
	o.TempDirectory = directory
	o.CheckerSocket = filepath.Join(directory, "checker.sock")
	o.ObjectEndpoint = "http://127.0.0.1:58333"
	o.ObjectAccessKey = "contract-admin"
	o.ObjectSecretKey = "contract-admin-test-only"
	o.Bucket = "files-browser-" + uuid.NewString()
	env.Config.Files = o
	if path := os.Getenv("INFRAPRO_E2E_OPS_CONFIG"); path != "" && os.Getenv("INFRAPRO_MONITORING_E2E") != "1" {
		raw, err := json.Marshal(map[string]string{"database_url": env.Config.DatabaseURL})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(path) })
	}
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4(o.ObjectAccessKey, o.ObjectSecretKey, ""), BucketLookup: minio.BucketLookupPath})
	if e != nil {
		t.Fatal(e)
	}
	if e = client.MakeBucket(ctx, o.Bucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(os.Getenv("INFRAPRO_FILES_CHECKER_EXECUTABLE"), "--directory", directory, "--socket", o.CheckerSocket)
	cmd.Env = []string{"GOMEMLIMIT=256MiB", "GOMAXPROCS=2"}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	ready := false
	for i := 0; i < 100; i++ {
		c, e := net.DialTimeout("unix", o.CheckerSocket, 100*time.Millisecond)
		if e == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("file checker startup failed")
	}
	service := runtime.Compose(env.Service.Access()).Configure(env.Config).Files
	worker, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if e := service.RunWorker(worker, "browser-files"); e != nil && worker.Err() == nil {
			t.Log("file worker", e)
		}
	}()
	return func() {
		stop()
		<-done
		cmd.Process.Kill()
		cmd.Wait()
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for o := range client.ListObjects(clean, env.Config.Files.Bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				client.RemoveObject(clean, env.Config.Files.Bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		client.RemoveBucket(clean, env.Config.Files.Bucket)
		os.RemoveAll(directory)
	}
}
