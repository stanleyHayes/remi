package handlers

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/models"
)

const memberClientEventsCollection = "chms_member_client_events"

var safeDigestPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,96}$`)

var memberClientEventTypes = map[string]bool{
	"route-error":           true,
	"global-error":          true,
	"unhandled-error":       true,
	"unhandled-rejection":   true,
	"navigation-slow":       true,
	"connectivity-lost":     true,
	"connectivity-restored": true,
}

var memberTelemetryRoutes = map[string]bool{
	"home": true, "account": true, "care": true, "community": true,
	"giving": true, "lead": true, "messages": true, "participation": true,
	"serving": true, "support": true, "unknown": true,
}

type memberClientEventInput struct {
	Type           string `json:"type"`
	Route          string `json:"route"`
	Digest         string `json:"digest"`
	DurationBucket string `json:"durationBucket"`
	Online         bool   `json:"online"`
}

func (h *Handler) RecordMemberClientEvent(w http.ResponseWriter, r *http.Request) {
	if _, ok := memberClaims(r); !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	var input memberClientEventInput
	if err := platform.DecodeJSON(w, r, &input, 4096); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Route = strings.ToLower(strings.TrimSpace(input.Route))
	input.Digest = strings.TrimSpace(input.Digest)
	input.DurationBucket = strings.ToLower(strings.TrimSpace(input.DurationBucket))
	if !memberClientEventTypes[input.Type] || !memberTelemetryRoutes[input.Route] {
		httpx.Error(w, http.StatusBadRequest, "unsupported client event")
		return
	}
	if input.Digest != "" && !safeDigestPattern.MatchString(input.Digest) {
		httpx.Error(w, http.StatusBadRequest, "invalid event digest")
		return
	}
	if input.DurationBucket != "" && input.DurationBucket != "under-1s" && input.DurationBucket != "1-3s" && input.DurationBucket != "3-8s" && input.DurationBucket != "over-8s" {
		httpx.Error(w, http.StatusBadRequest, "invalid duration bucket")
		return
	}
	if input.Type == "navigation-slow" && input.DurationBucket == "" {
		httpx.Error(w, http.StatusBadRequest, "duration bucket is required")
		return
	}
	now := models.Now()
	_, err := h.DB.Collection(memberClientEventsCollection).InsertOne(r.Context(), bson.M{
		"_id": bson.NewObjectID(), "organizationId": h.Cfg.CHMSOrganizationID,
		"type": input.Type, "route": input.Route, "digest": input.Digest,
		"durationBucket": input.DurationBucket, "online": input.Online,
		"recordedAt": now, "expiresAt": now.Add(30 * 24 * time.Hour),
	})
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, "client event unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
