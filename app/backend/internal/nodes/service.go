// Package nodes implements node registration and the signed agent control
// channel, including Docker Engine installation on opted-in nodes.
package nodes

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

const (
	// EnrollmentLifetime bounds how long a one-time enrollment token works.
	EnrollmentLifetime = 15 * time.Minute
	// ClockSkew is the accepted difference between agent and console clocks.
	ClockSkew = 60 * time.Second
	// OnlineWindow marks a node online when its last signed request is this recent.
	// Agents re-poll immediately after each long poll, so this spans several polls.
	OnlineWindow = 90 * time.Second
	// PollHold is how long a long poll waits for a change before answering.
	PollHold = 25 * time.Second
)

// Node is the console view of a node. Status is pending (created, not yet
// enrolled), active (enrolled) or revoked. Online is derived from the last
// signed agent request and is always false unless the node is active.
type Node struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Status              string      `json:"status"`
	Online              bool        `json:"online"`
	AgentVersion        *string     `json:"agentVersion"`
	CreatedAt           string      `json:"createdAt"`
	EnrolledAt          *string     `json:"enrolledAt"`
	RevokedAt           *string     `json:"revokedAt"`
	LastSeenAt          *string     `json:"lastSeenAt"`
	EnrollmentExpiresAt *string     `json:"enrollmentExpiresAt"`
	Docker              *DockerNode `json:"docker,omitempty"`
}

// Enrollment carries a plaintext enrollment token. It is returned exactly once.
type Enrollment struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

// PollResult is the answer to an agent long poll.
type PollResult struct {
	ConfigVersion int64          `json:"configVersion"`
	DockerInstall *DockerInstall `json:"dockerInstall,omitempty"`
}

type Service struct {
	auth *auth.Service
	db   *sql.DB
	now  func() time.Time
	hold time.Duration
	// agentLimit and agentWindow bound signed requests per node; enrollLimit
	// bounds enrollment attempts per client address.
	agentLimit  int
	agentWindow time.Duration
	enrollLimit int

	mu      sync.Mutex
	waiters map[string]chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func New(authentication *auth.Service) *Service {
	return &Service{
		auth: authentication, db: authentication.Database(), now: time.Now, hold: PollHold,
		agentLimit: 120, agentWindow: 5 * time.Minute, enrollLimit: 30,
		waiters: map[string]chan struct{}{}, closed: make(chan struct{}),
	}
}

// Shutdown releases every waiting long poll so the HTTP server can drain.
func (s *Service) Shutdown() { s.once.Do(func() { close(s.closed) }) }

func randomToken(size int) string {
	data := make([]byte, size)
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

// validName trims a node name and accepts 1–64 printable characters.
func validName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return "", false
		}
	}
	return name, true
}

const nodeColumns = `n.id,n.name,n.status,n.agent_version,n.created_at,n.enrolled_at,n.revoked_at,n.last_seen_at,
 (SELECT max(e.expires_at) FROM node_enrollments e WHERE e.node_id=n.id AND e.used_at IS NULL AND e.expires_at>?),
 d.sources,d.job_id,d.job_source,d.job_status,d.job_at,d.job_error`

const nodeTables = " FROM nodes n LEFT JOIN node_docker d ON d.node_id=n.id"

type scanner interface{ Scan(...any) error }

func (s *Service) scanNode(row scanner) (Node, error) {
	var node Node
	var version sql.NullString
	var created int64
	var enrolled, revoked, seen, enrollment sql.NullInt64
	var docker dockerRow
	if err := row.Scan(&node.ID, &node.Name, &node.Status, &version, &created, &enrolled, &revoked, &seen, &enrollment,
		&docker.sources, &docker.id, &docker.source, &docker.status, &docker.updated, &docker.failure); err != nil {
		return Node{}, err
	}
	if version.Valid {
		node.AgentVersion = &version.String
	}
	node.CreatedAt = timestamp(created)
	node.EnrolledAt = optionalTimestamp(enrolled)
	node.RevokedAt = optionalTimestamp(revoked)
	node.LastSeenAt = optionalTimestamp(seen)
	if node.Status == "pending" {
		node.EnrollmentExpiresAt = optionalTimestamp(enrollment)
	}
	node.Online = node.Status == "active" && seen.Valid && s.now().UnixMilli()-seen.Int64 <= OnlineWindow.Milliseconds()
	var err error
	node.Docker, err = docker.view(s.now())
	if err != nil {
		return Node{}, err
	}
	return node, nil
}

