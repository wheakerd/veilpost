package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"regexp"
)

// Seal and Unseal let other console packages keep secrets encrypted with
// master.key (AES-256-GCM, random nonce). The purpose is bound as additional
// data, so a ciphertext cannot be moved to another record or field; it must
// identify both, for example "apps:hysteria2:salamanderPassword". TOTP
// secrets use their own additional data and are not readable through Unseal.

var sealPurpose = regexp.MustCompile(`^[a-z][a-z0-9-]*(:[A-Za-z0-9_-]+)+$`)

func sealData(purpose string) ([]byte, error) {
	if !sealPurpose.MatchString(purpose) || len(purpose) > 256 {
		return nil, errors.New("invalid seal purpose")
	}
	// The label predates the Veilpost name; it is bound into every stored
	// ciphertext, so changing it would make existing secrets unreadable.
	return []byte("vpn-console:seal:v1:" + purpose), nil
}

func (s *Service) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts value for purpose.
func (s *Service) Seal(purpose string, value []byte) ([]byte, error) {
	data, err := sealData(purpose)
	if err != nil {
		return nil, err
	}
	gcm, err := s.gcm()
	if err != nil {
		return nil, err
	}
	nonce := randomBytes(gcm.NonceSize())
	return gcm.Seal(nonce, nonce, value, data), nil
}

// Unseal decrypts a value sealed for the same purpose.
func (s *Service) Unseal(purpose string, sealed []byte) ([]byte, error) {
	data, err := sealData(purpose)
	if err != nil {
		return nil, err
	}
	gcm, err := s.gcm()
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("invalid sealed value")
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], data)
}
