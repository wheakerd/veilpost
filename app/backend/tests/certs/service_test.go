package certs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wheakerd/veilpost/app/backend/internal/certs"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

const testPassword = "correct-password-for-certificate-tests"

// Secret values used by the tests; none may appear in responses, logs or
// the database file.
const (
	cloudflareToken = "cf-token-7d1b8f6a0e9c4b2d"
	alidnsSecret    = "ali-secret-3c5e7a9b1d2f"
	route53Secret   = "aws-secret-9f8e7d6c5b4a"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fixture struct {
	auth    *auth.Service
	certs   *certs.Service
	session string
	logs    *syncBuffer
	dir     string
	mu      *sync.Mutex
	now     *time.Time
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	service, err := auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	if err := service.CreateUser(context.Background(), "cert-admin", testPassword); err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), "cert-admin", testPassword, "", "test-ip")
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	certService := certs.New(service, slog.New(slog.NewJSONHandler(logs, nil)))
	now := time.Now()
	var mu sync.Mutex
	certService.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	// No test may reach a real provider.
	certService.SetCredentialVerifier(func(context.Context, string, map[string]string) error {
		t.Error("unexpected provider call")
		return errors.New("unexpected")
	})
	certService.SetProviderFactory(func(string, map[string]string) (challenge.Provider, error) {
		return nil, errors.New("no provider in unit tests")
	})
	return fixture{service, certService, result.Token, logs, dir, &mu, &now}
}

func (f fixture) advance(d time.Duration) { f.mu.Lock(); *f.now = f.now.Add(d); f.mu.Unlock() }

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var api *auth.APIError
	if !errors.As(err, &api) || api.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

