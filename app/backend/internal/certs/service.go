// Package certs manages DNS provider credentials and ACME certificates issued
// with DNS-01 validation. Issuance and renewal run in background jobs; the
// HTTP API only records requests and reports their state.
package certs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

const (
	// RenewBefore is how long before expiry a certificate is renewed. Short
	// lived certificates renew when a third of their lifetime remains.
	RenewBefore = 30 * 24 * time.Hour
	// CheckInterval is how often due renewals and expiry are checked.
	CheckInterval = time.Hour
	// maxBackoff caps the delay between failed renewal attempts.
	maxBackoff = 24 * time.Hour
	// verifyTimeout bounds a credential check so the browser request
	// (10 seconds) receives an answer.
	verifyTimeout = 8 * time.Second
	// workers bounds concurrent issuance jobs.
	workers = 2
)

type Service struct {
	auth   *auth.Service
	db     *sql.DB
	logger *slog.Logger
	now    func() time.Time

	checkInterval time.Duration
	// Test seams. Production uses real DNS providers, the system resolver
	// and the public ACME directories.
	acmeTransport    http.RoundTripper
	newProvider      func(provider string, secrets map[string]string) (challenge.Provider, error)
	verifyCredential func(ctx context.Context, provider string, secrets map[string]string) error
	challengeOptions []dns01.ChallengeOption
	skipPropagation  bool
	cloudflareURL    string

	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	jobs    map[string]context.CancelFunc
	slots   chan struct{}
	running sync.WaitGroup
	stopped chan struct{}
}

func New(authentication *auth.Service, logger *slog.Logger) *Service {
	s := &Service{
		auth: authentication, db: authentication.Database(), logger: logger, now: time.Now,
		checkInterval: CheckInterval, cloudflareURL: cloudflareAPI,
		jobs: map[string]context.CancelFunc{}, slots: make(chan struct{}, workers),
	}
	s.newProvider = s.dnsProvider
	s.verifyCredential = s.verify
	return s
}

// Start runs the renewal scheduler until ctx ends or Stop is called: it
// checks at once (resuming requests interrupted by a restart) and then every
// check interval.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.stopped = make(chan struct{})
	go s.loop(s.ctx)
}

// Stop cancels running jobs and waits up to timeout for them to return.
// Interrupted requests keep their state and resume at the next start.
func (s *Service) Stop(timeout time.Duration) {
	s.mu.Lock()
	cancel, stopped := s.cancel, s.stopped
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	done := make(chan struct{})
	go func() {
		<-stopped
		s.running.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		s.logger.Warn("certificate jobs did not stop in time")
	}
}

func (s *Service) loop(ctx context.Context) {
	defer close(s.stopped)
	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()
	for {
		if err := s.Check(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("certificate renewal check failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Check marks expired certificates and queues every request or renewal
// that is due.
func (s *Service) Check(ctx context.Context) error {
	now := s.now().UnixMilli()
	if _, err := s.db.ExecContext(ctx, "UPDATE certificates SET status='expired',updated_at=? WHERE status='valid' AND not_after<=?", now, now); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM certificates WHERE renew_at IS NOT NULL AND renew_at<=? ORDER BY renew_at", now)
	if err != nil {
		return err
	}
	var due []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		due = append(due, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range due {
		s.enqueue(id)
	}
	return nil
}

// enqueue starts a job for a certificate unless one is queued or running.
// Before Start, requests wait for the first check.
func (s *Service) enqueue(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.ctx.Err() != nil {
		return
	}
	if _, exists := s.jobs[id]; exists {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.jobs[id] = cancel
	s.running.Add(1)
	go func() {
		defer s.running.Done()
		defer func() {
			s.mu.Lock()
			delete(s.jobs, id)
			s.mu.Unlock()
			cancel()
		}()
		select {
		case s.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-s.slots }()
		s.issue(ctx, id)
	}()
}

// active reports whether a job for the certificate is queued or running.
func (s *Service) active(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.jobs[id]
	return exists
}

// cancelJob stops the job of a deleted certificate.
func (s *Service) cancelJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, exists := s.jobs[id]; exists {
		cancel()
	}
}

func randomID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		panic("secure randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func timestamp(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano) }

func optionalTimestamp(value sql.NullInt64) *string {
	if !value.Valid {
		return nil
	}
	text := timestamp(value.Int64)
	return &text
}

// limit applies a per-session budget to operations that reach external services.
func (s *Service) limit(ctx context.Context, kind, session string, maximum int, window time.Duration) error {
	return s.auth.Limit(ctx, "certs-"+kind+":"+digest(session), maximum, window)
}
