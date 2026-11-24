package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/apps"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
	"github.com/wheakerd/veilpost/app/backend/internal/certs"
	"github.com/wheakerd/veilpost/app/backend/internal/config"
	"github.com/wheakerd/veilpost/app/backend/internal/httpserver"
	"github.com/wheakerd/veilpost/app/backend/internal/nodes"
)

var version = "dev"
var commit = "unknown"

func main() {
	if len(os.Args) > 1 {
		if command, exists := adminCommands[os.Args[1]]; exists {
			if err := command(os.Args[2:]); err != nil {
				slog.Error(os.Args[1]+" failed", "error", err)
				os.Exit(1)
			}
			return
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if !healthy() {
			os.Exit(1)
		}
		return
	}
	showVersion := flag.Bool("version", false, "print build version and commit")
	listenFile := flag.String("listen-file", "", "write the bound IP, port and PID to an absolute JSON path")
	flag.Parse()
	if *showVersion {
		fmt.Printf("veilpost-api %s (%s)\n", version, commit)
		return
	}
	if err := run(*listenFile); err != nil {
		slog.Error("backend stopped", "error", err)
		os.Exit(1)
	}
}

func run(listenFile string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)
	gin.SetMode(gin.ReleaseMode)
	authentication, err := auth.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("initialize authentication storage: %w", err)
	}
	defer authentication.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go pruneExpired(ctx, authentication, logger)
	authHandler := &auth.Handler{Service: authentication, Development: cfg.Development, PublicOrigin: cfg.PublicOrigin, Logger: logger}
	nodeService := nodes.New(authentication)
	catalog, err := apps.LoadCatalog()
	if err != nil {
		return fmt.Errorf("load app catalog: %w", err)
	}
	// Certificate issuance and renewal run in their own scheduler; Stop
	// cancels running jobs before the database closes.
	certService := certs.New(authentication, logger)
	certService.Start(ctx)
	defer certService.Stop(cfg.ShutdownTimeout)
	server := httpserver.New(cfg.HTTPAddr, httpserver.NewRouter(logger, cfg.TrustedProxies, httpserver.Handlers{
		Auth:  authHandler,
		Nodes: &nodes.Handler{Service: nodeService, Auth: authHandler, Logger: logger},
		Apps:  &apps.Handler{Service: apps.New(authentication, catalog), Auth: authHandler, Logger: logger},
		Certs: &certs.Handler{Service: certService, Auth: authHandler, Logger: logger},
	}))
	// Answer waiting agent long polls so graceful shutdown can drain them.
	server.RegisterOnShutdown(nodeService.Shutdown)
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	if listenFile != "" {
		cleanup, err := httpserver.WriteListenInfo(listenFile, listener.Addr())
		if err != nil {
			return fmt.Errorf("write listen information: %w", err)
		}
		defer cleanup()
	}
	logger.Info("backend listening", "address", listener.Addr().String())
	if err := httpserver.Serve(ctx, server, listener, cfg.ShutdownTimeout); err != nil {
		return err
	}
	logger.Info("backend stopped")
	return nil
}

// healthy checks readiness through the configured listener, using loopback
// when the service listens on every interface.
func healthy() bool {
	cfg, err := config.Load()
	if err != nil {
		return false
	}
	host, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil || port == "0" {
		return false
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/ready")
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// pruneExpired removes expired authentication state, node enrollment tokens
// and agent nonces at startup and hourly.
func pruneExpired(ctx context.Context, service *auth.Service, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := service.Prune(ctx); err != nil && ctx.Err() == nil {
			logger.Error("expired authentication state cleanup failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Local administrator commands. Each needs access to the data directory and
// never accepts a password as an argument or environment variable.
var adminCommands = map[string]func([]string) error{
	"admin-create": func(args []string) error {
		username, password, err := adminInput("admin-create", args, true)
		if err != nil {
			return err
		}
		return withService(func(service *auth.Service) error {
			if err := service.CreateUser(context.Background(), username, password); err != nil {
				return err
			}
			fmt.Println("Account created.")
			return nil
		})
	},
	"admin-reset-password": func(args []string) error {
		username, password, err := adminInput("admin-reset-password", args, true)
		if err != nil {
			return err
		}
		return withService(func(service *auth.Service) error {
			if err := service.ResetPassword(context.Background(), username, password); err != nil {
				return err
			}
			fmt.Println("Password reset; all sessions of the account were revoked.")
			return nil
		})
	},
	"admin-reset-two-factor": func(args []string) error {
		username, _, err := adminInput("admin-reset-two-factor", args, false)
		if err != nil {
			return err
		}
		return withService(func(service *auth.Service) error {
			if err := service.ResetTwoFactor(context.Background(), username); err != nil {
				return err
			}
			fmt.Println("Two-step verification removed; all sessions of the account were revoked.")
			return nil
		})
	},
}

func adminInput(name string, args []string, needsPassword bool) (string, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	username := flags.String("username", "", "account name")
	stdin := false
	if needsPassword {
		flags.BoolVar(&stdin, "password-stdin", false, "read password from standard input")
	}
	if err := flags.Parse(args); err != nil {
		return "", "", err
	}
	if flags.NArg() > 0 {
		return "", "", fmt.Errorf("unexpected arguments")
	}
	if !needsPassword {
		return *username, "", nil
	}
	if !stdin {
		return "", "", fmt.Errorf("use --password-stdin; passwords must not be command-line arguments")
	}
	// Read at most one line of a bounded size; longer input is rejected, not truncated.
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 1025)).ReadString('\n')
	if err != nil && (line == "" || len(line) > 1024) {
		return "", "", fmt.Errorf("password input is required and must not exceed 1024 bytes")
	}
	return *username, strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func withService(action func(*auth.Service) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	service, err := auth.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer service.Close()
	return action(service)
}
