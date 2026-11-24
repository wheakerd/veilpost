package agent_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/wheakerd/veilpost/app/agent/internal/agent"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testToken = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"

// Must equal the vector in app/backend/internal/nodes/service_test.go.
func TestSignatureKnownAnswer(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	key := ed25519.NewKeyFromSeed(seed)
	message := agent.RequestMessage("POST", "/api/agent/poll", "AAAAAAAAAAAAAAAAAAAAAA", "1790000000", "bm9uY2Utbm9uY2Utbm9uY2U", []byte(`{"agentVersion":"1.0.0","configVersion":0}`))
	if got := hex.EncodeToString(ed25519.Sign(key, message)); got != "d0802d4a9b45eb87ef4740691ae9bba769874d391b7c107f31a44640ecf8993fc1ae3efcb8092369fea18fbfe55c379f268b02952b42bf9c294affb313a8f907" {
		t.Fatalf("request signature differs from the console: %s", got)
	}
	enroll := hex.EncodeToString(ed25519.Sign(key, agent.EnrollMessage("token-token-token-token-token-token-token-t", "public-key")))
	if enroll != "8a0ba83439b4f957d6db31b62f3b7df83d2a500abd54c86a28ee9d59a59671888b35a26cd53ad49c1878d406082b703dc4824dc1810cd6e0da3fd9ef90c9ec0c" {
		t.Fatalf("enroll signature differs from the console: %s", enroll)
	}
}

