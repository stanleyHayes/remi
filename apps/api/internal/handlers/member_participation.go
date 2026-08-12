package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/models"
)

func (h *Handler) GetMemberParticipation(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	now := time.Now().UTC()
	h.releaseExpiredMemberEventPayments(r.Context(), now)
	people, _ := h.memberRegistrationPeople(r.Context(), platform.ID(claims.PersonID), platform.ID(claims.HouseholdID))
	httpx.JSON(w, 200, bson.M{
		"generatedAt":        now,
		"registrationPeople": people,
		"registrations":      h.memberRegistrationItems(r.Context(), claims.PersonID, claims.Email),
		"availableEvents":    h.memberAvailableEvents(r.Context(), now),
		"groups":             h.memberGroupItems(r.Context(), claims.PersonID),
		"attendance":         h.memberAttendanceItems(r.Context(), claims.PersonID),
		"nextSteps":          h.memberPathwayProjection(r.Context(), platform.ID(claims.PersonID)),
		"childPrecheck":      h.memberPrecheckProjection(r.Context(), platform.ID(claims.PersonID), platform.ID(claims.HouseholdID), now),
		"attendancePolicy":   bson.M{"version": "member-attendance-v1", "visibleStatuses": []string{"present"}, "statement": "Your history shows confirmed participation only. Absences, staff corrections, confidence, source data and pastoral notes remain private to authorized teams.", "corrections": "Ask a church leader if a confirmed visit is missing; members cannot alter attendance records."},
	})
}

func (h *Handler) memberRegistrationPeople(ctx context.Context, personID, householdID platform.ID) ([]bson.M, error) {
	ids := []platform.ID{personID}
	if householdID.Valid() {
		cursor, err := h.DB.Collection("chms_household_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "householdId": householdID, "endedAt": bson.M{"$in": bson.A{nil}}}, options.Find().SetProjection(bson.M{"personId": 1}))
		if err != nil {
			return nil, err
		}
		defer cursor.Close(ctx)
		var rows []struct {
			PersonID platform.ID `bson:"personId"`
		}
		if err := cursor.All(ctx, &rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.PersonID.Valid() {
				ids = append(ids, row.PersonID)
			}
		}
	}
	seen, result := map[platform.ID]bool{}, []bson.M{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		person, err := h.delegationPerson(ctx, id)
		if err != nil {
			continue
		}
		result = append(result, bson.M{"id": id, "name": displayDelegationName(person), "self": id == personID})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i]["self"] == true {
			return true
		}
		if result[j]["self"] == true {
			return false
		}
		return fmt.Sprint(result[i]["name"]) < fmt.Sprint(result[j]["name"])
	})
	return result, nil
}

