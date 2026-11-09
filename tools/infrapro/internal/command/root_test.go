package command

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpAndInvalidCommandsDoNotLoadConfiguration(t *testing.T) {
	t.Setenv("INFRAPRO_CONFIG", "/nonexistent/infrapro-config.yaml")
	for _, args := range [][]string{nil, {"--help"}, {"help", "account"}, {"account"}, {"account", "--help"}, {"account", "bootstrap", "--help"}, {"database", "--help"}, {"database", "migrate", "--help"}, {"access"}, {"access", "--help"}, {"access", "recover-admin", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			app := New(nil, &out, &errOut)
			if err := app.Run(context.Background(), args); err != nil {
				t.Fatalf("help should work without configuration: %v", err)
			}
			if out.Len()+errOut.Len() == 0 {
				t.Fatal("help did not describe commands")
			}
		})
	}
	for _, args := range [][]string{{"missing"}, {"access", "missing"}, {"access", "recover-admin"}, {"access", "sync", "--skip-auth"}, {"account", "missing"}, {"database", "missing"}, {"database", "migrate", "extra"}, {"account", "bootstrap", "--password", "not-a-real-secret"}} {
		var out, errOut bytes.Buffer
		err := New(nil, &out, &errOut).Run(context.Background(), args)
		if err == nil || strings.Contains(err.Error(), "open config") {
			t.Fatalf("invalid command must fail before dependencies: %v", err)
		}
	}
}
