package handlers

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
	"remi-api/internal/models"
)

// MemberHome is a purpose-built, self-scoped read model. It intentionally
// projects only data suitable for a member: no staff notes, attendance
// corrections, safeguarding fields, giving data, retention signals or other
// members' contact details are read or returned.
func (h *Handler) MemberHome(w http.ResponseWriter, r *http.Request) {
	if !requireMember(r) {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	claims := middleware.ClaimsFrom(r)
	now := time.Now().UTC()
	organizationID := h.Cfg.CHMSOrganizationID

	var person bson.M
	if err := h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": claims.PersonID, "organizationId": organizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"names": 1, "photoUrl": 1, "photoAssetId": 1, "homeBranchId": 1, "membershipStage": 1})).Decode(&person); err != nil {
		if err == mongo.ErrNoDocuments {
			httpx.Error(w, http.StatusNotFound, "member profile not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "member home unavailable")
		return
	}
	branchID := strings.TrimSpace(stringValue(person["homeBranchId"]))

	nextGathering := h.memberNextGathering(r, organizationID, branchID, now)
	assignments := h.memberAssignments(r, organizationID, claims.PersonID, now)
	groups := h.memberGroups(r, organizationID, claims.PersonID)
	registrations := h.memberRegistrations(r, claims.Email, now)
	announcements := h.memberAnnouncements(r, now)
	nextSteps := memberNextSteps(stringValue(person["membershipStage"]))
	activity := memberActivity(assignments, groups, registrations)

	httpx.JSON(w, http.StatusOK, bson.M{
		"generatedAt":   now,
		"member":        bson.M{"personId": claims.PersonID, "name": homeMemberName(person["names"], claims.Name), "photoUrl": person["photoUrl"], "photoAssetId": person["photoAssetId"], "branchId": branchID, "branchName": h.memberBranchName(r, branchID), "membershipStage": person["membershipStage"]},
		"nextGathering": nextGathering,
		"registrations": registrations,
		"groups":        groups,
		"serving":       assignments,
		"nextSteps":     nextSteps,
		"announcements": announcements,
		"activity":      activity,
	})
}

func (h *Handler) memberNextGathering(r *http.Request, organizationID, branchID string, now time.Time) any {
	var value bson.M
	err := h.DB.Collection("chms_service_occurrences").FindOne(r.Context(), bson.M{"organizationId": organizationID, "homeBranchId": branchID, "status": "scheduled", "startsAt": bson.M{"$gte": now}}, options.FindOne().SetProjection(bson.M{"name": 1, "startsAt": 1, "endsAt": 1, "timezone": 1, "roomIds": 1, "homeBranchId": 1}).SetSort(bson.D{{Key: "startsAt", Value: 1}})).Decode(&value)
	if err != nil {
		return nil
	}
	return models.Normalize(value)
}

func (h *Handler) memberAssignments(r *http.Request, organizationID, personID string, now time.Time) []bson.M {
	cursor, err := h.DB.Collection("chms_assignments").Find(r.Context(), bson.M{"organizationId": organizationID, "personId": personID, "endsAt": bson.M{"$gte": now.Add(-24 * time.Hour)}, "status": bson.M{"$in": bson.A{"invited", "accepted", "substitute-requested"}}}, options.Find().SetProjection(bson.M{"planId": 1, "teamId": 1, "positionId": 1, "startsAt": 1, "endsAt": 1, "status": 1, "slot": 1, "version": 1}).SetSort(bson.D{{Key: "startsAt", Value: 1}}).SetLimit(8))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(r.Context())
	var values []bson.M
	if cursor.All(r.Context(), &values) != nil {
		return []bson.M{}
	}
	for index := range values {
		values[index]["teamName"] = h.memberResourceName(r, "chms_volunteer_teams", organizationID, stringValue(values[index]["teamId"]))
		values[index]["positionName"] = h.memberResourceName(r, "chms_volunteer_positions", organizationID, stringValue(values[index]["positionId"]))
	}
	return normalizeMaps(values)
}

func (h *Handler) memberGroups(r *http.Request, organizationID, personID string) []bson.M {
	cursor, err := h.DB.Collection("chms_group_memberships").Find(r.Context(), bson.M{"organizationId": organizationID, "personId": personID, "status": bson.M{"$in": bson.A{"active", "waitlisted", "applied", "requested"}}}, options.Find().SetProjection(bson.M{"groupId": 1, "role": 1, "status": 1, "joinedAt": 1, "requestedAt": 1}).SetLimit(12))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(r.Context())
	var memberships []bson.M
	if cursor.All(r.Context(), &memberships) != nil {
		return []bson.M{}
	}
	items := make([]bson.M, 0, len(memberships))
	for _, membership := range memberships {
		groupID := stringValue(membership["groupId"])
		var group bson.M
		if h.DB.Collection("chms_groups").FindOne(r.Context(), bson.M{"_id": groupID, "organizationId": organizationID, "status": bson.M{"$ne": "closed"}}, options.FindOne().SetProjection(bson.M{"name": 1, "type": 1, "meetingPattern": 1})).Decode(&group) != nil {
			continue
		}
		items = append(items, bson.M{"id": groupID, "name": group["name"], "type": group["type"], "meetingPattern": group["meetingPattern"], "role": membership["role"], "status": membership["status"], "joinedAt": membership["joinedAt"]})
	}
	return normalizeMaps(items)
}

func (h *Handler) memberRegistrations(r *http.Request, email string, now time.Time) []bson.M {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return []bson.M{}
	}
	cursor, err := h.DB.Collection("event_registrations").Find(r.Context(), bson.M{"$or": bson.A{bson.M{"normalizedEmail": normalizedEmail}, bson.M{"email": email}}}, options.Find().SetProjection(bson.M{"eventId": 1, "createdAt": 1}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(12))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(r.Context())
	var registrations []bson.M
	if cursor.All(r.Context(), &registrations) != nil {
		return []bson.M{}
	}
	items := []bson.M{}
	for _, registration := range registrations {
		eventID := stringValue(registration["eventId"])
		objectID, err := bson.ObjectIDFromHex(eventID)
		if err != nil {
			continue
		}
		var event bson.M
		filter := publicContentFilter()
		filter["_id"], filter["startAt"] = objectID, bson.M{"$gte": now.Add(-24 * time.Hour)}
		if h.DB.Collection("events").FindOne(r.Context(), filter, options.FindOne().SetProjection(bson.M{"title": 1, "slug": 1, "startAt": 1, "endAt": 1, "location": 1, "imageUrl": 1})).Decode(&event) != nil {
			continue
		}
		event["registeredAt"], event["registrationId"] = registration["createdAt"], normalizeID(registration["_id"])
		items = append(items, event)
	}
	sort.Slice(items, func(i, j int) bool { return timeValue(items[i]["startAt"]).Before(timeValue(items[j]["startAt"])) })
	return normalizeMaps(items)
}

func (h *Handler) memberAnnouncements(r *http.Request, now time.Time) []bson.M {
	filter := publicContentFilter()
	filter["publishAt"] = bson.M{"$lte": now}
	filter["$or"] = bson.A{bson.M{"expiresAt": nil}, bson.M{"expiresAt": bson.M{"$gt": now}}}
	cursor, err := h.DB.Collection("announcements").Find(r.Context(), filter, options.Find().SetProjection(bson.M{"title": 1, "body": 1, "slug": 1, "publishAt": 1, "expiresAt": 1}).SetSort(bson.D{{Key: "publishAt", Value: -1}}).SetLimit(5))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(r.Context())
	var values []bson.M
	if cursor.All(r.Context(), &values) != nil {
		return []bson.M{}
	}
	return normalizeMaps(values)
}

func (h *Handler) memberBranchName(r *http.Request, branchID string) string {
	objectID, err := bson.ObjectIDFromHex(branchID)
	if err != nil {
		return ""
	}
	var branch bson.M
	if h.DB.Collection("branches").FindOne(r.Context(), bson.M{"_id": objectID}, options.FindOne().SetProjection(bson.M{"name": 1})).Decode(&branch) != nil {
		return ""
	}
	return stringValue(branch["name"])
}

func (h *Handler) memberResourceName(r *http.Request, collection, organizationID, id string) string {
	var value bson.M
	if h.DB.Collection(collection).FindOne(r.Context(), bson.M{"_id": id, "organizationId": organizationID}, options.FindOne().SetProjection(bson.M{"name": 1})).Decode(&value) != nil {
		return ""
	}
	return stringValue(value["name"])
}

func memberNextSteps(stage string) []bson.M {
	steps := []bson.M{{"id": "visit", "title": "Plan a visit", "complete": true}, {"id": "attend", "title": "Join us on Sunday", "complete": stage != "guest"}, {"id": "welcome", "title": "Attend Welcome Lunch", "complete": stage == "member" || stage == "serving-member" || stage == "leader"}, {"id": "connect", "title": "Meet your community guide", "complete": stage == "serving-member" || stage == "leader"}}
	return steps
}

func memberActivity(assignments, groups, registrations []bson.M) []bson.M {
	items := []bson.M{}
	for _, value := range registrations {
		items = append(items, bson.M{"type": "registration", "title": "Registered for " + stringValue(value["title"]), "occurredAt": value["registeredAt"]})
	}
	for _, value := range groups {
		if value["joinedAt"] != nil {
			items = append(items, bson.M{"type": "group", "title": "Joined " + stringValue(value["name"]), "occurredAt": value["joinedAt"]})
		}
	}
	for _, value := range assignments {
		if stringValue(value["status"]) == "accepted" {
			items = append(items, bson.M{"type": "serving", "title": "Serving with " + stringValue(value["teamName"]), "occurredAt": value["startsAt"]})
		}
	}
	sort.Slice(items, func(i, j int) bool { return timeValue(items[i]["occurredAt"]).After(timeValue(items[j]["occurredAt"])) })
	if len(items) > 8 {
		items = items[:8]
	}
	return items
}

func homeMemberName(value any, fallback string) string {
	names, ok := value.(bson.M)
	if !ok {
		return fallback
	}
	if preferred := stringValue(names["preferred"]); preferred != "" {
		return preferred
	}
	return strings.TrimSpace(strings.Join([]string{stringValue(names["given"]), stringValue(names["family"])}, " "))
}

func normalizeMaps(values []bson.M) []bson.M {
	if values == nil {
		return []bson.M{}
	}
	for index := range values {
		values[index] = models.Normalize(values[index])
	}
	return values
}

func normalizeID(value any) any {
	if id, ok := value.(bson.ObjectID); ok {
		return id.Hex()
	}
	return value
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if id, ok := value.(bson.ObjectID); ok {
		return id.Hex()
	}
	if value, ok := value.(string); ok {
		return value
	}
	return ""
}

func timeValue(value any) time.Time {
	if value, ok := value.(time.Time); ok {
		return value
	}
	if value, ok := value.(bson.DateTime); ok {
		return value.Time()
	}
	return time.Time{}
}
