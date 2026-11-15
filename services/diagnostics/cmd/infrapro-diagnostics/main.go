package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/observer"
	diagnosticruntime "github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/runtime"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/web"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "diagnostics:", settings.Code(err))
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) >= 2 && os.Args[1] == "version" {
		if len(os.Args) > 3 || len(os.Args) == 3 && os.Args[2] != "--verify-assets" {
			return settings.Invalid
		}
		if len(os.Args) == 3 && !web.VerifyRelease() {
			return settings.Error("ui_not_built")
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"build_id": web.BuildID, "protocol_major": 1, "ui_ready": web.Ready()})
	}
	if len(os.Args) < 2 || !settings.OneOf(os.Args[1], "server", "observer") {
		fmt.Fprintln(os.Stderr, "Usage: infrapro-diagnostics server|observer --config <file> [--development]")
		return settings.Invalid
	}
	flags := flag.NewFlagSet("diagnostics", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("config", os.Getenv("INFRAPRO_DIAGNOSTICS_CONFIG"), "protected role configuration")
	development := flags.Bool("development", false, "allow explicit loopback development origins")
	if flags.Parse(os.Args[2:]) != nil || flags.NArg() != 0 || *file == "" {
		return settings.Invalid
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if os.Args[1] == "server" {
		s, err := diagnosticruntime.Open(*file, *development)
		if err != nil {
			return err
		}
		defer s.Close()
		return s.Run(ctx)
	}
	if *development {
		return settings.Invalid
	}
	o, err := observer.Open(*file)
	if err != nil {
		return err
	}
	defer o.Close()
	return o.Run(ctx)
}
