package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type EncryptedValue struct {
	KeyID      string `json:"keyId" bson:"keyId"`
	Algorithm  string `json:"algorithm" bson:"algorithm"`
	Nonce      string `json:"nonce" bson:"nonce"`
	Ciphertext string `json:"ciphertext" bson:"ciphertext"`
}

type EnvelopeCipher struct {
	KeyID string
	key   []byte
}

func NewEnvelopeCipher(keyID string, key []byte) (*EnvelopeCipher, error) {
	if keyID == "" {
		return nil, errors.New("encryption key id is required")
	}
	if len(key) != 32 {
		return nil, errors.New("AES-256 key must contain 32 bytes")
	}
	copyKey := append([]byte(nil), key...)
	return &EnvelopeCipher{KeyID: keyID, key: copyKey}, nil
}

func (c *EnvelopeCipher) Encrypt(plaintext, associatedData []byte) (EncryptedValue, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return EncryptedValue{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedValue{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedValue{}, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, associatedData)
	return EncryptedValue{KeyID: c.KeyID, Algorithm: "AES-256-GCM", Nonce: base64.RawStdEncoding.EncodeToString(nonce), Ciphertext: base64.RawStdEncoding.EncodeToString(sealed)}, nil
}

func (c *EnvelopeCipher) Decrypt(value EncryptedValue, associatedData []byte) ([]byte, error) {
	if value.KeyID != c.KeyID || value.Algorithm != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported encryption key or algorithm")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(value.Nonce)
	if err != nil {
		return nil, errors.New("invalid encrypted nonce")
	}
	sealed, err := base64.RawStdEncoding.DecodeString(value.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid encrypted value")
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, sealed, associatedData)
	if err != nil {
		return nil, errors.New("encrypted value authentication failed")
	}
	return plaintext, nil
}
