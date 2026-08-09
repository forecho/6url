package securepayload

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Sealer struct {
	aead cipher.AEAD
}

func New(base64Key string) (Sealer, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil || len(key) != 32 {
		return Sealer{}, errors.New("encryption key must be base64-encoded 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return Sealer{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Sealer{}, err
	}
	return Sealer{aead: aead}, nil
}

func (s Sealer) Seal(value any, associatedData string) (string, error) {
	if s.aead == nil {
		return "", errors.New("payload sealer is not configured")
	}
	plaintext, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := s.aead.Seal(nil, nonce, plaintext, []byte(associatedData))
	return base64.RawStdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s Sealer) Open(encoded, associatedData string, target any) error {
	if s.aead == nil {
		return errors.New("payload sealer is not configured")
	}
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(payload) < s.aead.NonceSize() {
		return errors.New("invalid encrypted payload")
	}
	nonce, ciphertext := payload[:s.aead.NonceSize()], payload[s.aead.NonceSize():]
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, []byte(associatedData))
	if err != nil {
		return errors.New("decrypt payload")
	}
	if err := json.Unmarshal(plaintext, target); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	return nil
}
