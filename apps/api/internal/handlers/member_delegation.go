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
)

const memberDelegations = "chms_household_access_delegations"

var delegationFields = map[string]bool{"profile": true, "attendance": true, "registrations": true, "groups": true, "serving": true, "giving-summary": true, "statements": true}

type delegationInput struct {
	DelegatePersonID platform.ID `json:"delegatePersonId"`
	Fields           []string    `json:"fields"`
	ExpiresAt        time.Time   `json:"expiresAt"`
}

type delegationPerson struct {
	ID    platform.ID `bson:"_id"`
	Names struct {
		Given     string `bson:"given"`
		Family    string `bson:"family"`
		Preferred string `bson:"preferred"`
	} `bson:"names"`
	DateOfBirth *struct {
		Value string `bson:"value"`
	} `bson:"dateOfBirth"`
}

type delegationRecord struct {
	ID               platform.ID `bson:"_id" json:"id"`
	GrantorPersonID  platform.ID `bson:"grantorPersonId" json:"-"`
	DelegatePersonID platform.ID `bson:"delegatePersonId" json:"-"`
	Fields           []string    `bson:"fields" json:"fields"`
	ExpiresAt        time.Time   `bson:"expiresAt" json:"expiresAt"`
	CreatedAt        time.Time   `bson:"createdAt" json:"createdAt"`
}

func normalizeDelegation(input *delegationInput, now time.Time) error {
	seen := map[string]bool{}
	fields := make([]string, 0, len(input.Fields))
	for _, field := range input.Fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if delegationFields[field] && !seen[field] {
			seen[field] = true
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	input.Fields = fields
	input.ExpiresAt = input.ExpiresAt.UTC()
	if !input.DelegatePersonID.Valid() || len(fields) == 0 || input.ExpiresAt.Before(now.Add(time.Hour)) || input.ExpiresAt.After(now.AddDate(1, 0, 1)) {
		return platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_delegation", Message: "Choose another adult household member, at least one access area, and an expiry within one year."})
	}
	return nil
}

func (h *Handler) ListMemberHouseholdDelegations(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	now := time.Now().UTC()
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "$or": bson.A{bson.M{"grantorPersonId": claims.PersonID}, bson.M{"delegatePersonId": claims.PersonID}}, "state": "active", "expiresAt": bson.M{"$gt": now}}
	cursor, err := h.DB.Collection(memberDelegations).Find(r.Context(), filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		httpx.Error(w, 500, "delegations unavailable")
		return
	}
	defer cursor.Close(r.Context())
	var values []delegationRecord
	if cursor.All(r.Context(), &values) != nil {
		httpx.Error(w, 500, "delegations unavailable")
		return
	}
	items := make([]bson.M, 0, len(values))
	for _, value := range values {
		direction, personID := "received", value.GrantorPersonID
		if value.GrantorPersonID == platform.ID(claims.PersonID) {
			direction, personID = "granted", value.DelegatePersonID
		}
		person, _ := h.delegationPerson(r.Context(), personID)
		items = append(items, bson.M{"id": value.ID, "direction": direction, "personName": displayDelegationName(person), "fields": value.Fields, "expiresAt": value.ExpiresAt, "createdAt": value.CreatedAt})
	}
	candidates, err := h.delegationCandidates(r.Context(), platform.ID(claims.PersonID), now)
	if err != nil {
		httpx.Error(w, 500, "delegations unavailable")
		return
	}
	httpx.JSON(w, 200, bson.M{"items": items, "candidates": candidates, "accessAreas": []bson.M{{"id": "profile", "label": "Profile details"}, {"id": "attendance", "label": "Attendance"}, {"id": "registrations", "label": "Registrations"}, {"id": "groups", "label": "Groups"}, {"id": "serving", "label": "Serving"}, {"id": "giving-summary", "label": "Giving summary"}, {"id": "statements", "label": "Statements"}}})
}

func (h *Handler) CreateMemberHouseholdDelegation(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input delegationInput
	if err := platform.DecodeJSON(w, r, &input, 16<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	now := time.Now().UTC()
	if err := normalizeDelegation(&input, now); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.DelegatePersonID == platform.ID(claims.PersonID) {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "delegatePersonId", Code: "self", Message: "Choose another household member."}))
		return
	}
	shared, err := h.sharedActiveHousehold(r.Context(), platform.ID(claims.PersonID), input.DelegatePersonID)
	if err != nil || shared == "" {
		httpx.Error(w, http.StatusNotFound, "eligible household member not found")
		return
	}
	if !h.activeAdultMember(r.Context(), input.DelegatePersonID, now) {
		httpx.Error(w, http.StatusNotFound, "eligible household member not found")
		return
	}
	id := platform.ID(bson.NewObjectID().Hex())
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	value := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "householdId": shared, "grantorPersonId": claims.PersonID, "delegatePersonId": input.DelegatePersonID, "fields": input.Fields, "state": "active", "expiresAt": input.ExpiresAt, "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err = store.WithTransaction(r.Context(), func(tx context.Context) error {
		if _, err := h.DB.Collection(memberDelegations).InsertOne(tx, value); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return &platform.DomainError{Code: "conflict", Message: "An active delegation already exists for this person."}
			}
			return err
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), BranchID: "", Actor: actor, Action: "member.household-delegation.create", ResourceType: "household-access-delegation", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID), input.DelegatePersonID}, ChangedFields: []string{"fields", "expiresAt", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, value)
}

