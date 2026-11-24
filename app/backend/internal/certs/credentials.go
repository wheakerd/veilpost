package certs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

// Credential is the API view of DNS provider credentials. Secret values are
// never returned: Fields only tells which values are stored.
type Credential struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Provider     string          `json:"provider"`
	Fields       map[string]bool `json:"fields"`
	Certificates int             `json:"certificates"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

func credentialPurpose(id string) string { return "dns-credential:" + id }

func (s *Service) sealSecrets(id string, secrets map[string]string) ([]byte, error) {
	plain, err := json.Marshal(secrets)
	if err != nil {
		return nil, err
	}
	return s.auth.Seal(credentialPurpose(id), plain)
}

func (s *Service) openSecrets(id string, sealed []byte) (map[string]string, error) {
	plain, err := s.auth.Unseal(credentialPurpose(id), sealed)
	if err != nil {
		return nil, err
	}
	secrets := map[string]string{}
	if err := json.Unmarshal(plain, &secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s *Service) credentialView(id, name, provider string, sealed []byte, certificates int, created, updated int64) Credential {
	fields := map[string]bool{}
	// A value that cannot be opened is reported as not set.
	secrets, _ := s.openSecrets(id, sealed)
	for _, f := range Providers[provider] {
		fields[f.Name] = secrets[f.Name] != ""
	}
	return Credential{ID: id, Name: name, Provider: provider, Fields: fields, Certificates: certificates,
		CreatedAt: timestamp(created), UpdatedAt: timestamp(updated)}
}

const credentialColumns = `c.id,c.name,c.provider,c.secrets,c.created_at,c.updated_at,
 (SELECT count(*) FROM certificates x WHERE x.dns_credential_id=c.id)`

type scanner interface{ Scan(...any) error }

func (s *Service) scanCredential(row scanner) (Credential, error) {
	var id, name, provider string
	var sealed []byte
	var created, updated int64
	var certificates int
	if err := row.Scan(&id, &name, &provider, &sealed, &created, &updated, &certificates); err != nil {
		return Credential{}, err
	}
	return s.credentialView(id, name, provider, sealed, certificates, created, updated), nil
}

// Credentials lists the stored DNS provider credentials by name.
func (s *Service) Credentials(ctx context.Context, session string) ([]Credential, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+credentialColumns+" FROM dns_credentials c ORDER BY c.name COLLATE NOCASE, c.created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Credential{}
	for rows.Next() {
		credential, err := s.scanCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, credential)
	}
	return result, rows.Err()
}

func (s *Service) credential(ctx context.Context, id string) (Credential, error) {
	credential, err := s.scanCredential(s.db.QueryRowContext(ctx, "SELECT "+credentialColumns+" FROM dns_credentials c WHERE c.id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Credential{}, auth.Failure("CREDENTIAL_NOT_FOUND", 404)
	}
	return credential, err
}

// CreateCredential stores sealed provider credentials after the current
// password is re-entered.
func (s *Service) CreateCredential(ctx context.Context, session, name, provider string, input map[string]string, password string) (Credential, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Credential{}, err
	}
	name, valid := validName(name)
	if !valid {
		return Credential{}, auth.Failure("INVALID_CREDENTIAL_NAME", 400)
	}
	secrets, err := mergeSecrets(provider, nil, input)
	if err != nil {
		return Credential{}, err
	}
	if _, err := s.auth.ConfirmPassword(ctx, session, password); err != nil {
		return Credential{}, err
	}
	id := randomID()
	sealed, err := s.sealSecrets(id, secrets)
	if err != nil {
		return Credential{}, err
	}
	now := s.now().UnixMilli()
	if _, err := s.db.ExecContext(ctx, "INSERT INTO dns_credentials(id,name,provider,secrets,created_at,updated_at) VALUES(?,?,?,?,?,?)",
		id, name, provider, sealed, now, now); err != nil {
		return Credential{}, err
	}
	return s.credential(ctx, id)
}

// confirmedCredential re-checks the password, then opens a transaction and
// reads the credential inside it.
func (s *Service) confirmedCredential(ctx context.Context, session, id, password string) (*sql.Tx, string, []byte, error) {
	if _, err := s.auth.ConfirmPassword(ctx, session, password); err != nil {
		return nil, "", nil, err
	}
	if !idPattern.MatchString(id) {
		return nil, "", nil, auth.Failure("CREDENTIAL_NOT_FOUND", 404)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", nil, err
	}
	var provider string
	var sealed []byte
	if err = tx.QueryRowContext(ctx, "SELECT provider,secrets FROM dns_credentials WHERE id=?", id).Scan(&provider, &sealed); err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", nil, auth.Failure("CREDENTIAL_NOT_FOUND", 404)
		}
		return nil, "", nil, err
	}
	return tx, provider, sealed, nil
}

// UpdateCredential renames credentials and replaces the submitted values;
// omitted values keep their stored value. The provider cannot change.
func (s *Service) UpdateCredential(ctx context.Context, session, id, name string, input map[string]string, password string) (Credential, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Credential{}, err
	}
	name, valid := validName(name)
	if !valid {
		return Credential{}, auth.Failure("INVALID_CREDENTIAL_NAME", 400)
	}
	tx, provider, sealed, err := s.confirmedCredential(ctx, session, id, password)
	if err != nil {
		return Credential{}, err
	}
	defer tx.Rollback()
	stored, err := s.openSecrets(id, sealed)
	if err != nil {
		// Unreadable values must be entered again in full.
		stored = nil
	}
	secrets, err := mergeSecrets(provider, stored, input)
	if err != nil {
		return Credential{}, err
	}
	if sealed, err = s.sealSecrets(id, secrets); err != nil {
		return Credential{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE dns_credentials SET name=?,secrets=?,updated_at=? WHERE id=?", name, sealed, s.now().UnixMilli(), id); err != nil {
		return Credential{}, err
	}
	if err = tx.Commit(); err != nil {
		return Credential{}, err
	}
	return s.credential(ctx, id)
}

// DeleteCredential removes credentials that no certificate references.
func (s *Service) DeleteCredential(ctx context.Context, session, id, password string) error {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return err
	}
	tx, _, _, err := s.confirmedCredential(ctx, session, id, password)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var used int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM certificates WHERE dns_credential_id=?", id).Scan(&used); err != nil {
		return err
	}
	if used > 0 {
		return auth.Failure("CREDENTIAL_IN_USE", 409)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM dns_credentials WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyCredential calls a read-only API of the provider with the stored
// values. It never creates DNS records.
func (s *Service) VerifyCredential(ctx context.Context, session, id string) error {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return err
	}
	if err := s.limit(ctx, "verify", session, 10, 5*time.Minute); err != nil {
		return err
	}
	if !idPattern.MatchString(id) {
		return auth.Failure("CREDENTIAL_NOT_FOUND", 404)
	}
	var provider string
	var sealed []byte
	err := s.db.QueryRowContext(ctx, "SELECT provider,secrets FROM dns_credentials WHERE id=?", id).Scan(&provider, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Failure("CREDENTIAL_NOT_FOUND", 404)
	}
	if err != nil {
		return err
	}
	secrets, err := s.openSecrets(id, sealed)
	if err != nil {
		return auth.Failure("CREDENTIAL_UNREADABLE", 409)
	}
	checkCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	if err := s.verifyCredential(checkCtx, provider, secrets); err != nil {
		if unreachable(err) {
			return auth.Failure("PROVIDER_UNREACHABLE", 424)
		}
		return auth.Failure("CREDENTIAL_REJECTED", 422)
	}
	return nil
}
