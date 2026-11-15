package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	directory := flag.String("directory", "", "private checker directory")
	socket := flag.String("socket", "", "private Unix socket")
	child := flag.Bool("child", false, "restricted content-check child")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var e error
	if *child {
		e = files.CheckerChild(ctx, *directory, os.Stdin, os.Stdout)
	} else {
		reporter := diagnosticreport.FromEnv()
		executable, err := os.Executable()
		if err != nil {
			e = err
		} else {
			e = files.RunChecker(ctx, files.CheckerOptions{Directory: *directory, Socket: *socket, Executable: executable, ChildArguments: []string{"--child"}, Timeout: 15 * time.Second, MemoryBytes: 256 << 20}, reporter)
		}
		if e != nil {
			reporter.Failed("checker_unavailable")
		}
		reporter.Close()
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "file checker failed:", e)
		os.Exit(1)
	}
}