func (f fixture) credential(t *testing.T, provider string, secrets map[string]string) certs.Credential {
	t.Helper()
	credential, err := f.certs.CreateCredential(context.Background(), f.session, "Test "+provider, provider, secrets, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func (f fixture) cloudflare(t *testing.T) certs.Credential {
	return f.credential(t, "cloudflare", map[string]string{"apiToken": cloudflareToken})
}

// assertNoSecrets checks responses, logs and the database files.
func (f fixture) assertNoSecrets(t *testing.T, texts ...string) {
	t.Helper()
	files, _ := os.ReadDir(f.dir)
	for _, file := range files {
		data, _ := os.ReadFile(f.dir + "/" + file.Name())
		texts = append(texts, string(data))
	}
	texts = append(texts, f.logs.String())
	for _, text := range texts {
		for _, secret := range []string{cloudflareToken, alidnsSecret, route53Secret} {
			if strings.Contains(text, secret) {
				t.Fatalf("secret value leaked")
			}
		}
	}
}

func TestNormalizeDomains(t *testing.T) {
	valid := map[string]string{
		"Example.COM.":          "example.com",
		" *.example.com ":       "*.example.com",
		"bücher.example":        "xn--bcher-kva.example",
		"a-b.c-d.example.org":   "a-b.c-d.example.org",
		"xn--bcher-kva.example": "xn--bcher-kva.example",
	}
	for input, want := range valid {
		names, err := certs.NormalizeDomains([]string{input})
		if err != nil || len(names) != 1 || names[0] != want {
			t.Errorf("%q: got %v %v, want %s", input, names, err, want)
		}
	}
	names, err := certs.NormalizeDomains([]string{"example.com", "EXAMPLE.com", "*.example.com"})
	if err != nil || strings.Join(names, ",") != "example.com,*.example.com" {
		t.Fatalf("duplicates: %v %v", names, err)
	}
	for _, input := range []string{"", "localhost", "*.*.example.com", "a*.example.com", "*example.com", "example.com/path",
		"1.2.3.4", "-a.example.com", "a-.example.com", "a..example.com", "_acme.example.com", "*.", "*.com.", "user@example.com",
		"exa mple.com", strings.Repeat("a", 64) + ".example.com", strings.Repeat("abcdefghi.", 26) + "com", "example.com:443"} {
		if _, err := certs.NormalizeDomains([]string{input}); err == nil {
			t.Errorf("%q accepted", input)
		} else {
			expectCode(t, err, "INVALID_DOMAIN")
		}
	}
	_, err = certs.NormalizeDomains(nil)
	expectCode(t, err, "INVALID_DOMAIN")
	many := make([]string, certs.MaxDomains)
	for i := range many {
		many[i] = fmt.Sprintf("host%d.example.com", i)
	}
	if names, err := certs.NormalizeDomains(many); err != nil || len(names) != certs.MaxDomains {
		t.Fatalf("100 names: %v", err)
	}
	_, err = certs.NormalizeDomains(append(many, "one-more.example.com"))
	expectCode(t, err, "TOO_MANY_DOMAINS")
}

func TestResolveDirectory(t *testing.T) {
	for _, c := range []struct{ kind, url, want string }{
		{"", "", certs.LetsEncryptProduction},
		{certs.DirectoryLetsEncrypt, "", certs.LetsEncryptProduction},
		{certs.DirectoryLetsEncryptStaging, "", certs.LetsEncryptStaging},
		{certs.DirectoryCustom, "https://acme.example.com/directory", "https://acme.example.com/directory"},
		{certs.DirectoryCustom, " https://acme.example.com:8443/v2/DV90?x=1 ", "https://acme.example.com:8443/v2/DV90?x=1"},
	} {
		got, err := certs.ResolveDirectory(c.kind, c.url)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %s %v", c.kind, c.url, got, err)
		}
	}
	for _, c := range []struct{ kind, url string }{
		{certs.DirectoryCustom, "http://acme.example.com/directory"},
		{certs.DirectoryCustom, "ftp://acme.example.com/directory"},
		{certs.DirectoryCustom, "https://user:secret@acme.example.com/directory"},
		{certs.DirectoryCustom, "https:///directory"},
		{certs.DirectoryCustom, "https://acme.example.com/directory#x"},
		{certs.DirectoryCustom, "acme.example.com/directory"},
		{certs.DirectoryCustom, ""},
		{certs.DirectoryLetsEncrypt, "https://acme.example.com/directory"},
		{"zerossl", ""},
	} {
		_, err := certs.ResolveDirectory(c.kind, c.url)
		expectCode(t, err, "INVALID_DIRECTORY")
	}
}

func TestEmailAndEAB(t *testing.T) {
	for _, email := range []string{"", "admin@example.com", " ops+acme@example.org "} {
		if _, err := certs.NormalizeEmail(email); err != nil {
			t.Errorf("%q: %v", email, err)
		}
	}
	for _, email := range []string{"admin", "Admin <admin@example.com>", "admin@localhost", "a@b@example.com"} {
		_, err := certs.NormalizeEmail(email)
		expectCode(t, err, "INVALID_EMAIL")
	}
	if eab, err := certs.ValidEAB(certs.DirectoryLetsEncrypt, certs.EAB{}); eab != nil || err != nil {
		t.Fatal("empty EAB must be accepted")
	}
	if eab, err := certs.ValidEAB(certs.DirectoryCustom, certs.EAB{KeyID: "kid-1", HMACKey: "c2VjcmV0LWhtYWMta2V5"}); eab == nil || err != nil {
		t.Fatalf("valid EAB: %v", err)
	}
	for _, c := range []struct {
		kind string
		eab  certs.EAB
	}{
		{certs.DirectoryLetsEncrypt, certs.EAB{KeyID: "kid", HMACKey: "c2VjcmV0"}},
		{certs.DirectoryCustom, certs.EAB{KeyID: "kid"}},
		{certs.DirectoryCustom, certs.EAB{HMACKey: "c2VjcmV0"}},
		{certs.DirectoryCustom, certs.EAB{KeyID: "kid", HMACKey: "not base64!"}},
		{certs.DirectoryCustom, certs.EAB{KeyID: "has space", HMACKey: "c2VjcmV0"}},
	} {
		_, err := certs.ValidEAB(c.kind, c.eab)
		expectCode(t, err, "INVALID_EAB")
	}
}

func TestCredentialsAreSealedAndNeverReturned(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cloudflare := f.cloudflare(t)
	route53 := f.credential(t, "route53", map[string]string{"accessKeyId": "AKIAEXAMPLE", "secretAccessKey": route53Secret, "region": "us-east-1"})
	if !cloudflare.Fields["apiToken"] || len(cloudflare.Fields) != 1 {
		t.Fatalf("cloudflare fields: %v", cloudflare.Fields)
	}
	if !route53.Fields["secretAccessKey"] || !route53.Fields["region"] || route53.Fields["hostedZoneId"] {
		t.Fatalf("route53 fields: %v", route53.Fields)
	}
	list, err := f.certs.Credentials(ctx, f.session)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %v", list, err)
	}
	encoded, _ := json.Marshal(list)
	f.assertNoSecrets(t, string(encoded))
	if strings.Contains(string(encoded), "AKIAEXAMPLE") {
		t.Fatal("access key ID returned")
	}

	// The sealed value is bound to its row: moving it to another row fails.
	var sealed []byte
	db := f.auth.Database()
	if err := db.QueryRow("SELECT secrets FROM dns_credentials WHERE id=?", cloudflare.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Unseal(certs.CredentialPurpose(route53.ID), sealed); err == nil {
		t.Fatal("sealed secrets opened for another credential")
	}
	if _, err := db.Exec("UPDATE dns_credentials SET secrets=? WHERE id=?", sealed, route53.ID); err != nil {
		t.Fatal(err)
	}
	moved, err := f.certs.Credential(ctx, route53.ID)
	if err != nil || moved.Fields["secretAccessKey"] {
		t.Fatalf("moved secrets must read as unset: %v %v", moved.Fields, err)
	}
	if _, err := f.auth.Seal("totp", []byte("x")); err == nil {
		t.Fatal("reserved purpose accepted")
	}
}

func TestCredentialValidationAndPassword(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.certs.CreateCredential(ctx, f.session, "DNS", "cloudflare", map[string]string{"apiToken": cloudflareToken}, "wrong-password")
	expectCode(t, err, "INVALID_PASSWORD")
	for _, c := range []struct {
		name, provider string
		secrets        map[string]string
		code           string
	}{
		{"", "cloudflare", map[string]string{"apiToken": "x"}, "INVALID_CREDENTIAL_NAME"},
		{"line\nbreak", "cloudflare", map[string]string{"apiToken": "x"}, "INVALID_CREDENTIAL_NAME"},
		{"DNS", "godaddy", map[string]string{"apiToken": "x"}, "INVALID_PROVIDER"},
		{"DNS", "cloudflare", map[string]string{}, "INVALID_SECRETS"},
		{"DNS", "cloudflare", map[string]string{"apiToken": "x", "email": "a@example.com"}, "INVALID_SECRETS"},
		{"DNS", "cloudflare", map[string]string{"apiToken": "has space"}, "INVALID_SECRETS"},
		{"DNS", "alidns", map[string]string{"accessKeyId": "id"}, "INVALID_SECRETS"},
		{"DNS", "route53", map[string]string{"accessKeyId": "id", "secretAccessKey": "s", "region": "mars"}, "INVALID_SECRETS"},
		{"DNS", "route53", map[string]string{"accessKeyId": "id", "secretAccessKey": "s", "region": "us-east-1", "hostedZoneId": "bad zone"}, "INVALID_SECRETS"},
	} {
		_, err := f.certs.CreateCredential(ctx, f.session, c.name, c.provider, c.secrets, testPassword)
		expectCode(t, err, c.code)
	}
	if list, _ := f.certs.Credentials(ctx, f.session); len(list) != 0 {
		t.Fatalf("rejected credentials were stored: %v", list)
	}
	_, err = f.certs.Credentials(ctx, "invalid-session")
	expectCode(t, err, "SESSION_EXPIRED")
}

func TestCredentialUpdateKeepsOmittedValues(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created := f.credential(t, "route53", map[string]string{"accessKeyId": "AKIAOLD", "secretAccessKey": route53Secret, "region": "us-east-1", "hostedZoneId": "Z123ABC"})
	f.advance(time.Minute)
	_, err := f.certs.UpdateCredential(ctx, f.session, created.ID, "Renamed", map[string]string{"accessKeyId": "AKIANEW"}, "wrong-password")
	expectCode(t, err, "INVALID_PASSWORD")
	updated, err := f.certs.UpdateCredential(ctx, f.session, created.ID, "Renamed", map[string]string{"accessKeyId": "AKIANEW", "hostedZoneId": ""}, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed" || updated.Fields["hostedZoneId"] || !updated.Fields["secretAccessKey"] || updated.UpdatedAt == created.UpdatedAt {
		t.Fatalf("update: %+v", updated)
	}
	var sealed []byte
	if err := f.auth.Database().QueryRow("SELECT secrets FROM dns_credentials WHERE id=?", created.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	secrets, err := f.certs.OpenSecrets(created.ID, sealed)
	if err != nil || secrets["accessKeyId"] != "AKIANEW" || secrets["secretAccessKey"] != route53Secret || secrets["region"] != "us-east-1" {
		t.Fatalf("merged secrets differ")
	}
	_, err = f.certs.UpdateCredential(ctx, f.session, created.ID, "Renamed", map[string]string{"secretAccessKey": ""}, testPassword)
	expectCode(t, err, "INVALID_SECRETS")
	_, err = f.certs.UpdateCredential(ctx, f.session, "AAAAAAAAAAAAAAAAAAAAAA", "Renamed", nil, testPassword)
	expectCode(t, err, "CREDENTIAL_NOT_FOUND")
	f.assertNoSecrets(t)
}

func TestCredentialInUseCannotBeDeleted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	credential := f.cloudflare(t)
	certificate, err := f.certs.Request(ctx, f.session, certs.RequestInput{Domains: []string{"example.com"}, CredentialID: credential.ID})
	if err != nil {
		t.Fatal(err)
	}
	if certificate.Status != "pending" || certificate.CredentialName != credential.Name || certificate.Directory != certs.DirectoryLetsEncrypt {
		t.Fatalf("request: %+v", certificate)
	}
	err = f.certs.DeleteCredential(ctx, f.session, credential.ID, testPassword)
	expectCode(t, err, "CREDENTIAL_IN_USE")
	err = f.certs.DeleteCredential(ctx, f.session, credential.ID, "wrong-password")
	expectCode(t, err, "INVALID_PASSWORD")
	listed, _ := f.certs.Credential(ctx, credential.ID)
	if listed.Certificates != 1 {
		t.Fatalf("certificate count: %d", listed.Certificates)
	}
	if err := f.certs.DeleteCertificate(ctx, f.session, certificate.ID); err != nil {
		t.Fatal(err)
	}
	expectCode(t, f.certs.DeleteCertificate(ctx, f.session, certificate.ID), "CERTIFICATE_NOT_FOUND")
	if err := f.certs.DeleteCredential(ctx, f.session, credential.ID, testPassword); err != nil {
		t.Fatal(err)
	}
	expectCode(t, f.certs.DeleteCredential(ctx, f.session, credential.ID, testPassword), "CREDENTIAL_NOT_FOUND")
}

func TestVerifyCredentialOutcomes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	credential := f.cloudflare(t)
	var got map[string]string
	outcome := error(nil)
	f.certs.SetCredentialVerifier(func(_ context.Context, provider string, secrets map[string]string) error {
		if provider != "cloudflare" {
			t.Errorf("provider %s", provider)
		}
		got = secrets
		return outcome
	})
	if err := f.certs.VerifyCredential(ctx, f.session, credential.ID); err != nil || got["apiToken"] != cloudflareToken {
		t.Fatalf("verify: %v", err)
	}
	outcome = errors.New("403 forbidden")
	expectCode(t, f.certs.VerifyCredential(ctx, f.session, credential.ID), "CREDENTIAL_REJECTED")
	outcome = &net.OpError{Op: "dial", Err: errors.New("refused")}
	expectCode(t, f.certs.VerifyCredential(ctx, f.session, credential.ID), "PROVIDER_UNREACHABLE")
	expectCode(t, f.certs.VerifyCredential(ctx, f.session, "AAAAAAAAAAAAAAAAAAAAAA"), "CREDENTIAL_NOT_FOUND")
	for range 6 {
		f.certs.VerifyCredential(ctx, f.session, credential.ID)
	}
	expectCode(t, f.certs.VerifyCredential(ctx, f.session, credential.ID), "TOO_MANY_ATTEMPTS")
}

