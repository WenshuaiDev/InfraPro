package files

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/contentcheck"
	"io"
	"net"
	"time"
)

type CheckRequest = contentcheck.Request
type CheckResult = contentcheck.Result
type CheckPolicy = contentcheck.Policy
type Checker interface {
	Check(context.Context, CheckRequest) (CheckResult, error)
}
type SocketChecker struct {
	Socket  string
	Timeout time.Duration
}

func (c SocketChecker) Check(ctx context.Context, in CheckRequest) (CheckResult, error) {
	var out CheckResult
	dial := net.Dialer{}
	conn, e := dial.DialContext(ctx, "unix", c.Socket)
	if e != nil {
		return out, fault("file_worker_unavailable", 503)
	}
	defer conn.Close()
	deadline := time.Now().Add(c.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if e = conn.SetDeadline(deadline); e != nil {
		return out, e
	}
	raw, e := json.Marshal(in)
	if e != nil || len(raw) > 16384 {
		return out, fmt.Errorf("checker request out of bounds")
	}
	if _, e = conn.Write(append(raw, '\n')); e != nil {
		return out, e
	}
	response, e := io.ReadAll(io.LimitReader(conn, 16385))
	if e != nil || len(response) > 16384 || strictWire(response, &out) != nil {
		return out, fault("file_worker_unavailable", 503)
	}
	if out.RequestID != in.RequestID || out.Nonce != in.Nonce || out.Status != "ok" && out.Status != "failed" {
		return CheckResult{}, fault("file_worker_unavailable", 503)
	}
	return out, nil
}
