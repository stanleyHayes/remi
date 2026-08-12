package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
	"remi-api/internal/models"
)

const (
	memberCareRequestsCollection = "chms_member_care_requests"
	memberAppointmentsCollection = "chms_member_pastoral_appointments"
	memberContentSavesCollection = "chms_member_content_saves"
)

type memberContentSaveRecord struct {
	ID            any                     `bson:"_id"`
	ContentType   string                  `bson:"contentType"`
	ContentID     string                  `bson:"contentId"`
	Title         string                  `bson:"title"`
	Slug          string                  `bson:"slug"`
	EncryptedNote platform.EncryptedValue `bson:"encryptedNote"`
	UpdatedAt     time.Time               `bson:"updatedAt"`
}

func memberSafeState(value any) string {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "acknowledged", "in-prayer", "assigned", "contacted", "scheduled", "confirmed":
		return "acknowledged"
	case "closed", "completed", "resolved":
		return "completed"
	default:
		return "received"
	}
}

func (h *Handler) memberNoteCipher() (*platform.EnvelopeCipher, error) {
	key := sha256.Sum256([]byte(h.Cfg.JWTSecret + ":member-private-content-notes:v1"))
	return platform.NewEnvelopeCipher("member-private-content-notes-v1", key[:])
}

func (h *Handler) GetMemberCareContent(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	claims := middleware.ClaimsFrom(r)
	now := models.Now()
	announcements := []bson.M{}
	if cursor, err := h.DB.Collection("announcements").Find(r.Context(), bson.M{"contentStatus": bson.M{"$in": models.PublicStatuses}, "$and": bson.A{bson.M{"$or": bson.A{bson.M{"publishAt": bson.M{"$exists": false}}, bson.M{"publishAt": bson.M{"$lte": now}}}}, bson.M{"$or": bson.A{bson.M{"expiresAt": bson.M{"$exists": false}}, bson.M{"expiresAt": nil}, bson.M{"expiresAt": bson.M{"$gt": now}}}}}}, options.Find().SetProjection(bson.M{"title": 1, "body": 1, "slug": 1, "publishAt": 1}).SetSort(bson.D{{Key: "publishAt", Value: -1}}).SetLimit(8)); err == nil {
		defer cursor.Close(r.Context())
		_ = cursor.All(r.Context(), &announcements)
	}
	sermons := []bson.M{}
	if cursor, err := h.DB.Collection("sermons").Find(r.Context(), publicContentFilter(), options.Find().SetProjection(bson.M{"title": 1, "slug": 1, "description": 1, "preacher": 1, "series": 1, "date": 1, "image": 1, "videoUrl": 1, "audioUrl": 1}).SetSort(bson.D{{Key: "date", Value: -1}}).SetLimit(24)); err == nil {
		defer cursor.Close(r.Context())
		_ = cursor.All(r.Context(), &sermons)
	}

	requests := []bson.M{}
	appendRequests := func(collection string, filter bson.M, kind string) {
		cursor, err := h.DB.Collection(collection).Find(r.Context(), filter, options.Find().SetProjection(bson.M{"request": 1, "summary": 1, "category": 1, "visibility": 1, "status": 1, "state": 1, "createdAt": 1, "preferredWindow": 1}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(20))
		if err != nil {
			return
		}
		defer cursor.Close(r.Context())
		var values []bson.M
		if cursor.All(r.Context(), &values) != nil {
			return
		}
		for _, value := range values {
			requests = append(requests, bson.M{"id": normalizeID(value["_id"]), "kind": kind, "summary": firstNonEmpty(fmt.Sprint(value["summary"]), fmt.Sprint(value["request"])), "category": value["category"], "visibility": value["visibility"], "preferredWindow": value["preferredWindow"], "acknowledgement": memberSafeState(firstNonEmpty(fmt.Sprint(value["state"]), fmt.Sprint(value["status"]))), "createdAt": value["createdAt"]})
		}
	}
	appendRequests("prayer_requests", bson.M{"source": "member-app", "personId": claims.PersonID, "identityMode": "identified"}, "prayer")
	appendRequests(memberCareRequestsCollection, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID}, "care")
	appendRequests(memberAppointmentsCollection, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID}, "appointment")

	saves := []bson.M{}
	cipher, _ := h.memberNoteCipher()
	if cursor, err := h.DB.Collection(memberContentSavesCollection).Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "saved": true}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}})); err == nil {
		defer cursor.Close(r.Context())
		var values []memberContentSaveRecord
		_ = cursor.All(r.Context(), &values)
		for _, value := range values {
			note := ""
			if cipher != nil && value.EncryptedNote.Ciphertext != "" {
				if plaintext, err := cipher.Decrypt(value.EncryptedNote, []byte(fmt.Sprintf("%s:%s:%s:%s", h.Cfg.CHMSOrganizationID, claims.PersonID, value.ContentType, value.ContentID))); err == nil {
					note = string(plaintext)
				}
			}
			saves = append(saves, bson.M{"id": normalizeID(value.ID), "contentType": value.ContentType, "contentId": value.ContentID, "title": value.Title, "slug": value.Slug, "note": note, "updatedAt": value.UpdatedAt})
		}
	}
	var settings bson.M
	_ = h.DB.Collection("settings").FindOne(r.Context(), bson.M{}).Decode(&settings)
	httpx.JSON(w, http.StatusOK, bson.M{"announcements": models.NormalizeAll(announcements), "sermons": models.NormalizeAll(sermons), "requests": models.NormalizeAll(requests), "saved": models.NormalizeAll(saves), "live": bson.M{"isLive": settings["isLive"] == true, "url": settings["livestreamUrl"]}})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return "Request received"
}

