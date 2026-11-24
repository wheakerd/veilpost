package certs

import (
	"context"
	"crypto"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-acme/lego/v4/acme"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	legolog "github.com/go-acme/lego/v4/log"
	"github.com/go-acme/lego/v4/registration"
)

func init() {
	// lego logs domain names, account URLs and provider responses. Logs of
	// this console record only HTTP route templates and outcomes.
	legolog.Logger = stdlog.New(io.Discard, "", 0)
}

// Issuance error categories stored in certificates.last_error. Details are
// kept only for ACME problem documents, which come from the CA and never
// contain DNS credentials or tokens.
const (
	errCredentialUnavailable = "CREDENTIAL_UNAVAILABLE"
	errACMEUnreachable       = "ACME_UNREACHABLE"
	errACMEAccount           = "ACME_ACCOUNT_FAILED"
	errACMERateLimited       = "ACME_RATE_LIMITED"
	errACMERejected          = "ACME_REJECTED"
	errDNSProvider           = "DNS_PROVIDER_FAILED"
	errDNSPropagation        = "DNS_PROPAGATION_TIMEOUT"
	errIssuance              = "ISSUANCE_FAILED"
)

type acmeUser struct {
	email        string
	key          crypto.PrivateKey
	registration *registration.Resource
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

func accountKeyPurpose(id string) string     { return "acme-account:" + id }
func accountEABPurpose(id string) string     { return "acme-eab:" + id }
func certificateKeyPurpose(id string) string { return "certificate-key:" + id }

// contextTransport binds every ACME request to the job context, so stopping
// the console or deleting the certificate aborts the exchange.
type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(request.WithContext(t.ctx))
}

