package certs_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/wheakerd/veilpost/app/backend/internal/certs"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/letsencrypt/pebble/v2/ca"
	"github.com/letsencrypt/pebble/v2/db"
	"github.com/letsencrypt/pebble/v2/va"
	"github.com/letsencrypt/pebble/v2/wfe"
	"github.com/miekg/dns"
)

// testDNS is an in-process authoritative server for TXT records. Pebble's
// validation authority queries it over TCP; lego's resolver is pointed at it
// as well, so no test sends DNS queries elsewhere.
type testDNS struct {
	addr string
	mu   sync.Mutex
	txt  map[string][]string
}

func startDNS(t *testing.T) *testDNS {
	t.Helper()
	server := &testDNS{txt: map[string][]string{}}
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, request *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(request)
		for _, question := range request.Question {
			if question.Qtype != dns.TypeTXT {
				continue
			}
			server.mu.Lock()
			for _, value := range server.txt[strings.ToLower(question.Name)] {
				reply.Answer = append(reply.Answer, &dns.TXT{
					Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET}, Txt: []string{value},
				})
			}
			server.mu.Unlock()
		}
		w.WriteMsg(reply)
	})
	var packet net.PacketConn
	var stream net.Listener
	for attempt := 0; stream == nil; attempt++ {
		var err error
		if packet, err = net.ListenPacket("udp", "127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
		if stream, err = net.Listen("tcp", packet.LocalAddr().String()); err != nil {
			packet.Close()
			if attempt > 20 {
				t.Fatal(err)
			}
		}
	}
	server.addr = packet.LocalAddr().String()
	udp := &dns.Server{PacketConn: packet, Handler: handler}
	tcp := &dns.Server{Listener: stream, Handler: handler}
	go udp.ActivateAndServe()
	go tcp.ActivateAndServe()
	t.Cleanup(func() { udp.Shutdown(); tcp.Shutdown() })
	return server
}

// testProvider is the in-process DNS provider: Present publishes the TXT
// record on testDNS, CleanUp removes it.
type testProvider struct {
	dns      *testDNS
	presents *atomic.Int32
}

func (p testProvider) Present(domain, _, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	p.dns.mu.Lock()
	defer p.dns.mu.Unlock()
	name := strings.ToLower(info.EffectiveFQDN)
	p.dns.txt[name] = append(p.dns.txt[name], info.Value)
	p.presents.Add(1)
	return nil
}

func (p testProvider) CleanUp(domain, _, keyAuth string) error {
	info := dns01.GetChallengeInfo(domain, keyAuth)
	p.dns.mu.Lock()
	defer p.dns.mu.Unlock()
	name := strings.ToLower(info.EffectiveFQDN)
	values := p.dns.txt[name][:0]
	for _, value := range p.dns.txt[name] {
		if value != info.Value {
			values = append(values, value)
		}
	}
	p.dns.txt[name] = values
	return nil
}

type pebble struct {
	directory string
	transport http.RoundTripper
	dns       *testDNS
}

const eabKeyID, eabHMAC = "kid-console-test", "zWNDZM6eQGHWpSRTPal5eIUYFTu7EajVIoguysqZ9wG44nMEtx3MUAsUDkMTQ12W"

// startPebble runs Let's Encrypt's Pebble test CA in-process over TLS on
// loopback. Its validation authority resolves DNS-01 records from testDNS.
func startPebble(t *testing.T, requireEAB bool) pebble {
	t.Helper()
	t.Setenv("PEBBLE_VA_NOSLEEP", "1")
	t.Setenv("PEBBLE_WFE_NONCEREJECT", "0")
	// Every order validates again, so renewal exercises DNS-01 as well.
	t.Setenv("PEBBLE_AUTHZREUSE", "0")
	// lego would follow CNAMEs of _acme-challenge names through the system
	// resolver; tests publish records directly.
	t.Setenv("LEGO_DISABLE_CNAME_SUPPORT", "true")
	records := startDNS(t)
	logger := log.New(io.Discard, "", 0)
	store := db.NewMemoryStore()
	if err := store.AddExternalAccountKeyByID(eabKeyID, eabHMAC); err != nil {
		t.Fatal(err)
	}
	authority := ca.New(logger, store, "", "ecdsa", 0, 1, map[string]ca.Profile{"default": {Description: "default"}})
	validator := va.New(logger, 0, 0, false, records.addr, store)
	frontEnd := wfe.New(logger, store, validator, authority, []string{"pebble.letsencrypt.org"}, false, requireEAB, 1, 1)
	server := httptest.NewTLSServer(frontEnd.Handler())
	t.Cleanup(server.Close)
	return pebble{directory: server.URL + wfe.DirectoryPath, transport: server.Client().Transport, dns: records}
}

