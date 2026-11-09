// infrapro is the project's unified operations entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/WenshuaiDev/InfraPro/tools/infrapro/internal/command"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := command.New(os.Stdin, os.Stdout, os.Stderr)
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "操作失败：", err)
		os.Exit(1)
	}
}