func (s *Service) acmeClient(ctx context.Context) *http.Client {
	base := s.acmeTransport
	if base == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSHandshakeTimeout = 15 * time.Second
		transport.ResponseHeaderTimeout = 30 * time.Second
		base = transport
	}
	return &http.Client{Timeout: time.Minute, Transport: contextTransport{ctx: ctx, base: base},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// tracker records which DNS-01 stage failed, because lego reports stages
// only in error text.
type tracker struct {
	challenge.Provider
	ctx            context.Context
	presentFailed  atomic.Bool
	propagationRan atomic.Bool
	propagated     atomic.Bool
}

func (t *tracker) Present(domain, token, keyAuth string) error {
	if err := t.ctx.Err(); err != nil {
		return err
	}
	if err := t.Provider.Present(domain, token, keyAuth); err != nil {
		t.presentFailed.Store(true)
		return err
	}
	return nil
}

// Timeout keeps the provider's propagation timeout and polling interval.
func (t *tracker) Timeout() (time.Duration, time.Duration) {
	if provider, ok := t.Provider.(challenge.ProviderTimeout); ok {
		return provider.Timeout()
	}
	return dns01.DefaultPropagationTimeout, dns01.DefaultPollingInterval
}

func (s *Service) precheck(t *tracker) dns01.WrapPreCheckFunc {
	return func(domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) {
		// Returning stop with an error ends lego's propagation wait at once.
		if err := t.ctx.Err(); err != nil {
			return true, err
		}
		t.propagationRan.Store(true)
		if s.skipPropagation {
			t.propagated.Store(true)
			return true, nil
		}
		done, err := check(fqdn, value)
		if done && err == nil {
			t.propagated.Store(true)
		}
		return done, err
	}
}

type issueFailure struct {
	code, detail string
}

func (f *issueFailure) Error() string { return f.code }

func failed(code string, err error) *issueFailure {
	failure := &issueFailure{code: code}
	var problem *acme.ProblemDetails
	if err != nil && errors.As(err, &problem) {
		failure.detail = problemDetail(problem)
		if code != errACMEAccount {
			failure.code = errACMERejected
			if problem.Type == acme.RateLimitedErr {
				failure.code = errACMERateLimited
			}
		}
	}
	return failure
}

// problemDetail keeps a short, printable summary of an ACME problem.
func problemDetail(problem *acme.ProblemDetails) string {
	text := strings.TrimPrefix(problem.Type, "urn:ietf:params:acme:error:")
	if problem.Detail != "" {
		text += ": " + problem.Detail
	}
	text = strings.Join(strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if utf8.RuneCountInString(text) > 300 {
		text = string([]rune(text)[:300]) + "…"
	}
	return text
}

type job struct {
	domains    []string
	directory  string
	accountID  string
	credential string
	issued     bool
	notAfter   sql.NullInt64
	failures   int
}

// issue obtains a certificate for one request or renewal and records the
// outcome. A canceled job records nothing and resumes at the next start.
func (s *Service) issue(ctx context.Context, id string) {
	var j job
	var domains string
	var pem sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT domains,ca_directory,acme_account_id,dns_credential_id,certificate_pem,not_after,failures
 FROM certificates WHERE id=?`, id).Scan(&domains, &j.directory, &j.accountID, &j.credential, &pem, &j.notAfter, &j.failures)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, sql.ErrNoRows) {
			s.logger.Error("certificate issuance failed", "category", errIssuance)
		}
		return
	}
	j.issued = pem.Valid
	if json.Unmarshal([]byte(domains), &j.domains) != nil || len(j.domains) == 0 {
		s.recordFailure(ctx, id, j, &issueFailure{code: errIssuance})
		return
	}
	resource, key, failure := s.obtain(ctx, id, j)
	if ctx.Err() != nil {
		return
	}
	if failure == nil {
		failure = s.recordSuccess(ctx, id, resource, key)
	}
	if failure != nil {
		s.logger.Warn("certificate issuance failed", "category", failure.code)
		s.recordFailure(ctx, id, j, failure)
	}
}

func (s *Service) obtain(ctx context.Context, id string, j job) (*certificate.Resource, crypto.PrivateKey, *issueFailure) {
	var credentialProvider string
	var sealedSecrets []byte
	if err := s.db.QueryRowContext(ctx, "SELECT provider,secrets FROM dns_credentials WHERE id=?", j.credential).Scan(&credentialProvider, &sealedSecrets); err != nil {
		return nil, nil, &issueFailure{code: errCredentialUnavailable}
	}
	secrets, err := s.openSecrets(j.credential, sealedSecrets)
	if err != nil {
		return nil, nil, &issueFailure{code: errCredentialUnavailable}
	}
	provider, err := s.newProvider(credentialProvider, secrets)
	if err != nil {
		return nil, nil, &issueFailure{code: errCredentialUnavailable}
	}
	user, err := s.account(ctx, j.accountID)
	if err != nil {
		return nil, nil, &issueFailure{code: errIssuance}
	}
	config := lego.NewConfig(user)
	config.CADirURL = j.directory
	config.HTTPClient = s.acmeClient(ctx)
	config.UserAgent = "veilpost"
	config.Certificate.KeyType = certcrypto.EC256
	client, err := lego.NewClient(config)
	if err != nil {
		var problem *acme.ProblemDetails
		if errors.As(err, &problem) {
			return nil, nil, failed(errACMERejected, err)
		}
		return nil, nil, &issueFailure{code: errACMEUnreachable}
	}
	if user.registration == nil {
		if failure := s.register(ctx, client, user, j.accountID); failure != nil {
			return nil, nil, failure
		}
	}
	t := &tracker{Provider: provider, ctx: ctx}
	options := append([]dns01.ChallengeOption{dns01.WrapPreCheck(s.precheck(t))}, s.challengeOptions...)
	if err := client.Challenge.SetDNS01Provider(t, options...); err != nil {
		return nil, nil, &issueFailure{code: errIssuance}
	}
	key, err := certcrypto.GeneratePrivateKey(certcrypto.EC256)
	if err != nil {
		return nil, nil, &issueFailure{code: errIssuance}
	}
	resource, err := client.Certificate.Obtain(certificate.ObtainRequest{Domains: j.domains, Bundle: true, PrivateKey: key})
	if err == nil {
		return resource, key, nil
	}
	switch {
	case t.presentFailed.Load():
		return nil, nil, &issueFailure{code: errDNSProvider}
	case t.propagationRan.Load() && !t.propagated.Load():
		return nil, nil, &issueFailure{code: errDNSPropagation}
	}
	var problem *acme.ProblemDetails
	if errors.As(err, &problem) {
		return nil, nil, failed(errACMERejected, err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return nil, nil, &issueFailure{code: errACMEUnreachable}
	}
	return nil, nil, &issueFailure{code: errIssuance}
}

// account loads the ACME account key and, when registered, its URL.
func (s *Service) account(ctx context.Context, id string) (*acmeUser, error) {
	var email string
	var sealedKey []byte
	var accountURL sql.NullString
	if err := s.db.QueryRowContext(ctx, "SELECT email,private_key,account_url FROM acme_accounts WHERE id=?", id).Scan(&email, &sealedKey, &accountURL); err != nil {
		return nil, err
	}
	plain, err := s.auth.Unseal(accountKeyPurpose(id), sealedKey)
	if err != nil {
		return nil, err
	}
	key, err := certcrypto.ParsePEMPrivateKey(plain)
	if err != nil {
		return nil, err
	}
	user := &acmeUser{email: email, key: key}
	if accountURL.Valid && accountURL.String != "" {
		user.registration = &registration.Resource{URI: accountURL.String}
	}
	return user, nil
}

// register creates the ACME account, agreeing to the CA's terms on the
// administrator's behalf, then stores its URL and forgets the EAB key.
func (s *Service) register(ctx context.Context, client *lego.Client, user *acmeUser, id string) *issueFailure {
	var sealedEAB []byte
	if err := s.db.QueryRowContext(ctx, "SELECT eab FROM acme_accounts WHERE id=?", id).Scan(&sealedEAB); err != nil {
		return &issueFailure{code: errIssuance}
	}
	var resource *registration.Resource
	var err error
	if sealedEAB != nil {
		var eab EAB
		plain, openErr := s.auth.Unseal(accountEABPurpose(id), sealedEAB)
		if openErr != nil || json.Unmarshal(plain, &eab) != nil {
			return &issueFailure{code: errACMEAccount}
		}
		resource, err = client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
			TermsOfServiceAgreed: true, Kid: eab.KeyID, HmacEncoded: eab.HMACKey,
		})
	} else {
		if client.GetExternalAccountRequired() {
			return &issueFailure{code: errACMEAccount, detail: "externalAccountRequired"}
		}
		resource, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	}
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) {
			return &issueFailure{code: errACMEUnreachable}
		}
		return failed(errACMEAccount, err)
	}
	if resource == nil || resource.URI == "" {
		return &issueFailure{code: errACMEAccount}
	}
	user.registration = resource
	if _, err := s.db.ExecContext(ctx, "UPDATE acme_accounts SET account_url=?,eab=NULL WHERE id=?", resource.URI, id); err != nil {
		return &issueFailure{code: errIssuance}
	}
	return nil
}

func issuerName(leaf *x509.Certificate) string {
	if len(leaf.Issuer.Organization) > 0 && leaf.Issuer.Organization[0] != "" {
		return leaf.Issuer.Organization[0]
	}
	return leaf.Issuer.CommonName
}

// renewAt is RenewBefore before expiry, or two thirds into the lifetime of
// certificates valid for less than 90 days.
func renewAt(notBefore, notAfter time.Time) time.Time {
	before := RenewBefore
	if third := notAfter.Sub(notBefore) / 3; third < before {
		before = third
	}
	return notAfter.Add(-before)
}

func (s *Service) recordSuccess(ctx context.Context, id string, resource *certificate.Resource, key crypto.PrivateKey) *issueFailure {
	chain, err := certcrypto.ParsePEMBundle(resource.Certificate)
	if err != nil || len(chain) == 0 {
		return &issueFailure{code: errIssuance}
	}
	leaf := chain[0]
	sealedKey, err := s.auth.Seal(certificateKeyPurpose(id), certcrypto.PEMEncode(key))
	if err != nil {
		return &issueFailure{code: errIssuance}
	}
	now := s.now()
	status := "valid"
	if !leaf.NotAfter.After(now) {
		status = "expired"
	}
	// A certificate deleted while the job ran stays deleted.
	_, err = s.db.ExecContext(ctx, `UPDATE certificates SET status=?,issuer=?,not_before=?,not_after=?,certificate_pem=?,private_key=?,
 last_error=NULL,last_error_detail=NULL,last_error_at=NULL,failures=0,renew_at=?,updated_at=? WHERE id=?`,
		status, issuerName(leaf), leaf.NotBefore.UnixMilli(), leaf.NotAfter.UnixMilli(), string(resource.Certificate), sealedKey,
		renewAt(leaf.NotBefore, leaf.NotAfter).UnixMilli(), now.UnixMilli(), id)
	if err != nil && ctx.Err() == nil {
		s.logger.Error("certificate issuance failed", "category", errIssuance)
	}
	return nil
}

// backoff is the delay before renewal attempt failures+1: one hour, doubled
// per consecutive failure, at most a day.
func backoff(failures int) time.Duration {
	delay := time.Hour
	for i := 1; i < failures && delay < maxBackoff; i++ {
		delay *= 2
	}
	return min(delay, maxBackoff)
}

// recordFailure stores the error category. A first issuance becomes failed
// and waits for a manual reissue; a renewal keeps the current certificate
// and retries with exponential backoff.
func (s *Service) recordFailure(ctx context.Context, id string, j job, failure *issueFailure) {
	now := s.now()
	failures := j.failures + 1
	var detail any
	if failure.detail != "" {
		detail = failure.detail
	}
	var err error
	if !j.issued {
		_, err = s.db.ExecContext(ctx, `UPDATE certificates SET status='failed',last_error=?,last_error_detail=?,last_error_at=?,failures=?,renew_at=NULL,updated_at=?
 WHERE id=?`, failure.code, detail, now.UnixMilli(), failures, now.UnixMilli(), id)
	} else {
		status := "valid"
		if j.notAfter.Valid && j.notAfter.Int64 <= now.UnixMilli() {
			status = "expired"
		}
		_, err = s.db.ExecContext(ctx, `UPDATE certificates SET status=?,last_error=?,last_error_detail=?,last_error_at=?,failures=?,renew_at=?,updated_at=?
 WHERE id=?`, status, failure.code, detail, now.UnixMilli(), failures, now.Add(backoff(failures)).UnixMilli(), now.UnixMilli(), id)
	}
	if err != nil && ctx.Err() == nil {
		s.logger.Error("certificate state update failed")
	}
}
