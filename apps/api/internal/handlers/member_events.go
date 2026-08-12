package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
)

func (h *Handler) DownloadMemberRegistrationCalendar(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	id, err := bson.ObjectIDFromHex(strings.TrimSpace(chi.URLParam(r, "registrationId")))
	if err != nil {
		httpx.Error(w, 404, "registration not found")
		return
	}
	var registration bson.M
	if h.DB.Collection("event_registrations").FindOne(r.Context(), bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "state": "confirmed", "$or": bson.A{bson.M{"personId": claims.PersonID}, bson.M{"registeredByPersonId": claims.PersonID}}}).Decode(&registration) != nil {
		httpx.Error(w, 404, "registration not found")
		return
	}
	eventID, err := bson.ObjectIDFromHex(fmt.Sprint(registration["eventId"]))
	if err != nil {
		httpx.Error(w, 404, "event not found")
		return
	}
	var event bson.M
	if h.DB.Collection("events").FindOne(r.Context(), bson.M{"_id": eventID}, options.FindOne().SetProjection(bson.M{"title": 1, "startAt": 1, "endAt": 1, "location": 1, "description": 1})).Decode(&event) != nil {
		httpx.Error(w, 404, "event not found")
		return
	}
	starts, ends := timeValue(event["startAt"]), timeValue(event["endAt"])
	if ends.IsZero() {
		ends = starts.Add(2 * time.Hour)
	}
	escape := func(value any) string {
		return strings.NewReplacer("\\", "\\\\", "\n", "\\n", ",", "\\,", ";", "\\;").Replace(strings.TrimSpace(fmt.Sprint(value)))
	}
	body := strings.Join([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//REMI//Member Events//EN", "CALSCALE:GREGORIAN", "BEGIN:VEVENT", "UID:" + id.Hex() + "@remi.church", "DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"), "DTSTART:" + starts.UTC().Format("20060102T150405Z"), "DTEND:" + ends.UTC().Format("20060102T150405Z"), "SUMMARY:" + escape(event["title"]), "LOCATION:" + escape(event["location"]), "DESCRIPTION:" + escape(event["description"]), "END:VEVENT", "END:VCALENDAR", ""}, "\r\n")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="remi-event.ics"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(body))
}

func (h *Handler) CreateMemberPathwayRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input struct {
		Type string `json:"type"`
		Note string `json:"note"`
	}
	if err := platform.DecodeJSON(w, r, &input, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Note = strings.TrimSpace(input.Note)
	labels := map[string]string{"membership-class": "Membership class", "baptism": "Baptism", "new-believer": "New believer journey"}
	if labels[input.Type] == "" || len(input.Note) > 500 {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "type", Code: "invalid", Message: "Choose an available next step."}))
		return
	}
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "type": input.Type, "state": bson.M{"$in": bson.A{"requested", "in-review", "scheduled"}}}
	var existing bson.M
	if h.DB.Collection("chms_member_pathway_requests").FindOne(r.Context(), filter).Decode(&existing) == nil {
		httpx.JSON(w, 200, models.Normalize(existing))
		return
	}
	now := time.Now().UTC()
	id := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	value := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": h.memberPersonBranch(r.Context(), actor.ID), "personId": claims.PersonID, "type": input.Type, "label": labels[input.Type], "note": input.Note, "state": "requested", "source": "member-self-service", "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		if _, e := h.DB.Collection("chms_member_pathway_requests").InsertOne(tx, value); e != nil {
			return e
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), BranchID: platform.ID(fmt.Sprint(value["branchId"])), Actor: actor, Action: "member.pathway-request.create", ResourceType: "member-pathway-request", ResourceID: id, SubjectIDs: []platform.ID{actor.ID}, ChangedFields: []string{"type", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		httpx.Error(w, 409, "next step request could not be created")
		return
	}
	httpx.JSON(w, 201, models.Normalize(value))
}

