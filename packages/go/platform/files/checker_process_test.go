package files

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real subprocess probe (not the parser) verifies supervision and environment isolation.
func TestCheckerProcessHelper(t *testing.T) {
	var mode string
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "probe=") {
			mode = strings.TrimPrefix(arg, "probe=")
		}
	}
	if mode == "" {
		return
	}
	if mode == "hang" {
		time.Sleep(10 * time.Second)
		os.Exit(2)
	}
	if mode == "crash" {
		os.Exit(9)
	}
	var in CheckRequest
	if json.NewDecoder(os.Stdin).Decode(&in) != nil {
		os.Exit(4)
	}
	for _, key := range []string{"INFRAPRO_DATABASE_URL", "AWS_SECRET_ACCESS_KEY", "PROBE_SECRET"} {
		if os.Getenv(key) != "" {
			os.Exit(5)
		}
	}
	result := CheckResult{RequestID: in.RequestID, Nonce: in.Nonce, Status: "ok"}
	if mode == "mismatch" {
		result.Nonce = uuid.NewString()
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(6)
	}
	os.Exit(0)
}
func TestCheckerSupervisionTimeoutCrashIdentityAndCleanEnvironment(t *testing.T) {
	t.Setenv("INFRAPRO_DATABASE_URL", "synthetic-not-a-real-credential")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "synthetic")
	t.Setenv("PROBE_SECRET", "synthetic")
	for _, mode := range []string{"ok", "hang", "crash", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			dir, e := os.MkdirTemp("", "fc-probe-")
			if e != nil {
				t.Fatal(e)
			}
			defer os.RemoveAll(dir)
			socket := filepath.Join(dir, "checker.sock")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- RunChecker(ctx, CheckerOptions{Directory: dir, Socket: socket, Executable: os.Args[0], ChildArguments: []string{"-test.run=^TestCheckerProcessHelper$", "--", "probe=" + mode}, Timeout: 1500 * time.Millisecond, MemoryBytes: 256 << 20})
			}()
			ready := false
			for i := 0; i < 100; i++ {
				c, e := net.DialTimeout("unix", socket, 20*time.Millisecond)
				if e == nil {
					c.Close()
					ready = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !ready {
				t.Fatal("checker unavailable")
			}
			in := CheckRequest{Protocol: 1, Operation: "validate", RequestID: uuid.NewString(), JobID: uuid.NewString(), Nonce: uuid.NewString(), Generation: "1", Fence: "1"}
			result, e := (SocketChecker{Socket: socket, Timeout: 3 * time.Second}).Check(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "ok":
				if result.Status != "ok" {
					t.Fatal(result)
				}
			case "hang":
				if result.Code != "content_check_timeout" {
					t.Fatal(result)
				}
			case "crash":
				if result.Code != "content_check_failed" {
					t.Fatal(result)
				}
			case "mismatch":
				if result.Code != "checker_invalid_request" {
					t.Fatal(result)
				}
			}
			cancel()
			if e = <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestCheckerCredentialGuardBeforeParsing(t *testing.T) {
	t.Setenv("INFRAPRO_DATABASE_URL", "synthetic")
	if e := CheckerChild(context.Background(), t.TempDir(), strings.NewReader("{}"), io.Discard); e == nil {
		t.Fatal("checker accepted deployment credentials")
	}
}
