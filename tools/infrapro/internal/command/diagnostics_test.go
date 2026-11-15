package command

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticsOfflineInitValidateAndNoSecretEcho(t *testing.T) {
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(parent, "config")
	var out, errs bytes.Buffer
	a := New(nil, &out, &errs)
	if e = a.Run(context.Background(), []string{"diagnostics", "config", "init", "--directory", dir, "--instance-id", "lab", "--profile", "mixed"}); e != nil {
		t.Fatal(e)
	}
	server := filepath.Join(dir, "server.json")
	if e = a.Run(context.Background(), []string{"diagnostics", "config", "validate", "--server", server}); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(server)
	if e = a.Run(context.Background(), []string{"diagnostics", "config", "init", "--directory", dir, "--instance-id", "lab"}); e == nil {
		t.Fatal("overwrote directory")
	}
	after, _ := os.ReadFile(server)
	if !bytes.Equal(before, after) {
		t.Fatal("changed existing config")
	}
	for _, args := range [][]string{{"auth", "init", "--server", server, "--username", "operator", "--password", "SENSITIVE-MARKER"}, {"auth", "init", "--server", server, "--username", "operator"}, {"auth", "set-enabled", "--server", server, "--username", "operator", "--enabled", "SENSITIVE-MARKER"}} {
		e = a.Run(context.Background(), append([]string{"diagnostics"}, args...))
		if e == nil || strings.Contains(e.Error()+out.String()+errs.String(), "SENSITIVE-MARKER") {
			t.Fatal("secret input accepted or echoed")
		}
	}
}
