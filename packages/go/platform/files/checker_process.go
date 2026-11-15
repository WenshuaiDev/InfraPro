package files

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/contentcheck"
	"github.com/google/uuid"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CheckerOptions struct {
	Directory, Socket, Executable string
	ChildArguments                []string
	Timeout                       time.Duration
	MemoryBytes                   int64
}
type limitedOutput struct {
	bytes.Buffer
	limit int
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if w.Len()+len(p) > w.limit {
		return 0, fmt.Errorf("checker response exceeded limit")
	}
	return w.Buffer.Write(p)
}

// RunChecker serves only Unix peers with the same OS identity. Each inspection
// is a new credential-free child, with an independent deadline and bounded IO.
func RunChecker(ctx context.Context, o CheckerOptions, reporters ...diagnosticreport.Sink) error {
	reporter := diagnosticreport.Optional(reporters)
	if o.Timeout <= 0 || o.Timeout > 15*time.Second || o.MemoryBytes < 32<<20 || o.MemoryBytes > 256<<20 {
		return fmt.Errorf("invalid checker budgets")
	}
	root, e := filepath.Abs(o.Directory)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return e
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return e
	}
	socket, e := filepath.Abs(o.Socket)
	if e != nil {
		return e
	}
	socketDir, e := filepath.EvalSymlinks(filepath.Dir(socket))
	if e != nil {
		return e
	}
	socket = filepath.Join(socketDir, filepath.Base(socket))
	if filepath.Dir(socket) != root {
		return fmt.Errorf("checker socket must be in private temporary directory")
	}
	if st, e := os.Lstat(socket); e == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("checker socket path occupied")
		}
		conn, err := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return fmt.Errorf("checker already running")
		}
		if e = os.Remove(socket); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	listener, e := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if e != nil {
		return e
	}
	defer listener.Close()
	defer os.Remove(socket)
	if e = os.Chmod(socket, 0600); e != nil {
		return e
	}
	reporter.Ready()
	reporter.Idle("socket", time.Time{})
	defer reporter.Stopping()
	go func() { <-ctx.Done(); listener.Close() }()
	// The listener is serial: at most one parser is active, even when callers
	// connect concurrently. Network and memory limits are also set in deployment.
	for {
		conn, e := listener.AcceptUnix()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		func() {
			defer conn.Close()
			if !checkerPeerAllowed(conn) {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(o.Timeout + time.Second))
			var in CheckRequest
			line, readErr := bufio.NewReaderSize(io.LimitReader(conn, 16385), 16385).ReadBytes('\n')
			if readErr != nil || len(line) > 16384 || strictWire(line, &in) != nil {
				return
			}

			result := CheckResult{RequestID: in.RequestID, Nonce: in.Nonce, Status: "failed", Code: "checker_invalid_request"}
			if in.Protocol != 1 || in.Operation != "validate" && in.Operation != "preview" || !validCheckerIdentity(in) {
				_ = json.NewEncoder(conn).Encode(result)
				return
			}
			input, e := json.Marshal(in)
			if e != nil || len(input) > 16384 {
				return
			}
			childCtx, cancel := context.WithTimeout(ctx, o.Timeout)
			defer cancel()
			args := append([]string{}, o.ChildArguments...)
			args = append(args, "--directory", root)
			cmd := exec.CommandContext(childCtx, o.Executable, args...)
			cmd.Env = []string{"GOMEMLIMIT=" + fmt.Sprint(o.MemoryBytes), "GOMAXPROCS=2", "TZ=UTC", "LANG=C"}
			cmd.Dir = root
			cmd.Stdin = bytes.NewReader(input)
			output := &limitedOutput{limit: 16384}
			cmd.Stdout = output
			cmd.Stderr = io.Discard
			e = cmd.Run()
			if e != nil {
				result.Code = "content_check_failed"
				if childCtx.Err() != nil {
					result.Code = "content_check_timeout"
				}
			} else {
				decoded := CheckResult{}
				if strictWire(output.Bytes(), &decoded) == nil && decoded.RequestID == in.RequestID && decoded.Nonce == in.Nonce && (decoded.Status == "ok" || decoded.Status == "failed") {
					result = decoded
				}
			}
			_ = json.NewEncoder(conn).Encode(result)
		}()
	}
}
func validCheckerIdentity(r CheckRequest) bool {
	for _, id := range []string{r.RequestID, r.JobID, r.Nonce} {
		if _, e := uuid.Parse(id); e != nil {
			return false
		}
	}
	for _, v := range []string{r.Generation, r.Fence} {
		if _, e := integer(v, 1); e != nil {
			return false
		}
	}
	return true
}

// CheckerChild is called only by the checker executable. It parses no deployment
// configuration and refuses credential-bearing environment variables.
func CheckerChild(ctx context.Context, directory string, input io.Reader, output io.Writer) error {
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.HasPrefix(name, "INFRAPRO_") || strings.Contains(name, "AWS_") || strings.Contains(name, "PASSWORD") || strings.Contains(name, "SECRET") {
			return fmt.Errorf("checker credentials forbidden")
		}
	}
	if e := restrictCheckerChild(); e != nil {
		return e
	}
	var in CheckRequest
	raw, e := io.ReadAll(io.LimitReader(input, 16385))
	if e != nil || len(raw) > 16384 {
		return fmt.Errorf("checker request limit")
	}
	if e := strictWire(raw, &in); e != nil {
		return e
	}
	if !validCheckerIdentity(in) {
		return fmt.Errorf("invalid checker identity")
	}
	result := contentcheck.Check(ctx, directory, in)
	raw, e = json.Marshal(result)
	if e != nil || len(raw) > 16384 {
		return fmt.Errorf("invalid checker result")
	}
	_, e = output.Write(raw)
	return e
}
