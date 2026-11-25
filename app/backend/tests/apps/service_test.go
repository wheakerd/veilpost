package apps_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/wheakerd/veilpost/app/backend/internal/apps"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
)

const (
	testPassword = "correct-password-for-app-tests"
	testSecret   = "salamander-secret-value-0123456789"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type fixture struct {
	dir     string
	auth    *auth.Service
	apps    *apps.Service
	session string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	service, err := auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	if err := service.CreateUser(context.Background(), "app-admin", testPassword); err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(context.Background(), "app-admin", testPassword, "", "test-ip")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := apps.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return fixture{dir, service, apps.New(service, catalog), result.Token}
}

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var api *auth.APIError
	var setting *apps.SettingError
	switch {
	case errors.As(err, &api) && api.Code == code:
	case errors.As(err, &setting) && setting.Code == code:
	default:
		t.Fatalf("got %v, want %s", err, code)
	}
}

func expectField(t *testing.T, err error, code, field string) {
	t.Helper()
	var setting *apps.SettingError
	if !errors.As(err, &setting) || setting.Code != code || setting.Field != field {
		t.Fatalf("got %#v, want %s for %s", err, code, field)
	}
}

func raw(values map[string]any) map[string]json.RawMessage {
	result := map[string]json.RawMessage{}
	for key, value := range values {
		data, _ := json.Marshal(value)
		result[key] = data
	}
	return result
}

func secret(value string) *string { return &value }

