package certs

import (
	"encoding/base64"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wheakerd/veilpost/app/backend/internal/auth"
	"golang.org/x/net/idna"
)

// MaxDomains is the ACME limit of names on one certificate.
const MaxDomains = 100

// Built-in ACME directories.
const (
	LetsEncryptProduction = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStaging    = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Directory kinds accepted by the API.
const (
	DirectoryLetsEncrypt        = "letsencrypt"
	DirectoryLetsEncryptStaging = "letsencrypt-staging"
	DirectoryCustom             = "custom"
)

var (
	labelPattern    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	idPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	regionPattern   = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$`)
	zoneIDPattern   = regexp.MustCompile(`^[A-Z0-9]{1,32}$`)
	eabKeyIDPattern = regexp.MustCompile(`^[\x21-\x7e]{1,256}$`)
)

// field describes one value of a provider's credential.
type field struct {
	Name     string
	Required bool
	// valid checks a trimmed, non-empty value.
	valid func(string) bool
}

func secretValue(value string) bool {
	if len(value) > 512 {
		return false
	}
	for _, r := range value {
		// API keys and tokens are printable ASCII without spaces.
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}

// Providers lists the DNS services supported for DNS-01 and their credential
// fields, in display order. Every field is stored sealed and never returned.
var Providers = map[string][]field{
	"cloudflare": {
		{Name: "apiToken", Required: true, valid: secretValue},
	},
	"alidns": {
		{Name: "accessKeyId", Required: true, valid: secretValue},
		{Name: "accessKeySecret", Required: true, valid: secretValue},
	},
	"tencentcloud": {
		{Name: "secretId", Required: true, valid: secretValue},
		{Name: "secretKey", Required: true, valid: secretValue},
	},
	"route53": {
		{Name: "accessKeyId", Required: true, valid: secretValue},
		{Name: "secretAccessKey", Required: true, valid: secretValue},
		{Name: "region", Required: true, valid: regionPattern.MatchString},
		{Name: "hostedZoneId", valid: func(value string) bool {
			return zoneIDPattern.MatchString(strings.TrimPrefix(value, "/hostedzone/"))
		}},
	},
}

// validName trims an optional credential note and accepts up to 64 printable characters.
func validName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			return "", false
		}
	}
	return name, true
}

// mergeSecrets applies submitted values to the stored ones. A non-empty value
// replaces a field, an empty value clears an optional field, and an absent
// field keeps its stored value. Unknown fields and missing required fields
// are rejected.
func mergeSecrets(provider string, stored, input map[string]string) (map[string]string, error) {
	fields, known := Providers[provider]
	if !known {
		return nil, auth.Failure("INVALID_PROVIDER", 400)
	}
	invalid := auth.Failure("INVALID_SECRETS", 400)
	result := map[string]string{}
	allowed := map[string]field{}
	for _, f := range fields {
		allowed[f.Name] = f
		if value, exists := stored[f.Name]; exists {
			result[f.Name] = value
		}
	}
	for name, value := range input {
		f, exists := allowed[name]
		if !exists {
			return nil, invalid
		}
		value = strings.TrimSpace(value)
		if value == "" {
			if f.Required {
				return nil, invalid
			}
			delete(result, name)
			continue
		}
		if !f.valid(value) {
			return nil, invalid
		}
		result[name] = value
	}
	for _, f := range fields {
		if f.Required && result[f.Name] == "" {
			return nil, invalid
		}
	}
	return result, nil
}

// NormalizeDomains validates certificate names: DNS names of at least two
// labels, optionally starting with a "*." wildcard label, Unicode converted
// to Punycode, lower case, duplicates removed, at most MaxDomains.
func NormalizeDomains(input []string) ([]string, error) {
	invalid := auth.Failure("INVALID_DOMAIN", 400)
	if len(input) == 0 {
		return nil, invalid
	}
	if len(input) > MaxDomains {
		return nil, auth.Failure("TOO_MANY_DOMAINS", 400)
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(input))
	for _, raw := range input {
		name, ok := normalizeDomain(raw)
		if !ok {
			return nil, invalid
		}
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result, nil
}

func normalizeDomain(raw string) (string, bool) {
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	wildcard := strings.HasPrefix(name, "*.")
	if wildcard {
		name = name[2:]
	}
	if name == "" || len(raw) > 512 || strings.ContainsAny(name, "*_/:@ ") {
		return "", false
	}
	ascii, err := idna.Lookup.ToASCII(name)
	if err != nil || len(ascii) > 253 {
		return "", false
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 {
		return "", false
	}
	for _, label := range labels {
		if !labelPattern.MatchString(label) {
			return "", false
		}
	}
	// A numeric top-level label would make the name an IP address.
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return "", false
	}
	if wildcard {
		ascii = "*." + ascii
		if len(ascii) > 253 {
			return "", false
		}
	}
	return ascii, true
}

// ResolveDirectory returns the ACME directory URL of a request. Custom
// directories must be absolute https:// URLs without credentials or fragments.
func ResolveDirectory(kind, custom string) (string, error) {
	invalid := auth.Failure("INVALID_DIRECTORY", 400)
	switch kind {
	case "", DirectoryLetsEncrypt:
		if custom != "" {
			return "", invalid
		}
		return LetsEncryptProduction, nil
	case DirectoryLetsEncryptStaging:
		if custom != "" {
			return "", invalid
		}
		return LetsEncryptStaging, nil
	case DirectoryCustom:
	default:
		return "", invalid
	}
	custom = strings.TrimSpace(custom)
	parsed, err := url.Parse(custom)
	if err != nil || len(custom) > 2048 || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" || strings.ContainsAny(custom, " \t\r\n") {
		return "", invalid
	}
	return parsed.String(), nil
}

// directoryKind maps a stored directory URL back to its API kind.
func directoryKind(directory string) string {
	switch directory {
	case LetsEncryptProduction:
		return DirectoryLetsEncrypt
	case LetsEncryptStaging:
		return DirectoryLetsEncryptStaging
	}
	return DirectoryCustom
}

// NormalizeEmail accepts an empty contact or a plain address.
func NormalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", nil
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email || len(email) > 254 || !strings.Contains(email[strings.LastIndex(email, "@")+1:], ".") {
		return "", auth.Failure("INVALID_EMAIL", 400)
	}
	return email, nil
}

// EAB holds External Account Binding credentials for custom directories.
type EAB struct {
	KeyID   string `json:"keyId"`
	HMACKey string `json:"hmacKey"`
}

// validEAB accepts no binding, or a key ID with a base64url HMAC key.
func validEAB(kind string, eab EAB) (*EAB, error) {
	eab.KeyID, eab.HMACKey = strings.TrimSpace(eab.KeyID), strings.TrimSpace(eab.HMACKey)
	if eab.KeyID == "" && eab.HMACKey == "" {
		return nil, nil
	}
	invalid := auth.Failure("INVALID_EAB", 400)
	if kind != DirectoryCustom || !eabKeyIDPattern.MatchString(eab.KeyID) || len(eab.HMACKey) > 512 {
		return nil, invalid
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(eab.HMACKey, "="))
	if err != nil || len(key) == 0 {
		return nil, invalid
	}
	return &eab, nil
}