func (f fixture) usePebble(p pebble, presents *atomic.Int32) {
	f.certs.SetACMETransport(p.transport)
	f.certs.SetSkipPropagation(true)
	f.certs.SetChallengeOptions(dns01.AddRecursiveNameservers([]string{p.dns.addr}))
	f.certs.SetProviderFactory(func(provider string, secrets map[string]string) (challenge.Provider, error) {
		if provider != "cloudflare" || secrets["apiToken"] != cloudflareToken {
			return nil, errors.New("unexpected credential")
		}
		return testProvider{dns: p.dns, presents: presents}, nil
	})
}

// waitFor polls until the certificate's job has finished and check passes.
func (f fixture) waitFor(t *testing.T, id string, check func(certs.Certificate) bool) certs.Certificate {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		certificate, err := f.certs.Certificate(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !certificate.Issuing && check(certificate) {
			return certificate
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %+v", certificate)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestIssueAndRenewWithPebble(t *testing.T) {
	f := newFixture(t)
	p := startPebble(t, false)
	var presents atomic.Int32
	f.usePebble(p, &presents)
	ctx := context.Background()
	f.certs.Start(ctx)
	defer f.certs.Stop(10 * time.Second)
	credential := f.cloudflare(t)

	requested, err := f.certs.Request(ctx, f.session, certs.RequestInput{
		Domains: []string{"example.test", "*.example.test"}, CredentialID: credential.ID,
		Directory: certs.DirectoryCustom, DirectoryURL: p.directory, Email: "ops@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requested.Status != "pending" {
		t.Fatalf("request: %+v", requested)
	}
	issued := f.waitFor(t, requested.ID, func(c certs.Certificate) bool { return c.Status != "pending" })
	if issued.Status != "valid" || issued.Issuer == nil || *issued.Issuer == "" || issued.NotAfter == nil || issued.RenewAt == nil || issued.LastError != nil {
		t.Fatalf("issued: %+v", issued)
	}
	pem, name, err := f.certs.Chain(ctx, f.session, issued.ID)
	if err != nil || name != "example.test" {
		t.Fatalf("chain: %v %s", err, name)
	}
	chain, err := certcrypto.ParsePEMBundle([]byte(pem))
	if err != nil || len(chain) < 2 {
		t.Fatalf("chain must hold the leaf and its issuer: %v", err)
	}
	leaf := chain[0]
	if strings.Join(leaf.DNSNames, ",") != "*.example.test,example.test" && strings.Join(leaf.DNSNames, ",") != "example.test,*.example.test" {
		t.Fatalf("names: %v", leaf.DNSNames)
	}
	if want := certs.Timestamp(certs.RenewAt(leaf.NotBefore, leaf.NotAfter).UnixMilli()); *issued.RenewAt != want {
		t.Fatalf("renew at %s, want %s", *issued.RenewAt, want)
	}
	key := f.privateKey(t, issued.ID)
	if !key.(*ecdsa.PrivateKey).PublicKey.Equal(leaf.PublicKey) {
		t.Fatal("stored key does not match the certificate")
	}
	var accountURL string
	database := f.auth.Database()
	database.QueryRow("SELECT account_url FROM acme_accounts WHERE email='ops@example.com'").Scan(&accountURL)
	if !strings.HasPrefix(accountURL, "https://") {
		t.Fatalf("account URL not stored")
	}
	f.assertNoSecrets(t)

	// Renewal: once the console clock reaches renew_at, the hourly check
	// issues a new certificate with a new key under the same ACME account.
	f.advance(leaf.NotAfter.Sub(f.certs.Now()) - 29*24*time.Hour)
	if err := f.certs.Check(ctx); err != nil {
		t.Fatal(err)
	}
	renewed := f.waitFor(t, issued.ID, func(c certs.Certificate) bool { return c.UpdatedAt != issued.UpdatedAt })
	newPEM, _, _ := f.certs.Chain(ctx, f.session, issued.ID)
	if renewed.Status != "valid" || newPEM == pem || renewed.LastError != nil {
		t.Fatalf("renewal: %+v", renewed)
	}
	if f.privateKey(t, issued.ID).(*ecdsa.PrivateKey).Equal(key) {
		t.Fatal("renewal reused the private key")
	}
	var accounts int
	var renewedURL string
	database.QueryRow("SELECT count(*), max(account_url) FROM acme_accounts").Scan(&accounts, &renewedURL)
	if accounts != 1 || renewedURL != accountURL {
		t.Fatalf("account reuse: %d %s", accounts, renewedURL)
	}
	if presents.Load() != 4 {
		t.Fatalf("DNS records presented %d times, want 4", presents.Load())
	}
}

func (f fixture) privateKey(t *testing.T, id string) crypto.PrivateKey {
	t.Helper()
	var sealed []byte
	if err := f.auth.Database().QueryRow("SELECT private_key FROM certificates WHERE id=?", id).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	plain, err := f.auth.Unseal(certs.CertificateKeyPurpose(id), sealed)
	if err != nil {
		t.Fatal(err)
	}
	key, err := certcrypto.ParsePEMPrivateKey(plain)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestExternalAccountBindingWithPebble(t *testing.T) {
	f := newFixture(t)
	p := startPebble(t, true)
	var presents atomic.Int32
	f.usePebble(p, &presents)
	ctx := context.Background()
	f.certs.Start(ctx)
	defer f.certs.Stop(10 * time.Second)
	credential := f.cloudflare(t)
	request := certs.RequestInput{Domains: []string{"eab.example.test"}, CredentialID: credential.ID, Directory: certs.DirectoryCustom, DirectoryURL: p.directory}

	missing, err := f.certs.Request(ctx, f.session, request)
	if err != nil {
		t.Fatal(err)
	}
	failed := f.waitFor(t, missing.ID, func(c certs.Certificate) bool { return c.Status != "pending" })
	if failed.Status != "failed" || failed.LastError == nil || failed.LastError.Code != certs.ErrACMEAccount || failed.RenewAt != nil {
		t.Fatalf("without EAB: %+v", failed)
	}

	request.EAB = certs.EAB{KeyID: eabKeyID, HMACKey: eabHMAC}
	bound, err := f.certs.Request(ctx, f.session, request)
	if err != nil {
		t.Fatal(err)
	}
	issued := f.waitFor(t, bound.ID, func(c certs.Certificate) bool { return c.Status != "pending" })
	if issued.Status != "valid" {
		t.Fatalf("with EAB: %+v", issued)
	}
	// The binding key is needed only once and is forgotten after registration.
	var eab []byte
	var accountURL string
	f.auth.Database().QueryRow("SELECT eab, account_url FROM acme_accounts").Scan(&eab, &accountURL)
	if eab != nil || accountURL == "" {
		t.Fatal("EAB key kept after registration")
	}
	// The failed request now succeeds on reissue with the registered account.
	if _, err := f.certs.Reissue(ctx, f.session, missing.ID); err != nil {
		t.Fatal(err)
	}
	if again := f.waitFor(t, missing.ID, func(c certs.Certificate) bool { return c.Status != "pending" }); again.Status != "valid" {
		t.Fatalf("reissue: %+v", again)
	}
}

func TestIssuanceErrorsAreCategorized(t *testing.T) {
	f := newFixture(t)
	p := startPebble(t, false)
	var presents atomic.Int32
	f.usePebble(p, &presents)
	ctx := context.Background()
	f.certs.Start(ctx)
	defer f.certs.Stop(10 * time.Second)
	credential := f.cloudflare(t)

	// The provider fails to create the record: no detail is kept.
	f.certs.SetProviderFactory(func(string, map[string]string) (challenge.Provider, error) {
		return failingProvider{}, nil
	})
	request := certs.RequestInput{Domains: []string{"fail.example.test"}, CredentialID: credential.ID, Directory: certs.DirectoryCustom, DirectoryURL: p.directory}
	certificate, err := f.certs.Request(ctx, f.session, request)
	if err != nil {
		t.Fatal(err)
	}
	failed := f.waitFor(t, certificate.ID, func(c certs.Certificate) bool { return c.Status != "pending" })
	if failed.LastError == nil || failed.LastError.Code != certs.ErrDNSProvider || failed.LastError.Detail != nil {
		t.Fatalf("provider failure: %+v", failed.LastError)
	}

	// The record is never published: the CA rejects the challenge and its
	// problem type and detail are kept.
	f.certs.SetProviderFactory(func(string, map[string]string) (challenge.Provider, error) {
		return silentProvider{}, nil
	})
	certificate, err = f.certs.Request(ctx, f.session, request)
	if err != nil {
		t.Fatal(err)
	}
	rejected := f.waitFor(t, certificate.ID, func(c certs.Certificate) bool { return c.Status != "pending" })
	if rejected.LastError == nil || rejected.LastError.Code != certs.ErrACMERejected || rejected.LastError.Detail == nil ||
		!strings.HasPrefix(*rejected.LastError.Detail, "unauthorized") {
		t.Fatalf("CA rejection: %+v", rejected.LastError)
	}

	// An unreachable directory.
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := listener.Addr().String()
	listener.Close()
	request.DirectoryURL = fmt.Sprintf("https://%s/dir", closed)
	certificate, err = f.certs.Request(ctx, f.session, request)
	if err != nil {
		t.Fatal(err)
	}
	unreachableResult := f.waitFor(t, certificate.ID, func(c certs.Certificate) bool { return c.Status != "pending" })
	if unreachableResult.LastError == nil || unreachableResult.LastError.Code != certs.ErrACMEUnreachable {
		t.Fatalf("unreachable: %+v", unreachableResult.LastError)
	}
	f.assertNoSecrets(t)
	if strings.Contains(f.logs.String(), "example.test") {
		t.Fatal("logs contain domain names")
	}
}

type failingProvider struct{}

func (failingProvider) Present(string, string, string) error {
	return errors.New("provider said: token " + cloudflareToken + " lacks permission")
}
func (failingProvider) CleanUp(string, string, string) error { return nil }

type silentProvider struct{}

func (silentProvider) Present(string, string, string) error { return nil }
func (silentProvider) CleanUp(string, string, string) error { return nil }

// hangingDirectory accepts connections and answers only when the request is
// canceled, standing in for a CA that does not respond.
func hangingDirectory(t *testing.T) (string, http.RoundTripper, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	return server.URL + "/dir", server.Client().Transport, &requests
}

func TestStopCancelsRunningJobs(t *testing.T) {
	f := newFixture(t)
	directory, transport, requests := hangingDirectory(t)
	silent := func(string, map[string]string) (challenge.Provider, error) { return silentProvider{}, nil }
	f.certs.SetACMETransport(transport)
	f.certs.SetProviderFactory(silent)
	ctx := context.Background()
	f.certs.Start(ctx)
	credential := f.cloudflare(t)
	certificate, err := f.certs.Request(ctx, f.session, certs.RequestInput{Domains: []string{"stop.example.test"}, CredentialID: credential.ID,
		Directory: certs.DirectoryCustom, DirectoryURL: directory})
	if err != nil {
		t.Fatal(err)
	}
	for requests.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if listed, _ := f.certs.Certificate(ctx, certificate.ID); !listed.Issuing {
		t.Fatal("job not reported as issuing")
	}
	started := time.Now()
	f.certs.Stop(10 * time.Second)
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("stop took %s", elapsed)
	}
	// The interrupted request records nothing and resumes after a restart.
	stopped, _ := f.certs.Certificate(ctx, certificate.ID)
	if stopped.Status != "pending" || stopped.LastError != nil || stopped.Issuing {
		t.Fatalf("after stop: %+v", stopped)
	}
	restarted := certs.New(f.auth, f.certs.Logger())
	restarted.SetACMETransport(transport)
	restarted.SetProviderFactory(silent)
	restarted.Start(ctx)
	for requests.Load() < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	// Deleting the certificate cancels its job.
	if err := restarted.DeleteCertificate(ctx, f.session, certificate.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for restarted.Active(certificate.ID) {
		if time.Now().After(deadline) {
			t.Fatal("deleted certificate's job still running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	restarted.Stop(10 * time.Second)
}

func TestSealedValuesUseDistinctPurposes(t *testing.T) {
	f := newFixture(t)
	sealed, err := f.auth.Seal(certs.CertificateKeyPurpose("a"), []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{certs.CertificateKeyPurpose("b"), certs.AccountKeyPurpose("a"), certs.CredentialPurpose("a")} {
		if _, err := f.auth.Unseal(purpose, sealed); err == nil {
			t.Fatalf("opened with %s", purpose)
		}
	}
	if plain, err := f.auth.Unseal(certs.CertificateKeyPurpose("a"), sealed); err != nil || string(plain) != "key" {
		t.Fatal("round trip failed")
	}
	if _, err := f.auth.Unseal(certs.CertificateKeyPurpose("a"), []byte(base64.StdEncoding.EncodeToString(sealed))); err == nil {
		t.Fatal("garbage opened")
	}
}