func (h *Handler) CreateMemberCareRequest(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	category := strings.ToLower(strings.TrimSpace(httpx.Str(body, "category")))
	summary := strings.TrimSpace(httpx.Str(body, "summary"))
	details := strings.TrimSpace(httpx.Str(body, "details"))
	visibility := strings.ToLower(strings.TrimSpace(httpx.Str(body, "visibility")))
	identity := strings.ToLower(strings.TrimSpace(httpx.Str(body, "identityMode")))
	if !map[string]bool{"emotional-support": true, "bereavement": true, "family": true, "health": true, "spiritual-guidance": true, "practical-support": true}[category] || len(summary) < 3 || len(summary) > 160 || len(details) < 10 || len(details) > 5000 || !map[string]bool{"pastors-only": true, "care-team": true}[visibility] || !map[string]bool{"identified": true, "anonymous": true}[identity] {
		httpx.Error(w, 400, "choose a care type, privacy options and provide a short summary and details")
		return
	}
	claims := middleware.ClaimsFrom(r)
	now := models.Now()
	id := platform.ID(bson.NewObjectID().Hex())
	doc := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "category": category, "summary": summary, "details": details, "visibility": visibility, "identityMode": identity, "state": "received", "source": "member-app", "createdAt": now, "updatedAt": now}
	if identity == "identified" {
		doc["personId"], doc["name"], doc["email"] = claims.PersonID, claims.Name, claims.Email
	}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	actorID := platform.ID(claims.PersonID)
	if identity == "anonymous" {
		actorID = "anonymous-member-request"
	}
	err = store.WithTransaction(r.Context(), func(ctx context.Context) error {
		if _, e := h.DB.Collection(memberCareRequestsCollection).InsertOne(ctx, doc); e != nil {
			return e
		}
		return store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: actorID}, Action: "member.care-request.create", ResourceType: "member-care-request", ResourceID: id, SubjectIDs: func() []platform.ID {
			if identity == "identified" {
				return []platform.ID{platform.ID(claims.PersonID)}
			}
			return nil
		}(), ChangedFields: []string{"category", "visibility", "identityMode", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		httpx.Error(w, 500, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, bson.M{"id": id, "message": "received", "trackable": identity == "identified"})
}

func (h *Handler) CreateMemberPastoralAppointment(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, 403, "member access required")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	topic := strings.TrimSpace(httpx.Str(body, "topic"))
	window := strings.ToLower(strings.TrimSpace(httpx.Str(body, "preferredWindow")))
	channel := strings.ToLower(strings.TrimSpace(httpx.Str(body, "contactChannel")))
	if len(topic) < 5 || len(topic) > 500 || !map[string]bool{"weekday-morning": true, "weekday-afternoon": true, "weekday-evening": true, "weekend": true, "flexible": true}[window] || !map[string]bool{"email": true, "sms": true, "phone": true}[channel] {
		httpx.Error(w, 400, "provide a topic, preferred window and contact channel")
		return
	}
	claims := middleware.ClaimsFrom(r)
	now := models.Now()
	id := platform.ID(bson.NewObjectID().Hex())
	doc := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "topic": topic, "preferredWindow": window, "contactChannel": channel, "state": "received", "createdAt": now, "updatedAt": now}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err = store.WithTransaction(r.Context(), func(ctx context.Context) error {
		if _, e := h.DB.Collection(memberAppointmentsCollection).InsertOne(ctx, doc); e != nil {
			return e
		}
		return store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, Action: "member.pastoral-appointment.request", ResourceType: "pastoral-appointment-request", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"preferredWindow", "contactChannel", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		httpx.Error(w, 500, "database error")
		return
	}
	httpx.JSON(w, 201, bson.M{"id": id, "message": "received"})
}

