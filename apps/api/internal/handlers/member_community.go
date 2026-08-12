package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/models"
	"remi-api/internal/services"
)

func (h *Handler) GetMemberCommunityGroup(w http.ResponseWriter, r *http.Request) {
	claims, group, membership, ok := h.currentMemberGroup(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	meetings := []bson.M{}
	cursor, err := h.DB.Collection("chms_group_meetings").Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": group["_id"], "startsAt": bson.M{"$gte": now.Add(-30 * 24 * time.Hour), "$lte": now.Add(180 * 24 * time.Hour)}, "status": bson.M{"$ne": "cancelled"}}, options.Find().SetProjection(bson.M{"topic": 1, "startsAt": 1, "endsAt": 1, "timezone": 1, "location": 1, "status": 1}).SetSort(bson.D{{Key: "startsAt", Value: 1}}).SetLimit(50))
	if err == nil {
		defer cursor.Close(r.Context())
		var rows []bson.M
		_ = cursor.All(r.Context(), &rows)
		for _, row := range rows {
			var response bson.M
			_ = h.DB.Collection("chms_group_meeting_responses").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "meetingId": fmt.Sprint(row["_id"]), "personId": claims.PersonID}, options.FindOne().SetProjection(bson.M{"status": 1, "version": 1, "updatedAt": 1})).Decode(&response)
			item := models.Normalize(row)
			item["response"] = models.Normalize(response)
			meetings = append(meetings, item)
		}
	}
	roster := h.memberVisibleGroupRoster(r.Context(), platform.ID(fmt.Sprint(group["_id"])), platform.ID(claims.PersonID))
	leaders := h.memberGroupLeaders(r.Context(), group["leaderPersonIds"])
	httpx.JSON(w, 200, bson.M{"generatedAt": now, "group": models.Normalize(projectMap(group, memberGroupProjection())), "membership": bson.M{"id": normalizeID(membership["_id"]), "role": membership["role"], "status": membership["status"], "directoryVisibility": defaultStringValue(fmt.Sprint(membership["directoryVisibility"]), "hidden"), "version": membership["version"]}, "meetings": meetings, "roster": roster, "leaders": leaders, "privacy": bson.M{"version": "member-community-v1", "roster": "Only active members who chose member visibility appear. Names and profile images only; contact details and leader notes never appear.", "responses": "Your calendar response is planning intent, not an attendance record.", "messages": "Leader messages are queued for current consent, suppression and verified-destination checks."}})
}

