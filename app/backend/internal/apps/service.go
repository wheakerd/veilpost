// Package apps implements the software store (docs/platform-design.md,
// section 2): a built-in catalog of VPN software and its installation state.
// Installing an app enables it in the console; deploying it to nodes depends
// on node phase 2 and is not implemented.
package apps

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

// App is a catalog entry with its installation state.
type App struct {
	ID          string        `json:"id"`
	Version     string        `json:"version"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Protocols   []Protocol    `json:"protocols"`
	Ports       []PortView    `json:"ports"`
	Settings    []Setting     `json:"settings"`
	Image       ImageView     `json:"image"`
	Installed   *Installation `json:"installed"`
}

// ImageView adds the digest-pinned pull reference (empty for local builds).
type ImageView struct {
	Image
	Reference string `json:"reference"`
}

// PortView is a port setting with its effective value: the configured value
// of an installed app, otherwise the default. Null means the port is chosen
// per node at deployment.
type PortView struct {
	Setting   string `json:"setting"`
	Protocol  string `json:"protocol"`
	Transport string `json:"transport"`
	Port      *int64 `json:"port"`
}

// Installation describes an installed app. Configured is false while a
// required setting has no value.
type Installation struct {
	Version     string `json:"version"`
	InstalledAt string `json:"installedAt"`
	UpdatedAt   string `json:"updatedAt"`
	Configured  bool   `json:"configured"`
}

// Settings is the readable configuration of an installed app. Values lists
// every non-sensitive setting (null when unset); Secrets reports only
// whether each sensitive setting is set and never contains its value.
type Settings struct {
	Values     map[string]any  `json:"values"`
	Secrets    map[string]bool `json:"secrets"`
	Configured bool            `json:"configured"`
	UpdatedAt  string          `json:"updatedAt"`
}

type Service struct {
	auth    *auth.Service
	db      *sql.DB
	catalog *Catalog
	now     func() time.Time
	// deployed reports whether an app runs on any node. Node phase 2 adds
	// deployments; until then nothing is deployed, so uninstalling never
	// answers APP_DEPLOYED. Replace this with a query of the deployment
	// table inside the uninstall transaction when it exists.
	deployed func(ctx context.Context, tx *sql.Tx, appID string) (bool, error)
}

func New(authentication *auth.Service, catalog *Catalog) *Service {
	return &Service{
		auth: authentication, db: authentication.Database(), catalog: catalog, now: time.Now,
		deployed: func(context.Context, *sql.Tx, string) (bool, error) { return false, nil },
	}
}

func timestamp(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano) }

func sealPurpose(appID, key string) string { return "apps:" + appID + ":" + key }

func (s *Service) manifest(id string) (*Manifest, error) {
	manifest, exists := s.catalog.Get(id)
	if !exists {
		return nil, auth.Failure("APP_NOT_FOUND", 404)
	}
	return manifest, nil
}

type installedRow struct {
	version   string
	settings  stored
	installed int64
	updated   int64
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readInstalled(ctx context.Context, db queryer, id string) (*installedRow, error) {
	var row installedRow
	var text string
	err := db.QueryRowContext(ctx, "SELECT version,settings,installed_at,updated_at FROM installed_apps WHERE app_id=?", id).
		Scan(&row.version, &text, &row.installed, &row.updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.settings, err = decodeStored(text); err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) view(manifest *Manifest, row *installedRow) App {
	app := App{
		ID: manifest.ID, Version: manifest.Version, Name: manifest.Name, Description: manifest.Description,
		Protocols: manifest.Protocols, Ports: []PortView{}, Settings: manifest.Settings,
		Image: ImageView{Image: manifest.Image, Reference: manifest.Image.Reference()},
	}
	var values map[string]any
	if row != nil {
		values = manifest.values(row.settings)
		app.Installed = &Installation{
			Version: row.version, InstalledAt: timestamp(row.installed), UpdatedAt: timestamp(row.updated),
			Configured: manifest.missing(values, row.settings.Secrets) == "",
		}
	}
	transports := map[string]string{}
	for _, protocol := range manifest.Protocols {
		transports[protocol.ID] = protocol.Transport
	}
	for _, port := range manifest.Ports {
		view := PortView{Setting: port.Setting, Protocol: port.Protocol, Transport: transports[port.Protocol]}
		var value any
		if row != nil {
			value = values[port.Setting]
		} else {
			for _, setting := range manifest.Settings {
				if setting.Key == port.Setting {
					value = setting.defaultValue
				}
			}
		}
		if number, ok := value.(int64); ok {
			view.Port = &number
		}
		app.Ports = append(app.Ports, view)
	}
	return app
}

// List returns the catalog with the installation state of every app.
func (s *Service) List(ctx context.Context, session string) ([]App, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return nil, err
	}
	apps := []App{}
	for i := range s.catalog.Apps() {
		manifest := &s.catalog.Apps()[i]
		row, err := readInstalled(ctx, s.db, manifest.ID)
		if err != nil {
			return nil, err
		}
		apps = append(apps, s.view(manifest, row))
	}
	return apps, nil
}

// Install enables an app in the console with its default settings.
// Required settings without a default stay unset until configured.
func (s *Service) Install(ctx context.Context, session, id string) (App, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return App{}, err
	}
	manifest, err := s.manifest(id)
	if err != nil {
		return App{}, err
	}
	settings, err := manifest.defaults().encode()
	if err != nil {
		return App{}, err
	}
	now := s.now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `INSERT INTO installed_apps(app_id,version,settings,installed_at,updated_at)
 VALUES(?,?,?,?,?) ON CONFLICT(app_id) DO NOTHING`, manifest.ID, manifest.Version, settings, now, now)
	if err != nil {
		return App{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return App{}, auth.Failure("APP_ALREADY_INSTALLED", 409)
	}
	row, err := readInstalled(ctx, s.db, manifest.ID)
	if err != nil || row == nil {
		return App{}, errors.Join(err, errors.New("installed app vanished"))
	}
	return s.view(manifest, row), nil
}

// Uninstall removes an app and all of its settings after the current
// password is re-entered. Apps deployed to nodes cannot be uninstalled.
func (s *Service) Uninstall(ctx context.Context, session, id, password string) error {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return err
	}
	manifest, err := s.manifest(id)
	if err != nil {
		return err
	}
	if _, err := s.auth.ConfirmPassword(ctx, session, password); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := readInstalled(ctx, tx, manifest.ID)
	if err != nil {
		return err
	}
	if row == nil {
		return auth.Failure("APP_NOT_INSTALLED", 404)
	}
	deployed, err := s.deployed(ctx, tx, manifest.ID)
	if err != nil {
		return err
	}
	if deployed {
		return auth.Failure("APP_DEPLOYED", 409)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM installed_apps WHERE app_id=?", manifest.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) settingsView(manifest *Manifest, row *installedRow) Settings {
	values := manifest.values(row.settings)
	secrets := map[string]bool{}
	for _, setting := range manifest.Settings {
		if setting.Sensitive {
			secrets[setting.Key] = len(row.settings.Secrets[setting.Key]) > 0
		}
	}
	return Settings{Values: values, Secrets: secrets, UpdatedAt: timestamp(row.updated),
		Configured: manifest.missing(values, row.settings.Secrets) == ""}
}

// Settings returns the configuration of an installed app without the
// values of sensitive settings.
func (s *Service) Settings(ctx context.Context, session, id string) (Settings, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Settings{}, err
	}
	manifest, err := s.manifest(id)
	if err != nil {
		return Settings{}, err
	}
	row, err := readInstalled(ctx, s.db, manifest.ID)
	if err != nil {
		return Settings{}, err
	}
	if row == nil {
		return Settings{}, auth.Failure("APP_NOT_INSTALLED", 404)
	}
	return s.settingsView(manifest, row), nil
}

// UpdateSettings validates a settings change against the manifest and
// stores it; sensitive values are sealed with master.key.
func (s *Service) UpdateSettings(ctx context.Context, session, id string, update Update) (Settings, error) {
	if _, err := s.auth.Authorize(ctx, session); err != nil {
		return Settings{}, err
	}
	manifest, err := s.manifest(id)
	if err != nil {
		return Settings{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback()
	row, err := readInstalled(ctx, tx, manifest.ID)
	if err != nil {
		return Settings{}, err
	}
	if row == nil {
		return Settings{}, auth.Failure("APP_NOT_INSTALLED", 404)
	}
	next, err := manifest.apply(row.settings, update, func(key, value string) ([]byte, error) {
		return s.auth.Seal(sealPurpose(manifest.ID, key), []byte(value))
	})
	if err != nil {
		return Settings{}, err
	}
	text, err := next.encode()
	if err != nil {
		return Settings{}, err
	}
	row.settings, row.updated = next, s.now().UnixMilli()
	if _, err = tx.ExecContext(ctx, "UPDATE installed_apps SET settings=?,updated_at=? WHERE app_id=?", text, row.updated, manifest.ID); err != nil {
		return Settings{}, err
	}
	if err = tx.Commit(); err != nil {
		return Settings{}, err
	}
	return s.settingsView(manifest, row), nil
}

// secret returns the plaintext of a sensitive setting for future
// deployment code and for tests; no API returns it.
func (s *Service) secret(ctx context.Context, id, key string) (string, error) {
	row, err := readInstalled(ctx, s.db, id)
	if err != nil || row == nil {
		return "", errors.Join(err, errors.New("app not installed"))
	}
	sealed := row.settings.Secrets[key]
	if len(sealed) == 0 {
		return "", errors.New("secret not set")
	}
	plain, err := s.auth.Unseal(sealPurpose(id, key), sealed)
	return string(plain), err
}
