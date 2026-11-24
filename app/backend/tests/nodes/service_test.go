package nodes_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/wheakerd/veilpost/app/backend/internal/nodes"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

const testPassword = "correct-password-for-node-tests"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fixture struct {
	auth    *auth.Service
	nodes   *nodes.Service
	session string
	now     *time.Time
	mu      *sync.Mutex
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	service, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	if err := service.CreateUser(context.Background(), "node-admin", testPassword); err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), "node-admin", testPassword, "", "test-ip")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	var mu sync.Mutex
	nodeService := nodes.New(service)
	nodeService.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	return fixture{service, nodeService, result.Token, &now, &mu}
}

func (f fixture) advance(d time.Duration) { f.mu.Lock(); *f.now = f.now.Add(d); f.mu.Unlock() }
func (f fixture) clock() time.Time        { f.mu.Lock(); defer f.mu.Unlock(); return *f.now }

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var api *auth.APIError
	if !errors.As(err, &api) || api.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}

type agent struct {
	id  string
	key ed25519.PrivateKey
}

func (f fixture) create(t *testing.T, name string) (nodes.Node, nodes.Enrollment) {
	t.Helper()
	node, enrollment, err := f.nodes.Create(context.Background(), f.session, name, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return node, enrollment
}

func enrollRequest(t *testing.T, token string) (nodes.EnrollRequest, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(public)
	return nodes.EnrollRequest{Token: token, PublicKey: encoded, AgentVersion: "1.0.0",
		Proof: base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, nodes.EnrollMessage(token, encoded)))}, private
}

func (f fixture) enrolled(t *testing.T, name string) agent {
	t.Helper()
	_, enrollment := f.create(t, name)
	request, key := enrollRequest(t, enrollment.Token)
	result, err := f.nodes.Enroll(context.Background(), "agent-ip", request)
	if err != nil {
		t.Fatal(err)
	}
	return agent{result.NodeID, key}
}

var nonceCounter int

func (a agent) signed(at time.Time, body string) nodes.SignedRequest {
	nonceCounter++
	nonce := base64.RawURLEncoding.EncodeToString([]byte("test-nonce-" + strconv.Itoa(nonceCounter) + "-padding"))
	return a.signedWith(at, nonce, "/api/agent/poll", body)
}

func (a agent) signedWith(at time.Time, nonce, route, body string) nodes.SignedRequest {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	signature := ed25519.Sign(a.key, nodes.RequestMessage("POST", route, a.id, timestamp, nonce, []byte(body)))
	return nodes.SignedRequest{Method: "POST", Route: route, NodeID: a.id, Timestamp: timestamp, Nonce: nonce,
		Signature: base64.RawURLEncoding.EncodeToString(signature), Body: []byte(body)}
}

// The same vector is pinned in app/agent so both sides sign identical bytes.
func TestSignatureKnownAnswer(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	key := ed25519.NewKeyFromSeed(seed)
	message := nodes.RequestMessage("POST", "/api/agent/poll", "AAAAAAAAAAAAAAAAAAAAAA", "1790000000", "bm9uY2Utbm9uY2Utbm9uY2U", []byte(`{"agentVersion":"1.0.0","configVersion":0}`))
	const wantMessage = "vpn-agent-v1\nPOST\n/api/agent/poll\nAAAAAAAAAAAAAAAAAAAAAA\n1790000000\nbm9uY2Utbm9uY2Utbm9uY2U\n"
	if !strings.HasPrefix(string(message), wantMessage) || len(message) != len(wantMessage)+64 {
		t.Fatalf("unexpected canonical message %q", message)
	}
	if got := hex.EncodeToString(ed25519.Sign(key, message)); got != knownSignature {
		t.Fatalf("signature changed: %s", got)
	}
	enroll := hex.EncodeToString(ed25519.Sign(key, nodes.EnrollMessage("token-token-token-token-token-token-token-t", "public-key")))
	if enroll != knownEnrollSignature {
		t.Fatalf("enroll signature changed: %s", enroll)
	}
}

const knownSignature = "d0802d4a9b45eb87ef4740691ae9bba769874d391b7c107f31a44640ecf8993fc1ae3efcb8092369fea18fbfe55c379f268b02952b42bf9c294affb313a8f907"
const knownEnrollSignature = "8a0ba83439b4f957d6db31b62f3b7df83d2a500abd54c86a28ee9d59a59671888b35a26cd53ad49c1878d406082b703dc4824dc1810cd6e0da3fd9ef90c9ec0c"

