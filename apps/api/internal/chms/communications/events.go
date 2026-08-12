package communications

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

func (s Service) RecordProviderEvent(ctx context.Context, provider string, raw []byte, signature string) (bool, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !map[string]bool{"resend": true, "arkesel": true, "meta-whatsapp": true}[provider] {
		return false, platform.ValidationError(platform.FieldError{Path: "provider", Code: "unsupported", Message: "Unknown communication provider."})
	}
	if strings.TrimSpace(s.WebhookSecret) == "" {
		return false, errors.New("communication webhook secret is not configured")
	}
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	_, _ = mac.Write(raw)
	provided, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(signature), "sha256="))
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return false, &platform.DomainError{Code: "unauthenticated", Message: "Invalid communication event signature."}
	}
	var in ProviderEventInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return false, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_json", Message: "Invalid delivery event."})
	}
	in.ProviderEventID, in.Reference, in.Type = strings.TrimSpace(in.ProviderEventID), strings.TrimSpace(in.Reference), strings.ToLower(strings.TrimSpace(in.Type))
	if in.ProviderEventID == "" || in.Reference == "" || !map[string]bool{"accepted": true, "delivered": true, "failed": true, "bounced": true, "complained": true, "opted-out": true}[in.Type] {
		return false, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_event", Message: "Event id, reference and supported type are required."})
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = s.now()
	} else {
		in.OccurredAt = in.OccurredAt.UTC()
	}
	delivery, err := s.Repository.FindDeliveryByReference(ctx, provider, in.Reference)
	if err != nil {
		return false, err
	}
	if delivery == nil {
		return false, &platform.DomainError{Code: "not_found", Message: "Delivery reference was not found."}
	}
	recorded := false
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		event := DeliveryEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: delivery.OrganizationID, CampaignID: delivery.CampaignID, DeliveryID: delivery.ID, Provider: provider, ProviderEventID: in.ProviderEventID, Type: in.Type, OccurredAt: in.OccurredAt, RecordedAt: s.now()}
		created, insertErr := s.Repository.InsertDeliveryEvent(tx, event)
		if insertErr != nil || !created {
			return insertErr
		}
		recorded = true
		state := in.Type
		if state == "opted-out" {
			state = "delivered"
		}
		if err := s.Repository.TransitionDelivery(tx, delivery.ID, state, s.now()); err != nil {
			return err
		}
		if in.Type == "bounced" || in.Type == "complained" || in.Type == "opted-out" {
			if err := s.Repository.SuppressFromProvider(tx, *delivery, in.Type, s.now()); err != nil {
				return err
			}
		}
		return s.Repository.RefreshCampaignCounts(tx, delivery.OrganizationID, delivery.CampaignID)
	})
	return recorded, err
}