func (h *Handler) SaveMemberContent(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, 403, "member access required")
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "type")))
	rawID := strings.TrimSpace(chi.URLParam(r, "id"))
	if contentType != "sermon" {
		httpx.Error(w, 404, "content not found")
		return
	}
	contentID, err := bson.ObjectIDFromHex(rawID)
	if err != nil {
		httpx.Error(w, 404, "content not found")
		return
	}
	var content bson.M
	if h.DB.Collection("sermons").FindOne(r.Context(), bson.M{"_id": contentID, "contentStatus": bson.M{"$in": models.PublicStatuses}}, options.FindOne().SetProjection(bson.M{"title": 1, "slug": 1})).Decode(&content) != nil {
		httpx.Error(w, 404, "content not found")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	saved, _ := body["saved"].(bool)
	note := strings.TrimSpace(httpx.Str(body, "note"))
	if len(note) > 4000 {
		httpx.Error(w, 400, "private note is too long")
		return
	}
	claims := middleware.ClaimsFrom(r)
	now := models.Now()
	cipher, err := h.memberNoteCipher()
	if err != nil {
		httpx.Error(w, 500, "private notes unavailable")
		return
	}
	aad := []byte(fmt.Sprintf("%s:%s:%s:%s", h.Cfg.CHMSOrganizationID, claims.PersonID, contentType, rawID))
	encrypted, err := cipher.Encrypt([]byte(note), aad)
	if err != nil {
		httpx.Error(w, 500, "private notes unavailable")
		return
	}
	id := platform.ID(bson.NewObjectID().Hex())
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "contentType": contentType, "contentId": rawID}
	update := bson.M{"$set": bson.M{"saved": saved, "encryptedNote": encrypted, "title": content["title"], "slug": content["slug"], "updatedAt": now}, "$setOnInsert": bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "contentType": contentType, "contentId": rawID, "createdAt": now}}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err = store.WithTransaction(r.Context(), func(ctx context.Context) error {
		result := h.DB.Collection(memberContentSavesCollection).FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After))
		var value bson.M
		if e := result.Decode(&value); e != nil {
			return e
		}
		id = platform.ID(fmt.Sprint(normalizeID(value["_id"])))
		return store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, Action: "member.content-save.update", ResourceType: "member-content-save", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"saved", "privateNote"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			httpx.Error(w, 409, "saved content changed; try again")
		} else {
			httpx.Error(w, 500, "database error")
		}
		return
	}
	httpx.JSON(w, 200, bson.M{"id": id, "saved": saved, "note": note})
}
