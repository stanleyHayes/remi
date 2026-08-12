package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

type CursorCodec struct{ key []byte }

func NewCursorCodec(key []byte) (*CursorCodec, error) {
	if len(key) < 32 {
		return nil, errors.New("cursor signing key must contain at least 32 bytes")
	}
	return &CursorCodec{key: append([]byte(nil), key...)}, nil
}

func (c *CursorCodec) Encode(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(payload)
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c *CursorCodec) Decode(encoded string, dst any) error {
	payloadPart, signaturePart, ok := strings.Cut(encoded, ".")
	if !ok {
		return errors.New("invalid cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return errors.New("invalid cursor")
	}
	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return errors.New("invalid cursor")
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return errors.New("invalid cursor signature")
	}
	if err := json.Unmarshal(payload, dst); err != nil {
		return errors.New("invalid cursor payload")
	}
	return nil
}