func TestVersionTwoDatabaseMigratesToStoreSchema(t *testing.T) {
	dir := t.TempDir()
	service, err := auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.CreateUser(context.Background(), "test-user", testPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Database().Exec("INSERT INTO nodes(id,name,status,created_at) VALUES('node-a','edge','pending',1)"); err != nil {
		t.Fatal(err)
	}
	// Recreate the released version 2 layout.
	for _, statement := range []string{"DROP TABLE node_docker", "DROP TABLE certificates", "DROP TABLE acme_accounts", "DROP TABLE dns_credentials", "DROP TABLE installed_apps", "PRAGMA user_version=2"} {
		if _, err := service.Database().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	service.Close()
	service, err = auth.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var version, users, nodes, apps int
	service.Database().QueryRow("PRAGMA user_version").Scan(&version)
	service.Database().QueryRow("SELECT count(*) FROM users").Scan(&users)
	service.Database().QueryRow("SELECT count(*) FROM nodes").Scan(&nodes)
	if err := service.Database().QueryRow("SELECT count(*) FROM installed_apps").Scan(&apps); err != nil || version != auth.SchemaVersion() || users != 1 || nodes != 1 || apps != 0 {
		t.Fatalf("migration: version %d, users %d, nodes %d, apps %d, error %v", version, users, nodes, apps, err)
	}
}

func TestInstallSettingsAndUninstallLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.apps.List(ctx, "not-a-session"); err == nil {
		t.Fatal("anonymous list allowed")
	}
	list, err := f.apps.List(ctx, f.session)
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %v %v", list, err)
	}
	for _, app := range list {
		if app.Installed != nil {
			t.Fatalf("%s installed before install", app.ID)
		}
	}
	if list[0].Ports[0].Port == nil || *list[0].Ports[0].Port != 443 || list[0].Ports[1].Port != nil || list[0].Ports[1].Transport != "tcp" {
		t.Fatalf("catalog ports: %+v", list[0].Ports)
	}
	var stored int
	f.auth.Database().QueryRow("SELECT count(*) FROM installed_apps").Scan(&stored)
	if stored != 0 {
		t.Fatal("listing must not store anything")
	}

	_, err = f.apps.Settings(ctx, f.session, "xray")
	expectCode(t, err, "APP_NOT_INSTALLED")
	_, err = f.apps.UpdateSettings(ctx, f.session, "xray", apps.Update{})
	expectCode(t, err, "APP_NOT_INSTALLED")
	_, err = f.apps.Install(ctx, f.session, "missing")
	expectCode(t, err, "APP_NOT_FOUND")
	_, err = f.apps.Settings(ctx, f.session, "missing")
	expectCode(t, err, "APP_NOT_FOUND")
	_, err = f.apps.Install(ctx, "not-a-session", "xray")
	expectCode(t, err, "SESSION_EXPIRED")

	app, err := f.apps.Install(ctx, f.session, "xray")
	if err != nil || app.Installed == nil || app.Installed.Version != "26.11.9" || app.Installed.Configured {
		t.Fatalf("install: %+v %v", app.Installed, err)
	}
	_, err = f.apps.Install(ctx, f.session, "xray")
	expectCode(t, err, "APP_ALREADY_INSTALLED")

	settings, err := f.apps.Settings(ctx, f.session, "xray")
	if err != nil || settings.Configured || settings.Values["visionPort"] != int64(443) || settings.Values["visionTarget"] != nil ||
		settings.Values["xhttpMode"] != "stream-one" || len(settings.Secrets) != 0 {
		t.Fatalf("default settings: %+v %v", settings, err)
	}
	if _, exists := settings.Values["xhttpPort"]; !exists {
		t.Fatal("unset settings must be listed as null")
	}

	_, err = f.apps.UpdateSettings(ctx, f.session, "xray", apps.Update{Values: raw(map[string]any{"visionPort": 443})})
	expectField(t, err, "SETTING_REQUIRED", "visionTarget")
	_, err = f.apps.UpdateSettings(ctx, f.session, "xray", apps.Update{Values: raw(map[string]any{"visionTarget": "www.example.com", "xhttpTarget": "www.example.org", "visionPort": 70000})})
	expectField(t, err, "INVALID_SETTING", "visionPort")
	_, err = f.apps.UpdateSettings(ctx, f.session, "xray", apps.Update{Values: raw(map[string]any{"visionTarget": "www.example.com", "xhttpTarget": "www.example.org", "extra": 1})})
	expectField(t, err, "UNKNOWN_SETTING", "")
	_, err = f.apps.UpdateSettings(ctx, f.session, "xray", apps.Update{Values: raw(map[string]any{"visionTarget": "www.example.com", "xhttpTarget": "www.example.org"}), Secrets: map[string]*string{"visionPort": secret("x")}})
	expectField(t, err, "UNKNOWN_SETTING", "")

	settings, err = f.apps.UpdateSettings(ctx, f.session, "xray", apps.Update{Values: raw(map[string]any{
		"visionTarget": "WWW.example.com", "xhttpTarget": "www.example.org:8443", "xhttpPort": 20443, "xhttpPath": "/sync",
	})})
	if err != nil || !settings.Configured || settings.Values["visionTarget"] != "www.example.com:443" ||
		settings.Values["visionPort"] != int64(443) || settings.Values["xhttpPort"] != int64(20443) {
		t.Fatalf("update: %+v %v", settings, err)
	}
	list, _ = f.apps.List(ctx, f.session)
	if !list[0].Installed.Configured || *list[0].Ports[1].Port != 20443 {
		t.Fatalf("configured state: %+v %+v", list[0].Installed, list[0].Ports)
	}

	err = f.apps.Uninstall(ctx, f.session, "xray", "wrong-password-value")
	expectCode(t, err, "INVALID_PASSWORD")
	err = f.apps.Uninstall(ctx, f.session, "missing", testPassword)
	expectCode(t, err, "APP_NOT_FOUND")
	err = f.apps.Uninstall(ctx, f.session, "hysteria2", testPassword)
	expectCode(t, err, "APP_NOT_INSTALLED")
	if err := f.apps.Uninstall(ctx, f.session, "xray", testPassword); err != nil {
		t.Fatal(err)
	}
	f.auth.Database().QueryRow("SELECT count(*) FROM installed_apps").Scan(&stored)
	if stored != 0 {
		t.Fatal("uninstall must delete the settings")
	}
	// A reinstall starts from defaults again.
	if _, err := f.apps.Install(ctx, f.session, "xray"); err != nil {
		t.Fatal(err)
	}
	settings, _ = f.apps.Settings(ctx, f.session, "xray")
	if settings.Values["visionTarget"] != nil || settings.Configured {
		t.Fatalf("reinstall kept old settings: %+v", settings)
	}
}

