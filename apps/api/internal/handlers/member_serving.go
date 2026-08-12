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
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/models"
	"remi-api/internal/services"
)

func (h *Handler) GetMemberServingWorkspace(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	now := time.Now().UTC()
	personID := platform.ID(claims.PersonID)
	branchID := h.memberPersonBranch(r.Context(), personID)
	profile := bson.M{"version": int64(0), "skills": bson.A{}, "preferredTeamIds": bson.A{}, "preferredPositionIds": bson.A{}, "status": "active"}
	var stored bson.M
	if h.DB.Collection("chms_volunteer_profiles").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID}, options.FindOne().SetProjection(bson.M{"skills": 1, "preferredTeamIds": 1, "preferredPositionIds": 1, "status": 1, "version": 1})).Decode(&stored) == nil {
		profile = models.Normalize(stored)
	}
	teams := h.memberServingTeams(r.Context(), branchID)
	availability := h.memberServingAvailability(r.Context(), personID, now)
	assignments := h.memberServingAssignments(r.Context(), personID, availability, now)
	httpx.JSON(w, 200, bson.M{"generatedAt": now, "profile": profile, "teams": teams, "availability": availability, "assignments": assignments, "policy": bson.M{"version": "member-serving-v1", "preferences": "Serving interests never change screening or safeguarding eligibility. Coordinators make final assignments.", "availability": "Availability guides scheduling and does not guarantee an assignment.", "substitutes": "A substitute request alerts coordinators; it never transfers the assignment directly.", "checkIn": "Self check-in opens two hours before service and closes four hours after it begins. It records arrival, not attendance or completion."}})
}
func (h *Handler) UpdateMemberServingPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input struct {
		ExpectedVersion      int64         `json:"expectedVersion"`
		Skills               []string      `json:"skills"`
		PreferredTeamIDs     []platform.ID `json:"preferredTeamIds"`
		PreferredPositionIDs []platform.ID `json:"preferredPositionIds"`
		Status               string        `json:"status"`
	}
	if err := platform.DecodeJSON(w, r, &input, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if input.Status != "active" && input.Status != "paused" {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "status", Code: "invalid", Message: "Choose active or paused."}))
		return
	}
	input.Skills = cleanServingSlugs(input.Skills, 20)
	if len(input.PreferredTeamIDs) > 20 || len(input.PreferredPositionIDs) > 40 {
		httpx.Error(w, 400, "too many serving preferences")
		return
	}
	branch := h.memberPersonBranch(r.Context(), platform.ID(claims.PersonID))
	if !h.validMemberServingChoices(r.Context(), branch, input.PreferredTeamIDs, input.PreferredPositionIDs) {
		httpx.Error(w, 400, "choose active serving options from your branch")
		return
	}
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID}
	var current bson.M
	found := h.DB.Collection("chms_volunteer_profiles").FindOne(r.Context(), filter).Decode(&current) == nil
	if found && numericInt(current["version"]) != int(input.ExpectedVersion) || !found && input.ExpectedVersion != 0 {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Serving preferences changed. Refresh and try again."})
		return
	}
	id := platform.ID(bson.NewObjectID().Hex())
	if found {
		id = platform.ID(fmt.Sprint(current["_id"]))
	}
	set := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "branchId": branch, "personId": claims.PersonID, "skills": input.Skills, "preferredTeamIds": input.PreferredTeamIDs, "preferredPositionIds": input.PreferredPositionIDs, "status": input.Status, "schemaVersion": 1, "version": input.ExpectedVersion + 1, "updatedAt": now, "updatedBy": actor}
	update := bson.M{"$set": set}
	if !found {
		update["$setOnInsert"] = bson.M{"_id": id, "createdAt": now, "createdBy": actor}
	}
	result, err := h.DB.Collection("chms_volunteer_profiles").UpdateOne(r.Context(), func() bson.M {
		if found {
			return bson.M{"_id": current["_id"], "version": input.ExpectedVersion}
		}
		return filter
	}(), update, options.UpdateOne().SetUpsert(!found))
	if err != nil || found && result.ModifiedCount != 1 {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Serving preferences changed. Refresh and try again."})
		return
	}
	h.appendMemberServingAudit(r.Context(), actor, branch, "member.serving-preferences.update", "volunteer-profile", id, []string{"skills", "preferredTeamIds", "preferredPositionIds", "status"}, platform.RequestIDFrom(r.Context()), now)
	httpx.JSON(w, 200, bson.M{"id": id, "version": input.ExpectedVersion + 1, "skills": input.Skills, "preferredTeamIds": input.PreferredTeamIDs, "preferredPositionIds": input.PreferredPositionIDs, "status": input.Status})
}
func (h *Handler) AddMemberServingAvailability(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input struct {
		StartsAt time.Time `json:"startsAt"`
		EndsAt   time.Time `json:"endsAt"`
		State    string    `json:"state"`
	}
	if err := platform.DecodeJSON(w, r, &input, 16<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.StartsAt = input.StartsAt.UTC()
	input.EndsAt = input.EndsAt.UTC()
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	if !input.EndsAt.After(input.StartsAt) || input.EndsAt.Sub(input.StartsAt) > 90*24*time.Hour || input.EndsAt.Before(time.Now().UTC()) || (input.State != "available" && input.State != "preferred" && input.State != "unavailable") {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "availability", Code: "invalid", Message: "Choose a future window up to 90 days and a supported state."}))
		return
	}
	now := time.Now().UTC()
	id := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	branch := h.memberPersonBranch(r.Context(), actor.ID)
	_, err := h.DB.Collection("chms_volunteer_availability").InsertOne(r.Context(), bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": branch, "personId": claims.PersonID, "startsAt": input.StartsAt, "endsAt": input.EndsAt, "state": input.State, "source": "member", "sourceKey": "member-" + string(id), "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor})
	if err != nil {
		httpx.Error(w, 409, "availability could not be added")
		return
	}
	h.appendMemberServingAudit(r.Context(), actor, branch, "member.serving-availability.create", "volunteer-availability", id, []string{"startsAt", "endsAt", "state"}, platform.RequestIDFrom(r.Context()), now)
	httpx.JSON(w, 201, bson.M{"id": id, "startsAt": input.StartsAt, "endsAt": input.EndsAt, "state": input.State, "version": 1})
}
func (h *Handler) CancelMemberServingAvailability(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	id := platform.ID(strings.TrimSpace(chi.URLParam(r, "availabilityId")))
	now := time.Now().UTC()
	result, err := h.DB.Collection("chms_volunteer_availability").UpdateOne(r.Context(), bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "cancelledAt": bson.M{"$in": bson.A{nil}}}, bson.M{"$set": bson.M{"cancelledAt": now, "updatedAt": now, "updatedBy": platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}}, "$inc": bson.M{"version": 1}})
	if err != nil || result.ModifiedCount != 1 {
		httpx.Error(w, 404, "availability not found")
		return
	}
	h.appendMemberServingAudit(r.Context(), platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, h.memberPersonBranch(r.Context(), platform.ID(claims.PersonID)), "member.serving-availability.cancel", "volunteer-availability", id, []string{"cancelledAt"}, platform.RequestIDFrom(r.Context()), now)
	w.WriteHeader(204)
}
func (h *Handler) RequestMemberServingSubstitute(w http.ResponseWriter, r *http.Request) {
	claims, assignment, ok := h.ownMemberAssignment(w, r, []string{"accepted"})
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Reason          string `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &input, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.Reason) < 3 || len(input.Reason) > 500 {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid", Message: "Add a short reason for your coordinator."}))
		return
	}
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	result, err := h.DB.Collection("chms_assignments").UpdateOne(r.Context(), bson.M{"_id": assignment["_id"], "version": input.ExpectedVersion, "status": "accepted", "personId": claims.PersonID}, bson.M{"$set": bson.M{"status": "substitute-requested", "substituteRequestReason": input.Reason, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil || result.ModifiedCount != 1 {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Assignment changed. Refresh and try again."})
		return
	}
	_, _ = h.DB.Collection("chms_assignment_events").InsertOne(r.Context(), bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": h.Cfg.CHMSOrganizationID, "branchId": assignment["branchId"], "assignmentId": assignment["_id"], "planId": assignment["planId"], "fromStatus": "accepted", "toStatus": "substitute-requested", "reasonCode": "member-request", "actor": actor, "requestId": platform.RequestIDFrom(r.Context()), "occurredAt": now})
	h.appendMemberServingAudit(r.Context(), actor, platform.ID(fmt.Sprint(assignment["branchId"])), "member.serving-substitute.request", "volunteer-assignment", platform.ID(fmt.Sprint(assignment["_id"])), []string{"status", "substituteRequestReason"}, platform.RequestIDFrom(r.Context()), now)
	httpx.JSON(w, 202, bson.M{"id": assignment["_id"], "status": "substitute-requested", "version": input.ExpectedVersion + 1})
}
func (h *Handler) CheckInMemberServingAssignment(w http.ResponseWriter, r *http.Request) {
	claims, assignment, ok := h.ownMemberAssignment(w, r, []string{"accepted", "substitute-requested"})
	if !ok {
		return
	}
	now := time.Now().UTC()
	starts := timeValue(assignment["startsAt"])
	if now.Before(starts.Add(-2*time.Hour)) || now.After(starts.Add(4*time.Hour)) {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Check-in opens two hours before serving and closes four hours after it begins."})
		return
	}
	id := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	var existing bson.M
	if h.DB.Collection("chms_serving_checkins").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "assignmentId": assignment["_id"], "personId": claims.PersonID}).Decode(&existing) == nil {
		httpx.JSON(w, http.StatusOK, models.Normalize(existing))
		return
	}
	_, err := h.DB.Collection("chms_serving_checkins").InsertOne(r.Context(), bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": assignment["branchId"], "assignmentId": assignment["_id"], "personId": claims.PersonID, "checkedInAt": now, "source": "member-self-check-in", "schemaVersion": 1, "createdAt": now, "createdBy": actor})
	if err != nil {
		var current bson.M
		if h.DB.Collection("chms_serving_checkins").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "assignmentId": assignment["_id"], "personId": claims.PersonID}).Decode(&current) == nil {
			httpx.JSON(w, 200, models.Normalize(current))
			return
		}
		httpx.Error(w, 409, "check-in could not be recorded")
		return
	}
	h.appendMemberServingAudit(r.Context(), actor, platform.ID(fmt.Sprint(assignment["branchId"])), "member.serving-checkin.create", "serving-checkin", id, []string{"assignmentId", "checkedInAt"}, platform.RequestIDFrom(r.Context()), now)
	httpx.JSON(w, 201, bson.M{"id": id, "assignmentId": assignment["_id"], "checkedInAt": now})
}

func (h *Handler) ownMemberAssignment(w http.ResponseWriter, r *http.Request, statuses []string) (*services.Claims, bson.M, bool) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return nil, nil, false
	}
	id := platform.ID(strings.TrimSpace(chi.URLParam(r, "assignmentId")))
	var value bson.M
	if h.DB.Collection("chms_assignments").FindOne(r.Context(), bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "status": bson.M{"$in": statuses}}).Decode(&value) != nil {
		httpx.Error(w, 404, "assignment not found")
		return nil, nil, false
	}
	return claims, value, true
}
func (h *Handler) memberServingTeams(ctx context.Context, branchID platform.ID) []bson.M {
	cursor, err := h.DB.Collection("chms_volunteer_teams").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "homeBranchId": branchID, "status": "active"}, options.Find().SetProjection(bson.M{"name": 1, "description": 1}).SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var teams []bson.M
	_ = cursor.All(ctx, &teams)
	result := []bson.M{}
	for _, team := range teams {
		positions := []bson.M{}
		c, e := h.DB.Collection("chms_volunteer_positions").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "teamId": fmt.Sprint(team["_id"]), "status": "active"}, options.Find().SetProjection(bson.M{"name": 1, "description": 1}).SetSort(bson.D{{Key: "name", Value: 1}}))
		if e == nil {
			_ = c.All(ctx, &positions)
			c.Close(ctx)
		}
		item := models.Normalize(team)
		item["positions"] = normalizeMaps(positions)
		result = append(result, item)
	}
	return result
}
func (h *Handler) memberServingAvailability(ctx context.Context, personID platform.ID, now time.Time) []bson.M {
	cursor, err := h.DB.Collection("chms_volunteer_availability").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "endsAt": bson.M{"$gte": now}, "cancelledAt": bson.M{"$in": bson.A{nil}}}, options.Find().SetProjection(bson.M{"startsAt": 1, "endsAt": 1, "state": 1, "source": 1, "version": 1}).SetSort(bson.D{{Key: "startsAt", Value: 1}}).SetLimit(50))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	_ = cursor.All(ctx, &rows)
	return normalizeMaps(rows)
}
func (h *Handler) memberServingAssignments(ctx context.Context, personID platform.ID, availability []bson.M, now time.Time) []bson.M {
	cursor, err := h.DB.Collection("chms_assignments").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "endsAt": bson.M{"$gte": now.Add(-6 * time.Hour)}}, options.Find().SetProjection(bson.M{"planId": 1, "teamId": 1, "positionId": 1, "startsAt": 1, "endsAt": 1, "status": 1, "version": 1, "reminder": 1, "substituteRequestReason": 1}).SetSort(bson.D{{Key: "startsAt", Value: 1}}).SetLimit(50))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	_ = cursor.All(ctx, &rows)
	items := []bson.M{}
	for _, row := range rows {
		item := models.Normalize(row)
		item["teamName"] = h.memberServingName(ctx, "chms_volunteer_teams", row["teamId"])
		item["positionName"] = h.memberServingName(ctx, "chms_volunteer_positions", row["positionId"])
		item["planName"] = h.memberServingName(ctx, "chms_service_plans", row["planId"])
		conflicts := []bson.M{}
		for _, window := range availability {
			if fmt.Sprint(window["state"]) == "unavailable" && timeValue(window["startsAt"]).Before(timeValue(row["endsAt"])) && timeValue(window["endsAt"]).After(timeValue(row["startsAt"])) {
				conflicts = append(conflicts, bson.M{"code": "member-unavailable", "message": "This assignment overlaps a time you marked unavailable."})
			}
		}
		item["conflicts"] = conflicts
		var checkin bson.M
		if h.DB.Collection("chms_serving_checkins").FindOne(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "assignmentId": fmt.Sprint(row["_id"]), "personId": personID}, options.FindOne().SetProjection(bson.M{"checkedInAt": 1})).Decode(&checkin) == nil {
			item["checkIn"] = models.Normalize(checkin)
		}
		starts := timeValue(row["startsAt"])
		item["checkInOpen"] = !now.Before(starts.Add(-2*time.Hour)) && !now.After(starts.Add(4*time.Hour))
		items = append(items, item)
	}
	return items
}
func (h *Handler) memberServingName(ctx context.Context, collection string, id any) string {
	var row bson.M
	if h.DB.Collection(collection).FindOne(ctx, bson.M{"_id": fmt.Sprint(id), "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(bson.M{"name": 1})).Decode(&row) == nil {
		return fmt.Sprint(row["name"])
	}
	return ""
}
func (h *Handler) validMemberServingChoices(ctx context.Context, branch platform.ID, teamIDs, positionIDs []platform.ID) bool {
	for _, id := range teamIDs {
		if h.DB.Collection("chms_volunteer_teams").FindOne(ctx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "homeBranchId": branch, "status": "active"}).Err() != nil {
			return false
		}
	}
	for _, id := range positionIDs {
		var p bson.M
		if h.DB.Collection("chms_volunteer_positions").FindOne(ctx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "status": "active"}).Decode(&p) != nil {
			return false
		}
		if h.DB.Collection("chms_volunteer_teams").FindOne(ctx, bson.M{"_id": p["teamId"], "homeBranchId": branch, "status": "active"}).Err() != nil {
			return false
		}
	}
	return true
}
func cleanServingSlugs(values []string, limit int) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, raw := range values {
		v := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(raw)), "-"))
		if v != "" && len(v) <= 60 && !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	if len(result) > limit {
		return result[:limit]
	}
	return result
}
func (h *Handler) appendMemberServingAudit(ctx context.Context, actor platform.Actor, branch platform.ID, action, resource string, id platform.ID, fields []string, requestID string, now time.Time) {
	store, err := platform.NewMongoPlatformStore(h.DB)
	if err == nil {
		_ = store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), BranchID: branch, Actor: actor, Action: action, ResourceType: resource, ResourceID: id, SubjectIDs: []platform.ID{actor.ID}, ChangedFields: fields, Outcome: "success", RequestID: requestID, OccurredAt: now})
	}
}
