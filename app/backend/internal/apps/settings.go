package apps

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SettingError reports a rejected configuration field. Field is a manifest
// key (never a value) or empty for keys the manifest does not define.
type SettingError struct {
	Code  string
	Field string
}

func (e *SettingError) Error() string { return e.Code }

func invalidSetting(key string) error  { return &SettingError{Code: "INVALID_SETTING", Field: key} }
func requiredSetting(key string) error { return &SettingError{Code: "SETTING_REQUIRED", Field: key} }

var errUnknownSetting = &SettingError{Code: "UNKNOWN_SETTING"}

// parse validates one JSON value against the setting and returns its
// normalized form, or nil for JSON null.
func (s *Setting) parse(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("invalid JSON")
	}
	if value == nil {
		return nil, nil
	}
	switch s.Type {
	case TypePort:
		return integer(value, 1, 65535)
	case TypeInteger:
		return integer(value, *s.Min, *s.Max)
	case TypeBoolean:
		if flag, ok := value.(bool); ok {
			return flag, nil
		}
		return nil, errors.New("not a boolean")
	}
	text, ok := value.(string)
	if !ok || !utf8.ValidString(text) || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return nil, errors.New("not a string")
	}
	switch s.Type {
	case TypeString:
		length := utf8.RuneCountInString(text)
		if length < s.MinLength || length > s.MaxLength || (s.pattern != nil && !s.pattern.MatchString(text)) {
			return nil, errors.New("out of range")
		}
		return text, nil
	case TypeEnum:
		if s.hasOption(text) {
			return text, nil
		}
	case TypeHostname:
		if name, ok := hostname(text); ok {
			return name, nil
		}
	case TypeHostPort:
		return hostPort(text)
	case TypeURL:
		return httpsURL(text)
	}
	return nil, errors.New("invalid value")
}

func integer(value any, minimum, maximum int64) (int64, error) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, errors.New("not a number")
	}
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New("out of range")
	}
	return parsed, nil
}

// hostname accepts a public DNS name with at least two labels and a
// non-numeric top-level label (so IP addresses are rejected) and returns it
// in lower case.
func hostname(text string) (string, bool) {
	name := strings.ToLower(text)
	if len(name) > 253 {
		return "", false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return "", false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return "", false
			}
		}
	}
	top := labels[len(labels)-1]
	if strings.Trim(top, "0123456789") == "" {
		return "", false
	}
	return name, true
}

// hostPort accepts "name" or "name:port" and returns "name:port", using 443
// when the port is omitted.
func hostPort(text string) (string, error) {
	host, port := text, "443"
	if strings.Contains(text, ":") {
		var err error
		if host, port, err = net.SplitHostPort(text); err != nil {
			return "", err
		}
	}
	name, ok := hostname(host)
	if !ok {
		return "", errors.New("invalid host")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return "", errors.New("invalid port")
	}
	return name + ":" + port, nil
}

// httpsURL accepts an https URL with a DNS host, an optional port and path,
// and no credentials, query or fragment.
func httpsURL(text string) (string, error) {
	if len(text) > 2048 {
		return "", errors.New("too long")
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(text, "#") {
		return "", errors.New("invalid URL")
	}
	name, ok := hostname(parsed.Hostname())
	if !ok {
		return "", errors.New("invalid host")
	}
	host := name
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
			return "", errors.New("invalid port")
		}
		host = net.JoinHostPort(name, port)
	} else if strings.HasSuffix(parsed.Host, ":") {
		return "", errors.New("empty port")
	}
	normalized := url.URL{Scheme: "https", Host: host, Path: parsed.Path, RawPath: parsed.RawPath}
	return normalized.String(), nil
}

// stored is the JSON kept in installed_apps.settings. Values holds
// non-sensitive settings; Secrets holds AES-GCM ciphertexts sealed with
// master.key and bound to the app id and setting key.
type stored struct {
	Values  map[string]json.RawMessage `json:"values"`
	Secrets map[string][]byte          `json:"secrets"`
}

