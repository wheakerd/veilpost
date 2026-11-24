package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/argon2"
)

func randomToken(size int) string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(size))
}

func randomBytes(size int) []byte {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic("secure randomness unavailable")
	}
	return data
}

func digest(value string) string {
	result := sha256.Sum256([]byte(value))
	return hex.EncodeToString(result[:])
}

func hashPassword(password string) string {
	salt := randomBytes(16)
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=2" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func encrypt(key []byte, value string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := randomBytes(gcm.NonceSize())
	// Legacy label from before the Veilpost name; stored TOTP secrets are
	// bound to it, so it must not change.
	return gcm.Seal(nonce, nonce, []byte(value), []byte("vpn-console:totp:v1")), nil
}

func decrypt(key, value []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(value) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	plain, err := gcm.Open(nil, value[:gcm.NonceSize()], value[gcm.NonceSize():], []byte("vpn-console:totp:v1"))
	return string(plain), err
}

func totpCounter(secret, code string, now time.Time, last int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	if _, err := strconv.ParseUint(code, 10, 32); err != nil {
		return 0, false
	}
	step := now.Unix() / 30
	for counter := step - 1; counter <= step+1; counter++ {
		if counter <= last {
			continue
		}
		expected, err := totp.GenerateCode(secret, time.Unix(counter*30, 0))
		if err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return counter, true
		}
	}
	return 0, false
}
