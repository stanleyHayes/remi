package handlers

import (
	"context"
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
	"remi-api/internal/models"
)

// SubmitMemberPrayer accepts a self-scoped prayer request. A person ID is
// stored only when the signed-in member explicitly consents to linkage.
func (h *Handler) SubmitMemberPrayer(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	request := strings.TrimSpace(httpx.Str(body, "request"))
	visibility := strings.ToLower(strings.TrimSpace(httpx.Str(body, "visibility")))
	identityMode := strings.ToLower(strings.TrimSpace(httpx.Str(body, "identityMode")))
	if identityMode == "" {
		identityMode = "identified"
	}
	if visibility == "" {
		visibility = "pastors-only"
	}
	if request == "" || len(request) > 10000 || (identityMode != "identified" && identityMode != "anonymous") || (visibility != "pastors-only" && visibility != "prayer-team") {
		httpx.Error(w, http.StatusBadRequest, "provide a valid request, identity choice and visibility")
		return
	}
	claims := middleware.ClaimsFrom(r)
	now := models.Now()
	doc := bson.M{"request": request, "identityMode": identityMode, "visibility": visibility, "status": "new", "source": "member-app", "createdAt": now, "profileLinkStatus": "not-requested"}
	if identityMode == "identified" {
		doc["name"], doc["email"] = claims.Name, strings.ToLower(strings.TrimSpace(claims.Email))
		if body["linkToProfile"] == true && body["linkConsent"] == true {
			doc["personId"] = claims.PersonID
			doc["profileLinkStatus"] = "linked"
			doc["profileLinkConsent"] = bson.M{"state": "granted", "source": "authenticated-member", "noticeVersion": "prayer-link-2026-01", "capturedAt": now}
		}
	}
	id := bson.NewObjectID()
	doc["_id"] = id
	store, _ := platform.NewMongoPlatformStore(h.DB)
	actorID := platform.ID(claims.PersonID)
	if identityMode == "anonymous" {
		actorID = "anonymous-member-prayer"
	}
	err = store.WithTransaction(r.Context(), func(ctx context.Context) error {
		if _, insertErr := h.DB.Collection("prayer_requests").InsertOne(ctx, doc); insertErr != nil {
			return insertErr
		}
		subjects := []platform.ID{}
		if identityMode == "identified" {
			subjects = append(subjects, platform.ID(claims.PersonID))
		}
		return store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: actorID}, Action: "member.prayer-request.create", ResourceType: "prayer-request", ResourceID: platform.ID(id.Hex()), SubjectIDs: subjects, ChangedFields: []string{"identityMode", "visibility", "status"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, bson.M{"id": id, "message": "received", "linkedToProfile": doc["profileLinkStatus"] == "linked", "trackable": identityMode == "identified" && doc["profileLinkStatus"] == "linked"})
}