func decodeStored(text string) (stored, error) {
	var settings stored
	if err := json.Unmarshal([]byte(text), &settings); err != nil {
		return stored{}, fmt.Errorf("stored app settings are corrupt")
	}
	if settings.Values == nil {
		settings.Values = map[string]json.RawMessage{}
	}
	if settings.Secrets == nil {
		settings.Secrets = map[string][]byte{}
	}
	return settings, nil
}

func (s stored) encode() (string, error) {
	data, err := json.Marshal(s)
	return string(data), err
}

// defaults returns the stored form of a fresh installation: every default
// and no secrets.
func (m *Manifest) defaults() stored {
	settings := stored{Values: map[string]json.RawMessage{}, Secrets: map[string][]byte{}}
	for _, setting := range m.Settings {
		if setting.defaultValue != nil {
			settings.Values[setting.Key] = setting.Default
		}
	}
	return settings
}

// values returns every non-sensitive setting, re-validated against the
// manifest; values a newer manifest no longer accepts read as unset (nil).
func (m *Manifest) values(settings stored) map[string]any {
	values := map[string]any{}
	for i := range m.Settings {
		setting := &m.Settings[i]
		if setting.Sensitive {
			continue
		}
		var value any
		if raw, exists := settings.Values[setting.Key]; exists {
			value, _ = setting.parse(raw)
		}
		values[setting.Key] = value
	}
	return values
}

// missing returns the first required setting that has no value.
func (m *Manifest) missing(values map[string]any, secrets map[string][]byte) string {
	for _, setting := range m.Settings {
		required := setting.Required
		if condition := setting.RequiredWhen; condition != nil {
			required = values[condition.Setting] == condition.Equals
		}
		if !required {
			continue
		}
		if setting.Sensitive {
			if len(secrets[setting.Key]) == 0 {
				return setting.Key
			}
		} else if values[setting.Key] == nil {
			return setting.Key
		}
	}
	return ""
}

// Update is a settings change. Values replaces every non-sensitive setting:
// omitted or null values fall back to the default or become unset. Secrets
// are write-only: an omitted key keeps the stored value, a string replaces
// it and null clears it.
type Update struct {
	Values  map[string]json.RawMessage `json:"values"`
	Secrets map[string]*string         `json:"secrets"`
}

// apply validates an update and returns the new stored settings. seal
// encrypts a sensitive value for the given setting key.
func (m *Manifest) apply(current stored, update Update, seal func(key, value string) ([]byte, error)) (stored, error) {
	settings := map[string]*Setting{}
	for i := range m.Settings {
		settings[m.Settings[i].Key] = &m.Settings[i]
	}
	for key := range update.Values {
		if setting := settings[key]; setting == nil || setting.Sensitive {
			return stored{}, errUnknownSetting
		}
	}
	for key := range update.Secrets {
		if setting := settings[key]; setting == nil || !setting.Sensitive {
			return stored{}, errUnknownSetting
		}
	}
	next := stored{Values: map[string]json.RawMessage{}, Secrets: map[string][]byte{}}
	values := map[string]any{}
	for i := range m.Settings {
		setting := &m.Settings[i]
		if setting.Sensitive {
			secret, given := update.Secrets[setting.Key]
			switch {
			case !given:
				if sealed := current.Secrets[setting.Key]; len(sealed) > 0 {
					next.Secrets[setting.Key] = sealed
				}
			case secret != nil:
				raw, _ := json.Marshal(*secret)
				if _, err := setting.parse(raw); err != nil {
					return stored{}, invalidSetting(setting.Key)
				}
				sealed, err := seal(setting.Key, *secret)
				if err != nil {
					return stored{}, err
				}
				next.Secrets[setting.Key] = sealed
			}
			continue
		}
		var value any
		if raw, given := update.Values[setting.Key]; given {
			parsed, err := setting.parse(raw)
			if err != nil {
				return stored{}, invalidSetting(setting.Key)
			}
			value = parsed
		}
		if value == nil {
			value = setting.defaultValue
		}
		if value == nil {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return stored{}, err
		}
		next.Values[setting.Key] = encoded
		values[setting.Key] = value
	}
	if key := m.missing(values, next.Secrets); key != "" {
		return stored{}, requiredSetting(key)
	}
	return next, nil
}
