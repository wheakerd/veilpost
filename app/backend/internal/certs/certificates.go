package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

// Certificate is the API view of a certificate request. The private key is
// never part of it.
type Certificate struct {
	ID                 string        `json:"id"`
	Domains            []string      `json:"domains"`
	Directory          string        `json:"directory"`
	DirectoryURL       string        `json:"directoryUrl"`
	Email              string        `json:"email"`
	CredentialID       string        `json:"dnsCredentialId"`
	CredentialName     string        `json:"dnsCredentialName"`
	CredentialProvider string        `json:"dnsCredentialProvider"`
	Status             string        `json:"status"`
	Issuing            bool          `json:"issuing"`
	Issuer             *string       `json:"issuer"`
	NotBefore          *string       `json:"notBefore"`
	NotAfter           *string       `json:"notAfter"`
	RenewAt            *string       `json:"renewAt"`
	LastError          *IssueProblem `json:"lastError"`
	CreatedAt          string        `json:"createdAt"`
	UpdatedAt          string        `json:"updatedAt"`
}

// IssueProblem is the category and, for ACME problems, a short description
// of the last failed attempt.
type IssueProblem struct {
	Code   string  `json:"code"`
	Detail *string `json:"detail"`
	At     string  `json:"at"`
}

// RequestInput is a certificate request submitted by the console.
type RequestInput struct {
	Domains      []string `json:"domains"`
	CredentialID string   `json:"dnsCredentialId"`
	Directory    string   `json:"directory"`
	DirectoryURL string   `json:"directoryUrl"`
	Email        string   `json:"email"`
	EAB          EAB      `json:"eab"`
}

const certificateColumns = `c.id,c.domains,c.ca_directory,a.email,c.dns_credential_id,d.name,d.provider,c.status,c.issuer,c.not_before,c.not_after,
 c.renew_at,c.last_error,c.last_error_detail,c.last_error_at,c.created_at,c.updated_at
 FROM certificates c JOIN acme_accounts a ON a.id=c.acme_account_id JOIN dns_credentials d ON d.id=c.dns_credential_id`

func (s *Service) scanCertificate(row scanner) (Certificate, error) {
	var c Certificate
	var domains string
	var issuer, lastError, lastDetail sql.NullString
	var notBefore, notAfter, renew, errorAt sql.NullInt64
	var created, updated int64
	if err := row.Scan(&c.ID, &domains, &c.DirectoryURL, &c.Email, &c.CredentialID, &c.CredentialName, &c.CredentialProvider, &c.Status, &issuer,
		&notBefore, &notAfter, &renew, &lastError, &lastDetail, &errorAt, &created, &updated); err != nil {
		return Certificate{}, err
	}
	if err := json.Unmarshal([]byte(domains), &c.Domains); err != nil {
		return Certificate{}, err
	}
	c.Directory = directoryKind(c.DirectoryURL)
	c.Issuing = s.active(c.ID)
	if issuer.Valid {
		c.Issuer = &issuer.String
	}
	c.NotBefore, c.NotAfter, c.RenewAt = optionalTimestamp(notBefore), optionalTimestamp(notAfter), optionalTimestamp(renew)
	if lastError.Valid {
		problem := &IssueProblem{Code: lastError.String, At: timestamp(errorAt.Int64)}
		if lastDetail.Valid {
			problem.Detail = &lastDetail.String
		}
		c.LastError = problem
	}
	c.CreatedAt, c.UpdatedAt = timestamp(created), timestamp(updated)
	return c, nil
}

// Certificates lists every certificate request, newest first.
func (s *Service) Certificates(ctx context.Context, session string) ([]Certificate, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+certificateColumns+" ORDER BY c.created_at DESC, c.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Certificate{}
	for rows.Next() {
		certificate, err := s.scanCertificate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, certificate)
	}
	return result, rows.Err()
}

