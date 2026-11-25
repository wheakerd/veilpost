package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"
)

// Runner long-polls the console. Each poll is a signed request that reports
// the agent version and the configuration version it holds; the console
// records the request time as the node's last contact.
type Runner struct {
	Config  Config
	Key     ed25519.PrivateKey
	Version string
	Logger  *slog.Logger
	// Client must allow longer than the console's long-poll hold (25 s).
	Client *http.Client
	Now    func() time.Time
	// Sleep waits between attempts; tests replace it.
	Sleep func(context.Context, time.Duration) error
	// MinInterval spaces poll starts when the console answers immediately.
	MinInterval time.Duration
	Docker      DockerInstaller
}

func NewRunner(config Config, key ed25519.PrivateKey, version string, logger *slog.Logger) *Runner {
	return &Runner{Config: config, Key: key, Version: version, Logger: logger, Client: newClient(60 * time.Second),
		Now: time.Now, Sleep: sleep, MinInterval: 2 * time.Second}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backoff returns the wait after the given number of consecutive failures:
// 5 s doubling to 5 min, with ±20% jitter so nodes do not retry in step.
func backoff(failures int) time.Duration {
	delay := 5 * time.Second
	for i := 1; i < failures && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	delay = min(delay, 5*time.Minute)
	return time.Duration(float64(delay) * (0.8 + 0.4*rand.Float64()))
}

type dockerJob struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type dockerReport struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type pollReply struct {
	ConfigVersion *int64     `json:"configVersion"`
	DockerInstall *dockerJob `json:"dockerInstall,omitempty"`
}

// Run polls until ctx ends (returns nil) or the node is revoked (ErrRevoked).
func (r *Runner) Run(ctx context.Context) error {
	var configVersion int64
	var report *dockerReport
	var installing <-chan dockerReport
	installCtx, cancelInstall := context.WithCancel(ctx)
	defer func() {
		cancelInstall()
		if installing != nil {
			<-installing
		}
	}()
	failures := 0
	r.Logger.Info("agent started", "node", r.Config.Name, "version", r.Version)
	for ctx.Err() == nil {
		select {
		case finished := <-installing:
			report = &finished
			installing = nil
		default:
		}
		started := r.Now()
		result, err := r.poll(ctx, configVersion, report, installing != nil)
		if ctx.Err() != nil {
			return nil
		}
		var api *APIError
		switch {
		case err == nil:
			if failures > 0 {
				r.Logger.Info("console reachable again")
			}
			failures = 0
			report = nil
			if result.DockerInstall != nil && r.Docker != nil && installing == nil {
				job := *result.DockerInstall
				done := make(chan dockerReport, 1)
				installing = done
				go func() {
					status, reason := r.Docker.Install(installCtx, job.Source)
					done <- dockerReport{ID: job.ID, Status: status, Error: reason}
				}()
			}
			if *result.ConfigVersion != configVersion {
				// Configuration delivery starts in a later phase.
				r.Logger.Info("console reports a configuration version this agent does not apply yet", "configVersion", *result.ConfigVersion)
				configVersion = *result.ConfigVersion
			}
			if wait := r.MinInterval - r.Now().Sub(started); wait > 0 {
				// A cancelled wait ends the loop through ctx.Err().
				_ = r.Sleep(ctx, wait)
			}
			continue
		case errors.As(err, &api) && api.Code == "NODE_REVOKED":
			r.Logger.Error("node revoked; stopping")
			return ErrRevoked
		case errors.As(err, &api) && api.Code == "CLOCK_SKEW":
			failures++
			r.Logger.Warn("system clock differs from the console by more than 60 seconds; synchronize time (NTP)")
		case errors.As(err, &api) && api.Code == "TOO_MANY_ATTEMPTS":
			failures = max(failures+1, 4)
			r.Logger.Warn("console rate limit reached; backing off")
		case errors.As(err, &api):
			failures++
			r.Logger.Error("console rejected the request", "status", api.Status, "code", api.Code)
		default:
			failures++
			r.Logger.Warn("console unreachable", "error", err)
		}
		if r.Sleep(ctx, backoff(failures)) != nil {
			return nil
		}
	}
	return nil
}

func (r *Runner) poll(ctx context.Context, configVersion int64, report *dockerReport, installing bool) (pollReply, error) {
	type dockerStatus struct {
		Sources []string      `json:"sources"`
		Result  *dockerReport `json:"result,omitempty"`
		Busy    bool          `json:"busy,omitempty"`
	}
	input := struct {
		AgentVersion  string        `json:"agentVersion"`
		ConfigVersion int64         `json:"configVersion"`
		Docker        *dockerStatus `json:"docker,omitempty"`
	}{AgentVersion: r.Version, ConfigVersion: configVersion}
	if r.Docker != nil {
		input.Docker = &dockerStatus{Sources: r.Docker.Sources(), Result: report, Busy: installing}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return pollReply{}, err
	}
	request, err := signed(r.Config.Server, pollRoute, r.Config.NodeID, r.Key, r.Now(), body)
	if err != nil {
		return pollReply{}, err
	}
	var result pollReply
	if err := exchange(ctx, r.Client, request, &result); err != nil {
		return pollReply{}, err
	}
	if result.ConfigVersion == nil || *result.ConfigVersion < 0 ||
		result.DockerInstall != nil && (!nodeIDPattern.MatchString(result.DockerInstall.ID) ||
			!dockerSourcePattern.MatchString(result.DockerInstall.Source)) {
		return pollReply{}, &APIError{Status: 200, Code: "UNEXPECTED_RESPONSE"}
	}
	return result, nil
}
