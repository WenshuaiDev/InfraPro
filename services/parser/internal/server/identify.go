package server

import (
	"bytes"
	"context"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func (s *Server) identify(w http.ResponseWriter, r *http.Request) {
	if s.Ingestion == nil {
		problem(w, 503, "ingestion_catalog_unavailable")
		return
	}
	if r.URL.RawQuery != "" || strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		problem(w, 400, "invalid_request")
		return
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, c.MaxRequest+1))
	if e != nil || len(b) > c.MaxRequest {
		problem(w, 413, "limit_exceeded")
		return
	}
	var req i.Request
	if e = c.StrictJSON(b, &req); e != nil {
		problem(w, 400, e.Error())
		return
	}
	var entry *i.Identification
	for _, m := range s.Ingestion.IdentificationMethods {
		if m.Method == req.Method {
			copy := m
			entry = &copy
			break
		}
	}
	if entry == nil {
		problem(w, 409, "method_unavailable")
		return
	}
	if _, e = i.DecodeInput(req, *entry); e != nil {
		problem(w, 400, e.Error())
		return
	}
	// At most three production calls may consume the shared four-slot parser;
	// the platform additionally coordinates per-tenant production queues.
	if !s.claimProduction() {
		problem(w, 429, "service_busy")
		return
	}
	defer s.releaseProduction()
	var slot int
	select {
	case slot = <-s.slots:
		defer func() { s.slots <- slot }()
	default:
		problem(w, 429, "service_busy")
		return
	}
	path, e := s.artifact(c.Entry{Manifest: c.Manifest{ArtifactDigest: entry.Method.ArtifactDigest}})
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
	e = cmd.Run()
	if ctx.Err() != nil {
		problem(w, 504, "execution_timeout")
		return
	}
	if e != nil || buf.exceeded {
		problem(w, 422, "invalid_response")
		return
	}
	var response i.Response
	if c.StrictJSON(buf.Bytes(), &response) != nil {
		problem(w, 422, "invalid_response")
		return
	}
	response.ServiceRelease = s.Catalog.ServiceRelease
	response.ServiceBuildDigest = s.Catalog.ServiceBuildDigest
	if i.ValidateResponse(req, *entry, response) != nil {
		problem(w, 422, "invalid_response")
		return
	}
	respond(w, 200, response)
}