// List returns every node, active and pending first, newest first.
func (s *Service) List(ctx context.Context, session string) ([]Node, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+nodeColumns+nodeTables+`
 ORDER BY CASE n.status WHEN 'revoked' THEN 1 ELSE 0 END, n.created_at DESC, n.id`, s.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []Node{}
	for rows.Next() {
		node, err := s.scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Service) node(ctx context.Context, id string) (Node, error) {
	return s.scanNode(s.db.QueryRowContext(ctx, "SELECT "+nodeColumns+nodeTables+" WHERE n.id=?", s.now().UnixMilli(), id))
}

// Create adds a pending node after the current password is re-entered and
// returns its one-time enrollment token. Only the token hash is stored.
func (s *Service) Create(ctx context.Context, session, name, password string) (Node, Enrollment, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Node{}, Enrollment{}, err
	}
	name, valid := validName(name)
	if !valid {
		return Node{}, Enrollment{}, auth.Failure("INVALID_NAME", 400)
	}
	if _, err := s.auth.ConfirmPassword(ctx, session, password); err != nil {
		return Node{}, Enrollment{}, err
	}
	id := randomToken(16)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, Enrollment{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO nodes(id,name,status,created_at) VALUES(?,?,'pending',?)", id, name, s.now().UnixMilli()); err != nil {
		return Node{}, Enrollment{}, err
	}
	return s.issueEnrollment(ctx, tx, id)
}

// issueEnrollment stores the hash of a new one-time token for a pending node,
// commits tx and returns the token once.
func (s *Service) issueEnrollment(ctx context.Context, tx *sql.Tx, id string) (Node, Enrollment, error) {
	token := randomToken(32)
	expires := s.now().Add(EnrollmentLifetime)
	if _, err := tx.ExecContext(ctx, "INSERT INTO node_enrollments(token_hash,node_id,expires_at) VALUES(?,?,?)", digest(token), id, expires.UnixMilli()); err != nil {
		return Node{}, Enrollment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Node{}, Enrollment{}, err
	}
	node, err := s.node(ctx, id)
	if err != nil {
		return Node{}, Enrollment{}, err
	}
	return node, Enrollment{Token: token, ExpiresAt: timestamp(expires.UnixMilli())}, nil
}

// confirmedStatus re-checks the password, then opens a transaction and reads
// the node's status inside it.
func (s *Service) confirmedStatus(ctx context.Context, session, id, password string) (*sql.Tx, string, error) {
	if _, err := s.auth.ConfirmPassword(ctx, session, password); err != nil {
		return nil, "", err
	}
	if !nodeIDPattern.MatchString(id) {
		return nil, "", auth.Failure("NODE_NOT_FOUND", 404)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM nodes WHERE id=?", id).Scan(&status); err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", auth.Failure("NODE_NOT_FOUND", 404)
		}
		return nil, "", err
	}
	return tx, status, nil
}

// Reissue replaces the enrollment token of a node that has not enrolled yet,
// for example after the previous token expired. Older unused tokens stop
// working at once.
func (s *Service) Reissue(ctx context.Context, session, id, password string) (Node, Enrollment, error) {
	tx, status, err := s.confirmedStatus(ctx, session, id, password)
	if err != nil {
		return Node{}, Enrollment{}, err
	}
	defer tx.Rollback()
	if status != "pending" {
		return Node{}, Enrollment{}, auth.Failure("NODE_NOT_PENDING", 409)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM node_enrollments WHERE node_id=?", id); err != nil {
		return Node{}, Enrollment{}, err
	}
	return s.issueEnrollment(ctx, tx, id)
}