func (h *Handler) CreateMemberChildPrecheck(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input struct {
		EventID       string      `json:"eventId"`
		ChildPersonID platform.ID `json:"childPersonId"`
	}
	if err := platform.DecodeJSON(w, r, &input, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	eventID, err := bson.ObjectIDFromHex(strings.TrimSpace(input.EventID))
	if err != nil {
		httpx.Error(w, 404, "event not found")
		return
	}
	householdID := platform.ID(claims.HouseholdID)
	if !householdID.Valid() {
		httpx.Error(w, 404, "household not found")
		return
	}
	guardianOK := h.DB.Collection("chms_household_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "householdId": householdID, "personId": claims.PersonID, "role": bson.M{"$in": bson.A{"primary", "adult", "guardian", "parent"}}, "endedAt": bson.M{"$in": bson.A{nil}}}).Err() == nil
	childOK := h.DB.Collection("chms_household_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "householdId": householdID, "personId": input.ChildPersonID, "role": bson.M{"$in": bson.A{"child", "dependent", "minor"}}, "endedAt": bson.M{"$in": bson.A{nil}}}).Err() == nil
	if !guardianOK || !childOK {
		httpx.Error(w, 404, "eligible household child not found")
		return
	}
	var event bson.M
	if h.DB.Collection("events").FindOne(r.Context(), bson.M{"_id": eventID, "contentStatus": "published", "childPrecheckEnabled": true, "startAt": bson.M{"$gt": time.Now().UTC(), "$lt": time.Now().UTC().Add(31 * 24 * time.Hour)}}).Decode(&event) != nil {
		httpx.Error(w, 404, "pre-check event not found")
		return
	}
	var existing bson.M
	if h.DB.Collection("chms_member_child_prechecks").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "eventId": input.EventID, "childPersonId": input.ChildPersonID, "guardianPersonId": claims.PersonID, "state": "prepared", "expiresAt": bson.M{"$gt": time.Now().UTC()}}).Decode(&existing) == nil {
		httpx.JSON(w, 200, bson.M{"id": normalizeID(existing["_id"]), "eventId": input.EventID, "childPersonId": input.ChildPersonID, "state": "prepared", "expiresAt": existing["expiresAt"], "alreadyPrepared": true})
		return
	}
	code, err := secureMemberPrecheckCode()
	if err != nil {
		httpx.Error(w, 500, "pre-check could not be prepared")
		return
	}
	sum := sha256.Sum256([]byte(code))
	now := time.Now().UTC()
	expires := timeValue(event["endAt"])
	if expires.IsZero() {
		expires = timeValue(event["startAt"]).Add(6 * time.Hour)
	} else {
		expires = expires.Add(4 * time.Hour)
	}
	id := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	value := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "branchId": h.memberPersonBranch(r.Context(), actor.ID), "eventId": input.EventID, "childPersonId": input.ChildPersonID, "guardianPersonId": claims.PersonID, "householdId": householdID, "codeHash": hex.EncodeToString(sum[:]), "state": "prepared", "expiresAt": expires, "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor}
	if _, err = h.DB.Collection("chms_member_child_prechecks").InsertOne(r.Context(), value); err != nil {
		httpx.Error(w, 409, "pre-check could not be prepared")
		return
	}
	h.appendMemberServingAudit(r.Context(), actor, platform.ID(fmt.Sprint(value["branchId"])), "member.child-precheck.prepare", "child-precheck", id, []string{"eventId", "childPersonId", "guardianPersonId", "expiresAt"}, platform.RequestIDFrom(r.Context()), now)
	httpx.JSON(w, 201, bson.M{"id": id, "eventId": input.EventID, "childPersonId": input.ChildPersonID, "state": "prepared", "securityCode": code, "expiresAt": expires, "notice": "Show this code only to the staffed check-in team. It is not a pickup code and does not record attendance."})
}

func (h *Handler) memberPathwayProjection(ctx context.Context, personID platform.ID) bson.M {
	cursor, err := h.DB.Collection("chms_member_pathway_requests").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID}, options.Find().SetProjection(bson.M{"type": 1, "label": 1, "state": 1, "createdAt": 1, "updatedAt": 1}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(20))
	items := []bson.M{}
	if err == nil {
		defer cursor.Close(ctx)
		_ = cursor.All(ctx, &items)
	}
	return bson.M{"available": bson.A{bson.M{"id": "membership-class", "label": "Membership class", "copy": "Explore REMI, its beliefs and what belonging looks like."}, bson.M{"id": "baptism", "label": "Baptism", "copy": "Begin a conversation about baptism and preparation."}, bson.M{"id": "new-believer", "label": "New believer journey", "copy": "Walk through foundational faith with a trusted guide."}}, "requests": normalizeMaps(items)}
}
func (h *Handler) memberPrecheckProjection(ctx context.Context, personID, householdID platform.ID, now time.Time) bson.M {
	children := []bson.M{}
	if householdID.Valid() {
		cursor, err := h.DB.Collection("chms_household_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "householdId": householdID, "role": bson.M{"$in": bson.A{"child", "dependent", "minor"}}, "endedAt": bson.M{"$in": bson.A{nil}}}, options.Find().SetProjection(bson.M{"personId": 1}))
		if err == nil {
			defer cursor.Close(ctx)
			var rows []bson.M
			_ = cursor.All(ctx, &rows)
			for _, row := range rows {
				person, err := h.delegationPerson(ctx, platform.ID(fmt.Sprint(row["personId"])))
				if err == nil {
					children = append(children, bson.M{"id": row["personId"], "name": displayDelegationName(person)})
				}
			}
		}
	}
	events := []bson.M{}
	cursor, err := h.DB.Collection("events").Find(ctx, bson.M{"contentStatus": "published", "childPrecheckEnabled": true, "startAt": bson.M{"$gt": now, "$lt": now.Add(31 * 24 * time.Hour)}}, options.Find().SetProjection(bson.M{"title": 1, "startAt": 1, "endAt": 1, "location": 1}).SetSort(bson.D{{Key: "startAt", Value: 1}}).SetLimit(20))
	if err == nil {
		defer cursor.Close(ctx)
		_ = cursor.All(ctx, &events)
	}
	return bson.M{"children": children, "events": normalizeMaps(events), "policy": "Preparation verifies current household roles again. It does not check a child in, create attendance or replace staffed pickup verification."}
}
func secureMemberPrecheckCode() (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	buffer := make([]byte, 8)
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i := range buffer {
		buffer[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(buffer), nil
}
