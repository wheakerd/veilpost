package apps

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
)

// LoadCatalogFS validates and loads manifests from files instead of the
// embedded catalog.
func LoadCatalogFS(files fs.FS) (*Catalog, error) { return loadCatalog(files) }

// Parse validates one raw value against the setting definition.
func (s *Setting) Parse(raw json.RawMessage) (any, error) { return s.parse(raw) }

// StoredSettings is the JSON layout of installed_apps.settings.
type StoredSettings = stored

// SealPurpose names the master.key purpose of one sensitive setting.
func SealPurpose(appID, key string) string { return sealPurpose(appID, key) }

// Secret returns the decrypted value of a sensitive setting, for the node
// deployment that will consume it.
func (s *Service) Secret(ctx context.Context, id, key string) (string, error) {
	return s.secret(ctx, id, key)
}

// SetDeployed replaces the check that reports whether an app runs on any
// node; node phase 2 installs the real one.
func (s *Service) SetDeployed(deployed func(ctx context.Context, tx *sql.Tx, appID string) (bool, error)) {
	s.deployed = deployed
}
