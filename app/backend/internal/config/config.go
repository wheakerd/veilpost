package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
	DataDir         string
	Development     bool
	PublicOrigin    string
	// TrustedProxies lists the reverse proxies whose X-Forwarded-For header
	// identifies the client for rate limiting. Empty trusts no forwarded header.
	TrustedProxies []string
}

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads the configuration through lookup, which reports whether a
// variable is set, as os.LookupEnv does.
func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	value := func(name, fallback string) string {
		if result, exists := lookup(name); exists {
			return result
		}
		return fallback
	}
	cfg := Config{HTTPAddr: value("VPN_HTTP_ADDR", "127.0.0.1:8080")}
	environment := value("VPN_ENV", "production")
	if environment != "production" && environment != "development" {
		return Config{}, fmt.Errorf("VPN_ENV must be production or development")
	}
	cfg.Development = environment == "development"
	cfg.DataDir = value("VPN_DATA_DIR", "data")
	if strings.TrimSpace(cfg.DataDir) == "" {
		return Config{}, fmt.Errorf("VPN_DATA_DIR must not be empty")
	}
	cfg.PublicOrigin = strings.TrimRight(value("VPN_PUBLIC_ORIGIN", ""), "/")
	if cfg.PublicOrigin != "" {
		origin, err := url.Parse(cfg.PublicOrigin)
		if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || (!cfg.Development && origin.Scheme != "https") || (origin.Scheme != "http" && origin.Scheme != "https") {
			return Config{}, fmt.Errorf("VPN_PUBLIC_ORIGIN must be an origin (HTTPS in production)")
		}
	}
	for _, entry := range strings.Split(value("VPN_TRUSTED_PROXIES", ""), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			address, addressErr := netip.ParseAddr(entry)
			if addressErr != nil || address.Zone() != "" {
				return Config{}, fmt.Errorf("VPN_TRUSTED_PROXIES must be a comma-separated list of IP addresses or CIDR ranges")
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, prefix.Masked().String())
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("VPN_HTTP_ADDR must be a host:port address")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return Config{}, fmt.Errorf("VPN_HTTP_ADDR must use a port between 0 and 65535")
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(value("VPN_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("VPN_LOG_LEVEL must be a valid slog level")
	}
	cfg.ShutdownTimeout, err = time.ParseDuration(value("VPN_SHUTDOWN_TIMEOUT", "10s"))
	if err != nil || cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("VPN_SHUTDOWN_TIMEOUT must be a positive duration, such as 10s")
	}
	return cfg, nil
}