func TestSensitiveSettingsAreSealedAndNeverReturned(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.apps.Install(ctx, f.session, "hysteria2"); err != nil {
		t.Fatal(err)
	}
	settings, err := f.apps.Settings(ctx, f.session, "hysteria2")
	if err != nil || settings.Secrets["salamanderPassword"] || settings.Configured {
		t.Fatalf("initial: %+v %v", settings, err)
	}
	_, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{Values: raw(map[string]any{"obfuscation": "salamander"})})
	expectField(t, err, "SETTING_REQUIRED", "salamanderPassword")
	_, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{Values: raw(map[string]any{"salamanderPassword": testSecret})})
	expectField(t, err, "UNKNOWN_SETTING", "")
	_, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{Values: raw(map[string]any{"obfuscation": "salamander"}), Secrets: map[string]*string{"salamanderPassword": secret("short")}})
	expectField(t, err, "INVALID_SETTING", "salamanderPassword")

	settings, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{
		Values: raw(map[string]any{"obfuscation": "salamander"}), Secrets: map[string]*string{"salamanderPassword": secret(testSecret)},
	})
	if err != nil || !settings.Secrets["salamanderPassword"] || !settings.Configured {
		t.Fatalf("set secret: %+v %v", settings, err)
	}
	encoded, _ := json.Marshal(settings)
	if strings.Contains(string(encoded), testSecret) {
		t.Fatal("settings response contains the secret")
	}
	var text string
	f.auth.Database().QueryRow("SELECT settings FROM installed_apps WHERE app_id='hysteria2'").Scan(&text)
	if strings.Contains(text, testSecret) || strings.Contains(text, "c2FsYW1hbmRlci") {
		t.Fatal("database contains the plaintext secret")
	}
	for _, file := range []string{"auth.sqlite", "auth.sqlite-wal"} {
		data, _ := os.ReadFile(f.dir + "/" + file)
		if bytes.Contains(data, []byte(testSecret)) {
			t.Fatalf("%s contains the plaintext secret", file)
		}
	}
	if plain, err := f.apps.Secret(ctx, "hysteria2", "salamanderPassword"); err != nil || plain != testSecret {
		t.Fatalf("unseal: %q %v", plain, err)
	}
	// The ciphertext is bound to its app and field.
	var current apps.StoredSettings
	json.Unmarshal([]byte(text), &current)
	if _, err := f.auth.Unseal(apps.SealPurpose("xray", "salamanderPassword"), current.Secrets["salamanderPassword"]); err == nil {
		t.Fatal("ciphertext opened for another app")
	}

	// Omitting a secret keeps it; switching modes keeps it; null clears it.
	settings, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{Values: raw(map[string]any{"obfuscation": "masquerade", "masqueradeUrl": "https://www.example.com/"})})
	if err != nil || !settings.Secrets["salamanderPassword"] || !settings.Configured {
		t.Fatalf("keep secret: %+v %v", settings, err)
	}
	_, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{Values: raw(map[string]any{"obfuscation": "salamander"}), Secrets: map[string]*string{"salamanderPassword": nil}})
	expectField(t, err, "SETTING_REQUIRED", "salamanderPassword")
	settings, err = f.apps.UpdateSettings(ctx, f.session, "hysteria2", apps.Update{Values: raw(map[string]any{"masqueradeUrl": "https://www.example.com/"}), Secrets: map[string]*string{"salamanderPassword": nil}})
	if err != nil || settings.Secrets["salamanderPassword"] {
		t.Fatalf("clear secret: %+v %v", settings, err)
	}
	if _, err := f.apps.Secret(ctx, "hysteria2", "salamanderPassword"); err == nil {
		t.Fatal("cleared secret still stored")
	}
}

func TestUninstallIsRefusedWhileDeployed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.apps.Install(ctx, f.session, "amneziawg"); err != nil {
		t.Fatal(err)
	}
	f.apps.SetDeployed(func(_ context.Context, tx *sql.Tx, id string) (bool, error) {
		return tx != nil && id == "amneziawg", nil
	})
	expectCode(t, f.apps.Uninstall(ctx, f.session, "amneziawg", testPassword), "APP_DEPLOYED")
	if _, err := f.apps.Settings(ctx, f.session, "amneziawg"); err != nil {
		t.Fatalf("deployed app lost its settings: %v", err)
	}
}

func newRouter(f fixture, logs io.Writer) *gin.Engine {
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	authHandler := &auth.Handler{Service: f.auth, Development: true, Logger: logger}
	router := gin.New()
	(&apps.Handler{Service: f.apps, Auth: authHandler, Logger: logger}).Register(router.Group("/api/apps"))
	return router
}

