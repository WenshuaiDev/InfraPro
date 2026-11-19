package main

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/parser/internal/server"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if run() != nil {
		slog.Error("parser stopped", "code", "parser_unavailable")
		os.Exit(1)
	}
}
func run() (resultErr error) {
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if resultErr != nil {
			reporter.Failed("configuration_invalid")
		}
		reporter.Close()
	}()
	reporter.Begin("catalog", time.Now().Add(10*time.Second))
	token, e := securefile.Read(os.Getenv("INFRAPRO_PARSER_TOKEN_FILE"), 4096, securefile.Private())
	if e != nil {
		return e
	}
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e := os.ReadFile(executable)
	if e != nil {
		return e
	}
	release := os.Getenv("INFRAPRO_PARSER_RELEASE")
	if release == "" {
		release = "local-1"
	}
	concurrency, memory := 4, 256
	if v := os.Getenv("INFRAPRO_PARSER_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 4 {
			return c.Fail("invalid_configuration")
		}
		concurrency = n
	}
	if v := os.Getenv("INFRAPRO_PARSER_MEMORY_MIB"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 64 || n > 256 {
			return c.Fail("invalid_configuration")
		}
		memory = n
	}
	s, e := server.Open(os.Getenv("INFRAPRO_PARSER_PACKAGE"), strings.TrimSpace(string(token)), release, c.Digest(binary), concurrency)
	if e != nil {
		return e
	}
	reporter.End("catalog", "", time.Time{})
	s.MemoryLimit = strconv.Itoa(memory) + "MiB"
	s.Reporter = reporter
	addr := os.Getenv("INFRAPRO_PARSER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8091"
	}
	l, e := net.Listen("tcp", addr)
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	h := &http.Server{Handler: s, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	cert, key := os.Getenv("INFRAPRO_PARSER_TLS_CERT"), os.Getenv("INFRAPRO_PARSER_TLS_KEY")
	if (cert == "") != (key == "") {
		return c.Fail("invalid_tls_configuration")
	}
	go func() {
		if cert != "" {
			done <- h.ServeTLS(l, cert, key)
		} else {
			done <- h.Serve(l)
		}
	}()
	reporter.Ready()
	diagnosticreport.SetMetric(reporter, "quota.parser_slots", int64(concurrency))
	reporter.Idle("parse", time.Time{})
	select {
	case e = <-done:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		e = h.Shutdown(shutdown)
		if e != nil {
			_ = h.Close()
		}
		return e
	}
}