func TestCreateStoresOnlyTokenHashAndRequiresPassword(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, _, err := f.nodes.Create(ctx, f.session, "edge-1", "wrong-password-value"); err == nil {
		t.Fatal("wrong password accepted")
	} else {
		expectCode(t, err, "INVALID_PASSWORD")
	}
	_, _, err := f.nodes.Create(ctx, "not-a-session", "edge-1", testPassword)
	expectCode(t, err, "SESSION_EXPIRED")
	for _, name := range []string{"", "   ", strings.Repeat("节", 65), "bad\nname", "bad name"} {
		_, _, err := f.nodes.Create(ctx, f.session, name, testPassword)
		expectCode(t, err, "INVALID_NAME")
	}
	node, enrollment := f.create(t, "  edge-1  ")
	if node.Name != "edge-1" || node.Status != "pending" || node.Online || node.EnrollmentExpiresAt == nil || *node.EnrollmentExpiresAt != enrollment.ExpiresAt {
		t.Fatalf("unexpected node %+v", node)
	}
	expires, _ := time.Parse(time.RFC3339Nano, enrollment.ExpiresAt)
	if !expires.Equal(f.clock().Add(nodes.EnrollmentLifetime)) || len(enrollment.Token) != 43 {
		t.Fatalf("unexpected enrollment %v", enrollment.ExpiresAt)
	}
	var stored int
	db := f.auth.Database()
	if err := db.QueryRow("SELECT count(*) FROM node_enrollments WHERE token_hash=?", auth.Digest(enrollment.Token)).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("token hash not stored: %v %d", err, stored)
	}
	if err := db.QueryRow("SELECT count(*) FROM node_enrollments WHERE token_hash=?", enrollment.Token).Scan(&stored); err != nil || stored != 0 {
		t.Fatal("plaintext token stored")
	}
	list, err := f.nodes.List(ctx, f.session)
	if err != nil || len(list) != 1 || list[0].ID != node.ID {
		t.Fatalf("list: %v %+v", err, list)
	}
	if _, err := f.nodes.List(ctx, ""); err == nil {
		t.Fatal("list without session")
	}
}