func TestHTTPContractAndWriteRules(t *testing.T) {
	f := newFixture(t)
	var logs bytes.Buffer
	router := newRouter(f, &logs)
	call := func(method, path, body string, header map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "http://console.test"+path, strings.NewReader(body))
		for key, value := range header {
			request.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	cookie := map[string]string{"Cookie": auth.CookieName + "=" + f.session}
	browser := map[string]string{"Origin": "http://console.test", "X-Requested-With": "Veilpost",
		"Content-Type": "application/json", "Cookie": auth.CookieName + "=" + f.session}

	for _, path := range []string{"/api/apps", "/api/apps/xray/settings"} {
		if response := call("GET", path, "", nil); response.Code != 401 || !strings.Contains(response.Body.String(), "SESSION_EXPIRED") {
			t.Fatalf("anonymous %s: %d %s", path, response.Code, response.Body)
		}
	}
	response := call("GET", "/api/apps", "", cookie)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"installed":null`) ||
		!strings.Contains(response.Body.String(), `"reference":"ghcr.io/xtls/xray-core:26.11.9@sha256:9812e382`) {
		t.Fatalf("list: %d %s", response.Code, response.Body)
	}
	for _, header := range []string{"Origin", "X-Requested-With", "Content-Type"} {
		changed := map[string]string{}
		for key, value := range browser {
			if key != header {
				changed[key] = value
			}
		}
		if response := call("POST", "/api/apps/hysteria2/install", `{}`, changed); response.Code != 403 {
			t.Fatalf("install without %s: %d", header, response.Code)
		}
	}
	crossSite := map[string]string{}
	for key, value := range browser {
		crossSite[key] = value
	}
	crossSite["Origin"] = "http://evil.test"
	for _, path := range []string{"/api/apps/hysteria2/install", "/api/apps/hysteria2/settings", "/api/apps/hysteria2/uninstall"} {
		if response := call("POST", path, `{}`, crossSite); response.Code != 403 {
			t.Fatalf("cross-site %s: %d", path, response.Code)
		}
	}
	if response := call("POST", "/api/apps/hysteria2/install", `{"version":"1"}`, browser); response.Code != 400 {
		t.Fatalf("install with unknown field: %d", response.Code)
	}
	if response := call("POST", "/api/apps/unknown/install", `{}`, browser); response.Code != 404 || !strings.Contains(response.Body.String(), "APP_NOT_FOUND") {
		t.Fatalf("install unknown: %d %s", response.Code, response.Body)
	}
	if response := call("GET", "/api/apps/hysteria2/settings", "", cookie); response.Code != 404 || !strings.Contains(response.Body.String(), "APP_NOT_INSTALLED") {
		t.Fatalf("settings before install: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/install", `{}`, browser); response.Code != 200 || !strings.Contains(response.Body.String(), `"configured":false`) {
		t.Fatalf("install: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/install", `{}`, browser); response.Code != 409 || !strings.Contains(response.Body.String(), "APP_ALREADY_INSTALLED") {
		t.Fatalf("reinstall: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/settings", `{"values":{"port":0}}`, browser); response.Code != 400 || response.Body.String() != `{"code":"INVALID_SETTING","field":"port"}` {
		t.Fatalf("invalid setting: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/settings", `{"values":{"other":1}}`, browser); response.Code != 400 || response.Body.String() != `{"code":"UNKNOWN_SETTING"}` {
		t.Fatalf("unknown setting: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/settings", `{"values":{},"extra":true}`, browser); response.Code != 400 || response.Body.String() != `{"code":"REQUEST_FAILED"}` {
		t.Fatalf("unknown body field: %d %s", response.Code, response.Body)
	}
	update := `{"values":{"obfuscation":"salamander"},"secrets":{"salamanderPassword":"` + testSecret + `"}}`
	response = call("POST", "/api/apps/hysteria2/settings", update, browser)
	if response.Code != 200 || strings.Contains(response.Body.String(), testSecret) || !strings.Contains(response.Body.String(), `"secrets":{"salamanderPassword":true}`) {
		t.Fatalf("update: %d %s", response.Code, response.Body)
	}
	for _, path := range []string{"/api/apps", "/api/apps/hysteria2/settings"} {
		if response := call("GET", path, "", cookie); response.Code != 200 || strings.Contains(response.Body.String(), testSecret) {
			t.Fatalf("%s returned the secret: %s", path, response.Body)
		}
	}
	if response := call("POST", "/api/apps/hysteria2/uninstall", `{}`, browser); response.Code != 403 || !strings.Contains(response.Body.String(), "INVALID_PASSWORD") {
		t.Fatalf("uninstall without password: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/uninstall", `{"password":"`+testPassword+`"}`, browser); response.Code != 204 {
		t.Fatalf("uninstall: %d %s", response.Code, response.Body)
	}
	if response := call("POST", "/api/apps/hysteria2/uninstall", `{"password":"`+testPassword+`"}`, browser); response.Code != 404 || !strings.Contains(response.Body.String(), "APP_NOT_INSTALLED") {
		t.Fatalf("uninstall twice: %d %s", response.Code, response.Body)
	}
	for _, secret := range []string{testSecret, testPassword} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("handler logs contain a secret")
		}
	}
}