// Delete removes a revoked node and, through foreign keys, its enrollment
// and nonce records. Active and pending nodes must be revoked first.
func (s *Service) Delete(ctx context.Context, session, id, password string) error {
	tx, status, err := s.confirmedStatus(ctx, session, id, password)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if status != "revoked" {
		return auth.Failure("NODE_NOT_REVOKED", 409)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM nodes WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// Revoke permanently disables a node after the current password is
// re-entered: its signatures stop working at once, unused enrollment tokens
// are deleted and waiting long polls are answered with NODE_REVOKED.
// Rejoining requires a new node and a new enrollment token.
func (s *Service) Revoke(ctx context.Context, session, id, password string) (Node, error) {
	tx, status, err := s.confirmedStatus(ctx, session, id, password)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback()
	if status != "revoked" {
		if _, err = tx.ExecContext(ctx, "UPDATE nodes SET status='revoked',revoked_at=? WHERE id=?", s.now().UnixMilli(), id); err != nil {
			return Node{}, err
		}
	}
	for _, statement := range []string{"DELETE FROM node_enrollments WHERE node_id=?", "DELETE FROM agent_nonces WHERE node_id=?"} {
		if _, err = tx.ExecContext(ctx, statement, id); err != nil {
			return Node{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Node{}, err
	}
	s.notify(id)
	return s.node(ctx, id)
}

// EnrollRequest is what an agent submits to redeem an enrollment token.
type EnrollRequest struct {
	Token        string `json:"token"`
	PublicKey    string `json:"publicKey"`
	Proof        string `json:"proof"`
	AgentVersion string `json:"agentVersion"`
}

// EnrollResult identifies the node an agent is now bound to.
type EnrollResult struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
}

// Enroll redeems a one-time token, binds the agent's Ed25519 public key to
// the pending node and activates it. The proof must be a signature of
// EnrollMessage by the submitted key.
func (s *Service) Enroll(ctx context.Context, clientIP string, input EnrollRequest) (EnrollResult, error) {
	if err := s.auth.Limit(ctx, "agent-enroll-ip:"+clientIP, s.enrollLimit, 5*time.Minute); err != nil {
		return EnrollResult{}, err
	}
	key, keyErr := base64.RawURLEncoding.DecodeString(input.PublicKey)
	proof, proofErr := base64.RawURLEncoding.DecodeString(input.Proof)
	if !tokenPattern.MatchString(input.Token) || keyErr != nil || len(key) != ed25519.PublicKeySize || proofErr != nil ||
		len(proof) != ed25519.SignatureSize || !versionPattern.MatchString(input.AgentVersion) {
		return EnrollResult{}, auth.Failure("REQUEST_FAILED", 400)
	}
	if !ed25519.Verify(ed25519.PublicKey(key), EnrollMessage(input.Token, input.PublicKey), proof) {
		return EnrollResult{}, auth.Failure("REQUEST_FAILED", 400)
	}
	now := s.now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EnrollResult{}, err
	}
	defer tx.Rollback()
	var result EnrollResult
	err = tx.QueryRowContext(ctx, `SELECT n.id,n.name FROM node_enrollments e JOIN nodes n ON n.id=e.node_id
 WHERE e.token_hash=? AND e.used_at IS NULL AND e.expires_at>? AND n.status='pending'`, digest(input.Token), now).Scan(&result.NodeID, &result.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return EnrollResult{}, auth.Failure("INVALID_ENROLLMENT", 401)
	}
	if err != nil {
		return EnrollResult{}, err
	}
	var reused int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM nodes WHERE public_key=?", key).Scan(&reused); err != nil {
		return EnrollResult{}, err
	}
	if reused != 0 {
		return EnrollResult{}, auth.Failure("KEY_IN_USE", 409)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE node_enrollments SET used_at=? WHERE token_hash=? AND used_at IS NULL", now, digest(input.Token)); err != nil {
		return EnrollResult{}, err
	}
	// Other unused tokens of this node become useless once it is bound.
	if _, err = tx.ExecContext(ctx, "DELETE FROM node_enrollments WHERE node_id=? AND used_at IS NULL", result.NodeID); err != nil {
		return EnrollResult{}, err
	}
	update, err := tx.ExecContext(ctx, "UPDATE nodes SET status='active',public_key=?,agent_version=?,enrolled_at=? WHERE id=? AND status='pending'",
		key, input.AgentVersion, now, result.NodeID)
	if err != nil {
		return EnrollResult{}, err
	}
	if count, _ := update.RowsAffected(); count != 1 {
		return EnrollResult{}, auth.Failure("INVALID_ENROLLMENT", 401)
	}
	return result, tx.Commit()
}

// SignedRequest holds the authentication headers and body of an agent call.
type SignedRequest struct {
	Method, Route                       string
	NodeID, Timestamp, Nonce, Signature string
	Body                                []byte
}

// Authenticate verifies a signed agent request and returns the node ID.
// Checks: header formats, timestamp within ClockSkew, an enrolled node,
// a valid Ed25519 signature, not revoked, a per-node rate limit and a nonce
// never seen while its timestamp is acceptable. Unauthenticated callers only
// learn AGENT_UNAUTHORIZED or CLOCK_SKEW; NODE_REVOKED requires a valid
// signature.
func (s *Service) Authenticate(ctx context.Context, request SignedRequest) (string, error) {
	unauthorized := auth.Failure("AGENT_UNAUTHORIZED", 401)
	signature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if !nodeIDPattern.MatchString(request.NodeID) || !noncePattern.MatchString(request.Nonce) || !timestampPattern.MatchString(request.Timestamp) ||
		err != nil || len(signature) != ed25519.SignatureSize {
		return "", unauthorized
	}
	seconds, err := strconv.ParseInt(request.Timestamp, 10, 64)
	if err != nil {
		return "", unauthorized
	}
	signed := time.Unix(seconds, 0)
	now := s.now()
	if now.Sub(signed) > ClockSkew || signed.Sub(now) > ClockSkew {
		return "", auth.Failure("CLOCK_SKEW", 401)
	}
	var status string
	var key []byte
	err = s.db.QueryRowContext(ctx, "SELECT status,public_key FROM nodes WHERE id=?", request.NodeID).Scan(&status, &key)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(key) != ed25519.PublicKeySize) {
		return "", unauthorized
	}
	if err != nil {
		return "", err
	}
	message := RequestMessage(request.Method, request.Route, request.NodeID, request.Timestamp, request.Nonce, request.Body)
	if !ed25519.Verify(ed25519.PublicKey(key), message, signature) {
		return "", unauthorized
	}
	if status != "active" {
		return "", auth.Failure("NODE_REVOKED", 403)
	}
	// The nonce must be remembered until its timestamp leaves the accepted window.
	expires := signed.Add(ClockSkew + time.Second).UnixMilli()
	result, err := s.db.ExecContext(ctx, "INSERT INTO agent_nonces(node_id,nonce,expires_at) VALUES(?,?,?) ON CONFLICT DO NOTHING", request.NodeID, request.Nonce, expires)
	if err != nil {
		return "", err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return "", unauthorized
	}
	if err := s.auth.Limit(ctx, "agent-node:"+request.NodeID, s.agentLimit, s.agentWindow); err != nil {
		return "", err
	}
	return request.NodeID, nil
}