func (h *Handler) CreateMemberRegistration(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input struct {
		EventID   string         `json:"eventId"`
		PersonIDs []platform.ID  `json:"personIds"`
		Answers   map[string]any `json:"answers"`
	}
	if err := platform.DecodeJSON(w, r, &input, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	now := time.Now().UTC()
	h.releaseExpiredMemberEventPayments(r.Context(), now)
	eventID, err := bson.ObjectIDFromHex(strings.TrimSpace(input.EventID))
	if err != nil {
		httpx.Error(w, 400, "choose an available event")
		return
	}
	people, err := h.memberRegistrationPeople(r.Context(), platform.ID(claims.PersonID), platform.ID(claims.HouseholdID))
	if err != nil {
		httpx.Error(w, 500, "registration unavailable")
		return
	}
	allowed := map[platform.ID]string{}
	for _, person := range people {
		allowed[person["id"].(platform.ID)] = fmt.Sprint(person["name"])
	}
	selected, seen := []platform.ID{}, map[platform.ID]bool{}
	for _, id := range input.PersonIDs {
		if _, exists := allowed[id]; !exists {
			httpx.Error(w, 404, "household member not found")
			return
		}
		if !seen[id] {
			seen[id] = true
			selected = append(selected, id)
		}
	}
	if len(selected) == 0 || len(selected) > 10 {
		httpx.Error(w, 400, "choose between one and ten household members")
		return
	}
	filter := publicContentFilter()
	filter["_id"], filter["registrationEnabled"], filter["startAt"] = eventID, true, bson.M{"$gt": now}
	var event bson.M
	if h.DB.Collection("events").FindOne(r.Context(), filter).Decode(&event) != nil {
		httpx.Error(w, 404, "event not found")
		return
	}
	answers, answerErr := validateMemberEventAnswers(event["registrationQuestions"], input.Answers)
	if answerErr != nil {
		platform.WriteError(w, r, answerErr)
		return
	}
	for _, id := range selected {
		if h.DB.Collection("event_registrations").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "eventId": input.EventID, "personId": id, "state": bson.M{"$in": bson.A{"confirmed", "waitlisted", "pending-payment"}}}).Err() == nil {
			httpx.Error(w, 409, allowed[id]+" is already registered")
			return
		}
	}
	paymentRequired, _ := event["paymentRequired"].(bool)
	priceMinor := int64(numericInt(event["priceMinor"]))
	currency := strings.ToUpper(strings.TrimSpace(fmt.Sprint(event["currency"])))
	if paymentRequired && (priceMinor <= 0 || priceMinor > 100_000_000 || currency == "" || currency == "<NIL>") {
		httpx.Error(w, 409, "This event's payment configuration is incomplete.")
		return
	}
	bookingID := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	created := make([]bson.M, 0, len(selected))
	registrationState := "confirmed"
	if paymentRequired {
		registrationState = "pending-payment"
	}
	err = store.WithTransaction(r.Context(), func(tx context.Context) error {
		capacity := numericInt(event["capacity"])
		eventFilter := bson.M{"_id": eventID, "registrationEnabled": true}
		if capacity > 0 {
			eventFilter["$expr"] = bson.M{"$lte": bson.A{bson.M{"$add": bson.A{bson.M{"$ifNull": bson.A{"$registeredCount", 0}}, len(selected)}}, capacity}}
		}
		result, err := h.DB.Collection("events").UpdateOne(tx, eventFilter, bson.M{"$inc": bson.M{"registeredCount": len(selected)}})
		if err != nil {
			return err
		}
		if result.ModifiedCount != 1 {
			if waitlist, _ := event["waitlistEnabled"].(bool); waitlist {
				registrationState = "waitlisted"
			} else {
				return &platform.DomainError{Code: "conflict", Message: "The event no longer has enough available places."}
			}
		}
		for _, personID := range selected {
			id := bson.NewObjectID()
			value := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "eventId": input.EventID, "bookingId": bookingID, "personId": personID, "registeredByPersonId": claims.PersonID, "name": allowed[personID], "email": claims.Email, "normalizedEmail": strings.ToLower(strings.TrimSpace(claims.Email)), "state": registrationState, "activeKey": true, "answers": answers, "source": "member-app", "createdAt": now, "updatedAt": now}
			if registrationState == "waitlisted" {
				value["waitlistedAt"] = now
			} else if registrationState == "pending-payment" {
				value["paymentExpiresAt"] = now.Add(20 * time.Minute)
			}
			if _, err := h.DB.Collection("event_registrations").InsertOne(tx, value); err != nil {
				return err
			}
			created = append(created, bson.M{"id": id.Hex(), "personId": personID, "personName": allowed[personID], "state": registrationState})
			if err := store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: actor, Action: "member.registration.create", ResourceType: "event-registration", ResourceID: platform.ID(id.Hex()), SubjectIDs: []platform.ID{personID}, ChangedFields: []string{"eventId", "personId", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	response := bson.M{"items": created, "eventId": input.EventID, "state": registrationState}
	if registrationState == "pending-payment" {
		response["payment"] = bson.M{"bookingId": bookingID, "amountMinor": priceMinor * int64(len(selected)), "currency": currency, "expiresAt": now.Add(20 * time.Minute)}
	}
	httpx.JSON(w, 201, response)
}

func (h *Handler) CancelMemberRegistration(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	id, err := bson.ObjectIDFromHex(chi.URLParam(r, "registrationId"))
	if err != nil {
		httpx.Error(w, 404, "registration not found")
		return
	}
	filter := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "state": bson.M{"$in": bson.A{"confirmed", "waitlisted", "pending-payment"}}, "$or": bson.A{bson.M{"personId": claims.PersonID}, bson.M{"registeredByPersonId": claims.PersonID}}}
	var registration bson.M
	if h.DB.Collection("event_registrations").FindOne(r.Context(), filter).Decode(&registration) != nil {
		httpx.Error(w, 404, "registration not found")
		return
	}
	now := time.Now().UTC()
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err = store.WithTransaction(r.Context(), func(tx context.Context) error {
		result, err := h.DB.Collection("event_registrations").UpdateOne(tx, filter, bson.M{"$set": bson.M{"state": "cancelled", "activeKey": false, "cancelledAt": now, "updatedAt": now}})
		if err != nil {
			return err
		}
		if result.ModifiedCount != 1 {
			return &platform.DomainError{Code: "conflict", Message: "Registration has already changed."}
		}
		eventID, e := bson.ObjectIDFromHex(fmt.Sprint(registration["eventId"]))
		if e == nil && (fmt.Sprint(registration["state"]) == "confirmed" || fmt.Sprint(registration["state"]) == "pending-payment") {
			var event bson.M
			_ = h.DB.Collection("events").FindOne(tx, bson.M{"_id": eventID}, options.FindOne().SetProjection(bson.M{"paymentRequired": 1})).Decode(&event)
			promotionState := "confirmed"
			promotion := bson.M{"state": promotionState, "promotedAt": now, "updatedAt": now}
			if required, _ := event["paymentRequired"].(bool); required {
				promotionState = "pending-payment"
				promotion["state"] = promotionState
				promotion["bookingId"] = platform.ID(bson.NewObjectID().Hex())
				promotion["paymentExpiresAt"] = now.Add(memberEventPaymentWindow)
			}
			var promoted bson.M
			promotionErr := h.DB.Collection("event_registrations").FindOneAndUpdate(tx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "eventId": fmt.Sprint(registration["eventId"]), "state": "waitlisted"}, bson.M{"$set": promotion}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "waitlistedAt", Value: 1}, {Key: "_id", Value: 1}}).SetReturnDocument(options.After)).Decode(&promoted)
			if promotionErr == nil {
				if auditErr := store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorSystem, ID: "event-waitlist"}, Action: "member.registration.promote", ResourceType: "event-registration", ResourceID: platform.ID(fmt.Sprint(normalizeID(promoted["_id"]))), ChangedFields: []string{"state", "promotedAt"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now}); auditErr != nil {
					return auditErr
				}
			} else if promotionErr == mongo.ErrNoDocuments {
				_, err = h.DB.Collection("events").UpdateOne(tx, bson.M{"_id": eventID, "registeredCount": bson.M{"$gt": 0}}, bson.M{"$inc": bson.M{"registeredCount": -1}})
			} else {
				return promotionErr
			}
			if err != nil {
				return err
			}
			if fmt.Sprint(registration["state"]) == "pending-payment" {
				_, err = h.DB.Collection("chms_event_registration_payments").UpdateOne(tx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "bookingId": registration["bookingId"], "state": bson.M{"$ne": "paid"}}, bson.M{"$set": bson.M{"state": "cancelled", "cancelledAt": now, "updatedAt": now}})
				if err != nil {
					return err
				}
			}
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, Action: "member.registration.cancel", ResourceType: "event-registration", ResourceID: platform.ID(id.Hex()), SubjectIDs: []platform.ID{platform.ID(fmt.Sprint(registration["personId"]))}, ChangedFields: []string{"state", "cancelledAt"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) memberRegistrationItems(ctx context.Context, personID, email string) []bson.M {
	filter := bson.M{"$or": bson.A{bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "$or": bson.A{bson.M{"personId": personID}, bson.M{"registeredByPersonId": personID}}}, bson.M{"normalizedEmail": strings.ToLower(strings.TrimSpace(email)), "organizationId": bson.M{"$exists": false}}}}
	cursor, err := h.DB.Collection("event_registrations").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(50))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	if cursor.All(ctx, &rows) != nil {
		return []bson.M{}
	}
	items := []bson.M{}
	for _, row := range rows {
		eventID, e := bson.ObjectIDFromHex(fmt.Sprint(row["eventId"]))
		if e != nil {
			continue
		}
		var event bson.M
		if h.DB.Collection("events").FindOne(ctx, bson.M{"_id": eventID}, options.FindOne().SetProjection(bson.M{"title": 1, "slug": 1, "startAt": 1, "endAt": 1, "location": 1, "imageUrl": 1, "priceMinor": 1, "currency": 1, "paymentRequired": 1})).Decode(&event) != nil {
			continue
		}
		item := bson.M{"id": normalizeID(row["_id"]), "personId": row["personId"], "personName": row["name"], "state": defaultStringValue(fmt.Sprint(row["state"]), "confirmed"), "registeredAt": row["createdAt"], "event": models.Normalize(event)}
		if fmt.Sprint(row["state"]) == "pending-payment" {
			item["payment"] = bson.M{"bookingId": row["bookingId"], "expiresAt": row["paymentExpiresAt"]}
		}
		items = append(items, item)
	}
	return items
}

