package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	chmsplatform "remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
	"remi-api/internal/models"
)

// crudResource describes an admin-managed content collection.
type crudResource struct {
	collection string
	// slugSource lists body fields to derive a slug from, in priority order.
	slugSource []string
}

func (h *Handler) AdminUpdateUserScopes(w http.ResponseWriter, r *http.Request) {
	claims := middleware.ClaimsFrom(r)
	if claims == nil {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := bson.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "user not found")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	reason := strings.TrimSpace(httpx.Str(body, "reason"))
	expectedVersion := int64FromBSON(body["expectedAccessVersion"])
	branchIDs, ministryIDs := stringsFromBSON(body["branchIds"]), stringsFromBSON(body["ministryIds"])
	if expectedVersion < 0 || len(reason) < 8 || len(reason) > 300 {
		httpx.Error(w, http.StatusBadRequest, "expected access version and a reason of 8 to 300 characters are required")
		return
	}
	var target bson.M
	if err = h.DB.Collection("users").FindOne(r.Context(), bson.M{"_id": id}).Decode(&target); err != nil {
		httpx.Error(w, http.StatusNotFound, "user not found")
		return
	}
	currentRole := strings.TrimSpace(fmt.Sprint(target["role"]))
	role := strings.TrimSpace(httpx.Str(body, "role"))
	if role == "" {
		role = currentRole
	}
	if !validStaffRole(role) {
		httpx.Error(w, http.StatusBadRequest, "role is not supported")
		return
	}
	roleChanged := role != currentRole
	now := time.Now().UTC()
	if roleChanged {
		if claims.MFAAt == 0 || now.Sub(time.Unix(claims.MFAAt, 0).UTC()) > 10*time.Minute || now.Before(time.Unix(claims.MFAAt, 0).UTC()) {
			httpx.Error(w, http.StatusForbidden, "recent MFA verification is required to change a role")
			return
		}
		if claims.UserID == id.Hex() {
			httpx.Error(w, http.StatusConflict, "you cannot change your own role")
			return
		}
		if currentRole == "super-admin" {
			count, countErr := h.DB.Collection("users").CountDocuments(r.Context(), bson.M{"role": "super-admin", "invitationStatus": bson.M{"$in": bson.A{"accepted", "active"}}})
			if countErr != nil {
				httpx.Error(w, http.StatusInternalServerError, "could not verify administrator coverage")
				return
			}
			if count <= 1 {
				httpx.Error(w, http.StatusConflict, "the final super administrator cannot be demoted")
				return
			}
		}
	}
	if role == "super-admin" {
		branchIDs, ministryIDs = []string{"*"}, []string{"*"}
	}
	if role != "editor" && role != "viewer" && role != "super-admin" && len(branchIDs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "choose at least one branch for this operational role")
		return
	}
	if role != "super-admin" && (!h.validScopeReferences(r, "branches", branchIDs) || !h.validScopeReferences(r, "ministries", ministryIDs)) {
		httpx.Error(w, http.StatusBadRequest, "one or more scope choices no longer exist")
		return
	}
	store, storeErr := chmsplatform.NewMongoPlatformStore(h.DB)
	if storeErr != nil {
		httpx.Error(w, http.StatusInternalServerError, "scope store is unavailable")
		return
	}
	err = store.WithTransaction(r.Context(), func(tx context.Context) error {
		versionFilter := bson.M{"accessVersion": expectedVersion}
		if expectedVersion == 0 {
			versionFilter = bson.M{"$or": bson.A{bson.M{"accessVersion": int64(0)}, bson.M{"accessVersion": bson.M{"$exists": false}}}}
		}
		filter := bson.M{"_id": id}
		for key, value := range versionFilter {
			filter[key] = value
		}
		result, updateErr := h.DB.Collection("users").UpdateOne(tx, filter, bson.M{"$set": bson.M{"role": role, "branchIds": branchIDs, "ministryIds": ministryIDs, "updatedAt": now}, "$inc": bson.M{"accessVersion": 1}})
		if updateErr != nil {
			return updateErr
		}
		if result.MatchedCount != 1 {
			return fmt.Errorf("access version conflict")
		}
		changed := []string{"branchIds", "ministryIds", "accessVersion"}
		action := "staff.access-scope.update"
		if roleChanged {
			changed = append(changed, "role")
			action = "staff.access-role.update"
		}
		return store.AppendAudit(tx, chmsplatform.AuditEvent{ID: chmsplatform.ID(bson.NewObjectID().Hex()), OrganizationID: chmsplatform.ID(h.Cfg.CHMSOrganizationID), Actor: chmsplatform.Actor{Type: chmsplatform.ActorStaff, ID: chmsplatform.ID(claims.UserID)}, Action: action, ResourceType: "staff-user", ResourceID: chmsplatform.ID(id.Hex()), ChangedFields: changed, Outcome: "success", Reason: reason, RequestID: chmsplatform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		if strings.Contains(err.Error(), "version conflict") {
			httpx.Error(w, http.StatusConflict, "access scope changed; reload before saving")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "could not update access scope")
		return
	}
	target["role"], target["branchIds"], target["ministryIds"], target["accessVersion"] = role, branchIDs, ministryIDs, expectedVersion+1
	httpx.JSON(w, http.StatusOK, publicUserWithScopes(target))
}

func (h *Handler) validScopeReferences(r *http.Request, collection string, ids []string) bool {
	for _, id := range ids {
		if id == "" || id == "*" || len(id) > 100 {
			return false
		}
		filters := bson.A{bson.M{"slug": id}, bson.M{"id": id}}
		if objectID, err := bson.ObjectIDFromHex(id); err == nil {
			filters = append(filters, bson.M{"_id": objectID})
		}
		if count, err := h.DB.Collection(collection).CountDocuments(r.Context(), bson.M{"$or": filters}); err != nil || count != 1 {
			return false
		}
	}
	return true
}

func publicUserWithScopes(user bson.M) bson.M {
	value := publicUser(user)
	value["branchIds"], value["ministryIds"], value["accessVersion"] = user["branchIds"], user["ministryIds"], int64FromBSON(user["accessVersion"])
	return value
}

func validStaffRole(role string) bool {
	return map[string]bool{"super-admin": true, "editor": true, "viewer": true, "pastor": true, "branch-admin": true, "membership-admin": true, "group-admin": true, "volunteer-coordinator": true, "finance-counter": true, "finance-admin": true, "finance-approver": true, "finance-auditor": true, "auditor": true, "data-protection-supervisor": true}[role]
}

var contentResources = map[string]crudResource{
	"pages":         {collection: "pages", slugSource: []string{"pageKey", "title"}},
	"leadership":    {collection: "leaders", slugSource: []string{"name"}},
	"ministries":    {collection: "ministries", slugSource: []string{"name", "title"}},
	"branches":      {collection: "branches", slugSource: []string{"name", "title"}},
	"sermons":       {collection: "sermons", slugSource: []string{"title"}},
	"events":        {collection: "events", slugSource: []string{"title"}},
	"announcements": {collection: "announcements", slugSource: []string{"title"}},
	"testimonies":   {collection: "testimonies", slugSource: []string{"title", "author"}},
}

// formResources maps the admin forms route segment to its collection.
var formResources = map[string]string{
	"prayer":      "prayer_requests",
	"contact":     "contact_submissions",
	"testimonies": "testimonies",
	"visits":      "visit_submissions",
}

// ContentResources exposes the admin-managed content resources for route registration.
func ContentResources() map[string]crudResource { return contentResources }

var validStatuses = map[string]bool{
	models.StatusAIDraft: true, models.StatusInReview: true,
	models.StatusApproved: true, models.StatusPublished: true,
}

func (h *Handler) actor(r *http.Request) string {
	if c := middleware.ClaimsFrom(r); c != nil {
		return c.Email
	}
	return "system"
}

// ── Generic content CRUD ──────────────────────────────────────────

func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request, res crudResource) {
	docs, ok := h.findAll(w, r, res.collection, bson.M{},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

func (h *Handler) AdminCreate(w http.ResponseWriter, r *http.Request, res crudResource) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	delete(body, "id")
	delete(body, "_id")

	if httpx.Str(body, "slug") == "" {
		for _, f := range res.slugSource {
			if s := httpx.Str(body, f); s != "" {
				body["slug"] = models.Slugify(s)
				break
			}
		}
		if httpx.Str(body, "slug") == "" {
			body["slug"] = "item-" + randHex(4)
		}
	}
	if httpx.Str(body, "contentStatus") == "" {
		body["contentStatus"] = models.StatusPublished
	}
	if httpx.Str(body, "title") == "" {
		for _, f := range []string{"name", "author", "pageKey"} {
			if s := httpx.Str(body, f); s != "" {
				body["title"] = s
				break
			}
		}
	}
	if res.collection == "events" {
		if _, ok := body["registeredCount"]; !ok {
			body["registeredCount"] = 0
		}
	}
	now := models.Now()
	body["createdBy"] = h.actor(r)
	body["updatedBy"] = h.actor(r)
	body["createdAt"] = now
	body["updatedAt"] = now

	result, err := h.DB.Collection(res.collection).InsertOne(r.Context(), body)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	body["_id"] = result.InsertedID
	httpx.JSON(w, http.StatusCreated, models.Normalize(body))
}

func (h *Handler) AdminUpdate(w http.ResponseWriter, r *http.Request, res crudResource) {
	id, ok := httpx.ObjectID(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	delete(body, "id")
	delete(body, "_id")
	delete(body, "createdAt")
	delete(body, "createdBy")
	body["updatedAt"] = models.Now()
	body["updatedBy"] = h.actor(r)

	var updated bson.M
	err = h.DB.Collection(res.collection).FindOneAndUpdate(r.Context(),
		bson.M{"_id": id},
		bson.M{"$set": body},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if err == mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(updated))
}

func (h *Handler) AdminDelete(w http.ResponseWriter, r *http.Request, res crudResource) {
	id, ok := httpx.ObjectID(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	res2, err := h.DB.Collection(res.collection).DeleteOne(r.Context(), bson.M{"_id": id})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if res2.DeletedCount == 0 {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"message": "deleted"})
}

// ── Settings (singleton) ──────────────────────────────────────────

func (h *Handler) AdminPutSettings(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	delete(body, "id")
	delete(body, "_id")
	body["updatedAt"] = models.Now()
	body["updatedBy"] = h.actor(r)

	var updated bson.M
	err = h.DB.Collection("settings").FindOneAndUpdate(r.Context(),
		bson.M{},
		bson.M{"$set": body},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&updated)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(updated))
}

// ── Content status transitions ────────────────────────────────────

func (h *Handler) AdminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	typeParam := chi.URLParam(r, "type")
	res, ok := contentResources[typeParam]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "unknown content type: "+typeParam)
		return
	}
	id, ok := httpx.ObjectID(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	status := httpx.Str(body, "contentStatus")
	if !validStatuses[status] {
		httpx.Error(w, http.StatusBadRequest, "contentStatus must be one of ai-draft, in-review, approved, published")
		return
	}
	var updated bson.M
	err = h.DB.Collection(res.collection).FindOneAndUpdate(r.Context(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"contentStatus": status, "updatedAt": models.Now(), "updatedBy": h.actor(r)}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if err == mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(updated))
}