func (s *Service) certificate(ctx context.Context, id string) (Certificate, error) {
	certificate, err := s.scanCertificate(s.db.QueryRowContext(ctx, "SELECT "+certificateColumns+" WHERE c.id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Certificate{}, auth.Failure("CERTIFICATE_NOT_FOUND", 404)
	}
	return certificate, err
}

// Request records a certificate request and queues its issuance. The ACME
// account of the directory and contact email is reused, or created with a
// new key that is sealed with master.key.
func (s *Service) Request(ctx context.Context, session string, input RequestInput) (Certificate, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Certificate{}, err
	}
	domains, err := NormalizeDomains(input.Domains)
	if err != nil {
		return Certificate{}, err
	}
	directory, err := ResolveDirectory(input.Directory, input.DirectoryURL)
	if err != nil {
		return Certificate{}, err
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return Certificate{}, err
	}
	eab, err := validEAB(directoryKind(directory), input.EAB)
	if err != nil {
		return Certificate{}, err
	}
	if !idPattern.MatchString(input.CredentialID) {
		return Certificate{}, auth.Failure("CREDENTIAL_NOT_FOUND", 404)
	}
	if err := s.limit(ctx, "issue", session, 20, 10*time.Minute); err != nil {
		return Certificate{}, err
	}
	encodedDomains, err := json.Marshal(domains)
	if err != nil {
		return Certificate{}, err
	}
	// Prepare the sealed values outside the write transaction.
	accountID := randomID()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Certificate{}, err
	}
	sealedKey, err := s.auth.Seal(accountKeyPurpose(accountID), certcrypto.PEMEncode(key))
	if err != nil {
		return Certificate{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Certificate{}, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM dns_credentials WHERE id=?", input.CredentialID).Scan(&exists); err != nil {
		return Certificate{}, err
	}
	if exists == 0 {
		return Certificate{}, auth.Failure("CREDENTIAL_NOT_FOUND", 404)
	}
	var existing string
	var registered sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT id,account_url FROM acme_accounts WHERE directory_url=? AND email=?", directory, email).Scan(&existing, &registered)
	now := s.now().UnixMilli()
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err = tx.ExecContext(ctx, "INSERT INTO acme_accounts(id,directory_url,email,private_key,created_at) VALUES(?,?,?,?,?)",
			accountID, directory, email, sealedKey, now); err != nil {
			return Certificate{}, err
		}
	case err != nil:
		return Certificate{}, err
	default:
		accountID = existing
	}
	// EAB is needed only until the account is registered; a registered
	// account ignores it.
	if eab != nil && !(registered.Valid && registered.String != "") {
		plain, _ := json.Marshal(eab)
		sealedEAB, err := s.auth.Seal(accountEABPurpose(accountID), plain)
		if err != nil {
			return Certificate{}, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE acme_accounts SET eab=? WHERE id=?", sealedEAB, accountID); err != nil {
			return Certificate{}, err
		}
	}
	id := randomID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO certificates(id,domains,ca_directory,acme_account_id,dns_credential_id,status,renew_at,created_at,updated_at)
 VALUES(?,?,?,?,?,'pending',?,?,?)`, id, string(encodedDomains), directory, accountID, input.CredentialID, now, now, now); err != nil {
		return Certificate{}, err
	}
	if err = tx.Commit(); err != nil {
		return Certificate{}, err
	}
	s.enqueue(id)
	return s.certificate(ctx, id)
}

// Reissue queues a new issuance at once. A failed request becomes pending
// again; a valid certificate stays in use until its replacement is issued.
func (s *Service) Reissue(ctx context.Context, session, id string) (Certificate, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Certificate{}, err
	}
	if !idPattern.MatchString(id) {
		return Certificate{}, auth.Failure("CERTIFICATE_NOT_FOUND", 404)
	}
	if !s.active(id) {
		if err := s.limit(ctx, "issue", session, 20, 10*time.Minute); err != nil {
			return Certificate{}, err
		}
		now := s.now().UnixMilli()
		result, err := s.db.ExecContext(ctx, `UPDATE certificates SET renew_at=?,updated_at=?,
 status=CASE WHEN certificate_pem IS NULL THEN 'pending' ELSE status END,
 last_error=CASE WHEN certificate_pem IS NULL THEN NULL ELSE last_error END,
 last_error_detail=CASE WHEN certificate_pem IS NULL THEN NULL ELSE last_error_detail END,
 last_error_at=CASE WHEN certificate_pem IS NULL THEN NULL ELSE last_error_at END WHERE id=?`, now, now, id)
		if err != nil {
			return Certificate{}, err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return Certificate{}, auth.Failure("CERTIFICATE_NOT_FOUND", 404)
		}
		s.enqueue(id)
	}
	return s.certificate(ctx, id)
}

// DeleteCertificate removes a certificate with its sealed key and stops a
// running issuance. The ACME account stays for later requests.
func (s *Service) DeleteCertificate(ctx context.Context, session, id string) error {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return err
	}
	if !idPattern.MatchString(id) {
		return auth.Failure("CERTIFICATE_NOT_FOUND", 404)
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM certificates WHERE id=?", id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return auth.Failure("CERTIFICATE_NOT_FOUND", 404)
	}
	s.cancelJob(id)
	return nil
}

// Chain returns the PEM certificate chain (leaf first) and the first name
// of an issued certificate.
func (s *Service) Chain(ctx context.Context, session, id string) (string, string, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return "", "", err
	}
	if !idPattern.MatchString(id) {
		return "", "", auth.Failure("CERTIFICATE_NOT_FOUND", 404)
	}
	var domains string
	var pem sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT domains,certificate_pem FROM certificates WHERE id=?", id).Scan(&domains, &pem)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", auth.Failure("CERTIFICATE_NOT_FOUND", 404)
	}
	if err != nil {
		return "", "", err
	}
	if !pem.Valid {
		return "", "", auth.Failure("CERTIFICATE_NOT_ISSUED", 409)
	}
	var names []string
	if json.Unmarshal([]byte(domains), &names) != nil || len(names) == 0 {
		return "", "", errors.New("invalid certificate names")
	}
	return pem.String, names[0], nil
}
