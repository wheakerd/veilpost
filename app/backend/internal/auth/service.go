package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
)

type APIError struct {
	Code   string
	Status int
}

func (e *APIError) Error() string           { return e.Code }
func failure(code string, status int) error { return &APIError{code, status} }

type Result struct {
	Session  Session
	Token    string
	Lifetime time.Duration
}

func (s *Service) issue(ctx context.Context, tx *sql.Tx, user User, status, old string, all bool) (Result, error) {
	if all {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", user.ID); err != nil {
			return Result{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=? OR expires_at<=?", digest(old), s.now().UnixMilli()); err != nil {
		return Result{}, err
	}
	lifetime := 8 * time.Hour
	if status == "mfa_required" {
		lifetime = 5 * time.Minute
	}
	token := randomToken(32)
	expires := s.now().Add(lifetime)
	if _, err := tx.ExecContext(ctx, "INSERT INTO sessions(token_hash,user_id,status,expires_at) VALUES(?,?,?,?)", digest(token), user.ID, status, expires.UnixMilli()); err != nil {
		return Result{}, err
	}
	session := Session{Status: status, User: &user}
	if status == "mfa_required" {
		session.User = nil
		session.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	}
	return Result{session, token, lifetime}, nil
}

func (s *Service) Login(ctx context.Context, username, password, old, ip string) (Result, error) {
	username = strings.TrimSpace(username)
	if err := s.limit(ctx, "login-ip:"+ip, 240, 5*time.Minute); err != nil {
		return Result{}, err
	}
	// A tight per-address account limit stops guessing without letting a single
	// client lock the owner out; the wider account limit bounds distributed guessing.
	if err := s.limit(ctx, "login-account-ip:"+username+"\x00"+ip, 10, 5*time.Minute); err != nil {
		return Result{}, err
	}
	if err := s.limit(ctx, "login-account:"+username, 100, 5*time.Minute); err != nil {
		return Result{}, err
	}
	var user account
	err := s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,totp_secret FROM users WHERE username=?", username).Scan(&user.ID, &user.Username, &user.password, &user.secret)
	if errors.Is(err, sql.ErrNoRows) {
		s.passwordMatches(ctx, s.dummyHash, password)
		return Result{}, failure("INVALID_CREDENTIALS", 401)
	}
	if err != nil {
		return Result{}, err
	}
	if len(password) > 1024 || !s.passwordMatches(ctx, user.password, password) {
		return Result{}, failure("INVALID_CREDENTIALS", 401)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	var currentHash string
	if err = tx.QueryRowContext(ctx, "SELECT password_hash,totp_secret FROM users WHERE id=?", user.ID).Scan(&currentHash, &user.secret); err != nil {
		return Result{}, err
	}
	if currentHash != user.password {
		return Result{}, failure("INVALID_CREDENTIALS", 401)
	}
	user.TwoFactorEnabled = len(user.secret) > 0
	status := "authenticated"
	if user.TwoFactorEnabled {
		status = "mfa_required"
	}
	result, err := s.issue(ctx, tx, user.User, status, old, false)
	if err != nil {
		return Result{}, err
	}
	return result, tx.Commit()
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if len(token) != 43 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", digest(token))
	return err
}

func (s *Service) Verify(ctx context.Context, token, method, code string) (Result, error) {
	before, err := s.readSession(ctx, s.db, token)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (before.status != "mfa_required" || !before.TwoFactorEnabled)) {
		return Result{}, failure("CHALLENGE_EXPIRED", 401)
	}
	if err != nil {
		return Result{}, err
	}
	if err := s.limit(ctx, "verify:"+digest(token), 10, 5*time.Minute); err != nil {
		return Result{}, err
	}
	// New challenges reset the per-challenge budget, so codes are also limited per account.
	if err := s.limit(ctx, "verify-account:"+before.ID, 30, time.Hour); err != nil {
		return Result{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	record, err := s.readSession(ctx, tx, token)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (record.status != "mfa_required" || !record.TwoFactorEnabled)) {
		return Result{}, failure("CHALLENGE_EXPIRED", 401)
	}
	if err != nil {
		return Result{}, err
	}
	if method == "recovery" {
		result, err := tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=? AND code_hash=?", record.ID, digest(code))
		if err != nil {
			return Result{}, err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return Result{}, failure("INVALID_RECOVERY_CODE", 400)
		}
	} else if method == "totp" {
		secret, err := decrypt(s.key, record.secret)
		if err != nil {
			return Result{}, err
		}
		counter, valid := totpCounter(secret, code, s.now(), record.last)
		if !valid {
			return Result{}, failure("INVALID_CODE", 400)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE users SET last_totp_step=? WHERE id=?", counter, record.ID); err != nil {
			return Result{}, err
		}
	} else {
		return Result{}, failure("INVALID_CODE", 400)
	}
	result, err := s.issue(ctx, tx, record.User, "authenticated", token, false)
	if err != nil {
		return Result{}, err
	}
	return result, tx.Commit()
}

type Setup struct {
	ID            string `json:"setupId"`
	Secret        string `json:"secret"`
	ExpiresAt     string `json:"expiresAt"`
	QRCodeDataURL string `json:"qrCodeDataUrl"`
}

func (s *Service) requireSession(ctx context.Context, db queryer, token string) (sessionRecord, error) {
	record, err := s.readSession(ctx, db, token)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && record.status != "authenticated") {
		return record, failure("SESSION_EXPIRED", 401)
	}
	return record, err
}

func (s *Service) BeginSetup(ctx context.Context, token, password string) (Setup, error) {
	record, err := s.confirmSecurityChange(ctx, token, password)
	if err != nil {
		return Setup{}, err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Veilpost", AccountName: record.Username, SecretSize: 20})
	if err != nil {
		return Setup{}, err
	}
	qr, err := key.Image(256, 256)
	if err != nil {
		return Setup{}, err
	}
	// Keep a white quiet zone inside the PNG so it also scans in dark themes.
	framed := image.NewRGBA(image.Rect(0, 0, 320, 320))
	draw.Draw(framed, framed.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(framed, image.Rect(32, 32, 288, 288), qr, qr.Bounds().Min, draw.Src)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, framed); err != nil {
		return Setup{}, err
	}
	encrypted, err := encrypt(s.key, key.Secret())
	if err != nil {
		return Setup{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Setup{}, err
	}
	defer tx.Rollback()
	record, err = s.requireSession(ctx, tx, token)
	if err != nil {
		return Setup{}, err
	}
	if record.TwoFactorEnabled {
		return Setup{}, failure("REQUEST_FAILED", 409)
	}
	setup := Setup{
		ID:            randomToken(24),
		Secret:        key.Secret(),
		ExpiresAt:     s.now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano),
		QRCodeDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes()),
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM setups WHERE session_hash=? OR expires_at<=?", digest(token), s.now().UnixMilli()); err != nil {
		return Setup{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO setups(id,session_hash,secret,expires_at) VALUES(?,?,?,?)", setup.ID, digest(token), encrypted, s.now().Add(5*time.Minute).UnixMilli()); err != nil {
		return Setup{}, err
	}
	return setup, tx.Commit()
}

func (s *Service) Enable(ctx context.Context, token, id, code string) (Result, []string, error) {
	if _, err := s.requireSession(ctx, s.db, token); err != nil {
		return Result{}, nil, err
	}
	if err := s.limit(ctx, "security:"+digest(token), 15, 5*time.Minute); err != nil {
		return Result{}, nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, nil, err
	}
	defer tx.Rollback()
	record, err := s.requireSession(ctx, tx, token)
	if err != nil {
		return Result{}, nil, err
	}
	var encrypted []byte
	err = tx.QueryRowContext(ctx, "SELECT secret FROM setups WHERE id=? AND session_hash=? AND expires_at>?", id, digest(token), s.now().UnixMilli()).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, nil, failure("SETUP_EXPIRED", 410)
	}
	if err != nil {
		return Result{}, nil, err
	}
	secret, err := decrypt(s.key, encrypted)
	if err != nil {
		return Result{}, nil, err
	}
	counter, valid := totpCounter(secret, code, s.now(), -1)
	if !valid {
		return Result{}, nil, failure("INVALID_CODE", 400)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET totp_secret=?,last_totp_step=? WHERE id=?", encrypted, counter, record.ID); err != nil {
		return Result{}, nil, err
	}
	codes, err := replaceRecoveryCodes(ctx, tx, record.ID)
	if err != nil {
		return Result{}, nil, err
	}
	record.TwoFactorEnabled = true
	result, err := s.issue(ctx, tx, record.User, "authenticated", token, true)
	if err != nil {
		return Result{}, nil, err
	}
	return result, codes, tx.Commit()
}

func (s *Service) Disable(ctx context.Context, token, password, code string) (Result, error) {
	if _, err := s.confirmSecurityChange(ctx, token, password); err != nil {
		return Result{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	record, err := s.requireSession(ctx, tx, token)
	if err != nil {
		return Result{}, err
	}
	if !record.TwoFactorEnabled {
		return Result{}, failure("REQUEST_FAILED", 409)
	}
	secret, err := decrypt(s.key, record.secret)
	if err != nil {
		return Result{}, err
	}
	if _, valid := totpCounter(secret, code, s.now(), record.last); !valid {
		return Result{}, failure("INVALID_CODE", 400)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET totp_secret=NULL,last_totp_step=-1 WHERE id=?", record.ID); err != nil {
		return Result{}, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=?", record.ID); err != nil {
		return Result{}, err
	}
	record.TwoFactorEnabled = false
	result, err := s.issue(ctx, tx, record.User, "authenticated", token, true)
	if err != nil {
		return Result{}, err
	}
	return result, tx.Commit()
}

func replaceRecoveryCodes(ctx context.Context, tx *sql.Tx, userID string) ([]string, error) {
	if _, err := tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=?", userID); err != nil {
		return nil, err
	}
	codes := make([]string, 8)
	for i := range codes {
		codes[i] = randomToken(16)
		if _, err := tx.ExecContext(ctx, "INSERT INTO recovery_codes(user_id,code_hash) VALUES(?,?)", userID, digest(codes[i])); err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// confirmSecurityChange applies the shared security-operation rate limit and
// verifies the current password outside a write transaction.
func (s *Service) confirmSecurityChange(ctx context.Context, token, password string) (sessionRecord, error) {
	if _, err := s.requireSession(ctx, s.db, token); err != nil {
		return sessionRecord{}, err
	}
	if err := s.limit(ctx, "security:"+digest(token), 15, 5*time.Minute); err != nil {
		return sessionRecord{}, err
	}
	record, err := s.requireSession(ctx, s.db, token)
	if err != nil {
		return sessionRecord{}, err
	}
	if !s.passwordMatches(ctx, record.password, password) {
		return sessionRecord{}, failure("INVALID_PASSWORD", 403)
	}
	return record, nil
}

// ChangePassword replaces the password of the signed-in account, revokes all
// of its other sessions and rotates the current one.
func (s *Service) ChangePassword(ctx context.Context, token, password, next string) (Result, error) {
	if _, err := s.confirmSecurityChange(ctx, token, password); err != nil {
		return Result{}, err
	}
	if !validPassword(next) {
		return Result{}, failure("WEAK_PASSWORD", 400)
	}
	if next == password {
		return Result{}, failure("PASSWORD_REUSED", 400)
	}
	hash := hashPassword(next)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	record, err := s.requireSession(ctx, tx, token)
	if err != nil {
		return Result{}, err
	}
	// The password may have changed after verification; fail instead of overwriting it.
	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=? WHERE id=? AND password_hash=?", hash, record.ID, record.password)
	if err != nil {
		return Result{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Result{}, failure("INVALID_PASSWORD", 403)
	}
	issued, err := s.issue(ctx, tx, record.User, "authenticated", token, true)
	if err != nil {
		return Result{}, err
	}
	return issued, tx.Commit()
}

type SecurityStatus struct {
	RecoveryCodesRemaining int `json:"recoveryCodesRemaining"`
	OtherSessions          int `json:"otherSessions"`
}

func (s *Service) SecurityStatus(ctx context.Context, token string) (SecurityStatus, error) {
	record, err := s.requireSession(ctx, s.db, token)
	if err != nil {
		return SecurityStatus{}, err
	}
	var status SecurityStatus
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM recovery_codes WHERE user_id=?", record.ID).Scan(&status.RecoveryCodesRemaining); err != nil {
		return SecurityStatus{}, err
	}
	err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE user_id=? AND status='authenticated' AND token_hash<>? AND expires_at>?",
		record.ID, digest(token), s.now().UnixMilli()).Scan(&status.OtherSessions)
	return status, err
}

// RegenerateRecoveryCodes replaces every unused recovery code after the owner
// proves both the password and the current verifier.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, token, password, code string) ([]string, error) {
	if _, err := s.confirmSecurityChange(ctx, token, password); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	record, err := s.requireSession(ctx, tx, token)
	if err != nil {
		return nil, err
	}
	if !record.TwoFactorEnabled {
		return nil, failure("REQUEST_FAILED", 409)
	}
	secret, err := decrypt(s.key, record.secret)
	if err != nil {
		return nil, err
	}
	counter, valid := totpCounter(secret, code, s.now(), record.last)
	if !valid {
		return nil, failure("INVALID_CODE", 400)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE users SET last_totp_step=? WHERE id=?", counter, record.ID); err != nil {
		return nil, err
	}
	codes, err := replaceRecoveryCodes(ctx, tx, record.ID)
	if err != nil {
		return nil, err
	}
	return codes, tx.Commit()
}

func (s *Service) RevokeOtherSessions(ctx context.Context, token string) error {
	if _, err := s.requireSession(ctx, s.db, token); err != nil {
		return err
	}
	if err := s.limit(ctx, "security:"+digest(token), 15, 5*time.Minute); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	record, err := s.requireSession(ctx, tx, token)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=? AND token_hash<>?", record.ID, digest(token)); err != nil {
		return err
	}
	return tx.Commit()
}
