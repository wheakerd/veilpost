package agent

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

const (
	keyFile    = "identity.key"
	configFile = "node.json"
)

var nodeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

// Config binds this agent to one node of one console. It holds no secrets;
// the private key lives only in identity.key.
type Config struct {
	Server string `json:"server"`
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
}

// prepareDir creates the state directory with mode 0700, or checks that an
// existing one is not accessible by group or others. It never loosens or
// silently changes the permissions of an existing directory.
func prepareDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("state path %s is not a directory", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("state directory %s must not be accessible by group or others (chmod 700)", dir)
	}
	return nil
}

// writeExclusive creates a new 0600 file and syncs it; it never overwrites.
func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return err
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}

// privateFile reads a regular file that only its owner can access.
func privateFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s must not be accessible by group or others (chmod 600)", path)
	}
	return os.ReadFile(path)
}

func encodeKey(key ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func loadKey(dir string) (ed25519.PrivateKey, error) {
	data, err := privateFile(filepath.Join(dir, keyFile))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("identity key is not a PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("identity key cannot be parsed")
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("identity key is not an Ed25519 key")
	}
	return key, nil
}

// Load reads the enrolled node configuration and private key.
func Load(dir string) (Config, ed25519.PrivateKey, error) {
	if err := prepareDir(dir); err != nil {
		return Config{}, nil, err
	}
	data, err := privateFile(filepath.Join(dir, configFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil, fmt.Errorf("this machine is not enrolled; run veilpost-agent enroll first")
	}
	if err != nil {
		return Config{}, nil, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil || !nodeIDPattern.MatchString(config.NodeID) {
		return Config{}, nil, fmt.Errorf("node configuration is invalid")
	}
	if _, err := ServerURL(config.Server); err != nil {
		return Config{}, nil, err
	}
	key, err := loadKey(dir)
	if err != nil {
		return Config{}, nil, err
	}
	return config, key, nil
}