func (h *Handler) RespondMemberGroupMeeting(w http.ResponseWriter, r *http.Request) {
	claims, group, _, ok := h.currentMemberGroup(w, r)
	if !ok {
		return
	}
	meetingID := platform.ID(strings.TrimSpace(chi.URLParam(r, "meetingId")))
	var meeting bson.M
	if h.DB.Collection("chms_group_meetings").FindOne(r.Context(), bson.M{"_id": meetingID, "organizationId": h.Cfg.CHMSOrganizationID, "groupId": group["_id"], "startsAt": bson.M{"$gt": time.Now().UTC()}, "status": bson.M{"$ne": "cancelled"}}).Decode(&meeting) != nil {
		httpx.Error(w, 404, "meeting not found")
		return
	}
	var input struct {
		Status          string `json:"status"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := platform.DecodeJSON(w, r, &input, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status != "going" && input.Status != "maybe" && input.Status != "not-going" {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "status", Code: "invalid", Message: "Choose going, maybe or not going."}))
		return
	}
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	id := platform.ID(bson.NewObjectID().Hex())
	nextVersion := int64(1)
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "meetingId": meetingID, "personId": claims.PersonID}
	var current bson.M
	found := h.DB.Collection("chms_group_meeting_responses").FindOne(r.Context(), filter).Decode(&current) == nil
	if found {
		if numericInt(current["version"]) != int(input.ExpectedVersion) {
			platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Your response changed. Refresh and try again."})
			return
		}
		id = platform.ID(fmt.Sprint(current["_id"]))
		nextVersion = int64(numericInt(current["version"]) + 1)
	}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		if found {
			result, e := h.DB.Collection("chms_group_meeting_responses").UpdateOne(tx, bson.M{"_id": current["_id"], "version": input.ExpectedVersion}, bson.M{"$set": bson.M{"status": input.Status, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
			if e != nil {
				return e
			}
			if result.ModifiedCount != 1 {
				return &platform.DomainError{Code: "conflict", Message: "Your response changed. Refresh and try again."}
			}
		} else {
			_, e := h.DB.Collection("chms_group_meeting_responses").InsertOne(tx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": group["homeBranchId"], "groupId": group["_id"], "meetingId": meetingID, "personId": claims.PersonID, "status": input.Status, "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor})
			if e != nil {
				return e
			}
		}
		_, e := h.DB.Collection("chms_group_meeting_response_events").InsertOne(tx, bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": h.Cfg.CHMSOrganizationID, "branchId": group["homeBranchId"], "groupId": group["_id"], "meetingId": meetingID, "personId": claims.PersonID, "status": input.Status, "version": nextVersion, "actor": actor, "requestId": platform.RequestIDFrom(r.Context()), "occurredAt": now})
		if e != nil {
			return e
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), BranchID: platform.ID(fmt.Sprint(group["homeBranchId"])), Actor: actor, Action: "member.group-meeting.respond", ResourceType: "group-meeting-response", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"status"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, 200, bson.M{"id": id, "meetingId": meetingID, "status": input.Status, "version": nextVersion, "updatedAt": now})
}

func (h *Handler) UpdateMemberGroupVisibility(w http.ResponseWriter, r *http.Request) {
	claims, _, membership, ok := h.currentMemberGroup(w, r)
	if !ok {
		return
	}
	var input struct {
		Visibility      string `json:"visibility"`
		ExpectedVersion int64  `json:"expectedVersion"`
	}
	if err := platform.DecodeJSON(w, r, &input, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Visibility = strings.ToLower(strings.TrimSpace(input.Visibility))
	if input.Visibility != "hidden" && input.Visibility != "members" {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "visibility", Code: "invalid", Message: "Choose hidden or members."}))
		return
	}
	if numericInt(membership["version"]) != int(input.ExpectedVersion) {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Group preferences changed. Refresh and try again."})
		return
	}
	now := time.Now().UTC()
	result, err := h.DB.Collection("chms_group_memberships").UpdateOne(r.Context(), bson.M{"_id": membership["_id"], "personId": claims.PersonID, "version": input.ExpectedVersion}, bson.M{"$set": bson.M{"directoryVisibility": input.Visibility, "updatedAt": now, "updatedBy": platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}}, "$inc": bson.M{"version": 1}})
	if err != nil || result.ModifiedCount != 1 {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Group preferences changed. Refresh and try again."})
		return
	}
	httpx.JSON(w, 200, bson.M{"visibility": input.Visibility, "version": input.ExpectedVersion + 1})
}

func (h *Handler) MessageMemberGroupLeaders(w http.ResponseWriter, r *http.Request) {
	claims, group, _, ok := h.currentMemberGroup(w, r)
	if !ok {
		return
	}
	var input struct {
		Message string `json:"message"`
		Channel string `json:"channel"`
	}
	if err := platform.DecodeJSON(w, r, &input, 16<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	if len(input.Message) < 10 || len(input.Message) > 2000 {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "message", Code: "invalid", Message: "Write a message between 10 and 2,000 characters."}))
		return
	}
	if input.Channel != "email" && input.Channel != "sms" && input.Channel != "whatsapp" {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "channel", Code: "invalid", Message: "Choose email, SMS or WhatsApp."}))
		return
	}
	leaders := idsFromAny(group["leaderPersonIds"])
	if len(leaders) == 0 {
		httpx.Error(w, 409, "This group does not have an available leader contact.")
		return
	}
	now := time.Now().UTC()
	id := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		_, e := h.DB.Collection("chms_communication_handoffs").InsertOne(tx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": group["homeBranchId"], "groupId": group["_id"], "requestedByPersonId": claims.PersonID, "audiencePersonIds": leaders, "purpose": "group-operations", "channel": input.Channel, "message": input.Message, "state": "pending-consent-review", "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor})
		if e != nil {
			return e
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), BranchID: platform.ID(fmt.Sprint(group["homeBranchId"])), Actor: actor, Action: "member.group.leader-message-requested", ResourceType: "communication-handoff", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"groupId", "purpose", "channel", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, 202, bson.M{"id": id, "state": "pending-consent-review", "leaderCount": len(leaders)})
}

func (h *Handler) currentMemberGroup(w http.ResponseWriter, r *http.Request) (*services.Claims, bson.M, bson.M, bool) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return nil, nil, nil, false
	}
	groupID := platform.ID(strings.TrimSpace(chi.URLParam(r, "groupId")))
	var membership bson.M
	if h.DB.Collection("chms_group_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": claims.PersonID, "status": "active"}).Decode(&membership) != nil {
		httpx.Error(w, 404, "group not found")
		return nil, nil, nil, false
	}
	var group bson.M
	if h.DB.Collection("chms_groups").FindOne(r.Context(), bson.M{"_id": groupID, "organizationId": h.Cfg.CHMSOrganizationID, "status": "active"}).Decode(&group) != nil {
		httpx.Error(w, 404, "group not found")
		return nil, nil, nil, false
	}
	return claims, group, membership, true
}
func (h *Handler) memberVisibleGroupRoster(ctx context.Context, groupID, viewerID platform.ID) []bson.M {
	cursor, err := h.DB.Collection("chms_group_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "status": "active", "$or": bson.A{bson.M{"personId": viewerID}, bson.M{"directoryVisibility": "members"}}}, options.Find().SetProjection(bson.M{"personId": 1, "role": 1, "directoryVisibility": 1}).SetLimit(200))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	_ = cursor.All(ctx, &rows)
	result := []bson.M{}
	for _, row := range rows {
		var person memberCommunityPerson
		if h.DB.Collection("chms_people").FindOne(ctx, bson.M{"_id": row["personId"], "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": bson.M{"$in": bson.A{nil}}}, options.FindOne().SetProjection(bson.M{"names": 1, "photoAssetId": 1, "customFields.member.directoryVisibility": 1})).Decode(&person) != nil {
			continue
		}
		result = append(result, bson.M{"id": row["personId"], "name": displayCommunityName(person.Names), "role": row["role"], "photoAssetId": person.PhotoAssetID, "self": fmt.Sprint(row["personId"]) == string(viewerID)})
	}
	return result
}
func (h *Handler) memberGroupLeaders(ctx context.Context, value any) []bson.M {
	result := []bson.M{}
	for _, id := range idsFromAny(value) {
		var person memberCommunityPerson
		if h.DB.Collection("chms_people").FindOne(ctx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": bson.M{"$in": bson.A{nil}}}, options.FindOne().SetProjection(bson.M{"names": 1, "photoAssetId": 1})).Decode(&person) == nil {
			result = append(result, bson.M{"id": id, "name": displayCommunityName(person.Names), "photoAssetId": person.PhotoAssetID})
		}
	}
	return result
}
func idsFromAny(value any) []platform.ID {
	result := []platform.ID{}
	switch values := value.(type) {
	case bson.A:
		for _, v := range values {
			if id := platform.ID(fmt.Sprint(v)); id.Valid() {
				result = append(result, id)
			}
		}
	case []platform.ID:
		result = append(result, values...)
	case []any:
		for _, v := range values {
			if id := platform.ID(fmt.Sprint(v)); id.Valid() {
				result = append(result, id)
			}
		}
	}
	return result
}

type memberCommunityPerson struct {
	Names        people.Names `bson:"names"`
	PhotoAssetID platform.ID  `bson:"photoAssetId"`
}

func displayCommunityName(names people.Names) string {
	preferred := strings.TrimSpace(names.Preferred)
	family := strings.TrimSpace(names.Family)
	if preferred != "" {
		return strings.TrimSpace(preferred + " " + family)
	}
	return strings.TrimSpace(names.Given + " " + family)
}
func projectMap(source, projection bson.M) bson.M {
	result := bson.M{"_id": source["_id"]}
	for key := range projection {
		if value, ok := source[key]; ok {
			result[key] = value
		}
	}
	return result
}