func (h *Handler) RevokeMemberHouseholdDelegation(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	id := platform.ID(strings.TrimSpace(chi.URLParam(r, "delegationId")))
	if !id.Valid() {
		httpx.Error(w, 404, "delegation not found")
		return
	}
	now := time.Now().UTC()
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		result, err := h.DB.Collection(memberDelegations).UpdateOne(tx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "grantorPersonId": claims.PersonID, "state": "active"}, bson.M{"$set": bson.M{"state": "revoked", "revokedAt": now, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
		if err != nil {
			return err
		}
		if result.ModifiedCount != 1 {
			return &platform.DomainError{Code: "not_found", Message: "Delegation not found."}
		}
		return store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: actor, Action: "member.household-delegation.revoke", ResourceType: "household-access-delegation", ResourceID: id, ChangedFields: []string{"state", "revokedAt"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) sharedActiveHousehold(ctx context.Context, grantor, delegate platform.ID) (platform.ID, error) {
	cursor, err := h.DB.Collection("chms_household_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": grantor, "endedAt": bson.M{"$in": bson.A{nil}}}, options.Find().SetProjection(bson.M{"householdId": 1}))
	if err != nil {
		return "", err
	}
	defer cursor.Close(ctx)
	var rows []bson.M
	if err = cursor.All(ctx, &rows); err != nil {
		return "", err
	}
	ids := bson.A{}
	for _, row := range rows {
		ids = append(ids, row["householdId"])
	}
	if len(ids) == 0 {
		return "", nil
	}
	var match bson.M
	err = h.DB.Collection("chms_household_memberships").FindOne(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": delegate, "householdId": bson.M{"$in": ids}, "endedAt": bson.M{"$in": bson.A{nil}}}).Decode(&match)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	return platform.ID(stringValue(match["householdId"])), err
}
func (h *Handler) activeAdultMember(ctx context.Context, id platform.ID, now time.Time) bool {
	var account bson.M
	if h.DB.Collection(memberAccounts).FindOne(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": id, "status": "active"}).Decode(&account) != nil {
		return false
	}
	person, err := h.delegationPerson(ctx, id)
	if err != nil {
		return false
	}
	if person.DateOfBirth == nil || person.DateOfBirth.Value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", person.DateOfBirth.Value)
	return err != nil || parsed.AddDate(18, 0, 0).Before(now) || parsed.AddDate(18, 0, 0).Equal(now)
}
func (h *Handler) delegationPerson(ctx context.Context, id platform.ID) (delegationPerson, error) {
	var person delegationPerson
	err := h.DB.Collection("chms_people").FindOne(ctx, bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": bson.M{"$in": bson.A{nil}}}, options.FindOne().SetProjection(bson.M{"names": 1, "dateOfBirth": 1})).Decode(&person)
	return person, err
}

func (h *Handler) delegationCandidates(ctx context.Context, grantor platform.ID, now time.Time) ([]bson.M, error) {
	cursor, err := h.DB.Collection("chms_household_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": grantor, "endedAt": bson.M{"$in": bson.A{nil}}}, options.Find().SetProjection(bson.M{"householdId": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var own []struct {
		HouseholdID platform.ID `bson:"householdId"`
	}
	if err := cursor.All(ctx, &own); err != nil {
		return nil, err
	}
	householdIDs := bson.A{}
	for _, row := range own {
		householdIDs = append(householdIDs, row.HouseholdID)
	}
	if len(householdIDs) == 0 {
		return []bson.M{}, nil
	}
	members, err := h.DB.Collection("chms_household_memberships").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "householdId": bson.M{"$in": householdIDs}, "endedAt": bson.M{"$in": bson.A{nil}}}, options.Find().SetProjection(bson.M{"personId": 1}))
	if err != nil {
		return nil, err
	}
	defer members.Close(ctx)
	var rows []struct {
		PersonID platform.ID `bson:"personId"`
	}
	if err := members.All(ctx, &rows); err != nil {
		return nil, err
	}
	seen, result := map[platform.ID]bool{}, []bson.M{}
	for _, row := range rows {
		if row.PersonID == grantor || seen[row.PersonID] || !h.activeAdultMember(ctx, row.PersonID, now) {
			continue
		}
		seen[row.PersonID] = true
		person, err := h.delegationPerson(ctx, row.PersonID)
		if err == nil {
			result = append(result, bson.M{"id": row.PersonID, "name": displayDelegationName(person)})
		}
	}
	sort.Slice(result, func(i, j int) bool { return fmt.Sprint(result[i]["name"]) < fmt.Sprint(result[j]["name"]) })
	return result, nil
}

func displayDelegationName(person delegationPerson) string {
	first := strings.TrimSpace(person.Names.Preferred)
	if first == "" {
		first = strings.TrimSpace(person.Names.Given)
	}
	return strings.TrimSpace(first + " " + person.Names.Family)
}