func TestEnrollmentTokenIsSingleUseAndExpires(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, enrollment := f.create(t, "edge-1")
	request, _ := enrollRequest(t, enrollment.Token)
	bad := request
	bad.Proof = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	_, err := f.nodes.Enroll(ctx, "ip", bad)
	expectCode(t, err, "REQUEST_FAILED")
	result, err := f.nodes.Enroll(ctx, "ip", request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "edge-1" {
		t.Fatalf("unexpected result %+v", result)
	}
	// The same token never works twice, not even with another key.
	again, _ := enrollRequest(t, enrollment.Token)
	_, err = f.nodes.Enroll(ctx, "ip", again)
	expectCode(t, err, "INVALID_ENROLLMENT")
	list, _ := f.nodes.List(ctx, f.session)
	if list[0].Status != "active" || list[0].EnrolledAt == nil || list[0].EnrollmentExpiresAt != nil || *list[0].AgentVersion != "1.0.0" {
		t.Fatalf("unexpected node %+v", list[0])
	}

	// A token is rejected once its 15 minutes pass.
	_, expiring := f.create(t, "edge-2")
	f.advance(nodes.EnrollmentLifetime)
	late, _ := enrollRequest(t, expiring.Token)
	_, err = f.nodes.Enroll(ctx, "ip", late)
	expectCode(t, err, "INVALID_ENROLLMENT")
	list, _ = f.nodes.List(ctx, f.session)
	for _, node := range list {
		if node.Name == "edge-2" && (node.Status != "pending" || node.EnrollmentExpiresAt != nil) {
			t.Fatalf("expired enrollment still offered: %+v", node)
		}
	}

	// A revoked pending node cannot be enrolled with its unused token.
	pending, token := f.create(t, "edge-3")
	if _, err := f.nodes.Revoke(ctx, f.session, pending.ID, testPassword); err != nil {
		t.Fatal(err)
	}
	revoked, _ := enrollRequest(t, token.Token)
	_, err = f.nodes.Enroll(ctx, "ip", revoked)
	expectCode(t, err, "INVALID_ENROLLMENT")
}

func TestReissueReplacesPendingTokensOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pending, first := f.create(t, "edge-reissue")
	f.advance(nodes.EnrollmentLifetime)
	_, _, err := f.nodes.Reissue(ctx, f.session, pending.ID, "wrong-password-value")
	expectCode(t, err, "INVALID_PASSWORD")
	node, second, err := f.nodes.Reissue(ctx, f.session, pending.ID, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token || node.Status != "pending" || node.EnrollmentExpiresAt == nil {
		t.Fatalf("unexpected reissue %+v %+v", node, second)
	}
	// Reissuing again invalidates the previous, still unexpired token.
	_, third, err := f.nodes.Reissue(ctx, f.session, pending.ID, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	stale, _ := enrollRequest(t, second.Token)
	_, err = f.nodes.Enroll(ctx, "ip", stale)
	expectCode(t, err, "INVALID_ENROLLMENT")
	fresh, _ := enrollRequest(t, third.Token)
	if _, err = f.nodes.Enroll(ctx, "ip", fresh); err != nil {
		t.Fatal(err)
	}
	_, _, err = f.nodes.Reissue(ctx, f.session, pending.ID, testPassword)
	expectCode(t, err, "NODE_NOT_PENDING")
	_, _, err = f.nodes.Reissue(ctx, f.session, "AAAAAAAAAAAAAAAAAAAAAA", testPassword)
	expectCode(t, err, "NODE_NOT_FOUND")
}

func TestDeleteRemovesOnlyRevokedNodesAndTheirRecords(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.enrolled(t, "edge-delete")
	if _, err := f.nodes.Authenticate(ctx, a.signed(f.clock(), "{}")); err != nil {
		t.Fatal(err)
	}
	expectCode(t, f.nodes.Delete(ctx, f.session, a.id, testPassword), "NODE_NOT_REVOKED")
	if _, err := f.nodes.Revoke(ctx, f.session, a.id, testPassword); err != nil {
		t.Fatal(err)
	}
	expectCode(t, f.nodes.Delete(ctx, f.session, a.id, "wrong-password-value"), "INVALID_PASSWORD")
	if err := f.nodes.Delete(ctx, f.session, a.id, testPassword); err != nil {
		t.Fatal(err)
	}
	list, err := f.nodes.List(ctx, f.session)
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted node still listed: %+v %v", list, err)
	}
	var rows int
	if err := f.auth.Database().QueryRow("SELECT (SELECT count(*) FROM node_enrollments)+(SELECT count(*) FROM agent_nonces)").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("records left behind: %d %v", rows, err)
	}
	expectCode(t, f.nodes.Delete(ctx, f.session, a.id, testPassword), "NODE_NOT_FOUND")
}

func TestEnrollRejectsReusedKeysAndConcurrentRedemption(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, first := f.create(t, "edge-1")
	request, key := enrollRequest(t, first.Token)
	if _, err := f.nodes.Enroll(ctx, "ip", request); err != nil {
		t.Fatal(err)
	}
	_, second := f.create(t, "edge-2")
	public := base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	reuse := nodes.EnrollRequest{Token: second.Token, PublicKey: public, AgentVersion: "1.0.0",
		Proof: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, nodes.EnrollMessage(second.Token, public)))}
	_, err := f.nodes.Enroll(ctx, "ip", reuse)
	expectCode(t, err, "KEY_IN_USE")

	_, third := f.create(t, "edge-3")
	var wins, losses int
	var mu sync.Mutex
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			request, _ := enrollRequest(t, third.Token)
			_, err := f.nodes.Enroll(ctx, "ip", request)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else {
				losses++
			}
		}()
	}
	group.Wait()
	if wins != 1 || losses != 7 {
		t.Fatalf("token redeemed %d times", wins)
	}
}

