package apps

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// The built-in catalog ships with the console binary. New software arrives
// only with a console upgrade; there is no remote source or upload.
//
//go:embed catalog/*.json
var catalogFiles embed.FS

// Setting types accepted in manifests. Only string settings can be sensitive.
const (
	TypePort     = "port"
	TypeInteger  = "integer"
	TypeString   = "string"
	TypeEnum     = "enum"
	TypeBoolean  = "boolean"
	TypeHostname = "hostname"
	TypeHostPort = "hostport"
	TypeURL      = "url"
)

// Manifest describes one catalog entry. Name, description and every label
// are translation keys under apps.catalog.<id> in the frontend language
// packs; the backend never returns interface text.
type Manifest struct {
	ID          string     `json:"id"`
	Version     string     `json:"version"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Protocols   []Protocol `json:"protocols"`
	Ports       []Port     `json:"ports"`
	Settings    []Setting  `json:"settings"`
	Image       Image      `json:"image"`
}

// Protocol is an entry protocol the software provides. Names are proper
// nouns and are not translated.
type Protocol struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
}

// Port links a port setting to the protocol listening on it.
type Port struct {
	Setting  string `json:"setting"`
	Protocol string `json:"protocol"`
}

// Setting defines one configuration field. A field without a default and
// without required/requiredWhen may stay unset.
type Setting struct {
	Key          string          `json:"key"`
	Type         string          `json:"type"`
	Label        string          `json:"label"`
	Hint         string          `json:"hint,omitempty"`
	Required     bool            `json:"required,omitempty"`
	RequiredWhen *Condition      `json:"requiredWhen,omitempty"`
	Default      json.RawMessage `json:"default,omitempty"`
	Min          *int64          `json:"min,omitempty"`
	Max          *int64          `json:"max,omitempty"`
	MinLength    int             `json:"minLength,omitempty"`
	MaxLength    int             `json:"maxLength,omitempty"`
	Pattern      string          `json:"pattern,omitempty"`
	Options      []Option        `json:"options,omitempty"`
	Sensitive    bool            `json:"sensitive,omitempty"`

	pattern      *regexp.Regexp
	defaultValue any
}

// Condition makes a setting required while another enum setting has a value.
type Condition struct {
	Setting string `json:"setting"`
	Equals  string `json:"equals"`
}

// Option is one allowed value of an enum setting.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Image pins a published image by its multi-architecture index digest, or
// names a directory of this repository for images that are built locally
// and have no published address yet.
type Image struct {
	Repository string   `json:"repository,omitempty"`
	Tag        string   `json:"tag,omitempty"`
	Digest     string   `json:"digest,omitempty"`
	Platforms  []string `json:"platforms,omitempty"`
	Build      string   `json:"build,omitempty"`
	Source     string   `json:"source"`
}

// Reference is the pull reference pinned by digest, or empty for a local build.
func (i Image) Reference() string {
	if i.Build != "" {
		return ""
	}
	return i.Repository + ":" + i.Tag + "@" + i.Digest
}

// Catalog is the validated, ordered set of manifests.
type Catalog struct {
	apps []Manifest
	byID map[string]*Manifest
}

// Apps returns the manifests in catalog order.
func (c *Catalog) Apps() []Manifest { return c.apps }

// Get returns the manifest of an app id.
func (c *Catalog) Get(id string) (*Manifest, bool) {
	manifest, exists := c.byID[id]
	return manifest, exists
}

// catalogOrder lists the first-version entries in display order; any other
// manifest sorts after them by id.
var catalogOrder = map[string]int{"xray": 0, "hysteria2": 1, "amneziawg": 2}

// LoadCatalog reads and validates the embedded manifests.
func LoadCatalog() (*Catalog, error) {
	sub, err := fs.Sub(catalogFiles, "catalog")
	if err != nil {
		return nil, err
	}
	return loadCatalog(sub)
}

func loadCatalog(files fs.FS) (*Catalog, error) {
	names, err := fs.Glob(files, "*.json")
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("app catalog is empty")
	}
	catalog := &Catalog{byID: map[string]*Manifest{}}
	for _, name := range names {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		manifest, err := parseManifest(data)
		if err != nil {
			return nil, fmt.Errorf("app manifest %s: %w", name, err)
		}
		if manifest.ID+".json" != path.Base(name) {
			return nil, fmt.Errorf("app manifest %s: file name must be the app id", name)
		}
		catalog.apps = append(catalog.apps, manifest)
	}
	sort.SliceStable(catalog.apps, func(i, j int) bool {
		a, aKnown := catalogOrder[catalog.apps[i].ID]
		b, bKnown := catalogOrder[catalog.apps[j].ID]
		if aKnown != bKnown {
			return aKnown
		}
		if aKnown {
			return a < b
		}
		return catalog.apps[i].ID < catalog.apps[j].ID
	})
	for i := range catalog.apps {
		catalog.byID[catalog.apps[i].ID] = &catalog.apps[i]
	}
	return catalog, nil
}

var (
	appIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	versionPattern    = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}$`)
	settingKeyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,31}$`)
	protocolIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	keySegmentPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	digestPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	repositoryPattern = regexp.MustCompile(`^[a-z0-9]+([.-][a-z0-9]+)*(:[0-9]+)?(/[a-z0-9]+([._-][a-z0-9]+)*)+$`)
	tagPattern        = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	platformPattern   = regexp.MustCompile(`^linux/[a-z0-9]+(/v[0-9]+)?$`)
	buildPattern      = regexp.MustCompile(`^docker/node/[a-z0-9][a-z0-9-]*$`)
)

// translationKey accepts keys below apps.catalog.<id>.
func translationKey(key, id string) bool {
	rest, found := strings.CutPrefix(key, "apps.catalog."+id+".")
	if !found || rest == "" {
		return false
	}
	for _, segment := range strings.Split(rest, ".") {
		if !keySegmentPattern.MatchString(segment) {
			return false
		}
	}
	return true
}

func parseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Manifest{}, fmt.Errorf("trailing data")
	}
	return manifest, manifest.validate()
}

func (m *Manifest) validate() error {
	if !appIDPattern.MatchString(m.ID) {
		return fmt.Errorf("invalid id")
	}
	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("invalid version")
	}
	if !translationKey(m.Name, m.ID) || !translationKey(m.Description, m.ID) {
		return fmt.Errorf("name and description must be translation keys under apps.catalog.%s", m.ID)
	}
	if len(m.Protocols) == 0 {
		return fmt.Errorf("no protocols")
	}
	protocols := map[string]bool{}
	for _, protocol := range m.Protocols {
		if !protocolIDPattern.MatchString(protocol.ID) || protocols[protocol.ID] || strings.TrimSpace(protocol.Name) == "" ||
			(protocol.Transport != "tcp" && protocol.Transport != "udp") {
			return fmt.Errorf("invalid protocol %q", protocol.ID)
		}
		protocols[protocol.ID] = true
	}
	settings := map[string]*Setting{}
	for i := range m.Settings {
		setting := &m.Settings[i]
		if !settingKeyPattern.MatchString(setting.Key) || settings[setting.Key] != nil {
			return fmt.Errorf("invalid or duplicate setting key %q", setting.Key)
		}
		if err := setting.validate(m.ID, settings); err != nil {
			return fmt.Errorf("setting %s: %w", setting.Key, err)
		}
		settings[setting.Key] = setting
	}
	ports := map[string]bool{}
	for _, port := range m.Ports {
		setting := settings[port.Setting]
		if setting == nil || setting.Type != TypePort || !protocols[port.Protocol] || ports[port.Setting] {
			return fmt.Errorf("invalid port %q", port.Setting)
		}
		ports[port.Setting] = true
	}
	for _, setting := range m.Settings {
		if setting.Type == TypePort && !ports[setting.Key] {
			return fmt.Errorf("port setting %s is not listed in ports", setting.Key)
		}
	}
	return m.Image.validate()
}

func (s *Setting) validate(id string, earlier map[string]*Setting) error {
	if !translationKey(s.Label, id) || (s.Hint != "" && !translationKey(s.Hint, id)) {
		return fmt.Errorf("labels must be translation keys under apps.catalog.%s", id)
	}
	numeric := s.Type == TypeInteger
	text := s.Type == TypeString
	switch s.Type {
	case TypePort, TypeInteger, TypeString, TypeEnum, TypeBoolean, TypeHostname, TypeHostPort, TypeURL:
	default:
		return fmt.Errorf("unknown type %q", s.Type)
	}
	if (s.Min != nil || s.Max != nil) && !numeric {
		return fmt.Errorf("min and max apply only to integers")
	}
	if numeric && (s.Min == nil || s.Max == nil || *s.Min > *s.Max) {
		return fmt.Errorf("integers need min <= max")
	}
	if (s.MinLength != 0 || s.MaxLength != 0 || s.Pattern != "") && !text {
		return fmt.Errorf("length and pattern apply only to strings")
	}
	if text {
		if s.MinLength < 1 || s.MaxLength < s.MinLength || s.MaxLength > 1024 {
			return fmt.Errorf("strings need 1 <= minLength <= maxLength <= 1024")
		}
		if s.Pattern != "" {
			if !strings.HasPrefix(s.Pattern, "^") || !strings.HasSuffix(s.Pattern, "$") {
				return fmt.Errorf("pattern must be anchored")
			}
			pattern, err := regexp.Compile(s.Pattern)
			if err != nil {
				return err
			}
			s.pattern = pattern
		}
	}
	if s.Sensitive && (!text || s.Default != nil) {
		return fmt.Errorf("only string settings without a default can be sensitive")
	}
	if (s.Type == TypeEnum) != (len(s.Options) > 0) {
		return fmt.Errorf("options apply only to, and are required for, enums")
	}
	values := map[string]bool{}
	for _, option := range s.Options {
		if option.Value == "" || len(option.Value) > 64 || values[option.Value] || !translationKey(option.Label, id) {
			return fmt.Errorf("invalid option %q", option.Value)
		}
		values[option.Value] = true
	}
	if s.Required && s.RequiredWhen != nil {
		return fmt.Errorf("required and requiredWhen are exclusive")
	}
	if condition := s.RequiredWhen; condition != nil {
		// Conditions refer to an earlier enum with a default, so they are
		// always decidable.
		other := earlier[condition.Setting]
		if other == nil || other.Type != TypeEnum || other.defaultValue == nil || !other.hasOption(condition.Equals) {
			return fmt.Errorf("requiredWhen must name an earlier enum setting with a default and one of its values")
		}
	}
	if s.Default != nil {
		value, err := s.parse(s.Default)
		if err != nil || value == nil {
			return fmt.Errorf("invalid default")
		}
		s.defaultValue = value
	}
	return nil
}

func (s *Setting) hasOption(value string) bool {
	for _, option := range s.Options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func (i Image) validate() error {
	if !strings.HasPrefix(i.Source, "https://github.com/") {
		return fmt.Errorf("image source must be the upstream repository")
	}
	if i.Build != "" {
		if i.Repository != "" || i.Tag != "" || i.Digest != "" || len(i.Platforms) != 0 || !buildPattern.MatchString(i.Build) {
			return fmt.Errorf("a locally built image names only its build directory")
		}
		return nil
	}
	if !repositoryPattern.MatchString(i.Repository) || !tagPattern.MatchString(i.Tag) || !digestPattern.MatchString(i.Digest) || len(i.Platforms) == 0 {
		return fmt.Errorf("published images need repository, tag, index digest and platforms")
	}
	seen := map[string]bool{}
	for _, platform := range i.Platforms {
		if !platformPattern.MatchString(platform) || seen[platform] {
			return fmt.Errorf("invalid platform %q", platform)
		}
		seen[platform] = true
	}
	return nil
}
