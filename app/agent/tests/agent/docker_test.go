package agent_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wheakerd/veilpost/app/agent/internal/agent"
)

func TestDockerSourceProfilesRequireEveryStanzaToUseATrustedKey(t *testing.T) {
	const stanza = "Types: deb\nURIs: https://mirror.example.test/docker\nSuites: noble\nComponents: stable\n"
	const signed = stanza + "Signed-By: /etc/apt/keyrings/docker.asc\n"
	approved := func(path string) bool { return path == "/etc/apt/keyrings/docker.asc" }
	if !agent.SignedDockerSources(signed+"\n"+signed, approved) {
		t.Fatal("two signed repository stanzas must be accepted")
	}
	for _, input := range []string{
		stanza, signed + "\n" + stanza, signed + "Trusted: yes\n",
		stanza + " Signed-By: /etc/apt/keyrings/docker.asc\n",
		stanza + "Signed-By: /tmp/key.asc\n",
	} {
		if agent.SignedDockerSources(input, approved) {
			t.Fatalf("accepted invalid profile %q", input)
		}
	}
}

type testDockerInstaller struct{ calls atomic.Int32 }

func (*testDockerInstaller) Sources() []string { return []string{"mirror"} }

func (d *testDockerInstaller) Install(_ context.Context, source string) (string, string) {
	d.calls.Add(1)
	if source != "mirror" {
		return "failed", "source_unavailable"
	}
	return "succeeded", ""
}

func TestDockerInstallResultIsRetriedWithoutRepeatingInstallation(t *testing.T) {
	installer := &testDockerInstaller{}
	const jobID = "abcdefghijklmnopqrstuv"
	var reports atomic.Int32
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path != agent.PollRoute || req.Header.Get(agent.HeaderSignature) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct {
			Docker struct {
				Sources []string
				Result  *struct{ ID, Status string }
			}
		}
		if json.NewDecoder(req.Body).Decode(&input) != nil || len(input.Docker.Sources) != 1 || input.Docker.Sources[0] != "mirror" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if polls.Add(1) == 1 {
			io.WriteString(w, `{"configVersion":0,"dockerInstall":{"id":"`+jobID+`","source":"mirror"}}`)
			return
		}
		if result := input.Docker.Result; result != nil && result.ID == jobID && result.Status == "succeeded" {
			if reports.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		} else if reports.Load() == 2 {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"code":"NODE_REVOKED"}`)
			return
		}
		io.WriteString(w, `{"configVersion":0}`)
	}))
	defer server.Close()
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.Config{Server: server.URL, NodeID: jobID}, key, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.Docker = installer
	runner.MinInterval = time.Millisecond
	runner.Sleep = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runner.Run(ctx); !errors.Is(err, agent.ErrRevoked) || installer.calls.Load() != 1 || reports.Load() != 2 {
		t.Fatalf("run: %v, installs: %d, reports: %d", err, installer.calls.Load(), reports.Load())
	}
}