func TestSignedRequestsRejectReplaySkewTamperingAndUnknownNodes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.enrolled(t, "edge-1")
	body := `{"agentVersion":"1.0.0","configVersion":0}`
	request := a.signed(f.clock(), body)
	if id, err := f.nodes.Authenticate(ctx, request); err != nil || id != a.id {
		t.Fatalf("valid request rejected: %v", err)
	}
	_, err := f.nodes.Authenticate(ctx, request)
	expectCode(t, err, "AGENT_UNAUTHORIZED")

	for _, offset := range []time.Duration{-nodes.ClockSkew - time.Second, nodes.ClockSkew + time.Second} {
		_, err = f.nodes.Authenticate(ctx, a.signed(f.clock().Add(offset), body))
		expectCode(t, err, "CLOCK_SKEW")
	}
	for _, offset := range []time.Duration{-nodes.ClockSkew, nodes.ClockSkew} {
		if _, err = f.nodes.Authenticate(ctx, a.signed(f.clock().Add(offset), body)); err != nil {
			t.Fatalf("request at the skew boundary rejected: %v", err)
		}
	}

	tampered := a.signed(f.clock(), body)
	tampered.Body = []byte(`{"agentVersion":"9.9.9","configVersion":0}`)
	_, err = f.nodes.Authenticate(ctx, tampered)
	expectCode(t, err, "AGENT_UNAUTHORIZED")
	otherRoute := a.signedWith(f.clock(), "b3RoZXItcm91dGUtbm9uY2U", "/api/agent/enroll", body)
	otherRoute.Route = "/api/agent/poll"
	_, err = f.nodes.Authenticate(ctx, otherRoute)
	expectCode(t, err, "AGENT_UNAUTHORIZED")

	stranger := agent{id: "AAAAAAAAAAAAAAAAAAAAAA", key: a.key}
	_, err = f.nodes.Authenticate(ctx, stranger.signed(f.clock(), body))
	expectCode(t, err, "AGENT_UNAUTHORIZED")
	_, impostorKey, _ := ed25519.GenerateKey(nil)
	impostor := agent{id: a.id, key: impostorKey}
	_, err = f.nodes.Authenticate(ctx, impostor.signed(f.clock(), body))
	expectCode(t, err, "AGENT_UNAUTHORIZED")
	pending, _ := f.create(t, "edge-pending")
	_, err = f.nodes.Authenticate(ctx, agent{id: pending.ID, key: a.key}.signed(f.clock(), body))
	expectCode(t, err, "AGENT_UNAUTHORIZED")

	malformed := a.signed(f.clock(), body)
	malformed.Nonce = "short"
	_, err = f.nodes.Authenticate(ctx, malformed)
	expectCode(t, err, "AGENT_UNAUTHORIZED")

	// A nonce stays remembered for its whole acceptance window, even after Prune.
	replay := a.signed(f.clock(), body)
	if _, err = f.nodes.Authenticate(ctx, replay); err != nil {
		t.Fatal(err)
	}
	f.advance(nodes.ClockSkew)
	if err := f.auth.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = f.nodes.Authenticate(ctx, replay)
	expectCode(t, err, "AGENT_UNAUTHORIZED")
}

