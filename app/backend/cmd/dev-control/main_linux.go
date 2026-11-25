package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/wheakerd/veilpost/app/backend/internal/devserver"
)

func main() {
	os.Exit(run())
}

func run() int {
	stack := flag.Bool("stack", false, "manage the backend and frontend together")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: dev-control [--stack] start|stop|restart|status|logs")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, err := devserver.New(".", os.Stdout)
	if err == nil {
		var frontend devserver.Frontend
		if *stack {
			frontend = runFrontend
		}
		var ready bool
		ready, err = server.Run(ctx, flag.Arg(0), frontend)
		if err == nil && ready {
			return 0
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
	}
	return 1
}

func runFrontend(ctx context.Context, lock *os.File, action string) (bool, error) {
	cmd := exec.CommandContext(ctx, "node", "scripts/dev-control.ts", action, "--backend-lock")
	cmd.Dir = "../frontend"
	// The frontend controller holds the shared lock on stdin until it exits.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = lock, os.Stdout, os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 20 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		var exit *exec.ExitError
		if action == "status" && errors.As(err, &exit) && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("frontend %s: %w", action, err)
	}
	return true, nil
}
