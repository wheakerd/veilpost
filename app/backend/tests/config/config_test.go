package config_test

import (
	"github.com/wheakerd/veilpost/app/backend/internal/config"
	"log/slog"
	"testing"
	"time"
)

func fromValues(values map[string]string) (config.Config, error) {
	return config.LoadFrom(func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	})
}

func TestDefaults(t *testing.T) {
	cfg, err := fromValues(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" || cfg.LogLevel != slog.LevelInfo || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	cfg, err := fromValues(map[string]string{
		"VPN_HTTP_ADDR": "[::1]:9090", "VPN_LOG_LEVEL": "debug", "VPN_SHUTDOWN_TIMEOUT": "250ms",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "[::1]:9090" || cfg.LogLevel != slog.LevelDebug || cfg.ShutdownTimeout != 250*time.Millisecond {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestInvalidConfigurationFails(t *testing.T) {
	for _, test := range []struct{ key, value string }{
		{"VPN_HTTP_ADDR", ""}, {"VPN_HTTP_ADDR", "8080"},
		{"VPN_HTTP_ADDR", "127.0.0.1:-1"}, {"VPN_HTTP_ADDR", "127.0.0.1:65536"},
		{"VPN_HTTP_ADDR", "127.0.0.1:http"}, {"VPN_LOG_LEVEL", "verbose"},
		{"VPN_SHUTDOWN_TIMEOUT", "0s"}, {"VPN_SHUTDOWN_TIMEOUT", "-1s"},
		{"VPN_SHUTDOWN_TIMEOUT", "10"}, {"VPN_SHUTDOWN_TIMEOUT", ""},
		{"VPN_TRUSTED_PROXIES", "frontend"}, {"VPN_TRUSTED_PROXIES", "10.0.0.0/33"},
		{"VPN_TRUSTED_PROXIES", "fe80::1%eth0"},
	} {
		t.Run(test.key+"/"+test.value, func(t *testing.T) {
			if _, err := fromValues(map[string]string{test.key: test.value}); err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
		})
	}
}

func TestOperatingSystemAssignedPort(t *testing.T) {
	cfg, err := fromValues(map[string]string{"VPN_HTTP_ADDR": "127.0.0.1:0"})
	if err != nil || cfg.HTTPAddr != "127.0.0.1:0" {
		t.Fatalf("port zero should be available for development: %+v, %v", cfg, err)
	}
}

func TestTrustedProxies(t *testing.T) {
	cfg, err := fromValues(nil)
	if err != nil || cfg.TrustedProxies != nil {
		t.Fatalf("no proxy is trusted by default: %+v, %v", cfg, err)
	}
	cfg, err = fromValues(map[string]string{"VPN_TRUSTED_PROXIES": " 10.1.2.3 , 172.16.5.0/12,::1,"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.1.2.3/32", "172.16.0.0/12", "::1/128"}
	if len(cfg.TrustedProxies) != len(want) {
		t.Fatalf("got %v, want %v", cfg.TrustedProxies, want)
	}
	for index := range want {
		if cfg.TrustedProxies[index] != want[index] {
			t.Fatalf("got %v, want %v", cfg.TrustedProxies, want)
		}
	}
}