func TestRevocationRejectsSignaturesAndWakesPolls(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.enrolled(t, "edge-1")
	f.nodes.SetPollHold(time.Minute)
	body := `{"agentVersion":"1.0.0","configVersion":0}`
	if _, err := f.nodes.Authenticate(ctx, a.signed(f.clock(), body)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.nodes.Poll(ctx, a.id, "1.0.0", 0)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_, err := f.nodes.Revoke(ctx, f.session, a.id, "wrong-password-value")
	expectCode(t, err, "INVALID_PASSWORD")
	node, err := f.nodes.Revoke(ctx, f.session, a.id, testPassword)
	if err != nil || node.Status != "revoked" || node.RevokedAt == nil || node.Online {
		t.Fatalf("revoke: %v %+v", err, node)
	}
	select {
	case err := <-done:
		expectCode(t, err, "NODE_REVOKED")
	case <-time.After(5 * time.Second):
		t.Fatal("waiting poll was not released by revocation")
	}
	_, err = f.nodes.Authenticate(ctx, a.signed(f.clock(), body))
	expectCode(t, err, "NODE_REVOKED")
	_, err = f.nodes.Poll(ctx, a.id, "1.0.0", 0)
	expectCode(t, err, "NODE_REVOKED")
	// Revocation status is not disclosed without a valid signature.
	_, other, _ := ed25519.GenerateKey(nil)
	_, err = f.nodes.Authenticate(ctx, agent{id: a.id, key: other}.signed(f.clock(), body))
	expectCode(t, err, "AGENT_UNAUTHORIZED")
	again, err := f.nodes.Revoke(ctx, f.session, a.id, testPassword)
	if err != nil || again.Status != "revoked" {
		t.Fatalf("revoking twice: %v", err)
	}
	_, err = f.nodes.Revoke(ctx, f.session, "AAAAAAAAAAAAAAAAAAAAAA", testPassword)
	expectCode(t, err, "NODE_NOT_FOUND")
	_, err = f.nodes.Revoke(ctx, f.session, "../etc", testPassword)
	expectCode(t, err, "NODE_NOT_FOUND")
}

func TestPerNodeRateLimitIsIndependent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.nodes.SetLimits(3, 30)
	a, b := f.enrolled(t, "edge-1"), f.enrolled(t, "edge-2")
	body := `{}`
	for range 3 {
		if _, err := f.nodes.Authenticate(ctx, a.signed(f.clock(), body)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.nodes.Authenticate(ctx, a.signed(f.clock(), body))
	expectCode(t, err, "TOO_MANY_ATTEMPTS")
	if _, err := f.nodes.Authenticate(ctx, b.signed(f.clock(), body)); err != nil {
		t.Fatalf("another node shares the budget: %v", err)
	}
	// Agent traffic does not consume the login budget.
	if _, err := f.auth.Login(ctx, "node-admin", testPassword, "", "agent-ip"); err != nil {
		t.Fatal(err)
	}
	// Invalid signatures do not consume a node's budget: b has two requests left.
	_, stranger, _ := ed25519.GenerateKey(nil)
	for range 5 {
		_, err := f.nodes.Authenticate(ctx, agent{id: b.id, key: stranger}.signed(f.clock(), body))
		expectCode(t, err, "AGENT_UNAUTHORIZED")
	}
	for range 2 {
		if _, err := f.nodes.Authenticate(ctx, b.signed(f.clock(), body)); err != nil {
			t.Fatalf("invalid signatures consumed the budget: %v", err)
		}
	}

	f.nodes.SetLimits(120, 2)
	for range 2 {
		request, _ := enrollRequest(t, strings.Repeat("A", 43))
		_, err := f.nodes.Enroll(ctx, "enroll-ip", request)
		expectCode(t, err, "INVALID_ENROLLMENT")
	}
	request, _ := enrollRequest(t, strings.Repeat("A", 43))
	_, err = f.nodes.Enroll(ctx, "enroll-ip", request)
	expectCode(t, err, "TOO_MANY_ATTEMPTS")
}

func TestPollRecordsOnlineStatusAndHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.enrolled(t, "edge-1")
	list, _ := f.nodes.List(ctx, f.session)
	if list[0].Online || list[0].LastSeenAt != nil {
		t.Fatalf("enrolled node online before any signed request: %+v", list[0])
	}
	f.nodes.SetPollHold(20 * time.Millisecond)
	started := time.Now()
	result, err := f.nodes.Poll(ctx, a.id, "1.2.3", 0)
	if err != nil || result.ConfigVersion != 0 || time.Since(started) < f.nodes.PollHold() {
		t.Fatalf("poll: %v %+v", err, result)
	}
	list, _ = f.nodes.List(ctx, f.session)
	if !list[0].Online || list[0].LastSeenAt == nil || *list[0].AgentVersion != "1.2.3" {
		t.Fatalf("poll did not record status: %+v", list[0])
	}
	f.advance(nodes.OnlineWindow + time.Second)
	list, _ = f.nodes.List(ctx, f.session)
	if list[0].Online {
		t.Fatal("stale node still online")
	}
	// A different configuration version answers at once.
	f.nodes.SetPollHold(time.Minute)
	started = time.Now()
	if result, err = f.nodes.Poll(ctx, a.id, "1.2.3", 7); err != nil || result.ConfigVersion != 0 || time.Since(started) > 5*time.Second {
		t.Fatalf("version mismatch poll: %v %+v", err, result)
	}
	_, err = f.nodes.Poll(ctx, a.id, "bad version", 0)
	expectCode(t, err, "REQUEST_FAILED")
	// Shutdown releases waiting polls.
	done := make(chan error, 1)
	go func() { _, err := f.nodes.Poll(ctx, a.id, "1.2.3", 0); done <- err }()
	time.Sleep(50 * time.Millisecond)
	f.nodes.Shutdown()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not release the poll")
	}
}

