// Package devserver manages the repository's native development processes.
package devserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const loopback = "127.0.0.1"

var healthClient = &http.Client{
	Timeout:   time.Second,
	Transport: &http.Transport{DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// processRecord is also read by app/frontend/scripts/dev-control.ts.
type processRecord struct {
	PID        int    `json:"pid"`
	StartTicks string `json:"start_ticks"`
	Session    int    `json:"session"`
	CWD        string `json:"cwd"`
	IP         string `json:"ip,omitempty"`
	Port       *int   `json:"port,omitempty"`
}

type Server struct {
	directory string
	runtime   string
	output    io.Writer
}

// Frontend runs an action in the frontend controller with the backend lock
// inherited on stdin, while the caller retains its own descriptor.
type Frontend func(ctx context.Context, lock *os.File, action string) (ready bool, err error)

func New(backendDirectory string, output io.Writer) (*Server, error) {
	directory, err := filepath.Abs(backendDirectory)
	if err != nil {
		return nil, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	return &Server{
		directory: directory,
		runtime:   filepath.Join(directory, "../../.runtime/backend-dev"),
		output:    output,
	}, nil
}

// Run serializes backend and combined actions with Bun's frontend commands.
// A false result without an error means a status check found a service unready.
func (s *Server) Run(ctx context.Context, action string, frontend Frontend) (bool, error) {
	switch action {
	case "start", "stop", "restart", "status", "logs":
	default:
		return false, fmt.Errorf("unknown development action %q", action)
	}
	lock, err := s.lock(ctx)
	if err != nil {
		return false, fmt.Errorf("lock backend development state: %w", err)
	}
	defer lock.Close()
	if action == "stop" || action == "restart" {
		if frontend != nil {
			if _, err := frontend(ctx, lock, "stop"); err != nil {
				return false, err
			}
		}
		if err := s.stop(); err != nil {
			return false, err
		}
	}
	switch action {
	case "start", "restart":
		started, err := s.start(ctx)
		if err == nil && frontend != nil {
			_, err = frontend(ctx, lock, "start")
			if err != nil && started {
				err = errors.Join(err, s.stop())
			}
		}
		return err == nil, err
	case "status":
		ready, err := s.status(ctx)
		if err != nil || frontend == nil {
			return ready, err
		}
		frontendReady, err := frontend(ctx, lock, "status")
		return ready && frontendReady, err
	case "logs":
		if err := s.logs(); err != nil {
			return false, err
		}
		if frontend != nil {
			return frontend(ctx, lock, "logs")
		}
	}
	return true, nil
}

func (s *Server) lock(ctx context.Context) (*os.File, error) {
	if err := os.MkdirAll(s.runtime, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(s.runtime, "control.lock"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	// Closing the descriptor releases this lock once the frontend's inherited
	// descriptor has also closed.
	if err := waitForLock(ctx, int(file.Fd())); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func waitForLock(ctx context.Context, fd int) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}

func (s *Server) stateFile() string {
	return filepath.Join(s.runtime, "process.json")
}

func (s *Server) logFile() string {
	return filepath.Join(s.runtime, "server.log")
}

func (s *Server) readRecord() (*processRecord, error) {
	path := s.stateFile()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record processRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("invalid process record %s: %w", path, err)
	}
	if record.PID <= 1 {
		return nil, fmt.Errorf("invalid process record: %s", path)
	}
	if record.Port != nil && !validPort(*record.Port) {
		return nil, fmt.Errorf("invalid process port: %s", path)
	}
	if record.IP != "" {
		if _, err := netip.ParseAddr(record.IP); err != nil {
			return nil, fmt.Errorf("invalid process address: %s", path)
		}
	}
	return &record, nil
}

func (s *Server) writeRecord(record *processRecord) error {
	file, err := os.CreateTemp(s.runtime, ".process-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := errors.Join(json.NewEncoder(file).Encode(record), file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.stateFile())
}

func processIdentity(pid int) *processRecord {
	proc := filepath.Join("/proc", strconv.Itoa(pid))
	data, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return nil
	}
	// The process name is parenthesized and may contain a closing parenthesis.
	end := strings.LastIndexByte(string(data), ')')
	if end == -1 {
		return nil
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) <= 19 || fields[0] == "Z" {
		return nil
	}
	session, err := strconv.Atoi(fields[3])
	if err != nil {
		return nil
	}
	cwd, err := os.Readlink(filepath.Join(proc, "cwd"))
	if err != nil {
		return nil
	}
	return &processRecord{PID: pid, StartTicks: fields[19], Session: session, CWD: cwd}
}

func (s *Server) owns(record *processRecord) bool {
	if record == nil || record.CWD != s.directory || record.Session != record.PID {
		return false
	}
	current := processIdentity(record.PID)
	return current != nil && current.StartTicks == record.StartTicks &&
		current.Session == record.Session && current.CWD == record.CWD
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func (record *processRecord) address() string {
	host := record.IP
	if host == "" {
		host = loopback
	}
	port := 0
	if record.Port != nil {
		port = *record.Port
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func (record *processRecord) ready(ctx context.Context) bool {
	if record.Port == nil || (record.IP != "" && record.IP != loopback) {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, record.address()+"/api/health", nil)
	if err != nil {
		return false
	}
	response, err := healthClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	var health map[string]string
	return err == nil && json.Unmarshal(body, &health) == nil &&
		len(health) == 2 && health["status"] == "ok" && health["service"] == "veilpost"
}

func (s *Server) printStatus(record *processRecord, ready bool) {
	state := "NOT READY"
	if ready {
		state = "RUNNING"
	}
	fmt.Fprintf(s.output, "backend: %s %s (PID %d)\nLog: %s\n",
		state, record.address(), record.PID, s.logFile())
}

func (s *Server) status(ctx context.Context) (bool, error) {
	record, err := s.readRecord()
	if err != nil {
		return false, err
	}
	if !s.owns(record) {
		fmt.Fprintln(s.output, "backend: STOPPED")
		return false, nil
	}
	ready := record.ready(ctx)
	s.printStatus(record, ready)
	return ready, nil
}

func (s *Server) stop() error {
	record, err := s.readRecord()
	if err != nil {
		return err
	}
	return s.stopRecord(record)
}

func (s *Server) stopRecord(record *processRecord) error {
	owned := s.owns(record)
	if owned {
		for _, attempt := range []struct {
			signal syscall.Signal
			wait   time.Duration
		}{
			{syscall.SIGTERM, 12 * time.Second},
			{syscall.SIGKILL, 3 * time.Second},
		} {
			if !s.owns(record) {
				break
			}
			if err := syscall.Kill(-record.PID, attempt.signal); err != nil {
				if errors.Is(err, syscall.ESRCH) {
					break
				}
				return err
			}
			deadline := time.Now().Add(attempt.wait)
			for time.Now().Before(deadline) && s.owns(record) {
				time.Sleep(50 * time.Millisecond)
			}
		}
		if s.owns(record) {
			return errors.New("failed to stop backend; process record retained")
		}
	}
	if err := removeIfPresent(s.stateFile()); err != nil {
		return err
	}
	if owned {
		fmt.Fprintf(s.output, "backend: STOPPED (PID %d)\n", record.PID)
	} else {
		fmt.Fprintln(s.output, "backend: STOPPED (no owned process)")
	}
	return nil
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Server) start(ctx context.Context) (bool, error) {
	record, err := s.readRecord()
	if err != nil {
		return false, err
	}
	if s.owns(record) {
		if !record.ready(ctx) {
			return false, fmt.Errorf("backend is running but not ready; inspect %s", s.logFile())
		}
		s.printStatus(record, true)
		return false, nil
	}
	build := exec.CommandContext(ctx, "make", "--no-print-directory", "build")
	build.Dir, build.Stdout, build.Stderr = s.directory, s.output, s.output
	if err := build.Run(); err != nil {
		return false, fmt.Errorf("build development backend: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	announcement := filepath.Join(s.runtime, "listen.json")
	if err := removeIfPresent(announcement); err != nil {
		return false, err
	}
	log, err := os.OpenFile(s.logFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer log.Close()
	if _, err := fmt.Fprintf(log, "\n--- start %s ---\n", time.Now().Format(time.RFC3339)); err != nil {
		return false, err
	}

	cmd := exec.Command(filepath.Join(s.directory, "bin/veilpost-api"), "--listen-file", announcement)
	cmd.Dir, cmd.Stdout, cmd.Stderr = s.directory, log, log
	cmd.Env = append(os.Environ(),
		"VPN_HTTP_ADDR="+loopback+":0", "VPN_SHUTDOWN_TIMEOUT=10s", "VPN_ENV=development",
		"VPN_DATA_DIR="+filepath.Join(s.runtime, "../backend-data"),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	record = processIdentity(cmd.Process.Pid)
	if !s.owns(record) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return false, fmt.Errorf("backend exited immediately; inspect %s", s.logFile())
	}
	go func() { _ = cmd.Wait() }()

	err = s.writeRecord(record)
	if err == nil {
		err = s.waitReady(ctx, record, announcement)
	}
	if err != nil {
		return false, errors.Join(err, s.stopRecord(record))
	}
	s.printStatus(record, true)
	return true, nil
}

func (s *Server) waitReady(ctx context.Context, record *processRecord, announcement string) error {
	wait, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for wait.Err() == nil {
		if !s.owns(record) {
			return fmt.Errorf("backend exited; inspect %s", s.logFile())
		}
		if record.Port == nil {
			if err := s.discover(record, announcement); err != nil {
				return err
			}
		}
		if record.ready(wait) && s.owns(record) {
			return nil
		}
		select {
		case <-wait.Done():
		case <-ticker.C:
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("backend did not become ready; inspect %s", s.logFile())
}

func (s *Server) discover(record *processRecord, announcement string) error {
	data, err := os.ReadFile(announcement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var address struct {
		PID  int    `json:"pid"`
		IP   string `json:"ip"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(data, &address); err != nil {
		return fmt.Errorf("backend listen information: %w", err)
	}
	if address.PID != record.PID {
		return nil
	}
	if address.IP != loopback || !validPort(address.Port) {
		return errors.New("development backend must report a valid loopback address and port")
	}
	record.IP, record.Port = address.IP, &address.Port
	return s.writeRecord(record)
}

func (s *Server) logs() error {
	fmt.Fprintln(s.output, "backend log:")
	file, err := os.Open(s.logFile())
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(s.output, "backend: no log yet.")
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var lines [60]string
	count := 0
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			lines[count%len(lines)] = line
			count++
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	for index := max(0, count-len(lines)); index < count; index++ {
		if _, err := fmt.Fprint(s.output, lines[index%len(lines)]); err != nil {
			return err
		}
	}
	return nil
}
