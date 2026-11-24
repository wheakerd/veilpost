package agent

import "time"

// Wire names shared with the console (app/backend/internal/nodes).
const (
	HeaderNode      = headerNode
	HeaderTimestamp = headerTimestamp
	HeaderNonce     = headerNonce
	HeaderSignature = headerSignature
	EnrollRoute     = enrollRoute
	PollRoute       = pollRoute
)

// State file names inside the agent's state directory.
const (
	KeyFile    = keyFile
	ConfigFile = configFile
)

// RequestMessage is the signed text of an agent request.
func RequestMessage(method, route, nodeID, timestamp, nonce string, body []byte) []byte {
	return requestMessage(method, route, nodeID, timestamp, nonce, body)
}

// EnrollMessage is the proof-of-possession text signed at enrollment.
func EnrollMessage(token, publicKey string) []byte { return enrollMessage(token, publicKey) }

// Backoff is the wait before the next poll after failures.
func Backoff(failures int) time.Duration { return backoff(failures) }
