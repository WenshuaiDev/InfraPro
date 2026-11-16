package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Catalog     c.Catalog
	Directory   string
	Token       string
	Timeout     time.Duration
	MemoryLimit string
	Reporter    diagnosticreport.Sink
	slots       chan int
	reportOnce  sync.Once
	group       *diagnosticreport.Group
}

func Open(directory, token, release, build string, concurrency int) (*Server, error) {
	if len(token) < 32 || concurrency < 1 || concurrency > 4 {
		return nil, c.Fail("invalid_configuration")
	}
	catalog, e := c.VerifyPackage(directory, nil)
	if e != nil {
		return nil, e
	}
	s := &Server{Catalog: catalog, Directory: directory, Token: token, Timeout: 5 * time.Second, MemoryLimit: "256MiB", Reporter: diagnosticreport.Optional(nil), slots: make(chan int, concurrency)}
	for i := 0; i < concurrency; i++ {
		s.slots <- i
	}
	s.Catalog.ServiceRelease = release
	s.Catalog.ServiceBuildDigest = build
	s.Catalog.Limits = c.Limits{Input: c.MaxInput, Response: c.MaxResponse, Records: c.MaxRecords, Steps: c.MaxSteps, Depth: c.MaxDepth, Seconds: 5, Concurrency: concurrency}
	if e = s.Preflight(); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *Server) artifact(e c.Entry) (string, error) {
	if !c.IsDigest(e.ArtifactDigest) {
		return "", c.Fail("invalid_manifest")
	}
	path := filepath.Join(s.Directory, "artifacts", e.ArtifactDigest)
	f, err := os.Lstat(path)
	if err != nil || !f.Mode().IsRegular() || f.Mode().Perm()&0222 != 0 {
		return "", c.Fail("artifact_unavailable")
	}
	b, err := os.ReadFile(path)
	if err != nil || c.Digest(b) != e.ArtifactDigest {
		return "", c.Fail("artifact_digest_mismatch")
	}
	return path, nil
}
func (s *Server) Preflight() error {
	for _, e := range s.Catalog.Entries {
		if _, err := s.artifact(e); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == "GET" && r.URL.Path == "/healthz" {
		respond(w, 200, map[string]string{"status": "ok"})
		return
	}
	if r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Token)) != 1 {
		problem(w, 401, "service_auth_failed")
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/readyz":
		if e := s.Preflight(); e != nil {
			problem(w, 503, e.Error())
			return
		}
		respond(w, 200, map[string]any{"status": "ready", "contract_version": c.Version})
	case r.Method == "GET" && r.URL.Path == "/v1/catalog":
		respond(w, 200, s.Catalog)
	case r.Method == "POST" && r.URL.Path == "/v1/parse":
		s.parse(w, r)
	default:
		problem(w, 404, "not_found")
	}
}
func (s *Server) parse(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		problem(w, 400, "invalid_request")
		return
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, c.MaxRequest+1))
	if e != nil || len(b) > c.MaxRequest {
		problem(w, 413, "limit_exceeded")
		return
	}
	var req c.Request
	if e = c.StrictJSON(b, &req); e != nil {
		problem(w, 400, e.Error())
		return
	}
	var entry *c.Entry
	for i := range s.Catalog.Entries {
		v := &s.Catalog.Entries[i]
		if v.MethodID == req.Method.ID && v.MethodVersion == req.Method.Version {
			entry = v
			break
		}
	}
	if entry == nil {
		problem(w, 404, "method_unavailable")
		return
	}
	if _, e = c.DecodeInput(req, *entry); e != nil {
		status := 400
		if e.Error() == "version_conflict" || e.Error() == "contract_mismatch" {
			status = 409
		}
		if e.Error() == "limit_exceeded" {
			status = 413
		}
		problem(w, status, e.Error())
		return
	}
	var slot int
	select {
	case slot = <-s.slots:
		defer func() { s.slots <- slot }()
	default:
		problem(w, 429, "service_busy")
		return
	}
	path, e := s.artifact(*entry)
	if e != nil {
		problem(w, 503, e.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.Dir = s.Directory
	cmd.Env = []string{"TZ=UTC", "GOMEMLIMIT=" + s.MemoryLimit, "GOMAXPROCS=1"}
	cmd.Stdin = bytes.NewReader(b)
	buf := &boundedBuffer{max: c.MaxResponse}
	cmd.Stdout = buf
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	s.reportOnce.Do(func() { s.group = diagnosticreport.NewGroup(s.Reporter, "parse", cap(s.slots)) })
	s.group.Begin(slot, time.Now().Add(s.Timeout))
	reason := "loop_failed"
	defer func() { s.group.End(slot, reason, time.Time{}) }()
	e = cmd.Run()
	if ctx.Err() != nil {
		reason = "deadline_exceeded"
		problem(w, 504, "execution_timeout")
		return
	}
	if buf.exceeded {

		problem(w, 422, "limit_exceeded")
		return
	}
	if e != nil {

		problem(w, 500, "parser_error")
		return
	}
	var response c.Response
	if c.StrictJSON(buf.Bytes(), &response) != nil {
		problem(w, 500, "invalid_response")
		return
	}
	response.ServiceRelease = s.Catalog.ServiceRelease
	response.ServiceBuildDigest = s.Catalog.ServiceBuildDigest
	if e = c.ValidateResponse(req, *entry, response); e != nil {
		problem(w, 500, "invalid_response")
		return
	}
	reason = ""
	respond(w, 200, response)
}

type boundedBuffer struct {
	bytes.Buffer
	max      int
	exceeded bool
}

func (b *boundedBuffer) Write(v []byte) (int, error) {
	if b.Len()+len(v) > b.max {
		b.exceeded = true
		return 0, errors.New("limit_exceeded")
	}
	return b.Buffer.Write(v)
}
func respond(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code string) {
	respond(w, status, c.Error{Code: code, Message: code})
}
