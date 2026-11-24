package auth_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

const testPassword = "correct-password-for-real-tests"

func fixture(t *testing.T) (*auth.Service, *time.Time, string) {
	t.Helper()
	dir := t.TempDir()
	service, err := auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })
	if err := service.CreateUser(context.Background(), "test-user", testPassword); err != nil {
		t.Fatal(err)
	}
	return service, &now, dir
}
func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var api *auth.APIError
	if !errors.As(err, &api) || api.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}
func loginOK(t *testing.T, s *auth.Service) auth.Result {
	t.Helper()
	result, err := s.Login(context.Background(), "test-user", testPassword, "", "test-ip")
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func otpCode(t *testing.T, secret string, now time.Time) string {
	t.Helper()
	value, err := totp.GenerateCode(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func setupMFA(t *testing.T, s *auth.Service, token string, now time.Time) (auth.Result, auth.Setup, []string) {
	t.Helper()
	setup, err := s.BeginSetup(context.Background(), token, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	result, codes, err := s.Enable(context.Background(), token, setup.ID, otpCode(t, setup.Secret, now))
	if err != nil {
		t.Fatal(err)
	}
	return result, setup, codes
}

func TestSessionPersistsWithoutPlaintextSecrets(t *testing.T) {
	s, now, dir := fixture(t)
	result := loginOK(t, s)
	var password, storedToken string
	if err := s.Database().QueryRow("SELECT password_hash FROM users").Scan(&password); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(password, "$argon2id$") || strings.Contains(password, testPassword) {
		t.Fatal("password not securely hashed")
	}
	if err := s.Database().QueryRow("SELECT token_hash FROM sessions").Scan(&storedToken); err != nil {
		t.Fatal(err)
	}
	if storedToken != auth.Digest(result.Token) || storedToken == result.Token {
		t.Fatal("raw session token was persisted")
	}
	s.Close()
	reopened, err := auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.SetClock(func() time.Time { return *now })
	session, err := reopened.Session(context.Background(), result.Token)
	if err != nil || session.Status != "authenticated" {
		t.Fatalf("session did not survive restart: %+v %v", session, err)
	}
	if err := reopened.Logout(context.Background(), result.Token); err != nil {
		t.Fatal(err)
	}
	session, _ = reopened.Session(context.Background(), result.Token)
	if session.Status != "anonymous" {
		t.Fatal("logout did not revoke session")
	}
}

func TestRealMFAReplayProtectionAndDisabling(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	initial := loginOK(t, s)
	enabled, setup, codes := setupMFA(t, s, initial.Token, *now)
	var encrypted []byte
	if err := s.Database().QueryRow("SELECT totp_secret FROM users").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte(setup.Secret)) {
		t.Fatal("binding secret stored in plaintext")
	}
	var hash string
	if err := s.Database().QueryRow("SELECT code_hash FROM recovery_codes LIMIT 1").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	for _, code := range codes {
		if hash == code {
			t.Fatal("recovery code stored in plaintext")
		}
	}
	old, _ := s.Session(ctx, initial.Token)
	if old.Status != "anonymous" {
		t.Fatal("MFA enable did not rotate/revoke prior session")
	}
	if !enabled.Session.User.TwoFactorEnabled {
		t.Fatal("MFA not enabled")
	}
	pending := loginOK(t, s)
	if pending.Session.Status != "mfa_required" {
		t.Fatal("MFA bypassed")
	}
	if _, err := s.BeginSetup(ctx, pending.Token, testPassword); err == nil {
		t.Fatal("pending session accessed protected action")
	}
	_, err := s.Verify(ctx, pending.Token, "totp", otpCode(t, setup.Secret, *now))
	expectCode(t, err, "INVALID_CODE")
	*now = now.Add(30 * time.Second)
	verified, err := s.Verify(ctx, pending.Token, "totp", otpCode(t, setup.Secret, *now))
	if err != nil {
		t.Fatal(err)
	}
	next := loginOK(t, s)
	_, err = s.Verify(ctx, next.Token, "totp", otpCode(t, setup.Secret, *now))
	expectCode(t, err, "INVALID_CODE")
	*now = now.Add(30 * time.Second)
	disabled, err := s.Disable(ctx, verified.Token, testPassword, otpCode(t, setup.Secret, *now))
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Session.User.TwoFactorEnabled {
		t.Fatal("MFA remained enabled")
	}
	other, _ := s.Session(ctx, next.Token)
	if other.Status != "anonymous" {
		t.Fatal("other sessions not revoked")
	}
	var count int
	s.Database().QueryRow("SELECT count(*) FROM recovery_codes").Scan(&count)
	if count != 0 {
		t.Fatal("recovery codes survived disabling")
	}
}

func TestSetupReturnsPrivateInlineQRCodeWithQuietZone(t *testing.T) {
	s, _, _ := fixture(t)
	signed := loginOK(t, s)
	setup, err := s.BeginSetup(context.Background(), signed.Token, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(setup.QRCodeDataURL, prefix) {
		t.Fatal("setup must include an inline PNG, never an external QR service")
	}
	content, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(setup.QRCodeDataURL, prefix))
	if err != nil {
		t.Fatal("QR payload is not base64")
	}
	qr, err := png.Decode(bytes.NewReader(content))
	if err != nil || qr.Bounds().Dx() != 320 || qr.Bounds().Dy() != 320 {
		t.Fatal("QR payload must be a 320px PNG")
	}
	darkPixels := 0
	for y := range 320 {
		for x := range 320 {
			r, g, b, a := qr.At(x, y).RGBA()
			if x < 32 || x >= 288 || y < 32 || y >= 288 {
				if r != 65535 || g != 65535 || b != 65535 || a != 65535 {
					t.Fatal("QR quiet zone must remain opaque white")
				}
			} else if r == 0 && g == 0 && b == 0 {
				darkPixels++
			}
		}
	}
	if darkPixels == 0 {
		t.Fatal("QR image is blank")
	}
	session, err := s.Session(context.Background(), signed.Token)
	if err != nil || session.User == nil || session.User.TwoFactorEnabled {
		t.Fatal("showing a QR must not enable MFA before code verification")
	}
}

func TestRecoveryConsumptionIsAtomic(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	_, _, codes := setupMFA(t, s, loginOK(t, s).Token, *now)
	pending := loginOK(t, s)
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := s.Verify(ctx, pending.Token, "recovery", codes[0])
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("recovery code accepted %d times", success)
	}
	pending = loginOK(t, s)
	_, err := s.Verify(ctx, pending.Token, "recovery", codes[0])
	expectCode(t, err, "INVALID_RECOVERY_CODE")
	result, err := s.Verify(ctx, pending.Token, "recovery", codes[1])
	if err != nil || !result.Session.User.TwoFactorEnabled {
		t.Fatal("valid recovery login failed or disabled MFA")
	}
}

func TestExpiryAndSetupBinding(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	first := loginOK(t, s)
	other := loginOK(t, s)
	setup, err := s.BeginSetup(ctx, first.Token, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.Enable(ctx, other.Token, setup.ID, otpCode(t, setup.Secret, *now))
	expectCode(t, err, "SETUP_EXPIRED")
	*now = now.Add(6 * time.Minute)
	_, _, err = s.Enable(ctx, first.Token, setup.ID, otpCode(t, setup.Secret, *now))
	expectCode(t, err, "SETUP_EXPIRED")
	_, _, _ = setupMFA(t, s, first.Token, *now)
	pending := loginOK(t, s)
	*now = now.Add(6 * time.Minute)
	_, err = s.Verify(ctx, pending.Token, "recovery", "invalid")
	expectCode(t, err, "CHALLENGE_EXPIRED")
	*now = now.Add(8 * time.Hour)
	session, err := s.Session(ctx, first.Token)
	if err != nil || session.Status != "anonymous" {
		t.Fatal("expired session remained authenticated")
	}
}

func TestRateLimitAndPasswordErrors(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	for range 10 {
		_, err := s.Login(ctx, "test-user", "incorrect", "", "test-ip")
		expectCode(t, err, "INVALID_CREDENTIALS")
	}
	_, err := s.Login(ctx, "test-user", testPassword, "", "test-ip")
	expectCode(t, err, "TOO_MANY_ATTEMPTS")
	*now = now.Add(5*time.Minute + time.Second)
	signed := loginOK(t, s)
	_, err = s.BeginSetup(ctx, signed.Token, "incorrect")
	expectCode(t, err, "INVALID_PASSWORD")
}

func TestHTTPOriginCookiesAndRealResponses(t *testing.T) {
	s, _, _ := fixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &auth.Handler{Service: s, Development: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler.Register(router.Group("/api/auth"))
	request := func(origin, requested string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://console.local/api/auth/login", strings.NewReader(body))
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Requested-With", requested)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	body := `{"username":"test-user","password":"` + testPassword + `"}`
	if request("http://evil.local", "Veilpost", body).Code != 403 {
		t.Fatal("foreign origin accepted")
	}
	if request("http://console.local", "", body).Code != 403 {
		t.Fatal("missing CSRF header accepted")
	}
	if request("http://console.local", "Veilpost", body+"{}").Code != 400 {
		t.Fatal("trailing JSON accepted")
	}
	response := request("http://console.local", "Veilpost", body)
	if response.Code != 200 {
		t.Fatalf("login failed: %d %s", response.Code, response.Body.String())
	}
	var session auth.Session
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || session.Status != "authenticated" {
		t.Fatal("invalid session contract")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" {
		t.Fatal("cookie boundary invalid")
	}
	handler.Development = false
	response = request("https://console.local", "Veilpost", body)
	if response.Code != 200 || !response.Result().Cookies()[0].Secure {
		t.Fatal("production cookie must be Secure")
	}
}

func TestUnknownSessionDoesNotCreateRateLimitState(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	_, err := s.Verify(ctx, testToken(), "totp", "000000")
	expectCode(t, err, "CHALLENGE_EXPIRED")
	_, err = s.BeginSetup(ctx, testToken(), testPassword)
	expectCode(t, err, "SESSION_EXPIRED")
	_, _, err = s.Enable(ctx, testToken(), "invalid", "000000")
	expectCode(t, err, "SESSION_EXPIRED")
	var count int
	if err := s.Database().QueryRow("SELECT count(*) FROM attempts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("unauthenticated requests created limiter records")
	}
}

func TestConcurrentAccountInitializationOnExistingDatabase(t *testing.T) {
	_, _, directory := fixture(t)
	var wait sync.WaitGroup
	failures := make(chan error, 4)
	for i := range 4 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			service, err := auth.Open(directory)
			if err == nil {
				defer service.Close()
				err = service.CreateUser(context.Background(), fmt.Sprintf("parallel-%d", index), testPassword)
			}
			failures <- err
		}(i)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A read-then-write transaction must not fail when another process commits
// between its read and its write; deferred transactions would hit
// SQLITE_BUSY_SNAPSHOT there instead of waiting on the busy timeout.
func TestTransactionsWaitForWritersInOtherProcesses(t *testing.T) {
	first, _, directory := fixture(t)
	second, err := auth.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx := context.Background()
	tx, err := first.Database().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var users int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	other := make(chan error, 1)
	go func() { other <- second.CreateUser(ctx, "other-process", testPassword) }()
	time.Sleep(200 * time.Millisecond)
	if _, err = tx.ExecContext(ctx, "DELETE FROM attempts WHERE bucket=?", "none"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-other; err != nil {
		t.Fatal(err)
	}
}

func wrongOTP(t *testing.T, secret string, now time.Time) string {
	t.Helper()
	for candidate := 0; ; candidate++ {
		value := fmt.Sprintf("%06d", candidate)
		valid := false
		for offset := -1; offset <= 1; offset++ {
			valid = valid || value == otpCode(t, secret, now.Add(time.Duration(offset)*30*time.Second))
		}
		if !valid {
			return value
		}
	}
}

func TestLoginLimitIsPerAddressAndAccount(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	for range 10 {
		_, err := s.Login(ctx, "test-user", "incorrect", "", "attacker")
		expectCode(t, err, "INVALID_CREDENTIALS")
	}
	_, err := s.Login(ctx, "test-user", testPassword, "", "attacker")
	expectCode(t, err, "TOO_MANY_ATTEMPTS")
	// Another address is not locked out by the attacker's failures.
	if _, err := s.Login(ctx, "test-user", testPassword, "", "owner"); err != nil {
		t.Fatal(err)
	}
	for index := range 99 {
		s.Login(ctx, "test-user", "incorrect", "", fmt.Sprintf("distributed-%d", index))
	}
	_, err = s.Login(ctx, "test-user", testPassword, "", "fresh-address")
	expectCode(t, err, "TOO_MANY_ATTEMPTS")
}

func TestVerificationAttemptsAreLimitedPerAccount(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	_, setup, _ := setupMFA(t, s, loginOK(t, s).Token, *now)
	*now = now.Add(time.Minute)
	// Fresh challenges reset the per-challenge budget but not the account budget.
	for range 3 {
		challenge := loginOK(t, s)
		for range 10 {
			_, err := s.Verify(ctx, challenge.Token, "totp", wrongOTP(t, setup.Secret, *now))
			expectCode(t, err, "INVALID_CODE")
		}
	}
	_, err := s.Verify(ctx, loginOK(t, s).Token, "totp", otpCode(t, setup.Secret, *now))
	expectCode(t, err, "TOO_MANY_ATTEMPTS")
	*now = now.Add(time.Hour + time.Second)
	if _, err := s.Verify(ctx, loginOK(t, s).Token, "totp", otpCode(t, setup.Secret, *now)); err != nil {
		t.Fatal(err)
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	current, other := loginOK(t, s), loginOK(t, s)
	_, err := s.ChangePassword(ctx, current.Token, "incorrect", "a-new-password-value")
	expectCode(t, err, "INVALID_PASSWORD")
	_, err = s.ChangePassword(ctx, current.Token, testPassword, "short")
	expectCode(t, err, "WEAK_PASSWORD")
	_, err = s.ChangePassword(ctx, current.Token, testPassword, testPassword)
	expectCode(t, err, "PASSWORD_REUSED")
	changed, err := s.ChangePassword(ctx, current.Token, testPassword, "a-new-password-value")
	if err != nil || changed.Session.Status != "authenticated" || changed.Token == current.Token {
		t.Fatalf("change failed or did not rotate the session: %+v %v", changed, err)
	}
	for _, token := range []string{current.Token, other.Token} {
		if session, _ := s.Session(ctx, token); session.Status != "anonymous" {
			t.Fatal("previous sessions must be revoked")
		}
	}
	if session, _ := s.Session(ctx, changed.Token); session.Status != "authenticated" {
		t.Fatal("rotated session must stay signed in")
	}
	_, err = s.Login(ctx, "test-user", testPassword, "", "test-ip")
	expectCode(t, err, "INVALID_CREDENTIALS")
	if _, err := s.Login(ctx, "test-user", "a-new-password-value", "", "test-ip"); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityStatusRecoveryRegenerationAndSessionRevocation(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	enabled, setup, original := setupMFA(t, s, loginOK(t, s).Token, *now)
	*now = now.Add(30 * time.Second)
	other, err := s.Verify(ctx, loginOK(t, s).Token, "recovery", original[0])
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.SecurityStatus(ctx, enabled.Token)
	if err != nil || status != (auth.SecurityStatus{RecoveryCodesRemaining: 7, OtherSessions: 1}) {
		t.Fatalf("unexpected status: %+v %v", status, err)
	}
	_, err = s.SecurityStatus(ctx, "")
	expectCode(t, err, "SESSION_EXPIRED")
	_, err = s.RegenerateRecoveryCodes(ctx, enabled.Token, testPassword, wrongOTP(t, setup.Secret, *now))
	expectCode(t, err, "INVALID_CODE")
	*now = now.Add(30 * time.Second)
	valid := otpCode(t, setup.Secret, *now)
	codes, err := s.RegenerateRecoveryCodes(ctx, enabled.Token, testPassword, valid)
	if err != nil || len(codes) != 8 {
		t.Fatalf("regeneration failed: %v %v", codes, err)
	}
	_, err = s.RegenerateRecoveryCodes(ctx, enabled.Token, testPassword, valid)
	expectCode(t, err, "INVALID_CODE")
	_, err = s.Verify(ctx, loginOK(t, s).Token, "recovery", original[1])
	expectCode(t, err, "INVALID_RECOVERY_CODE")
	if err := s.RevokeOtherSessions(ctx, enabled.Token); err != nil {
		t.Fatal(err)
	}
	if session, _ := s.Session(ctx, other.Token); session.Status != "anonymous" {
		t.Fatal("other sessions must be revoked")
	}
	status, err = s.SecurityStatus(ctx, enabled.Token)
	if err != nil || status != (auth.SecurityStatus{RecoveryCodesRemaining: 8, OtherSessions: 0}) {
		t.Fatalf("unexpected status after revocation: %+v %v", status, err)
	}
}

func TestAdministratorResetsRevokeSessions(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	if err := s.ResetTwoFactor(ctx, "test-user"); err == nil {
		t.Fatal("reset must report an account without two-step verification")
	}
	if err := s.ResetPassword(ctx, "missing-user", "a-new-password-value"); err == nil {
		t.Fatal("reset must report a missing account")
	}
	if err := s.ResetPassword(ctx, "test-user", "short"); err == nil {
		t.Fatal("reset must enforce the password policy")
	}
	signed, _, _ := setupMFA(t, s, loginOK(t, s).Token, *now)
	if err := s.ResetTwoFactor(ctx, "test-user"); err != nil {
		t.Fatal(err)
	}
	if session, _ := s.Session(ctx, signed.Token); session.Status != "anonymous" {
		t.Fatal("two-step reset must revoke sessions")
	}
	if loginOK(t, s).Session.Status != "authenticated" {
		t.Fatal("two-step verification must be removed")
	}
	if err := s.ResetPassword(ctx, "test-user", "a-new-password-value"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Login(ctx, "test-user", testPassword, "", "test-ip")
	expectCode(t, err, "INVALID_CREDENTIALS")
}

func TestPruneRemovesOnlyExpiredState(t *testing.T) {
	s, now, _ := fixture(t)
	ctx := context.Background()
	expired := loginOK(t, s)
	s.Login(ctx, "test-user", "incorrect", "", "test-ip")
	*now = now.Add(8*time.Hour + time.Second)
	current := loginOK(t, s)
	if err := s.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	var sessions, attempts int
	s.Database().QueryRow("SELECT count(*) FROM sessions").Scan(&sessions)
	s.Database().QueryRow("SELECT count(*) FROM attempts").Scan(&attempts)
	if sessions != 1 || attempts != 3 {
		t.Fatalf("got %d sessions and %d attempt buckets", sessions, attempts)
	}
	if session, _ := s.Session(ctx, current.Token); session.Status != "authenticated" || expired.Token == current.Token {
		t.Fatal("unexpired session must remain")
	}
}

func TestVersionOneDatabaseMigratesToNodeSchema(t *testing.T) {
	dir := t.TempDir()
	service, err := auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.CreateUser(context.Background(), "test-user", testPassword); err != nil {
		t.Fatal(err)
	}
	// Recreate the released version 1 layout: drop the node tables and version.
	for _, statement := range []string{"DROP TABLE agent_nonces", "DROP TABLE node_enrollments", "DROP TABLE nodes", "PRAGMA user_version=1"} {
		if _, err := service.Database().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	service.Close()
	service, err = auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var version, users, nodes int
	service.Database().QueryRow("PRAGMA user_version").Scan(&version)
	service.Database().QueryRow("SELECT count(*) FROM users").Scan(&users)
	if err := service.Database().QueryRow("SELECT count(*) FROM nodes").Scan(&nodes); err != nil || version != auth.SchemaVersion() || users != 1 {
		t.Fatalf("migration: version %d, users %d, nodes error %v", version, users, err)
	}
	if _, err := service.Database().Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	service.Close()
	if newer, err := auth.Open(dir); err == nil {
		newer.Close()
		t.Fatal("newer schema accepted")
	}
}

// testToken is a well-formed session token that the server never issued.
func testToken() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
