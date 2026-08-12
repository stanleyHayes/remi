package communications

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

type OptOutClaims struct {
	OrganizationID platform.ID `json:"organizationId"`
	BranchID       platform.ID `json:"branchId"`
	PersonID       platform.ID `json:"personId"`
	CampaignID     platform.ID `json:"campaignId"`
	Purpose        string      `json:"purpose"`
	Channel        string      `json:"channel"`
	ExpiresAt      time.Time   `json:"expiresAt"`
}
type OptOutCodec struct{ key []byte }

func NewOptOutCodec(key []byte) (*OptOutCodec, error) {
	if len(key) < 32 {
		return nil, errors.New("opt-out signing key must be at least 32 bytes")
	}
	return &OptOutCodec{key: append([]byte(nil), key...)}, nil
}
func (c *OptOutCodec) Sign(v OptOutClaims) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (c *OptOutCodec) Verify(token string, now time.Time) (OptOutClaims, error) {
	var out OptOutClaims
	body, sig, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok {
		return out, errors.New("invalid opt-out token")
	}
	provided, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return out, errors.New("invalid opt-out token")
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write([]byte(body))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return out, errors.New("invalid opt-out token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || json.Unmarshal(raw, &out) != nil || !out.OrganizationID.Valid() || !out.BranchID.Valid() || !out.PersonID.Valid() || !out.CampaignID.Valid() || !purposes[out.Purpose] || !channels[out.Channel] || !out.ExpiresAt.After(now) {
		return OptOutClaims{}, errors.New("invalid or expired opt-out token")
	}
	return out, nil
}

func (s Service) OptOut(ctx context.Context, token string) error {
	if s.OptOuts == nil {
		return errors.New("opt-out service is unavailable")
	}
	claims, err := s.OptOuts.Verify(token, s.now())
	if err != nil {
		return platform.ValidationError(platform.FieldError{Path: "token", Code: "invalid", Message: "This opt-out link is invalid or expired."})
	}
	return s.Repository.SuppressPerson(ctx, claims.OrganizationID, claims.BranchID, claims.PersonID, claims.Purpose, claims.Channel, "member-link", s.now())
}