// ── Form submissions admin ────────────────────────────────────────

func (h *Handler) AdminListForms(w http.ResponseWriter, r *http.Request) {
	coll, ok := formResources[chi.URLParam(r, "formType")]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "unknown form type")
		return
	}
	docs, ok := h.findAll(w, r, coll, bson.M{},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

func (h *Handler) AdminUpdateFormStatus(w http.ResponseWriter, r *http.Request) {
	coll, ok := formResources[chi.URLParam(r, "formType")]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "unknown form type")
		return
	}
	id, ok := httpx.ObjectID(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	status := httpx.Str(body, "status")
	if status == "" {
		httpx.Error(w, http.StatusBadRequest, "status is required")
		return
	}
	var updated bson.M
	err = h.DB.Collection(coll).FindOneAndUpdate(r.Context(),
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"status": status, "updatedAt": models.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updated)
	if err == mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(updated))
}

// ── Event registrations ───────────────────────────────────────────

func (h *Handler) AdminListRegistrations(w http.ResponseWriter, r *http.Request) {
	filter := bson.M{}
	if ev := r.URL.Query().Get("eventId"); ev != "" {
		filter["eventId"] = ev
	}
	docs, ok := h.findAll(w, r, "event_registrations", filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

// ── Stats ─────────────────────────────────────────────────────────

func (h *Handler) AdminStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	count := func(coll string, filter bson.M) int64 {
		n, err := h.DB.Collection(coll).CountDocuments(ctx, filter)
		if err != nil {
			return 0
		}
		return n
	}

	var pending int64
	statusCounts := bson.M{
		models.StatusAIDraft: int64(0), models.StatusInReview: int64(0),
		models.StatusApproved: int64(0), models.StatusPublished: int64(0),
	}
	contentMix := bson.A{}
	var totalContent int64
	for _, res := range contentResources {
		for _, status := range []string{models.StatusAIDraft, models.StatusInReview, models.StatusApproved, models.StatusPublished} {
			n := count(res.collection, bson.M{"contentStatus": status})
			statusCounts[status] = statusCounts[status].(int64) + n
		}
	}
	for _, item := range []struct{ label, collection string }{
		{"Pages", "pages"}, {"Sermons", "sermons"}, {"Events", "events"},
		{"Ministries", "ministries"}, {"Leaders", "leaders"}, {"Branches", "branches"},
		{"Announcements", "announcements"}, {"Testimonies", "testimonies"},
	} {
		n := count(item.collection, bson.M{})
		totalContent += n
		contentMix = append(contentMix, bson.M{"label": item.label, "value": n})
	}
	pending = statusCounts[models.StatusAIDraft].(int64) + statusCounts[models.StatusInReview].(int64)
	published := statusCounts[models.StatusPublished].(int64)

	trend := bson.A{}
	now := time.Now().UTC()
	engagementCollections := []string{"prayer_requests", "contact_submissions", "visit_submissions", "event_registrations", "subscribers"}
	for offset := 6; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)
		var value int64
		for _, coll := range engagementCollections {
			value += count(coll, bson.M{"createdAt": bson.M{"$gte": start, "$lt": end}})
		}
		trend = append(trend, bson.M{"date": start.Format("2006-01-02"), "label": start.Format("Mon"), "value": value})
	}

	prayerTotal := count("prayer_requests", bson.M{})
	contactTotal := count("contact_submissions", bson.M{})
	visitTotal := count("visit_submissions", bson.M{})
	registrationTotal := count("event_registrations", bson.M{})
	subscriberTotal := count("subscribers", bson.M{})

	httpx.JSON(w, http.StatusOK, bson.M{
		"pages":             count("pages", bson.M{}),
		"sermons":           count("sermons", bson.M{}),
		"events":            count("events", bson.M{}),
		"pendingReview":     pending,
		"newPrayerRequests": count("prayer_requests", bson.M{"status": "new"}),
		"newContacts":       count("contact_submissions", bson.M{"status": "new"}),
		"registrations":     registrationTotal,
		"subscribers":       subscriberTotal,
		"upcomingEvents":    count("events", bson.M{"startAt": bson.M{"$gte": now}}),
		"totalContent":      totalContent,
		"publishedContent":  published,
		"publishingRate":    percentage(published, totalContent),
		"contentStatus":     statusCounts,
		"contentMix":        contentMix,
		"engagementTrend":   trend,
		"engagementTotal":   prayerTotal + contactTotal + visitTotal + registrationTotal + subscriberTotal,
		"submissionMix": bson.A{
			bson.M{"label": "Prayer", "value": prayerTotal},
			bson.M{"label": "Contact", "value": contactTotal},
			bson.M{"label": "Visits", "value": visitTotal},
			bson.M{"label": "Registrations", "value": registrationTotal},
			bson.M{"label": "Subscribers", "value": subscriberTotal},
		},
	})
}

func percentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// ── Users (super-admin) ───────────────────────────────────────────

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	docs, ok := h.findAll(w, r, "users", bson.M{})
	if !ok {
		return
	}
	users := make([]bson.M, 0, len(docs))
	for _, doc := range docs {
		user := bson.M{}
		for _, field := range []string{"email", "name", "role", "invitationStatus", "invitedAt", "activatedAt", "createdAt", "lastLoginAt", "branchIds", "ministryIds", "assignedResourceIds", "accessVersion"} {
			if value, exists := doc[field]; exists {
				user[field] = value
			}
		}
		if id, exists := doc["_id"]; exists {
			user["_id"] = id
		}
		users = append(users, models.Normalize(user))
	}
	httpx.JSON(w, http.StatusOK, users)
}

func (h *Handler) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(httpx.Str(body, "email")))
	role := httpx.Str(body, "role")
	if email == "" || !strings.Contains(email, "@") {
		httpx.Error(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if !validStaffRole(role) {
		httpx.Error(w, http.StatusBadRequest, "role is not supported")
		return
	}
	branchIDs := stringsFromBSON(body["branchIds"])
	ministryIDs := stringsFromBSON(body["ministryIds"])
	if role == "super-admin" {
		branchIDs, ministryIDs = []string{"*"}, []string{"*"}
	} else if role != "editor" && role != "viewer" && len(branchIDs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "choose at least one branch for this operational role")
		return
	}
	for _, value := range append(append([]string{}, branchIDs...), ministryIDs...) {
		if (value == "*" && role != "super-admin") || len(value) > 100 {
			httpx.Error(w, http.StatusBadRequest, "scope identifiers are invalid")
			return
		}
	}
	if !h.validScopeReferences(r, "branches", branchIDs) || !h.validScopeReferences(r, "ministries", ministryIDs) {
		httpx.Error(w, http.StatusBadRequest, "one or more scope choices no longer exist")
		return
	}
	var existing bson.M
	err = h.DB.Collection("users").FindOne(r.Context(), bson.M{"email": email}).Decode(&existing)
	if err == nil && fmt.Sprint(existing["passwordHash"]) != "" {
		httpx.Error(w, http.StatusConflict, "an active user with that email already exists")
		return
	}
	if err != nil && err != mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	token := randHex(32)
	sum := sha256.Sum256([]byte(token))
	now, expires := models.Now(), time.Now().Add(48*time.Hour)
	update := bson.M{"$set": bson.M{"email": email, "role": role, "branchIds": branchIDs, "ministryIds": ministryIDs, "assignedResourceIds": bson.A{}, "accessVersion": int64(0), "name": "", "invitationStatus": "pending", "invitationTokenHash": hex.EncodeToString(sum[:]), "invitedAt": now, "invitationExpiresAt": expires, "updatedAt": now}, "$setOnInsert": bson.M{"createdAt": now}}
	result, err := h.DB.Collection("users").UpdateOne(r.Context(), bson.M{"email": email}, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not create invitation")
		return
	}
	inviteURL := h.Cfg.AdminAppURL + "/invite/" + token
	if err := h.Email.Send(email, "You are invited to the REMI workspace", fmt.Sprintf(`<h2>Join the REMI workspace</h2><p>You have been invited as <b>%s</b>.</p><p><a href="%s">Accept invitation and create your account</a></p><p>This link expires in 48 hours and can only be used once.</p>`, role, inviteURL)); err != nil {
		httpx.Error(w, http.StatusBadGateway, "invitation created but email delivery failed; resend the invitation")
		return
	}
	id := ""
	if result.UpsertedID != nil {
		if oid, ok := result.UpsertedID.(bson.ObjectID); ok {
			id = oid.Hex()
		}
	} else if oid, ok := existing["_id"].(bson.ObjectID); ok {
		id = oid.Hex()
	}
	response := bson.M{"id": id, "email": email, "role": role, "invitationStatus": "pending", "expiresAt": expires}
	if h.Cfg.ResendAPIKey == "" {
		response["demoInvitationUrl"] = inviteURL
	}
	httpx.JSON(w, http.StatusCreated, response)
}

// ── Uploads (Cloudinary signature) ────────────────────────────────

func (h *Handler) AdminUploadSignature(w http.ResponseWriter, r *http.Request) {
	if !h.Cloud.Configured() {
		httpx.Error(w, http.StatusNotImplemented, "cloudinary is not configured; use placeholder image URLs")
		return
	}
	body, _ := httpx.Decode(r)
	var params map[string]any
	var err error
	if httpx.Str(body, "purpose") == "finance-settlement" {
		claims := middleware.ClaimsFrom(r)
		if claims == nil || (claims.Role != "finance-admin" && claims.Role != "super-admin") {
			httpx.Error(w, http.StatusForbidden, "finance administrator access required")
			return
		}
		params, err = h.Cloud.AuthenticatedSignature("remi/finance/settlements")
	} else {
		params, err = h.Cloud.Signature(httpx.Str(body, "folder"))
	}
	if err != nil {
		httpx.Error(w, http.StatusNotImplemented, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, params)
}