func TestServerURLAndToken(t *testing.T) {
	for raw, want := range map[string]string{
		"https://console.example/":       "https://console.example",
		"https://console.example/prefix": "https://console.example/prefix",
		"http://127.0.0.1:3000":          "http://127.0.0.1:3000",
		"http://localhost:3000":          "http://localhost:3000",
		"http://[::1]:3000":              "http://[::1]:3000",
	} {
		if got, err := agent.ServerURL(raw); err != nil || got != want {
			t.Fatalf("%s: got %q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"http://console.example", "ftp://console.example", "https://user@console.example", "https://console.example/?a=1", "console.example", ""} {
		if _, err := agent.ServerURL(raw); err == nil {
			t.Fatalf("%s accepted", raw)
		}
	}
	if token, err := agent.ReadToken(strings.NewReader(" " + testToken + " \r\n")); err != nil || token != testToken {
		t.Fatalf("token: %q %v", token, err)
	}
	for _, input := range []string{"", "\n", "short\n", strings.Repeat("A", 300) + "\n"} {
		if _, err := agent.ReadToken(strings.NewReader(input)); err == nil {
			t.Fatalf("token %q accepted", input)
		}
	}
}

type console struct {
	*httptest.Server
	mu        sync.Mutex
	public    ed25519.PublicKey
	nodeID    string
	enrollErr int
	polls     []http.Header
	answers   []func(http.ResponseWriter)
}

func newConsole(t *testing.T) *console {
	c := &console{nodeID: "NODEIDNODEIDNODEIDNODE"}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		defer c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case agent.EnrollRoute:
			if c.enrollErr != 0 {
				w.WriteHeader(c.enrollErr)
				w.Write([]byte(`{"code":"INVALID_ENROLLMENT"}`))
				return
			}
			var input map[string]string
			json.Unmarshal(body, &input)
			key, _ := base64.RawURLEncoding.DecodeString(input["publicKey"])
			proof, _ := base64.RawURLEncoding.DecodeString(input["proof"])
			if input["token"] != testToken || len(key) != 32 || !ed25519.Verify(key, agent.EnrollMessage(input["token"], input["publicKey"]), proof) {
				w.WriteHeader(400)
				w.Write([]byte(`{"code":"REQUEST_FAILED"}`))
				return
			}
			c.public = key
			w.Write([]byte(`{"nodeId":"` + c.nodeID + `","name":"edge-1"}`))
		case agent.PollRoute:
			signature, _ := base64.RawURLEncoding.DecodeString(r.Header.Get(agent.HeaderSignature))
			message := agent.RequestMessage(r.Method, r.URL.Path, r.Header.Get(agent.HeaderNode), r.Header.Get(agent.HeaderTimestamp), r.Header.Get(agent.HeaderNonce), body)
			if r.Header.Get(agent.HeaderNode) != c.nodeID || !ed25519.Verify(c.public, message, signature) {
				w.WriteHeader(401)
				w.Write([]byte(`{"code":"AGENT_UNAUTHORIZED"}`))
				return
			}
			c.polls = append(c.polls, r.Header.Clone())
			if len(c.answers) > 0 {
				answer := c.answers[0]
				c.answers = c.answers[1:]
				answer(w)
				return
			}
			w.WriteHeader(403)
			w.Write([]byte(`{"code":"NODE_REVOKED"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(c.Close)
	return c
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestEnrollStoresPrivateKeyAndConfig(t *testing.T) {
	server := newConsole(t)
	dir := filepath.Join(t.TempDir(), "state")
	config, err := agent.Enroll(context.Background(), dir, server.URL, testToken, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if config.NodeID != server.nodeID || config.Server != server.URL || config.Name != "edge-1" {
		t.Fatalf("unexpected config %+v", config)
	}
	if mode(t, dir) != 0o700 || mode(t, filepath.Join(dir, agent.KeyFile)) != 0o600 || mode(t, filepath.Join(dir, agent.ConfigFile)) != 0o600 {
		t.Fatal("state permissions are too open")
	}
	loaded, key, err := agent.Load(dir)
	if err != nil || loaded != config || !key.Public().(ed25519.PublicKey).Equal(server.public) {
		t.Fatalf("load: %v %+v", err, loaded)
	}
	data, _ := os.ReadFile(filepath.Join(dir, agent.ConfigFile))
	if strings.Contains(string(data), testToken) {
		t.Fatal("token persisted")
	}
	// An existing identity is never overwritten.
	if _, err := agent.Enroll(context.Background(), dir, server.URL, testToken, "1.0.0"); err == nil {
		t.Fatal("second enrollment overwrote the identity")
	}
	// Group-readable keys are refused.
	os.Chmod(filepath.Join(dir, agent.KeyFile), 0o640)
	if _, _, err := agent.Load(dir); err == nil {
		t.Fatal("group-readable key accepted")
	}
	os.Chmod(filepath.Join(dir, agent.KeyFile), 0o600)
	os.Chmod(dir, 0o750)
	if _, _, err := agent.Load(dir); err == nil {
		t.Fatal("group-accessible state directory accepted")
	}
}

func TestFailedEnrollmentRemovesKey(t *testing.T) {
	server := newConsole(t)
	server.enrollErr = 401
	dir := filepath.Join(t.TempDir(), "state")
	_, err := agent.Enroll(context.Background(), dir, server.URL, testToken, "1.0.0")
	var api *agent.APIError
	if !errors.As(err, &api) || api.Code != "INVALID_ENROLLMENT" {
		t.Fatalf("got %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("failed enrollment left files: %v", entries)
	}
	if _, err := agent.Enroll(context.Background(), dir, "http://console.example", testToken, "1.0.0"); err == nil {
		t.Fatal("plain HTTP to a remote console accepted")
	}
}

func TestRunSignsPollsBacksOffAndStopsWhenRevoked(t *testing.T) {
	server := newConsole(t)
	dir := filepath.Join(t.TempDir(), "state")
	config, err := agent.Enroll(context.Background(), dir, server.URL, testToken, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := agent.Load(dir)
	ok := func(w http.ResponseWriter) { w.Write([]byte(`{"configVersion":0}`)) }
	server.answers = []func(http.ResponseWriter){
		ok,
		func(w http.ResponseWriter) { w.WriteHeader(401); w.Write([]byte(`{"code":"CLOCK_SKEW"}`)) },
		func(w http.ResponseWriter) { w.WriteHeader(502) },
		ok,
	}
	var waits []time.Duration
	runner := agent.NewRunner(config, key, "1.0.0", slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	runner.MinInterval = 0
	if err := runner.Run(context.Background()); !errors.Is(err, agent.ErrRevoked) {
		t.Fatalf("got %v, want ErrRevoked", err)
	}
	if len(server.polls) != 5 || len(waits) != 2 || waits[0] < 4*time.Second || waits[1] <= waits[0]*5/8 {
		t.Fatalf("polls %d, waits %v", len(server.polls), waits)
	}
	nonces := map[string]bool{}
	for _, header := range server.polls {
		nonces[header.Get(agent.HeaderNonce)] = true
	}
	if len(nonces) != len(server.polls) {
		t.Fatal("nonce reused")
	}
	// Cancellation stops the loop without an error.
	server.answers = []func(http.ResponseWriter){ok}
	ctx, cancel := context.WithCancel(context.Background())
	runner.Sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	runner.MinInterval = time.Hour
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("cancelled run: %v", err)
	}
}

func TestBackoffGrowsAndIsBounded(t *testing.T) {
	for failures, limits := range map[int][2]time.Duration{
		1: {4 * time.Second, 6 * time.Second}, 2: {8 * time.Second, 12 * time.Second}, 20: {4 * time.Minute, 6 * time.Minute},
	} {
		for range 20 {
			if got := agent.Backoff(failures); got < limits[0] || got > limits[1] {
				t.Fatalf("backoff(%d) = %v", failures, got)
			}
		}
	}
}
