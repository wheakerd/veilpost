package auth

import (
	"context"
	"database/sql"
	"mime"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// This file exposes the session, password and request checks that other
// console APIs (such as /api/nodes) must reuse instead of reimplementing.

// Failure returns an API error with a stable code and HTTP status.
func Failure(code string, status int) error { return failure(code, status) }

// Database returns the shared SQLite handle. The schema and its migrations
// stay in this package so one file holds every table, backup and Prune.
func (s *Service) Database() *sql.DB { return s.db }

// Authorize returns the account of a complete (two-step verified) session or
// SESSION_EXPIRED.
func (s *Service) Authorize(ctx context.Context, token string) (User, error) {
	record, err := s.requireSession(ctx, s.db, token)
	if err != nil {
		return User{}, err
	}
	return record.User, nil
}

// ConfirmPassword re-verifies the current password for a sensitive operation.
// It shares the per-session security-operation limit with account security.
func (s *Service) ConfirmPassword(ctx context.Context, token, password string) (User, error) {
	if len(password) > 1024 {
		return User{}, failure("INVALID_PASSWORD", 403)
	}
	record, err := s.confirmSecurityChange(ctx, token, password)
	if err != nil {
		return User{}, err
	}
	return record.User, nil
}

// Limit counts one attempt in a persistent bucket. Callers must prefix keys
// with their own namespace so budgets are never shared by accident.
func (s *Service) Limit(ctx context.Context, key string, maximum int, window time.Duration) error {
	return s.limit(ctx, key, maximum, window)
}

// Token returns the session cookie value of a request, if any.
func (h *Handler) Token(c *gin.Context) string { return h.token(c) }

// Fail writes an API error response and reports whether err was non-nil.
// Unexpected errors are logged without details and returned as 503.
func (h *Handler) Fail(c *gin.Context, err error) bool { return h.failed(c, err) }

// RequireWrite enforces the browser write-request rules: same-origin (and
// HTTPS in production) Origin, the custom X-Requested-With header and a JSON
// body. It aborts with 403 and returns false when a rule fails.
func (h *Handler) RequireWrite(c *gin.Context) bool {
	origin, err := url.Parse(c.GetHeader("Origin"))
	contentType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || origin.Host == "" || origin.Host != c.Request.Host || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" ||
		(!h.Development && origin.Scheme != "https") || (h.Development && origin.Scheme != "http" && origin.Scheme != "https") ||
		(h.PublicOrigin != "" && c.GetHeader("Origin") != h.PublicOrigin) || c.GetHeader("X-Requested-With") != "Veilpost" || contentType != "application/json" {
		c.AbortWithStatusJSON(403, gin.H{"code": "REQUEST_FAILED"})
		return false
	}
	return true
}
