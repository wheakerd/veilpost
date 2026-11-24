// Package agent implements the veilpost-agent control channel of design phase 1:
// enrollment with a one-time token, Ed25519-signed requests and long
// polling. It manages no containers and applies no VPN configuration.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Request headers and the canonical message must match
// app/backend/internal/nodes/signature.go; both test suites pin one vector.
const (
	headerNode      = "X-VPN-Node"
	headerTimestamp = "X-VPN-Timestamp"
	headerNonce     = "X-VPN-Nonce"
	headerSignature = "X-VPN-Signature"
	enrollRoute     = "/api/agent/enroll"
	pollRoute       = "/api/agent/poll"
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func requestMessage(method, route, nodeID, timestamp, nonce string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{"vpn-agent-v1", method, route, nodeID, timestamp, nonce, hex.EncodeToString(sum[:])}, "\n"))
}

func enrollMessage(token, publicKey string) []byte {
	sum := sha256.Sum256([]byte(token))
	return []byte(strings.Join([]string{"vpn-agent-enroll-v1", hex.EncodeToString(sum[:]), publicKey}, "\n"))
}

// ServerURL validates a console base URL. HTTPS is required except for
// loopback addresses used in local development.
func ServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", fmt.Errorf("server must be a URL such as https://console.example")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", fmt.Errorf("server must use https (plain http is allowed only for loopback addresses)")
	}
	return parsed.String(), nil
}

// ReadToken reads one enrollment token line from standard input.
func ReadToken(input io.Reader) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(input, 257)).ReadString('\n')
	if err != nil && (line == "" || len(line) > 256) {
		return "", fmt.Errorf("an enrollment token is required on standard input")
	}
	token := strings.TrimSpace(line)
	if !tokenPattern.MatchString(token) {
		return "", fmt.Errorf("the enrollment token is malformed; copy it again from the console")
	}
	return token, nil
}

// APIError is a non-2xx console answer with its stable code.
type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string { return fmt.Sprintf("console answered %d %s", e.Status, e.Code) }

// ErrRevoked means the console revoked this node; enrolling again needs a
// new node and a new token.
var ErrRevoked = errors.New("this node was revoked in the console; create a new node to enroll again")

func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		// Never follow redirects: a signed request must reach the console itself.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func exchange(ctx context.Context, client *http.Client, request *http.Request, target any) error {
	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		var failure struct {
			Code string `json:"code"`
		}
		json.Unmarshal(data, &failure)
		if failure.Code == "" {
			failure.Code = "UNEXPECTED_RESPONSE"
		}
		return &APIError{Status: response.StatusCode, Code: failure.Code}
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") || json.Unmarshal(data, target) != nil {
		return &APIError{Status: response.StatusCode, Code: "UNEXPECTED_RESPONSE"}
	}
	return nil
}

// Enroll generates a new Ed25519 key, stores it (0600, directory 0700) and
// redeems the one-time token. The key is written before the token is used
// so a storage failure cannot waste the token; it is removed again when
// enrollment fails. Existing identities are never overwritten.
func Enroll(ctx context.Context, dir, server, token, version string) (Config, error) {
	server, err := ServerURL(server)
	if err != nil {
		return Config{}, err
	}
	if !tokenPattern.MatchString(token) {
		return Config{}, fmt.Errorf("the enrollment token is malformed")
	}
	if err := prepareDir(dir); err != nil {
		return Config{}, err
	}
	for _, name := range []string{keyFile, configFile} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("%s already holds an agent identity; remove the state directory to enroll again", dir)
		}
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Config{}, err
	}
	encoded, err := encodeKey(private)
	if err != nil {
		return Config{}, err
	}
	keyPath := filepath.Join(dir, keyFile)
	if err := writeExclusive(keyPath, encoded); err != nil {
		return Config{}, err
	}
	config, err := redeem(ctx, server, token, version, public, private)
	if err == nil {
		var data []byte
		data, err = json.MarshalIndent(config, "", "  ")
		if err == nil {
			err = writeExclusive(filepath.Join(dir, configFile), append(data, '\n'))
		}
	}
	if err != nil {
		os.Remove(keyPath)
		return Config{}, err
	}
	return config, nil
}

func redeem(ctx context.Context, server, token, version string, public ed25519.PublicKey, private ed25519.PrivateKey) (Config, error) {
	publicKey := base64.RawURLEncoding.EncodeToString(public)
	body, err := json.Marshal(map[string]string{
		"token": token, "publicKey": publicKey, "agentVersion": version,
		"proof": base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, enrollMessage(token, publicKey))),
	})
	if err != nil {
		return Config{}, err
	}
	request, err := http.NewRequest("POST", server+enrollRoute, bytes.NewReader(body))
	if err != nil {
		return Config{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	var result struct {
		NodeID string `json:"nodeId"`
		Name   string `json:"name"`
	}
	if err := exchange(ctx, newClient(30*time.Second), request, &result); err != nil {
		return Config{}, err
	}
	if !nodeIDPattern.MatchString(result.NodeID) {
		return Config{}, &APIError{Status: 200, Code: "UNEXPECTED_RESPONSE"}
	}
	return Config{Server: server, NodeID: result.NodeID, Name: result.Name}, nil
}

// signed builds a request carrying the node ID, Unix timestamp, a random
// nonce and an Ed25519 signature over requestMessage.
func signed(server, route, nodeID string, key ed25519.PrivateKey, now time.Time, body []byte) (*http.Request, error) {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	encodedNonce := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	request, err := http.NewRequest("POST", server+route, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set(headerNode, nodeID)
	request.Header.Set(headerTimestamp, timestamp)
	request.Header.Set(headerNonce, encodedNonce)
	request.Header.Set(headerSignature, base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(key, requestMessage("POST", route, nodeID, timestamp, encodedNonce, body))))
	return request, nil
}
