package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/wheakerd/veilpost/app/backend/internal/httpserver"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wheakerd/veilpost/app/backend/internal/apps"
	"github.com/wheakerd/veilpost/app/backend/internal/auth"
	"github.com/wheakerd/veilpost/app/backend/internal/nodes"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestHTTPContract(t *testing.T) {
	router := httpserver.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), nil, httpserver.Handlers{})
	for _, test := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/api/health", 200, `{"status":"ok","service":"veilpost"}`},
		{"POST", "/api/health", 405, `{"code":"METHOD_NOT_ALLOWED"}`},
		{"GET", "/api/health/", 404, `{"code":"NOT_FOUND"}`},
		{"GET", "/missing", 404, `{"code":"NOT_FOUND"}`},
		{"GET", "/api/auth/session", 404, `{"code":"NOT_FOUND"}`},
		{"POST", "/api/auth/login", 404, `{"code":"NOT_FOUND"}`},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status: got %d, want %d", response.Code, test.status)
			}
			var got, want map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(test.body), &want); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("unexpected response: %v", got)
			}
			for key, value := range want {
				if got[key] != value {
					t.Fatalf("%s: got %q, want %q", key, got[key], value)
				}
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("missing response headers: %v", response.Header())
			}
			if response.Header().Get("Set-Cookie") != "" || response.Header().Get("Location") != "" {
				t.Fatal("initialization must not issue sessions or redirects")
			}
		})
	}
}

func TestRequestLogsExcludeSensitiveValues(t *testing.T) {
	var logs bytes.Buffer
	router := httpserver.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), nil, httpserver.Handlers{})
	router.POST("/test/:id", func(c *gin.Context) { c.JSON(200, gin.H{"ip": c.ClientIP()}) })
	request := httptest.NewRequest("POST", "/test/path-secret?code=query-secret", strings.NewReader(`{"password":"body-secret"}`))
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Authorization", "Bearer header-secret")
	request.Header.Set("Cookie", "session=cookie-secret")
	request.Header.Set("X-Forwarded-For", "203.0.113.10")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Body.String() != `{"ip":"127.0.0.1"}` {
		t.Fatalf("untrusted forwarded IP was used: %s", response.Body.String())
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/unknown/path-secret?code=query-secret", nil))
	for _, secret := range []string{"path-secret", "query-secret", "body-secret", "header-secret", "cookie-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("sensitive %s appeared in logs", secret)
		}
	}
	if !strings.Contains(logs.String(), `"route":"/test/:id"`) || !strings.Contains(logs.String(), `"route":"unmatched"`) {
		t.Fatalf("route templates missing: %s", logs.String())
	}
}

func TestNodeRoutesLogOnlyTemplates(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	authHandler := &auth.Handler{Service: service, Development: true, Logger: logger}
	router := httpserver.NewRouter(logger, nil, httpserver.Handlers{Auth: authHandler, Nodes: &nodes.Handler{Service: nodes.New(service), Auth: authHandler, Logger: logger}})
	request := httptest.NewRequest("POST", "http://console.test/api/nodes/node-id-secret/revoke", strings.NewReader(`{"password":"body-secret"}`))
	request.Header.Set("Origin", "http://console.test")
	request.Header.Set("X-Requested-With", "Veilpost")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatalf("anonymous revoke: %d", response.Code)
	}
	agent := httptest.NewRequest("POST", "/api/agent/enroll", strings.NewReader(`{"token":"token-secret"}`))
	agent.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), agent)
	for _, secret := range []string{"node-id-secret", "body-secret", "token-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("sensitive %s appeared in logs", secret)
		}
	}
	if !strings.Contains(logs.String(), `"route":"/api/nodes/:id/revoke"`) || !strings.Contains(logs.String(), `"route":"/api/agent/enroll"`) {
		t.Fatalf("route templates missing: %s", logs.String())
	}
}

func TestPanicRecoveryDoesNotExposePanicValue(t *testing.T) {
	var logs bytes.Buffer
	router := httpserver.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), nil, httpserver.Handlers{})
	router.GET("/test/panic", func(*gin.Context) { panic("panic-secret") })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/test/panic", nil))
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"code":"INTERNAL_ERROR"}` {
		t.Fatalf("unexpected panic response: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(logs.String(), "panic-secret") {
		t.Fatal("panic value appeared in logs")
	}
	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest("GET", "/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatal("router did not recover")
	}
}

func TestForwardedClientAddressRequiresTrustedProxy(t *testing.T) {
	for _, test := range []struct {
		trusted []string
		want    string
	}{
		{nil, "10.0.0.5"},
		{[]string{"10.0.0.0/24"}, "203.0.113.7"},
		{[]string{"192.0.2.1/32"}, "10.0.0.5"},
	} {
		router := httpserver.NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), test.trusted, httpserver.Handlers{})
		router.GET("/client", func(c *gin.Context) { c.String(200, c.ClientIP()) })
		request := httptest.NewRequest("GET", "/client", nil)
		request.RemoteAddr = "10.0.0.5:4000"
		request.Header.Set("X-Forwarded-For", "198.51.100.9, 203.0.113.7")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Body.String() != test.want {
			t.Fatalf("trusted %v: got %q, want %q", test.trusted, response.Body.String(), test.want)
		}
	}
}

func TestAppRoutesLogOnlyTemplates(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service, err := auth.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	const password = "password-secret-for-router"
	if err := service.CreateUser(context.Background(), "router-admin", password); err != nil {
		t.Fatal(err)
	}
	session, err := service.Login(context.Background(), "router-admin", password, "", "test-ip")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := apps.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	authHandler := &auth.Handler{Service: service, Development: true, Logger: logger}
	router := httpserver.NewRouter(logger, nil, httpserver.Handlers{Auth: authHandler, Apps: &apps.Handler{Service: apps.New(service, catalog), Auth: authHandler, Logger: logger}})
	send := func(path, body string) int {
		request := httptest.NewRequest("POST", "http://console.test"+path, strings.NewReader(body))
		request.Header.Set("Origin", "http://console.test")
		request.Header.Set("X-Requested-With", "Veilpost")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Cookie", auth.CookieName+"="+session.Token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}
	if code := send("/api/apps/hysteria2/install", `{}`); code != 200 {
		t.Fatalf("install: %d", code)
	}
	if code := send("/api/apps/hysteria2/settings", `{"values":{"obfuscation":"salamander"},"secrets":{"salamanderPassword":"settings-secret-0123456789"}}`); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	if code := send("/api/apps/hysteria2/uninstall", `{"password":"`+password+`"}`); code != 204 {
		t.Fatalf("uninstall: %d", code)
	}
	for _, secret := range []string{"hysteria2", "settings-secret", password, session.Token} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("sensitive %s appeared in logs", secret)
		}
	}
	for _, route := range []string{"/api/apps/:id/install", "/api/apps/:id/settings", "/api/apps/:id/uninstall"} {
		if !strings.Contains(logs.String(), `"route":"`+route+`"`) {
			t.Fatalf("route template %s missing: %s", route, logs.String())
		}
	}
}
