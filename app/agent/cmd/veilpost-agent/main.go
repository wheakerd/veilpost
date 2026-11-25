// Command veilpost-agent connects a node to Veilpost over a signed,
// node-initiated control channel.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/wheakerd/veilpost/app/agent/internal/agent"
)

var version = "dev"
var commit = "unknown"

const defaultStateDir = "/var/lib/veilpost-agent"

// Exit codes: 0 stopped by signal, 1 failure, 2 usage error, 3 node revoked.
const (
	exitFailure = 1
	exitUsage   = 2
	exitRevoked = 3
)

const usage = `Usage:
  veilpost-agent enroll --server https://console.example --token-stdin [--state-dir DIR]
  veilpost-agent run [--state-dir DIR] [--docker-sources-dir DIR]
  veilpost-agent version

enroll  Generate this node's Ed25519 key and redeem a one-time enrollment
        token read from standard input (never pass it as an argument).
run     Keep the signed control channel open and report online status.
        Exits with status 3 when the node is revoked in the console.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if !versionPattern.MatchString(version) {
		fmt.Fprintln(stderr, "invalid build version")
		return exitFailure
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	switch args[0] {
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "veilpost-agent %s (%s)\n", version, commit)
		return 0
	case "enroll":
		flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
		flags.SetOutput(stderr)
		server := flags.String("server", "", "console base URL, such as https://console.example")
		tokenStdin := flags.Bool("token-stdin", false, "read the one-time enrollment token from standard input")
		stateDir := flags.String("state-dir", defaultStateDir, "directory for the private key and node configuration")
		if flags.Parse(args[1:]) != nil || flags.NArg() > 0 || *server == "" {
			fmt.Fprint(stderr, usage)
			return exitUsage
		}
		if !*tokenStdin {
			fmt.Fprintln(stderr, "use --token-stdin; enrollment tokens must not be command-line arguments")
			return exitUsage
		}
		if _, err := agent.ServerURL(*server); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		if file, ok := stdin.(*os.File); ok {
			if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprint(stderr, "Paste the enrollment token and press Enter: ")
			}
		}
		token, err := agent.ReadToken(stdin)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		config, err := agent.Enroll(ctx, *stateDir, *server, token, version)
		if err != nil {
			fmt.Fprintln(stderr, "enrollment failed:", err)
			return exitFailure
		}
		fmt.Fprintf(stdout, "Enrolled as node %q. Start the agent with: veilpost-agent run --state-dir %s\n", config.Name, *stateDir)
		return 0
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		flags.SetOutput(stderr)
		stateDir := flags.String("state-dir", defaultStateDir, "directory for the private key and node configuration")
		dockerSourcesDir := flags.String("docker-sources-dir", "", "enable Docker Engine installation using signed APT sources from this directory")
		if flags.Parse(args[1:]) != nil || flags.NArg() > 0 {
			fmt.Fprint(stderr, usage)
			return exitUsage
		}
		config, key, err := agent.Load(*stateDir)
		if err != nil {
			logger.Error("cannot load agent state", "error", err)
			return exitFailure
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		runner := agent.NewRunner(config, key, version, logger)
		if *dockerSourcesDir != "" {
			runner.Docker, err = agent.NewDockerInstaller(*dockerSourcesDir)
			if err != nil {
				logger.Error("cannot enable Docker installation", "error", err)
				return exitFailure
			}
		}
		if err := runner.Run(ctx); errors.Is(err, agent.ErrRevoked) {
			return exitRevoked
		} else if err != nil {
			logger.Error("agent stopped", "error", err)
			return exitFailure
		}
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
}