// The Cloudflare check is a single read-only GET of the zone list.
func TestVerifyCloudflareIsReadOnly(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		code := status
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+cloudflareToken {
			code = http.StatusForbidden
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"success":%t,"result":[]}`, code == http.StatusOK)
	}))
	defer server.Close()
	ctx := context.Background()
	if err := certs.VerifyCloudflare(ctx, server.URL, cloudflareToken); err != nil {
		t.Fatal(err)
	}
	if err := certs.VerifyCloudflare(ctx, server.URL, "other-token"); err == nil || certs.Unreachable(err) {
		t.Fatalf("rejected token: %v", err)
	}
	mu.Lock()
	status = http.StatusBadGateway
	mu.Unlock()
	if err := certs.VerifyCloudflare(ctx, server.URL, cloudflareToken); !certs.Unreachable(err) {
		t.Fatalf("provider outage: %v", err)
	}
	server.Close()
	if err := certs.VerifyCloudflare(ctx, server.URL, cloudflareToken); !certs.Unreachable(err) {
		t.Fatalf("closed server: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, request := range requests {
		if request != "GET /zones?per_page=1" {
			t.Fatalf("unexpected request %s", request)
		}
	}
}

func TestRequestValidationAndAccountReuse(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	credential := f.cloudflare(t)
	for _, c := range []struct {
		input certs.RequestInput
		code  string
	}{
		{certs.RequestInput{Domains: []string{"bad domain"}, CredentialID: credential.ID}, "INVALID_DOMAIN"},
		{certs.RequestInput{Domains: []string{"example.com"}, CredentialID: credential.ID, Directory: certs.DirectoryCustom, DirectoryURL: "http://acme.example.com/dir"}, "INVALID_DIRECTORY"},
		{certs.RequestInput{Domains: []string{"example.com"}, CredentialID: credential.ID, Email: "not-an-email"}, "INVALID_EMAIL"},
		{certs.RequestInput{Domains: []string{"example.com"}, CredentialID: credential.ID, EAB: certs.EAB{KeyID: "kid", HMACKey: "c2VjcmV0"}}, "INVALID_EAB"},
		{certs.RequestInput{Domains: []string{"example.com"}, CredentialID: "AAAAAAAAAAAAAAAAAAAAAA"}, "CREDENTIAL_NOT_FOUND"},
		{certs.RequestInput{Domains: []string{"example.com"}, CredentialID: "../x"}, "CREDENTIAL_NOT_FOUND"},
	} {
		_, err := f.certs.Request(ctx, f.session, c.input)
		expectCode(t, err, c.code)
	}
	request := func(directory, url, email string) certs.Certificate {
		t.Helper()
		certificate, err := f.certs.Request(ctx, f.session, certs.RequestInput{Domains: []string{"*.example.com", "example.com"}, CredentialID: credential.ID,
			Directory: directory, DirectoryURL: url, Email: email})
		if err != nil {
			t.Fatal(err)
		}
		return certificate
	}
	first := request(certs.DirectoryLetsEncryptStaging, "", "ops@example.com")
	request(certs.DirectoryLetsEncryptStaging, "", "ops@example.com")
	request(certs.DirectoryLetsEncryptStaging, "", "")
	custom := request(certs.DirectoryCustom, "https://acme.example.net/directory", "ops@example.com")
	if first.Directory != certs.DirectoryLetsEncryptStaging || first.DirectoryURL != certs.LetsEncryptStaging || first.Email != "ops@example.com" ||
		strings.Join(first.Domains, ",") != "*.example.com,example.com" || first.LastError != nil || first.NotAfter != nil {
		t.Fatalf("certificate: %+v", first)
	}
	if custom.Directory != certs.DirectoryCustom {
		t.Fatalf("custom: %+v", custom)
	}
	var accounts, certificates int
	db := f.auth.Database()
	db.QueryRow("SELECT count(*) FROM acme_accounts").Scan(&accounts)
	db.QueryRow("SELECT count(DISTINCT acme_account_id) FROM certificates").Scan(&certificates)
	if accounts != 3 || certificates != 3 {
		t.Fatalf("accounts %d, distinct %d", accounts, certificates)
	}
	// Account keys are sealed PEM, never stored in the clear.
	var key []byte
	db.QueryRow("SELECT private_key FROM acme_accounts LIMIT 1").Scan(&key)
	if bytes.Contains(key, []byte("PRIVATE KEY")) {
		t.Fatal("account key stored in the clear")
	}
	_, err := f.certs.Request(ctx, f.session, certs.RequestInput{Domains: []string{"example.com"}, CredentialID: credential.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = f.certs.Chain(ctx, f.session, first.ID)
	expectCode(t, err, "CERTIFICATE_NOT_ISSUED")
	_, _, err = f.certs.Chain(ctx, f.session, "AAAAAAAAAAAAAAAAAAAAAA")
	expectCode(t, err, "CERTIFICATE_NOT_FOUND")
}

func TestBackoffAndRenewalTime(t *testing.T) {
	for failures, want := range map[int]time.Duration{1: time.Hour, 2: 2 * time.Hour, 3: 4 * time.Hour, 5: 16 * time.Hour, 6: 24 * time.Hour, 40: 24 * time.Hour} {
		if got := certs.Backoff(failures); got != want {
			t.Errorf("backoff(%d) = %s, want %s", failures, got, want)
		}
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := certs.RenewAt(start, start.Add(90*24*time.Hour)); !got.Equal(start.Add(60 * 24 * time.Hour)) {
		t.Errorf("90-day certificate renews at %s", got)
	}
	if got := certs.RenewAt(start, start.Add(6*24*time.Hour)); !got.Equal(start.Add(4 * 24 * time.Hour)) {
		t.Errorf("6-day certificate renews at %s", got)
	}
}

func TestCheckMarksExpiredAndFailureState(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	credential := f.cloudflare(t)
	certificate, err := f.certs.Request(ctx, f.session, certs.RequestInput{Domains: []string{"example.com"}, CredentialID: credential.ID})
	if err != nil {
		t.Fatal(err)
	}
	db := f.auth.Database()
	// A first issuance failure waits for a manual reissue.
	f.certs.RecordFailure(ctx, certificate.ID, certs.JobState{}, certs.ErrDNSProvider, "")
	failed, _ := f.certs.Certificate(ctx, certificate.ID)
	if failed.Status != "failed" || failed.LastError == nil || failed.LastError.Code != certs.ErrDNSProvider || failed.LastError.Detail != nil || failed.RenewAt != nil {
		t.Fatalf("failed: %+v", failed)
	}
	reissued, err := f.certs.Reissue(ctx, f.session, certificate.ID)
	if err != nil || reissued.Status != "pending" || reissued.LastError != nil || reissued.RenewAt == nil {
		t.Fatalf("reissue: %+v %v", reissued, err)
	}
	// A renewal failure keeps the certificate and backs off.
	now := f.certs.Now()
	db.Exec("UPDATE certificates SET status='valid',certificate_pem='x',not_before=?,not_after=?,failures=2 WHERE id=?",
		now.Add(-80*24*time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), certificate.ID)
	notAfter := now.Add(time.Hour)
	f.certs.RecordFailure(ctx, certificate.ID, certs.JobState{Issued: true, Failures: 2, NotAfter: &notAfter},
		certs.ErrACMERateLimited, "rateLimited: too many")
	renewing, _ := f.certs.Certificate(ctx, certificate.ID)
	if renewing.Status != "valid" || renewing.LastError == nil || *renewing.LastError.Detail != "rateLimited: too many" ||
		renewing.RenewAt == nil || *renewing.RenewAt != certs.Timestamp(now.Add(4*time.Hour).UnixMilli()) {
		t.Fatalf("renewal failure: %+v", renewing)
	}
	f.advance(2 * time.Hour)
	if err := f.certs.Check(ctx); err != nil {
		t.Fatal(err)
	}
	expired, _ := f.certs.Certificate(ctx, certificate.ID)
	if expired.Status != "expired" {
		t.Fatalf("status %s", expired.Status)
	}
}

func TestPrecheckStopsWhenCanceled(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.certs.SetSkipPropagation(true)
	wrap, present := f.certs.Precheck(ctx)
	if done, err := wrap("example.com", "_acme-challenge.example.com.", "v", nil); !done || err != nil {
		t.Fatalf("skip: %v %v", done, err)
	}
	cancel()
	if done, err := wrap("example.com", "_acme-challenge.example.com.", "v", nil); !done || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v %v", done, err)
	}
	if err := present("example.com", "t", "k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("present after cancel: %v", err)
	}
}

func TestHTTPBoundaries(t *testing.T) {
	f := newFixture(t)
	router := gin.New()
	authHandler := &auth.Handler{Service: f.auth, Development: true, Logger: slog.New(slog.NewJSONHandler(f.logs, nil))}
	handler := &certs.Handler{Service: f.certs, Auth: authHandler, Logger: authHandler.Logger}
	handler.RegisterCredentials(router.Group("/api/dns-credentials"))
	handler.RegisterCertificates(router.Group("/api/certificates"))
	call := func(method, path, body string, session, write bool) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		request := httptest.NewRequest(method, "http://console.test"+path, reader)
		if session {
			request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: f.session})
		}
		if write {
			request.Header.Set("Origin", "http://console.test")
			request.Header.Set("X-Requested-With", "Veilpost")
			request.Header.Set("Content-Type", "application/json")
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	expect := func(r *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if r.Code != status || (code != "" && !strings.Contains(r.Body.String(), `"code":"`+code+`"`)) {
			t.Fatalf("got %d %s, want %d %s", r.Code, r.Body.String(), status, code)
		}
	}
	expect(call("GET", "/api/dns-credentials", "", false, false), 401, "SESSION_EXPIRED")
	expect(call("GET", "/api/certificates", "", false, false), 401, "SESSION_EXPIRED")
	create := fmt.Sprintf(`{"name":"CF","provider":"cloudflare","secrets":{"apiToken":%q},"password":%q}`, cloudflareToken, testPassword)
	expect(call("POST", "/api/dns-credentials", create, true, false), 403, "REQUEST_FAILED")
	expect(call("POST", "/api/dns-credentials", `{"name":"CF","extra":1}`, true, true), 400, "REQUEST_FAILED")
	created := call("POST", "/api/dns-credentials", create, true, true)
	expect(created, 201, "")
	var body struct {
		Credential certs.Credential `json:"credential"`
	}
	json.Unmarshal(created.Body.Bytes(), &body)
	listed := call("GET", "/api/dns-credentials", "", true, false)
	expect(listed, 200, "")
	request := fmt.Sprintf(`{"domains":["example.com"],"dnsCredentialId":%q,"directory":"letsencrypt-staging","directoryUrl":"","email":"","eab":{"keyId":"","hmacKey":""}}`, body.Credential.ID)
	accepted := call("POST", "/api/certificates", request, true, true)
	expect(accepted, 202, "")
	var certificate struct {
		Certificate certs.Certificate `json:"certificate"`
	}
	json.Unmarshal(accepted.Body.Bytes(), &certificate)
	expect(call("GET", "/api/certificates/"+certificate.Certificate.ID+"/chain", "", true, false), 409, "CERTIFICATE_NOT_ISSUED")
	expect(call("POST", "/api/dns-credentials/"+body.Credential.ID+"/delete", fmt.Sprintf(`{"password":%q}`, testPassword), true, true), 409, "CREDENTIAL_IN_USE")
	expect(call("POST", "/api/certificates/"+certificate.Certificate.ID+"/reissue", `{}`, true, true), 202, "")
	expect(call("POST", "/api/certificates/"+certificate.Certificate.ID+"/delete", `{}`, true, true), 204, "")
	expect(call("POST", "/api/certificates/"+certificate.Certificate.ID+"/delete", `{}`, true, true), 404, "CERTIFICATE_NOT_FOUND")

	// An issued chain downloads as PEM; the private key is never served.
	issued, err := f.certs.Request(context.Background(), f.session, certs.RequestInput{Domains: []string{"*.example.org"}, CredentialID: body.Credential.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.auth.Database().Exec("UPDATE certificates SET status='valid',certificate_pem=?,private_key=x'00' WHERE id=?", "-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----\n", issued.ID)
	chain := call("GET", "/api/certificates/"+issued.ID+"/chain", "", true, false)
	expect(chain, 200, "")
	if chain.Header().Get("Content-Type") != "application/pem-certificate-chain" ||
		chain.Header().Get("Content-Disposition") != `attachment; filename=wildcard.example.org.pem` || strings.Contains(chain.Body.String(), "PRIVATE") {
		t.Fatalf("chain response: %v %s", chain.Header(), chain.Body.String())
	}
	all := call("GET", "/api/certificates", "", true, false)
	expect(all, 200, "")
	for _, word := range []string{"private", "PRIVATE", "certificate_pem", "-----BEGIN"} {
		if strings.Contains(all.Body.String(), word) {
			t.Fatalf("list exposes %s", word)
		}
	}
	f.assertNoSecrets(t, created.Body.String(), listed.Body.String(), accepted.Body.String(), all.Body.String())
}
