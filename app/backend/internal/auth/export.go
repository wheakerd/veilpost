package auth

import "time"

// SetClock replaces the clock behind expiry and rate-limit windows. The
// server keeps time.Now; tests and tools use it to make windows
// deterministic.
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SchemaVersion is the database version this build migrates to.
func SchemaVersion() int { return len(migrations) }

// Digest is the SHA-256 hex digest under which tokens and codes are stored.
func Digest(value string) string { return digest(value) }