func TestPruneRemovesExpiredEnrollmentsAndNonces(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := f.enrolled(t, "edge-1")
	_, fresh := f.create(t, "edge-2")
	// Rows stamped in the past expire relative to the auth clock (real time).
	f.advance(-24 * time.Hour)
	_, old := f.create(t, "edge-3")
	if _, err := f.nodes.Authenticate(ctx, a.signed(f.clock(), `{}`)); err != nil {
		t.Fatal(err)
	}
	if err := f.auth.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	db := f.auth.Database()
	var count int
	db.QueryRow("SELECT count(*) FROM node_enrollments WHERE token_hash=?", auth.Digest(old.Token)).Scan(&count)
	if count != 0 {
		t.Fatal("expired enrollment kept")
	}
	db.QueryRow("SELECT count(*) FROM node_enrollments WHERE token_hash=?", auth.Digest(fresh.Token)).Scan(&count)
	if count != 1 {
		t.Fatal("valid enrollment pruned")
	}
	db.QueryRow("SELECT count(*) FROM agent_nonces").Scan(&count)
	if count != 0 {
		t.Fatal("expired nonce kept")
	}
}

func newRouter(f fixture, development bool) *gin.Engine {
	router := gin.New()
	authHandler := &auth.Handler{Service: f.auth, Development: development, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := &nodes.Handler{Service: f.nodes, Auth: authHandler, Logger: authHandler.Logger}
	handler.RegisterConsole(router.Group("/api/nodes"))
	handler.RegisterAgent(router.Group("/api/agent"))
	return router
}

func TestHTTPConsoleAndAgentFlow(t *testing.T) {
	f := newFixture(t)
	router := newRouter(f, true)
	call := func(method, path, body string, header map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "http://console.test"+path, strings.NewReader(body))
		for key, value := range header {
			request.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	browser := map[string]string{"Origin": "http://console.test", "X-Requested-With": "Veilpost",
		"Content-Type": "application/json", "Cookie": auth.CookieName + "=" + f.session}

	if response := call("GET", "/api/nodes", "", nil); response.Code != 401 {
		t.Fatalf("anonymous list: %d", response.Code)
	}
	response := call("GET", "/api/nodes", "", map[string]string{"Cookie": auth.CookieName + "=" + f.session})
	if response.Code != 200 || response.Body.String() != `{"nodes":[]}` {
		t.Fatalf("empty list: %d %s", response.Code, response.Body)
	}
	for _, header := range []string{"Origin", "X-Requested-With", "Content-Type"} {
		changed := map[string]string{}
		for key, value := range browser {
			if key != header {
				changed[key] = value
			}
		}
		if response := call("POST", "/api/nodes", `{"name":"edge","password":"`+testPassword+`"}`, changed); response.Code != 403 {
			t.Fatalf("write without %s: %d", header, response.Code)
		}
	}
	crossSite := map[string]string{}
	for key, value := range browser {
		crossSite[key] = value
	}
	crossSite["Origin"] = "http://evil.test"
	if response := call("POST", "/api/nodes", `{"name":"edge","password":"`+testPassword+`"}`, crossSite); response.Code != 403 {
		t.Fatalf("cross-site write: %d", response.Code)
	}
	if response := call("POST", "/api/nodes", `{"name":"edge","password":"`+testPassword+`","extra":1}`, browser); response.Code != 400 {
		t.Fatalf("unknown field: %d", response.Code)
	}
	response = call("POST", "/api/nodes", `{"name":"edge","password":"`+testPassword+`"}`, browser)
	if response.Code != 201 {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}
	var created struct {
		Node       nodes.Node       `json:"node"`
		Enrollment nodes.Enrollment `json:"enrollment"`
	}
	json.Unmarshal(response.Body.Bytes(), &created)
	if created.Node.Status != "pending" || len(created.Enrollment.Token) != 43 {
		t.Fatalf("unexpected create response %s", response.Body)
	}
	// The token is never returned again.
	response = call("GET", "/api/nodes", "", map[string]string{"Cookie": auth.CookieName + "=" + f.session})
	if strings.Contains(response.Body.String(), created.Enrollment.Token) {
		t.Fatal("list returned the enrollment token")
	}

	enroll, key := enrollRequest(t, created.Enrollment.Token)
	payload, _ := json.Marshal(enroll)
	if response := call("POST", "/api/agent/enroll", string(payload), map[string]string{"Content-Type": "text/plain"}); response.Code != 415 {
		t.Fatalf("enroll content type: %d", response.Code)
	}
	response = call("POST", "/api/agent/enroll", string(payload), map[string]string{"Content-Type": "application/json"})
	if response.Code != 200 || !strings.Contains(response.Body.String(), created.Node.ID) {
		t.Fatalf("enroll: %d %s", response.Code, response.Body)
	}
	a := agent{created.Node.ID, key}
	f.nodes.SetPollHold(10 * time.Millisecond)
	signedCall := func(request nodes.SignedRequest) *httptest.ResponseRecorder {
		return call("POST", request.Route, string(request.Body), map[string]string{"Content-Type": "application/json",
			nodes.HeaderNode: request.NodeID, nodes.HeaderTimestamp: request.Timestamp, nodes.HeaderNonce: request.Nonce, nodes.HeaderSignature: request.Signature})
	}
	poll := a.signed(f.clock(), `{"agentVersion":"1.0.0","configVersion":0,"future":true}`)
	if response := signedCall(poll); response.Code != 200 || response.Body.String() != `{"configVersion":0}` {
		t.Fatalf("poll: %d %s", response.Code, response.Body)
	}
	if response := signedCall(poll); response.Code != 401 || !strings.Contains(response.Body.String(), "AGENT_UNAUTHORIZED") {
		t.Fatalf("replayed poll: %d %s", response.Code, response.Body)
	}
	if response := signedCall(a.signed(f.clock().Add(-2*time.Minute), `{}`)); response.Code != 401 || !strings.Contains(response.Body.String(), "CLOCK_SKEW") {
		t.Fatalf("skewed poll: %d %s", response.Code, response.Body)
	}
	oversized := a.signed(f.clock(), `{"pad":"`+strings.Repeat("x", nodes.AgentBodyLimit)+`"}`)
	if response := signedCall(oversized); response.Code != 413 {
		t.Fatalf("oversized poll: %d", response.Code)
	}

	revokePath := "/api/nodes/" + created.Node.ID + "/revoke"
	if response := call("POST", revokePath, `{"password":"wrong-password-value"}`, browser); response.Code != 403 || !strings.Contains(response.Body.String(), "INVALID_PASSWORD") {
		t.Fatalf("revoke with wrong password: %d %s", response.Code, response.Body)
	}
	if response := call("POST", revokePath, `{"password":"`+testPassword+`"}`, browser); response.Code != 200 || !strings.Contains(response.Body.String(), `"status":"revoked"`) {
		t.Fatalf("revoke: %d %s", response.Code, response.Body)
	}
	if response := signedCall(a.signed(f.clock(), `{"agentVersion":"1.0.0","configVersion":0}`)); response.Code != 403 || !strings.Contains(response.Body.String(), "NODE_REVOKED") {
		t.Fatalf("revoked poll: %d %s", response.Code, response.Body)
	}
	deletePath := "/api/nodes/" + created.Node.ID + "/delete"
	if response := call("POST", deletePath, `{"password":"`+testPassword+`"}`, crossSite); response.Code != 403 {
		t.Fatalf("cross-site delete: %d", response.Code)
	}
	if response := call("POST", deletePath, `{"password":"`+testPassword+`"}`, browser); response.Code != 204 {
		t.Fatalf("delete: %d %s", response.Code, response.Body)
	}

	response = call("POST", "/api/nodes", `{"name":"later","password":"`+testPassword+`"}`, browser)
	json.Unmarshal(response.Body.Bytes(), &created)
	reissuePath := "/api/nodes/" + created.Node.ID + "/enrollment"
	if response := call("POST", reissuePath, `{"password":"`+testPassword+`"}`, crossSite); response.Code != 403 {
		t.Fatalf("cross-site reissue: %d", response.Code)
	}
	response = call("POST", reissuePath, `{"password":"`+testPassword+`"}`, browser)
	var reissued struct {
		Enrollment nodes.Enrollment `json:"enrollment"`
	}
	json.Unmarshal(response.Body.Bytes(), &reissued)
	if response.Code != 200 || len(reissued.Enrollment.Token) != 43 || reissued.Enrollment.Token == created.Enrollment.Token {
		t.Fatalf("reissue: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/nodes/"+created.Node.ID+"/delete", `{"password":"`+testPassword+`"}`, browser); response.Code != 409 || !strings.Contains(response.Body.String(), "NODE_NOT_REVOKED") {
		t.Fatalf("delete pending node: %d %s", response.Code, response.Body)
	}
}
