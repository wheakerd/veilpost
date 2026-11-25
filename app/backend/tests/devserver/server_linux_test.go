package devserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wheakerd/veilpost/app/backend/internal/devserver"
)

const fixtureMode = "VEILPOST_DEV_TEST_MODE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fixtureMode); mode != "" {
		if err := serveFixture(mode); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func serveFixture(mode string) error {
	listener, err := net.Listen("tcp", os.Getenv("VPN_HTTP_ADDR"))
	if err != nil {
		return err
	}
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)
	pid := os.Getpid()
	if mode == "stale-listen-file" {
		pid--
	}
	data, err := json.Marshal(map[string]any{"pid": pid, "ip": address.IP.String(), "port": address.Port})
	if err != nil {
		return err
	}
	announcement := os.Args[2]
	if err := os.WriteFile(announcement+".tmp", data, 0o600); err != nil {
		return err
	}
	for line := 1; line <= 65; line++ {
		fmt.Printf("fixture: %d\n", line)
	}
	if err := os.Rename(announcement+".tmp", announcement); err != nil {
		return err
	}
	handler := http.NewServeMux()
	handler.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"status":"ok","service":"veilpost"}`)
	})
	return http.Serve(listener, handler)
}

type fixture struct {
	server    *devserver.Server
	output    *bytes.Buffer
	stateFile string
}

func newFixture(t *testing.T, mode string) fixture {
	t.Helper()
	t.Setenv(fixtureMode, mode)
	root := t.TempDir()
	directory := filepath.Join(root, "app/backend")
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "Makefile"), []byte("build:\n\t@:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(directory, "bin/veilpost-api")); err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	server, err := devserver.New(directory, output)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := server.Run(context.Background(), "stop", nil); err != nil {
			t.Errorf("stop fixture: %v", err)
		}
	})
	return fixture{server, output, filepath.Join(root, ".runtime/backend-dev/process.json")}
}

type record struct {
	PID        int    `json:"pid"`
	StartTicks string `json:"start_ticks"`
	Session    int    `json:"session"`
	CWD        string `json:"cwd"`
	IP         string `json:"ip,omitempty"`
	Port       int    `json:"port"`
}

func (f fixture) read(t *testing.T) record {
	t.Helper()
	data, err := os.ReadFile(f.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state record
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func (f fixture) write(t *testing.T, state record) {
	t.Helper()
	// The Python controller included an object named parameters in its records.
	data, err := json.Marshal(struct {
		record
		Parameters map[string]string `json:"parameters"`
	}{state, map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.stateFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) run(t *testing.T, action string, ready bool) {
	t.Helper()
	got, err := f.server.Run(context.Background(), action, nil)
	if got != ready || err != nil {
		t.Fatalf("%s: ready = %t, error = %v; output:\n%s", action, got, err, f.output)
	}
}

func TestBackendLifecycleAndLegacyRecord(t *testing.T) {
	f := newFixture(t, "ready")
	f.run(t, "status", false)
	f.run(t, "start", true)
	first := f.read(t)
	if first.PID <= 1 || first.Session != first.PID || first.Port <= 0 || first.IP != "127.0.0.1" {
		t.Fatalf("invalid backend process record: %+v", first)
	}
	legacy := first
	legacy.IP = ""
	f.write(t, legacy)
	f.run(t, "status", true)
	f.run(t, "start", true)
	if pid := f.read(t).PID; pid != first.PID {
		t.Fatalf("start replaced running backend: %d → %d", first.PID, pid)
	}
	f.run(t, "restart", true)
	if pid := f.read(t).PID; pid == first.PID {
		t.Fatalf("restart reused backend PID %d", pid)
	}

	f.output.Reset()
	f.run(t, "logs", true)
	lines := strings.Split(strings.TrimSuffix(f.output.String(), "\n"), "\n")
	if len(lines) != 61 || lines[0] != "backend log:" || lines[1] != "fixture: 6" || lines[60] != "fixture: 65" {
		t.Fatalf("unexpected logs: %q", lines)
	}
	f.run(t, "stop", true)
	f.run(t, "status", false)
}

func TestStaleProcessRecordLeavesBackendRunning(t *testing.T) {
	f := newFixture(t, "ready")
	f.run(t, "start", true)
	running := f.read(t)
	stale := running
	stale.StartTicks = "0"
	f.write(t, stale)
	func() {
		defer f.write(t, running)
		f.run(t, "status", false)
		f.run(t, "stop", true)
		if _, err := os.Stat(f.stateFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale record still present: %v", err)
		}
	}()
	f.run(t, "status", true)
}

func TestInvalidProcessRecordStopsBeforeSignalling(t *testing.T) {
	f := newFixture(t, "ready")
	f.run(t, "start", true)
	running := f.read(t)
	bad := running
	bad.Port = -1
	f.write(t, bad)
	func() {
		defer f.write(t, running)
		_, err := f.server.Run(context.Background(), "stop", nil)
		if err == nil || !strings.Contains(err.Error(), "invalid process port") {
			t.Fatalf("stop error = %v", err)
		}
	}()
	f.run(t, "status", true)
}

func TestStartupRejectsAnotherProcessListenFileAndCleansUp(t *testing.T) {
	f := newFixture(t, "stale-listen-file")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := f.server.Run(ctx, "start", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("start error = %v; output:\n%s", err, f.output)
	}
	if _, err := os.Stat(f.stateFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed startup left a process record: %v", err)
	}
	f.run(t, "status", false)
}

func TestStackStartupFailureRollsBackOnlyNewBackend(t *testing.T) {
	f := newFixture(t, "ready")
	failure := errors.New("frontend failed to start")
	frontend := func(_ context.Context, lock *os.File, action string) (bool, error) {
		wanted := filepath.Join(filepath.Dir(f.stateFile), "control.lock")
		if action != "start" || lock.Name() != wanted {
			t.Fatalf("frontend called with action %q and lock %q", action, lock.Name())
		}
		f.runStatusWithoutLock(t)
		return false, failure
	}
	_, err := f.server.Run(context.Background(), "start", frontend)
	if !errors.Is(err, failure) {
		t.Fatalf("stack start error = %v", err)
	}
	f.run(t, "status", false)
	f.run(t, "start", true)
	existing := f.read(t)
	_, err = f.server.Run(context.Background(), "start", frontend)
	if !errors.Is(err, failure) {
		t.Fatalf("stack start with running backend error = %v", err)
	}
	if pid := f.read(t).PID; pid != existing.PID {
		t.Fatalf("existing backend replaced: %d → %d", existing.PID, pid)
	}
	f.run(t, "status", true)
}

func (f fixture) runStatusWithoutLock(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := f.server.Run(ctx, "status", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("backend lock released before frontend finished: %v", err)
	}
}
