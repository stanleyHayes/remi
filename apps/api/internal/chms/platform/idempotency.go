package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type IdempotencyRecord struct {
	Scope          string     `bson:"scope"`
	Key            string     `bson:"key"`
	RequestHash    string     `bson:"requestHash"`
	State          string     `bson:"state"`
	ResponseStatus int        `bson:"responseStatus,omitempty"`
	ResponseBody   []byte     `bson:"responseBody,omitempty"`
	ResourceID     ID         `bson:"resourceId,omitempty"`
	CreatedAt      time.Time  `bson:"createdAt"`
	ExpiresAt      *time.Time `bson:"expiresAt,omitempty"`
}

func ValidateIdempotencyKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 128 {
		return errors.New("idempotency key must be 8 to 128 characters")
	}
	for _, r := range key {
		if r < 0x21 || r > 0x7e {
			return errors.New("idempotency key must use printable ASCII without spaces")
		}
	}
	return nil
}

func RequestHash(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

type IdempotencyStore interface {
	Begin(context.Context, string, string, string, time.Time, *time.Time) (record IdempotencyRecord, acquired bool, err error)
	Complete(context.Context, string, string, string, int, []byte, ID) error
	Release(context.Context, string, string, string) error
}