// Poll records the status of an agent that does not report Docker capabilities.
func (s *Service) Poll(ctx context.Context, nodeID, agentVersion string, configVersion int64) (PollResult, error) {
	return s.poll(ctx, nodeID, agentVersion, configVersion, nil)
}

func (s *Service) poll(ctx context.Context, nodeID, agentVersion string, configVersion int64, docker *DockerPoll) (PollResult, error) {
	if !versionPattern.MatchString(agentVersion) || configVersion < 0 {
		return PollResult{}, auth.Failure("REQUEST_FAILED", 400)
	}
	// Subscribe before reading state so a change in between is not missed.
	changed := s.wait(nodeID)
	var current int64
	err := s.db.QueryRowContext(ctx, "UPDATE nodes SET last_seen_at=?,agent_version=? WHERE id=? AND status='active' RETURNING config_version",
		s.now().UnixMilli(), agentVersion, nodeID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return PollResult{}, auth.Failure("NODE_REVOKED", 403)
	}
	if err != nil {
		return PollResult{}, err
	}
	sources, err := s.recordDockerPoll(ctx, nodeID, docker)
	if err != nil {
		return PollResult{}, err
	}
	job, err := s.claimDockerInstall(ctx, nodeID, sources)
	if err != nil || job != nil || current != configVersion {
		return PollResult{ConfigVersion: current, DockerInstall: job}, err
	}
	timer := time.NewTimer(s.hold)
	defer timer.Stop()
	select {
	case <-changed:
	case <-timer.C:
	case <-s.closed:
	case <-ctx.Done():
		return PollResult{}, ctx.Err()
	}
	var status string
	if err = s.db.QueryRowContext(ctx, "SELECT status,config_version FROM nodes WHERE id=?", nodeID).Scan(&status, &current); err != nil {
		return PollResult{}, err
	}
	if status != "active" {
		return PollResult{}, auth.Failure("NODE_REVOKED", 403)
	}
	job, err = s.claimDockerInstall(ctx, nodeID, sources)
	return PollResult{ConfigVersion: current, DockerInstall: job}, err
}

func (s *Service) wait(id string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, exists := s.waiters[id]
	if !exists {
		channel = make(chan struct{})
		s.waiters[id] = channel
	}
	return channel
}

// notify wakes every long poll waiting on a node.
func (s *Service) notify(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if channel, exists := s.waiters[id]; exists {
		close(channel)
		delete(s.waiters, id)
	}
}
