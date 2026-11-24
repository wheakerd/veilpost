package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// migrations[i] upgrades the schema from user_version i to i+1. Append new
// steps; never edit a released one.
var migrations = []string{schemaV1, schemaV2, schemaV3, schemaV4}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
 totp_secret BLOB, last_totp_step INTEGER NOT NULL DEFAULT -1
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 status TEXT NOT NULL CHECK(status IN ('authenticated','mfa_required')), expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS setups (
 id TEXT PRIMARY KEY, session_hash TEXT NOT NULL UNIQUE REFERENCES sessions(token_hash) ON DELETE CASCADE,
 secret BLOB NOT NULL, expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS recovery_codes (
 user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, code_hash TEXT NOT NULL,
 PRIMARY KEY(user_id,code_hash)
);
CREATE TABLE IF NOT EXISTS attempts (bucket TEXT PRIMARY KEY, count INTEGER NOT NULL, expires_at INTEGER NOT NULL);
`

// schemaV2 adds node registration and the agent control channel. Nodes keep
// only their Ed25519 public key; enrollment tokens are stored as SHA-256
// hashes; agent_nonces remembers signed-request nonces until their timestamp
// falls outside the accepted clock skew.
const schemaV2 = `
CREATE TABLE IF NOT EXISTS nodes (
 id TEXT PRIMARY KEY, name TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','active','revoked')),
 public_key BLOB, agent_version TEXT, config_version INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL, enrolled_at INTEGER, revoked_at INTEGER, last_seen_at INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS nodes_public_key ON nodes(public_key) WHERE public_key IS NOT NULL;
CREATE TABLE IF NOT EXISTS node_enrollments (
 token_hash TEXT PRIMARY KEY, node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
 expires_at INTEGER NOT NULL, used_at INTEGER
);
CREATE INDEX IF NOT EXISTS node_enrollments_node ON node_enrollments(node_id);
CREATE INDEX IF NOT EXISTS node_enrollments_expiry ON node_enrollments(expires_at);
CREATE TABLE IF NOT EXISTS agent_nonces (
 node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE, nonce TEXT NOT NULL, expires_at INTEGER NOT NULL,
 PRIMARY KEY(node_id,nonce)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS agent_nonces_expiry ON agent_nonces(expires_at);
`

// schemaV3 adds the software store. A row exists only while an app is
// installed; settings is JSON whose sensitive values are sealed with
// master.key (see internal/apps).
const schemaV3 = `
CREATE TABLE IF NOT EXISTS installed_apps (
 app_id TEXT PRIMARY KEY, version TEXT NOT NULL, settings TEXT NOT NULL,
 installed_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
`

// schemaV4 adds DNS provider credentials, ACME accounts and certificates
// (internal/certs). secrets, ACME account keys, EAB keys and certificate
// private keys are sealed with master.key; certificate_pem holds only the
// public chain. A credential referenced by a certificate cannot be deleted.
const schemaV4 = `
CREATE TABLE IF NOT EXISTS dns_credentials (
 id TEXT PRIMARY KEY, name TEXT NOT NULL,
 provider TEXT NOT NULL CHECK(provider IN ('cloudflare','alidns','tencentcloud','route53')),
 secrets BLOB NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS acme_accounts (
 id TEXT PRIMARY KEY, directory_url TEXT NOT NULL, email TEXT NOT NULL,
 private_key BLOB NOT NULL, account_url TEXT, eab BLOB, created_at INTEGER NOT NULL,
 UNIQUE(directory_url,email)
);
CREATE TABLE IF NOT EXISTS certificates (
 id TEXT PRIMARY KEY, domains TEXT NOT NULL, ca_directory TEXT NOT NULL,
 acme_account_id TEXT NOT NULL REFERENCES acme_accounts(id),
 dns_credential_id TEXT NOT NULL REFERENCES dns_credentials(id),
 status TEXT NOT NULL CHECK(status IN ('pending','valid','failed','expired')),
 issuer TEXT, not_before INTEGER, not_after INTEGER, certificate_pem TEXT, private_key BLOB,
 last_error TEXT, last_error_detail TEXT, last_error_at INTEGER, failures INTEGER NOT NULL DEFAULT 0,
 renew_at INTEGER, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS certificates_renew ON certificates(renew_at) WHERE renew_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS certificates_credential ON certificates(dns_credential_id);
CREATE INDEX IF NOT EXISTS certificates_account ON certificates(acme_account_id);
`

type User struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	TwoFactorEnabled bool   `json:"twoFactorEnabled"`
}

type Session struct {
	SetupRequired bool   `json:"setupRequired,omitempty"`
	Status        string `json:"status"`
	User          *User  `json:"user,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
}

type Service struct {
	db            *sql.DB
	key           []byte
	now           func() time.Time
	dummyHash     string
	passwordSlots chan struct{}
}

func Open(directory string) (*Service, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(absolute, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(absolute, "master.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		if _, exists := os.Stat(filepath.Join(absolute, "auth.sqlite")); exists == nil {
			return nil, fmt.Errorf("encryption key missing for existing authentication database")
		}
		key = randomBytes(32)
		file, createErr := os.CreateTemp(absolute, ".key-*")
		if createErr != nil {
			return nil, createErr
		}
		defer os.Remove(file.Name())
		if _, err = file.Write(key); err != nil {
			file.Close()
			return nil, err
		}
		if err = file.Close(); err != nil {
			return nil, err
		}
		if err = os.Link(file.Name(), keyPath); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		key, err = os.ReadFile(keyPath)
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("cannot read authentication encryption key")
	}
	uri := url.URL{Scheme: "file", Path: filepath.Join(absolute, "auth.sqlite")}
	// Immediate transactions take the write lock up front, so a concurrent
	// writer in another process (such as admin-create) makes them wait on the
	// busy timeout instead of failing with SQLITE_BUSY_SNAPSHOT mid-transaction.
	db, err := sql.Open("sqlite", uri.String()+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version > len(migrations) {
		db.Close()
		return nil, fmt.Errorf("unsupported authentication database")
	}
	if version < len(migrations) {
		err = migrate(db)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(filepath.Join(absolute, "auth.sqlite"), 0600); err != nil {
		db.Close()
		return nil, err
	}
	return &Service{db: db, key: key, now: time.Now, dummyHash: hashPassword(randomToken(32)), passwordSlots: make(chan struct{}, 4)}, nil
}

func migrate(db *sql.DB) error {
	ctx := context.Background()
	connection, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	// Take the writer lock before reading schema state. A deferred read-to-write
	// upgrade can fail immediately with SQLITE_BUSY under concurrent initializers.
	if _, err = connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer connection.ExecContext(ctx, "ROLLBACK")
	var version int
	if err = connection.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("unsupported authentication database")
	}
	if version == len(migrations) {
		_, err = connection.ExecContext(ctx, "COMMIT")
		return err
	}
	for ; version < len(migrations); version++ {
		if _, err = connection.ExecContext(ctx, migrations[version]); err != nil {
			return err
		}
	}
	if _, err = connection.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", len(migrations))); err != nil {
		return err
	}
	_, err = connection.ExecContext(ctx, "COMMIT")
	return err
}

func (s *Service) Close() error                    { return s.db.Close() }
func (s *Service) Ready(ctx context.Context) error { return s.db.PingContext(ctx) }

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,63}$`)

func validPassword(password string) bool {
	return utf8.RuneCountInString(password) >= 12 && len(password) <= 256
}

func (s *Service) CreateUser(ctx context.Context, username, password string) error {
	if !usernamePattern.MatchString(username) || !validPassword(password) {
		return fmt.Errorf("username must be 3–64 ASCII letters/digits/._-; password must contain at least 12 characters and at most 256 bytes")
	}
	_, err := s.db.ExecContext(ctx, "INSERT INTO users(id,username,password_hash) VALUES(?,?,?)", randomToken(16), username, hashPassword(password))
	if err != nil {
		return fmt.Errorf("cannot create account (the username may already exist)")
	}
	return nil
}

// ResetPassword is the local administrator recovery path. It replaces the
// password and revokes every session of the account.
func (s *Service) ResetPassword(ctx context.Context, username, password string) error {
	if !validPassword(password) {
		return fmt.Errorf("password must contain at least 12 characters and at most 256 bytes")
	}
	return s.resetAccount(ctx, username, false, "UPDATE users SET password_hash=? WHERE id=?", hashPassword(password))
}

// ResetTwoFactor removes the verifier and recovery codes of an account whose
// owner lost both, and revokes every session of the account.
func (s *Service) ResetTwoFactor(ctx context.Context, username string) error {
	return s.resetAccount(ctx, username, true, "UPDATE users SET totp_secret=NULL,last_totp_step=-1 WHERE id=? AND totp_secret IS NOT NULL")
}

func (s *Service) resetAccount(ctx context.Context, username string, twoFactor bool, update string, args ...any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE username=?", username).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("account not found")
	} else if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, update, append(args, id)...)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return fmt.Errorf("two-step verification is not enabled for this account")
	}
	if twoFactor {
		if _, err = tx.ExecContext(ctx, "DELETE FROM recovery_codes WHERE user_id=?", id); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// Prune removes expired sessions, pending setups, rate-limit buckets, node
// enrollment tokens (used or not) and remembered agent request nonces.
func (s *Service) Prune(ctx context.Context) error {
	now := s.now().UnixMilli()
	for _, table := range []string{"sessions", "setups", "attempts", "node_enrollments", "agent_nonces"} {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE expires_at<=?", now); err != nil {
			return err
		}
	}
	return nil
}

type account struct {
	User
	password string
	secret   []byte
	last     int64
}
type sessionRecord struct {
	account
	status  string
	expires int64
}
type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) readSession(ctx context.Context, db queryer, token string) (sessionRecord, error) {
	var record sessionRecord
	if len(token) != 43 {
		return record, sql.ErrNoRows
	}
	err := db.QueryRowContext(ctx, `SELECT u.id,u.username,u.password_hash,u.totp_secret,u.last_totp_step,s.status,s.expires_at
 FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, digest(token)).Scan(&record.ID, &record.Username, &record.password, &record.secret, &record.last, &record.status, &record.expires)
	record.TwoFactorEnabled = len(record.secret) > 0
	if err == nil && record.expires <= s.now().UnixMilli() {
		return record, sql.ErrNoRows
	}
	return record, err
}

func (s *Service) Session(ctx context.Context, token string) (Session, error) {
	record, err := s.readSession(ctx, s.db, token)
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
			return Session{}, err
		}
		return Session{Status: "anonymous", SetupRequired: count == 0}, nil
	}
	if err != nil {
		return Session{}, err
	}
	if record.status == "mfa_required" {
		return Session{Status: record.status, ExpiresAt: time.UnixMilli(record.expires).UTC().Format(time.RFC3339Nano)}, nil
	}
	return Session{Status: record.status, User: &record.User}, nil
}

func (s *Service) limit(ctx context.Context, key string, maximum int, window time.Duration) error {
	now := s.now().UnixMilli()
	if _, err := s.db.ExecContext(ctx, "DELETE FROM attempts WHERE expires_at<=?", now); err != nil {
		return err
	}
	var count int
	err := s.db.QueryRowContext(ctx, `INSERT INTO attempts(bucket,count,expires_at) VALUES(?,1,?)
 ON CONFLICT(bucket) DO UPDATE SET count=attempts.count+1 RETURNING count`, digest(key), now+window.Milliseconds()).Scan(&count)
	if err != nil {
		return err
	}
	if count > maximum {
		return failure("TOO_MANY_ATTEMPTS", 429)
	}
	return nil
}

func (s *Service) passwordMatches(ctx context.Context, hash, password string) bool {
	select {
	case s.passwordSlots <- struct{}{}:
		defer func() { <-s.passwordSlots }()
		return verifyPassword(hash, password)
	case <-ctx.Done():
		return false
	}
}