func (h *Handler) memberAvailableEvents(ctx context.Context, now time.Time) []bson.M {
	filter := publicContentFilter()
	filter["registrationEnabled"], filter["startAt"] = true, bson.M{"$gt": now}
	cursor, err := h.DB.Collection("events").Find(ctx, filter, options.Find().SetProjection(bson.M{"title": 1, "slug": 1, "startAt": 1, "endAt": 1, "location": 1, "imageUrl": 1, "capacity": 1, "registeredCount": 1, "waitlistEnabled": 1, "registrationQuestions": 1, "priceMinor": 1, "currency": 1, "paymentRequired": 1, "childPrecheckEnabled": 1, "nextStepType": 1}).SetSort(bson.D{{Key: "startAt", Value: 1}}).SetLimit(30))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var values []bson.M
	if cursor.All(ctx, &values) != nil {
		return []bson.M{}
	}
	return normalizeMaps(values)
}

func (h *Handler) memberGroupItems(ctx context.Context, personID string) bson.M {
	cursor, err := h.DB.Collection("chms_group_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "status": bson.M{"$nin": bson.A{"ended", "declined"}}}, options.Find().SetProjection(bson.M{"groupId": 1, "role": 1, "status": 1, "joinedAt": 1, "requestedAt": 1, "version": 1}))
	if err != nil {
		return bson.M{"memberships": []bson.M{}, "discoverable": []bson.M{}}
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	_ = cursor.All(ctx, &rows)
	memberships := []bson.M{}
	memberIDs := bson.A{}
	for _, row := range rows {
		groupID := fmt.Sprint(row["groupId"])
		memberIDs = append(memberIDs, groupID)
		var group bson.M
		if h.DB.Collection("chms_groups").FindOne(ctx, bson.M{"_id": groupID, "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(memberGroupProjection())).Decode(&group) == nil {
			memberships = append(memberships, bson.M{"membershipId": normalizeID(row["_id"]), "membershipStatus": row["status"], "role": row["role"], "version": row["version"], "group": models.Normalize(group)})
		}
	}
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "status": "active", "discoverability": bson.M{"$in": bson.A{"members", "public"}}}
	if branchID := h.memberPersonBranch(ctx, platform.ID(personID)); branchID.Valid() {
		filter["homeBranchId"] = branchID
	}
	if len(memberIDs) > 0 {
		filter["_id"] = bson.M{"$nin": memberIDs}
	}
	discover, err := h.DB.Collection("chms_groups").Find(ctx, filter, options.Find().SetProjection(memberGroupProjection()).SetSort(bson.D{{Key: "name", Value: 1}}).SetLimit(50))
	available := []bson.M{}
	if err == nil {
		defer discover.Close(ctx)
		_ = discover.All(ctx, &available)
	}
	return bson.M{"memberships": memberships, "discoverable": normalizeMaps(available)}
}
func memberGroupProjection() bson.M {
	return bson.M{"name": 1, "type": 1, "description": 1, "homeBranchId": 1, "capacity": 1, "activeMemberCount": 1, "meetingPattern": 1, "privacy": 1, "status": 1}
}

func (h *Handler) memberPersonBranch(ctx context.Context, personID platform.ID) platform.ID {
	var person struct {
		HomeBranchID platform.ID `bson:"homeBranchId"`
	}
	if h.DB.Collection("chms_people").FindOne(ctx, bson.M{"_id": personID, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": bson.M{"$in": bson.A{nil}}}, options.FindOne().SetProjection(bson.M{"homeBranchId": 1})).Decode(&person) != nil {
		return ""
	}
	return person.HomeBranchID
}

func (h *Handler) JoinMemberGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	groupID := platform.ID(strings.TrimSpace(chi.URLParam(r, "groupId")))
	if !groupID.Valid() {
		httpx.Error(w, 404, "group not found")
		return
	}
	var group bson.M
	if h.DB.Collection("chms_groups").FindOne(r.Context(), bson.M{"_id": groupID, "organizationId": h.Cfg.CHMSOrganizationID, "homeBranchId": h.memberPersonBranch(r.Context(), platform.ID(claims.PersonID)), "status": "active", "discoverability": bson.M{"$in": bson.A{"members", "public"}}, "privacy": bson.M{"$ne": "invite-only"}}).Decode(&group) != nil {
		httpx.Error(w, 404, "group not found")
		return
	}
	var previous bson.M
	previousErr := h.DB.Collection("chms_group_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": claims.PersonID}).Decode(&previous)
	if previousErr == nil && fmt.Sprint(previous["status"]) != "ended" && fmt.Sprint(previous["status"]) != "declined" {
		httpx.Error(w, 409, "You already have a current relationship with this group.")
		return
	}
	now := time.Now().UTC()
	status := "applied"
	if fmt.Sprint(group["privacy"]) == "open" {
		status = "active"
	}
	id := platform.ID(bson.NewObjectID().Hex())
	if previousErr == nil {
		id = platform.ID(fmt.Sprint(previous["_id"]))
	}
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		if status == "active" {
			capacity := numericInt(group["capacity"])
			filter := bson.M{"_id": groupID, "organizationId": h.Cfg.CHMSOrganizationID}
			if capacity > 0 {
				filter["$expr"] = bson.M{"$lt": bson.A{bson.M{"$ifNull": bson.A{"$activeMemberCount", 0}}, capacity}}
			}
			result, err := h.DB.Collection("chms_groups").UpdateOne(tx, filter, bson.M{"$inc": bson.M{"activeMemberCount": 1}})
			if err != nil {
				return err
			}
			if result.ModifiedCount != 1 {
				status = "waitlisted"
			}
		}
		value := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": group["homeBranchId"], "groupId": groupID, "personId": claims.PersonID, "role": "member", "status": status, "source": "member-self-service", "directoryVisibility": "hidden", "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor}
		if status == "active" {
			value["joinedAt"] = now
		} else if status == "waitlisted" {
			value["waitlistedAt"] = now
		} else {
			value["requestedAt"] = now
		}
		if previousErr == nil {
			set := bson.M{"status": status, "role": "member", "source": "member-self-service", "directoryVisibility": "hidden", "lastReason": "Member rejoined group", "endedAt": nil, "updatedAt": now, "updatedBy": actor, "requestedAt": nil, "waitlistedAt": nil, "joinedAt": nil}
			if status == "active" {
				set["joinedAt"] = now
			} else if status == "waitlisted" {
				set["waitlistedAt"] = now
			} else {
				set["requestedAt"] = now
			}
			result, err := h.DB.Collection("chms_group_memberships").UpdateOne(tx, bson.M{"_id": previous["_id"], "version": previous["version"], "status": previous["status"]}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
			if err != nil {
				return err
			}
			if result.ModifiedCount != 1 {
				return &platform.DomainError{Code: "conflict", Message: "Group membership has changed."}
			}
		} else if _, err := h.DB.Collection("chms_group_memberships").InsertOne(tx, value); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return &platform.DomainError{Code: "conflict", Message: "A group relationship already exists."}
			}
			return err
		}
		fromStatus := ""
		if previousErr == nil {
			fromStatus = fmt.Sprint(previous["status"])
		}
		if _, err := h.DB.Collection("chms_group_membership_events").InsertOne(tx, bson.M{"_id": bson.NewObjectID().Hex(), "organizationId": h.Cfg.CHMSOrganizationID, "branchId": group["homeBranchId"], "groupId": groupID, "membershipId": id, "personId": claims.PersonID, "fromStatus": fromStatus, "toStatus": status, "role": "member", "reason": defaultStringValue(map[bool]string{true: "Member rejoined group"}[previousErr == nil], ""), "actor": actor, "requestId": platform.RequestIDFrom(r.Context()), "occurredAt": now}); err != nil {
			return err
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), BranchID: platform.ID(fmt.Sprint(group["homeBranchId"])), Actor: actor, Action: "member.group-membership.join", ResourceType: "group-membership", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"groupId", "status", "role"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, 201, bson.M{"id": id, "groupId": groupID, "status": status, "role": "member"})
}

