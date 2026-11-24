package apps_test

import (
	"encoding/json"
	"github.com/wheakerd/veilpost/app/backend/internal/apps"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedCatalogLoadsInOrderWithPinnedImages(t *testing.T) {
	catalog, err := apps.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, app := range catalog.Apps() {
		ids = append(ids, app.ID)
	}
	if strings.Join(ids, ",") != "xray,hysteria2,amneziawg" {
		t.Fatalf("catalog order: %v", ids)
	}
	want := map[string]string{
		"xray":      "ghcr.io/xtls/xray-core:26.11.9@sha256:9812e382e229e9ed63fd179bf1a4e3fe244c12ff3ae4fd8128745d79c87575ab",
		"hysteria2": "docker.io/tobyxdd/hysteria:v2.14.0@sha256:b15cedc3de200ae7a4f177908a83ef23ed3de2935458b22b7d46a7a8697038a9",
		"amneziawg": "",
	}
	for id, reference := range want {
		manifest, exists := catalog.Get(id)
		if !exists || manifest.Image.Reference() != reference {
			t.Fatalf("%s image: %+v", id, manifest)
		}
	}
	if awg, _ := catalog.Get("amneziawg"); awg.Image.Build != "docker/node/amneziawg" {
		t.Fatalf("amneziawg must be built from this repository: %+v", awg.Image)
	}
	// REALITY borrowed sites have no default and are required.
	xray, _ := catalog.Get("xray")
	for _, setting := range xray.Settings {
		if strings.HasSuffix(setting.Key, "Target") && (!setting.Required || setting.Default != nil) {
			t.Fatalf("%s must be required without a default", setting.Key)
		}
	}
	if _, exists := catalog.Get("unknown"); exists {
		t.Fatal("unknown app found")
	}
}

func manifestJSON(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	manifest := map[string]any{
		"id": "demo", "version": "1.2.3", "name": "apps.catalog.demo.name", "description": "apps.catalog.demo.description",
		"protocols": []any{map[string]any{"id": "demo", "name": "Demo", "transport": "udp"}},
		"ports":     []any{map[string]any{"setting": "port", "protocol": "demo"}},
		"settings": []any{
			map[string]any{"key": "port", "type": "port", "label": "apps.catalog.demo.settings.port.label", "required": true, "default": 443},
			map[string]any{"key": "mode", "type": "enum", "label": "apps.catalog.demo.settings.mode.label", "default": "a",
				"options": []any{map[string]any{"value": "a", "label": "apps.catalog.demo.settings.mode.options.a"},
					map[string]any{"value": "b", "label": "apps.catalog.demo.settings.mode.options.b"}}},
			map[string]any{"key": "secret", "type": "string", "label": "apps.catalog.demo.settings.secret.label",
				"sensitive": true, "minLength": 8, "maxLength": 32, "requiredWhen": map[string]any{"setting": "mode", "equals": "b"}},
		},
		"image": map[string]any{"repository": "docker.io/example/demo", "tag": "1.2.3",
			"digest": "sha256:" + strings.Repeat("a", 64), "platforms": []any{"linux/amd64"}, "source": "https://github.com/example/demo"},
	}
	if change != nil {
		change(manifest)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func setting(manifest map[string]any, index int) map[string]any {
	return manifest["settings"].([]any)[index].(map[string]any)
}

func TestManifestValidationRejectsInvalidCatalogs(t *testing.T) {
	if _, err := apps.LoadCatalogFS(fstest.MapFS{"demo.json": {Data: manifestJSON(t, nil)}}); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	cases := map[string]func(map[string]any){
		"unknown field":         func(m map[string]any) { m["remote"] = "https://example.com/catalog" },
		"bad id":                func(m map[string]any) { m["id"] = "Demo" },
		"bad version":           func(m map[string]any) { m["version"] = "latest" },
		"plain text name":       func(m map[string]any) { m["name"] = "Demo" },
		"foreign key":           func(m map[string]any) { m["description"] = "apps.catalog.other.description" },
		"no protocols":          func(m map[string]any) { m["protocols"] = []any{} },
		"bad transport":         func(m map[string]any) { m["protocols"].([]any)[0].(map[string]any)["transport"] = "sctp" },
		"unknown port setting":  func(m map[string]any) { m["ports"] = []any{map[string]any{"setting": "other", "protocol": "demo"}} },
		"unlisted port setting": func(m map[string]any) { m["ports"] = []any{} },
		"unknown type":          func(m map[string]any) { setting(m, 0)["type"] = "file" },
		"duplicate key":         func(m map[string]any) { setting(m, 1)["key"] = "port" },
		"default out of range":  func(m map[string]any) { setting(m, 0)["default"] = 70000 },
		"default not an option": func(m map[string]any) { setting(m, 1)["default"] = "c" },
		"sensitive enum":        func(m map[string]any) { setting(m, 1)["sensitive"] = true },
		"sensitive default":     func(m map[string]any) { setting(m, 2)["default"] = "default-secret" },
		"unanchored pattern":    func(m map[string]any) { setting(m, 2)["pattern"] = "[a-z]+" },
		"condition unknown": func(m map[string]any) {
			setting(m, 2)["requiredWhen"] = map[string]any{"setting": "nope", "equals": "b"}
		},
		"condition value": func(m map[string]any) {
			setting(m, 2)["requiredWhen"] = map[string]any{"setting": "mode", "equals": "z"}
		},
		"tag without digest": func(m map[string]any) { delete(m["image"].(map[string]any), "digest") },
		"short digest":       func(m map[string]any) { m["image"].(map[string]any)["digest"] = "sha256:abc" },
		"no platforms":       func(m map[string]any) { m["image"].(map[string]any)["platforms"] = []any{} },
		"build and digest":   func(m map[string]any) { m["image"].(map[string]any)["build"] = "docker/node/demo" },
		"build outside nodes": func(m map[string]any) {
			m["image"] = map[string]any{"build": "../demo", "source": "https://github.com/example/demo"}
		},
		"integer without range": func(m map[string]any) {
			m["settings"] = append(m["settings"].([]any), map[string]any{"key": "mtu", "type": "integer", "label": "apps.catalog.demo.settings.mtu.label"})
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := apps.LoadCatalogFS(fstest.MapFS{"demo.json": {Data: manifestJSON(t, change)}}); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	for name, files := range map[string]fstest.MapFS{
		"empty":             {},
		"file name differs": {"other.json": {Data: manifestJSON(t, nil)}},
		"trailing data":     {"demo.json": {Data: append(manifestJSON(t, nil), []byte(" {}")...)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := apps.LoadCatalogFS(fs.FS(files)); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestSettingValuesAreStrictlyValidated(t *testing.T) {
	catalog, err := apps.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	find := func(app, key string) *apps.Setting {
		manifest, _ := catalog.Get(app)
		for i := range manifest.Settings {
			if manifest.Settings[i].Key == key {
				return &manifest.Settings[i]
			}
		}
		t.Fatalf("no setting %s.%s", app, key)
		return nil
	}
	cases := []struct {
		app, key, input string
		want            any
	}{
		{"xray", "visionPort", `8443`, int64(8443)},
		{"xray", "visionPort", `0`, nil},
		{"xray", "visionPort", `65536`, nil},
		{"xray", "visionPort", `443.5`, nil},
		{"xray", "visionPort", `"443"`, nil},
		{"xray", "visionTarget", `"WWW.Example.com"`, "www.example.com:443"},
		{"xray", "visionTarget", `"www.example.com:8443"`, "www.example.com:8443"},
		{"xray", "visionTarget", `"www.example.com:0443"`, nil},
		{"xray", "visionTarget", `"203.0.113.10:443"`, nil},
		{"xray", "visionTarget", `"localhost"`, nil},
		{"xray", "visionTarget", `"-bad.example.com"`, nil},
		{"xray", "visionServerName", `"www.example.com"`, "www.example.com"},
		{"xray", "visionServerName", `"www.example.com:443"`, nil},
		{"xray", "xhttpPath", `"/api/v2"`, "/api/v2"},
		{"xray", "xhttpPath", `"api"`, nil},
		{"xray", "xhttpPath", `"/a//b"`, nil},
		{"xray", "xhttpPath", `"/a?b"`, nil},
		{"xray", "xhttpMode", `"stream-up"`, "stream-up"},
		{"xray", "xhttpMode", `"auto"`, nil},
		{"hysteria2", "masqueradeUrl", `"https://www.example.com/path"`, "https://www.example.com/path"},
		{"hysteria2", "masqueradeUrl", `"https://www.example.com:8443"`, "https://www.example.com:8443"},
		{"hysteria2", "masqueradeUrl", `"http://www.example.com"`, nil},
		{"hysteria2", "masqueradeUrl", `"https://user:pass@www.example.com"`, nil},
		{"hysteria2", "masqueradeUrl", `"https://www.example.com/?q=1"`, nil},
		{"hysteria2", "masqueradeUrl", `"https://www.example.com/#top"`, nil},
		{"hysteria2", "masqueradeUrl", `"https://127.0.0.1/"`, nil},
		{"hysteria2", "salamanderPassword", `"0123456789abcdef"`, "0123456789abcdef"},
		{"hysteria2", "salamanderPassword", `"short"`, nil},
		{"hysteria2", "salamanderPassword", `"line\nbreak-0123456789"`, nil},
		{"amneziawg", "mtu", `1420`, int64(1420)},
		{"amneziawg", "mtu", `1279`, nil},
		{"amneziawg", "mtu", `1421`, nil},
		{"amneziawg", "requireIpv6", `true`, true},
		{"amneziawg", "requireIpv6", `"true"`, nil},
	}
	for _, test := range cases {
		got, err := find(test.app, test.key).Parse(json.RawMessage(test.input))
		if test.want == nil {
			if err == nil {
				t.Errorf("%s.%s accepted %s as %v", test.app, test.key, test.input, got)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("%s.%s %s: got %v (%v), want %v", test.app, test.key, test.input, got, err, test.want)
		}
	}
	if value, err := find("xray", "visionPort").Parse(json.RawMessage(`null`)); err != nil || value != nil {
		t.Fatalf("null must mean unset: %v %v", value, err)
	}
}
