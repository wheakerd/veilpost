package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Agent request headers. The canonical message below must stay byte-for-byte
// identical to app/agent; both test suites pin the same known-answer vector.
const (
	HeaderNode      = "X-VPN-Node"
	HeaderTimestamp = "X-VPN-Timestamp"
	HeaderNonce     = "X-VPN-Nonce"
	HeaderSignature = "X-VPN-Signature"
)

var (
	nodeIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	noncePattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)
	timestampPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
	tokenPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	versionPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$`)
)

// RequestMessage is the signed text of an agent request: a version tag, the
// method, the route path (without any proxy prefix or query), node ID, Unix
// timestamp in seconds, nonce and the hex SHA-256 of the exact body.
func RequestMessage(method, route, nodeID, timestamp, nonce string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{"vpn-agent-v1", method, route, nodeID, timestamp, nonce, hex.EncodeToString(sum[:])}, "\n"))
}

// EnrollMessage is the proof-of-possession text an agent signs with its new
// key when it redeems an enrollment token.
func EnrollMessage(token, publicKey string) []byte {
	sum := sha256.Sum256([]byte(token))
	return []byte(strings.Join([]string{"vpn-agent-enroll-v1", hex.EncodeToString(sum[:]), publicKey}, "\n"))
}