func (h *Handler) LeaveMemberGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	groupID := platform.ID(strings.TrimSpace(chi.URLParam(r, "groupId")))
	var membership bson.M
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": claims.PersonID, "status": bson.M{"$in": bson.A{"active", "applied", "requested", "waitlisted"}}}
	if h.DB.Collection("chms_group_memberships").FindOne(r.Context(), filter).Decode(&membership) != nil {
		httpx.Error(w, 404, "group membership not found")
		return
	}
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		result, err := h.DB.Collection("chms_group_memberships").UpdateOne(tx, bson.M{"_id": membership["_id"], "version": membership["version"], "status": membership["status"]}, bson.M{"$set": bson.M{"status": "ended", "endedAt": now, "lastReason": "Member left group", "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
		if err != nil {
			return err
		}
		if result.ModifiedCount != 1 {
			return &platform.DomainError{Code: "conflict", Message: "Group membership has changed."}
		}
		if fmt.Sprint(membership["status"]) == "active" {
			_, err = h.DB.Collection("chms_groups").UpdateOne(tx, bson.M{"_id": groupID, "activeMemberCount": bson.M{"$gt": 0}}, bson.M{"$inc": bson.M{"activeMemberCount": -1}})
			if err != nil {
				return err
			}
		}
		if _, err := h.DB.Collection("chms_group_membership_events").InsertOne(tx, bson.M{"_id": bson.NewObjectID().Hex(), "organizationId": h.Cfg.CHMSOrganizationID, "branchId": membership["branchId"], "groupId": groupID, "membershipId": membership["_id"], "personId": claims.PersonID, "fromStatus": membership["status"], "toStatus": "ended", "role": membership["role"], "reason": "Member left group", "actor": actor, "requestId": platform.RequestIDFrom(r.Context()), "occurredAt": now}); err != nil {
			return err
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: actor, Action: "member.group-membership.leave", ResourceType: "group-membership", ResourceID: platform.ID(fmt.Sprint(membership["_id"])), SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"status", "endedAt"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) memberAttendanceItems(ctx context.Context, personID string) []bson.M {
	items := []bson.M{}
	cursor, err := h.DB.Collection("chms_attendance").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "status": "present"}, options.Find().SetProjection(bson.M{"occurrenceId": 1, "updatedAt": 1}).SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(60))
	if err == nil {
		defer cursor.Close(ctx)
		var rows []bson.M
		_ = cursor.All(ctx, &rows)
		for _, row := range rows {
			var occurrence bson.M
			if h.DB.Collection("chms_service_occurrences").FindOne(ctx, bson.M{"_id": row["occurrenceId"], "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(bson.M{"name": 1, "startsAt": 1, "endsAt": 1, "timezone": 1, "homeBranchId": 1})).Decode(&occurrence) == nil {
				items = append(items, bson.M{"id": "service-" + fmt.Sprint(row["occurrenceId"]), "kind": "service", "title": occurrence["name"], "startsAt": occurrence["startsAt"], "endsAt": occurrence["endsAt"], "status": "confirmed"})
			}
		}
	}
	groupCursor, err := h.DB.Collection("chms_group_attendance").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "status": "present"}, options.Find().SetProjection(bson.M{"groupId": 1, "meetingId": 1}).SetLimit(60))
	if err == nil {
		defer groupCursor.Close(ctx)
		var rows []bson.M
		_ = groupCursor.All(ctx, &rows)
		for _, row := range rows {
			var meeting bson.M
			if h.DB.Collection("chms_group_meetings").FindOne(ctx, bson.M{"_id": row["meetingId"], "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(bson.M{"topic": 1, "startsAt": 1, "endsAt": 1, "timezone": 1})).Decode(&meeting) != nil {
				continue
			}
			var group bson.M
			_ = h.DB.Collection("chms_groups").FindOne(ctx, bson.M{"_id": row["groupId"], "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(bson.M{"name": 1})).Decode(&group)
			items = append(items, bson.M{"id": "group-" + fmt.Sprint(row["meetingId"]), "kind": "group", "title": meeting["topic"], "context": group["name"], "startsAt": meeting["startsAt"], "endsAt": meeting["endsAt"], "status": "confirmed"})
		}
	}
	sort.Slice(items, func(i, j int) bool { return timeValue(items[i]["startsAt"]).After(timeValue(items[j]["startsAt"])) })
	if len(items) > 80 {
		items = items[:80]
	}
	return items
}

func numericInt(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

type memberEventQuestion struct {
	ID       string   `bson:"id"`
	Type     string   `bson:"type"`
	Required bool     `bson:"required"`
	Options  []string `bson:"options"`
}

func validateMemberEventAnswers(raw any, answers map[string]any) (bson.M, error) {
	var questions []memberEventQuestion
	if raw != nil {
		encoded, err := bson.Marshal(bson.M{"items": raw})
		if err != nil {
			return nil, &platform.DomainError{Code: "validation_failed", Message: "Registration form is unavailable."}
		}
		var wrapper struct {
			Items []memberEventQuestion `bson:"items"`
		}
		if bson.Unmarshal(encoded, &wrapper) != nil {
			return nil, &platform.DomainError{Code: "validation_failed", Message: "Registration form is unavailable."}
		}
		questions = wrapper.Items
	}
	allowed := map[string]memberEventQuestion{}
	for _, q := range questions {
		q.ID = strings.TrimSpace(q.ID)
		q.Type = strings.ToLower(strings.TrimSpace(q.Type))
		if q.ID != "" {
			allowed[q.ID] = q
		}
	}
	result := bson.M{}
	for key, value := range answers {
		q, ok := allowed[key]
		if !ok {
			return nil, platform.ValidationError(platform.FieldError{Path: "answers." + key, Code: "unknown", Message: "This registration question is no longer available."})
		}
		switch q.Type {
		case "boolean":
			if _, ok := value.(bool); !ok {
				return nil, platform.ValidationError(platform.FieldError{Path: "answers." + key, Code: "invalid", Message: "Choose yes or no."})
			}
		case "choice":
			selected := strings.TrimSpace(fmt.Sprint(value))
			valid := false
			for _, option := range q.Options {
				if selected == option {
					valid = true
					break
				}
			}
			if !valid {
				return nil, platform.ValidationError(platform.FieldError{Path: "answers." + key, Code: "invalid", Message: "Choose one of the available options."})
			}
			result[key] = selected
		default:
			text := strings.TrimSpace(fmt.Sprint(value))
			if len(text) > 500 {
				return nil, platform.ValidationError(platform.FieldError{Path: "answers." + key, Code: "too_long", Message: "Keep this answer under 500 characters."})
			}
			result[key] = text
		}
	}
	for _, q := range questions {
		if q.Required {
			value, exists := result[q.ID]
			if !exists || strings.TrimSpace(fmt.Sprint(value)) == "" {
				return nil, platform.ValidationError(platform.FieldError{Path: "answers." + q.ID, Code: "required", Message: "Complete this required question."})
			}
		}
	}
	return result, nil
}
