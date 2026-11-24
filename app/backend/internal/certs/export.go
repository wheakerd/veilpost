package certs

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
)

// The exports below let the console's black-box tests and other embedders
// replace collaborators (clock, ACME transport, DNS providers) and reach the
// pure parts of issuance. Production keeps the defaults set by New.

// Issuance error categories, as stored in certificates.last_error.
const (
	ErrACMEUnreachable = errACMEUnreachable
	ErrACMEAccount     = errACMEAccount
	ErrACMERateLimited = errACMERateLimited
	ErrACMERejected    = errACMERejected
	ErrDNSProvider     = errDNSProvider
)

// SetClock replaces the clock behind renewal, expiry and backoff.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Now reads the service clock.
func (s *Service) Now() time.Time { return s.now() }

// Logger is the logger the service was created with.
func (s *Service) Logger() *slog.Logger { return s.logger }

// SetACMETransport sends ACME requests through transport, for example to a
// private CA that uses its own roots.
func (s *Service) SetACMETransport(transport http.RoundTripper) {
	s.acmeTransport = transport
}

// SetProviderFactory replaces how DNS-01 providers are built from stored
// credentials.
func (s *Service) SetProviderFactory(factory func(provider string, secrets map[string]string) (challenge.Provider, error)) {
	s.newProvider = factory
}

// SetCredentialVerifier replaces the read-only provider check.
func (s *Service) SetCredentialVerifier(verify func(ctx context.Context, provider string, secrets map[string]string) error) {
	s.verifyCredential = verify
}

// SetChallengeOptions adds DNS-01 options such as recursive resolvers.
func (s *Service) SetChallengeOptions(options ...dns01.ChallengeOption) {
	s.challengeOptions = options
}

// SetSkipPropagation trusts the provider instead of polling DNS for the
// challenge record.
func (s *Service) SetSkipPropagation(skip bool) { s.skipPropagation = skip }

// Certificate reads one certificate without a session check.
func (s *Service) Certificate(ctx context.Context, id string) (Certificate, error) {
	return s.certificate(ctx, id)
}

// Credential reads one DNS credential's public view without a session check.
func (s *Service) Credential(ctx context.Context, id string) (Credential, error) {
	return s.credential(ctx, id)
}

// OpenSecrets decrypts the secrets sealed for credential id.
func (s *Service) OpenSecrets(id string, sealed []byte) (map[string]string, error) {
	return s.openSecrets(id, sealed)
}

// Active reports whether an issuance job for id is running.
func (s *Service) Active(id string) bool { return s.active(id) }

// JobState describes the certificate an issuance job worked on.
type JobState struct {
	Issued   bool
	Failures int
	// NotAfter is the current certificate's expiry, when one exists.
	NotAfter *time.Time
}

// RecordFailure stores a failed issuance as the scheduler does, with the
// category code and an optional ACME problem detail.
func (s *Service) RecordFailure(ctx context.Context, id string, state JobState, code, detail string) {
	j := job{issued: state.Issued, failures: state.Failures}
	if state.NotAfter != nil {
		j.notAfter = sql.NullInt64{Int64: state.NotAfter.UnixMilli(), Valid: true}
	}
	s.recordFailure(ctx, id, j, &issueFailure{code: code, detail: detail})
}

// Precheck returns the DNS propagation check used for one issuance bound to
// ctx, and a Present function that refuses to run once ctx ends.
func (s *Service) Precheck(ctx context.Context) (dns01.WrapPreCheckFunc, func(domain, token, keyAuth string) error) {
	t := &tracker{ctx: ctx}
	return s.precheck(t), t.Present
}

// Timestamp formats Unix milliseconds as the API does.
func Timestamp(ms int64) string { return timestamp(ms) }

// RenewAt is when a certificate valid from notBefore to notAfter renews.
func RenewAt(notBefore, notAfter time.Time) time.Time { return renewAt(notBefore, notAfter) }

// Backoff is the wait before retrying a renewal after failures.
func Backoff(failures int) time.Duration { return backoff(failures) }

// ValidEAB checks external account binding input for a directory kind.
func ValidEAB(kind string, eab EAB) (*EAB, error) { return validEAB(kind, eab) }

// VerifyCloudflare checks a Cloudflare API token against base.
func VerifyCloudflare(ctx context.Context, base, token string) error {
	return verifyCloudflare(ctx, base, token)
}

// Unreachable reports whether err means the provider could not be reached.
func Unreachable(err error) bool { return unreachable(err) }

// Seal purposes of the stored secrets.
func CredentialPurpose(id string) string     { return credentialPurpose(id) }
func AccountKeyPurpose(id string) string     { return accountKeyPurpose(id) }
func CertificateKeyPurpose(id string) string { return certificateKeyPurpose(id) }
