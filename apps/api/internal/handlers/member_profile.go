package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
	"remi-api/internal/services"
)

const memberDataRequests = "chms_data_requests"

type memberProfileInput struct {
	ExpectedVersion          int64                           `json:"expectedVersion"`
	Names                    people.Names                    `json:"names"`
	PhotoAssetID             platform.ID                     `json:"photoAssetId"`
	DateOfBirth              *people.PartialDate             `json:"dateOfBirth"`
	Gender                   *string                         `json:"gender"`
	ContactPoints            []people.ContactPoint           `json:"contactPoints"`
	Addresses                []people.Address                `json:"addresses"`
	CommunicationPreferences people.CommunicationPreferences `json:"communicationPreferences"`
	EmergencyContact         memberEmergencyContact          `json:"emergencyContact"`
	DirectoryVisibility      string                          `json:"directoryVisibility"`
}

type memberEmergencyContact struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
}

type memberProfileUOW struct {
	base   *platform.MongoPlatformStore
	db     *mongo.Database
	events []bson.M
}

func (u memberProfileUOW) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return u.base.WithTransaction(ctx, func(tx context.Context) error {
		if err := fn(tx); err != nil {
			return err
		}
		if len(u.events) > 0 {
			values := make([]any, len(u.events))
			for index := range u.events {
				values[index] = u.events[index]
			}
			if _, err := u.db.Collection("chms_consent_events").InsertMany(tx, values); err != nil {
				return fmt.Errorf("append member consent events: %w", err)
			}
		}
		return nil
	})
}
func (u memberProfileUOW) AppendAudit(ctx context.Context, value platform.AuditEvent) error {
	return u.base.AppendAudit(ctx, value)
}
func (u memberProfileUOW) EnqueueEvent(ctx context.Context, value platform.OutboxRecord) error {
	return u.base.EnqueueEvent(ctx, value)
}

func (h *Handler) GetMemberProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	repository, _ := people.NewMongoRepository(h.DB)
	person, err := repository.FindByID(r.Context(), platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(claims.PersonID))
	if err != nil || person == nil || person.ArchivedAt != nil {
		httpx.Error(w, http.StatusNotFound, "member profile not found")
		return
	}
	consents := h.memberConsentProjection(r.Context(), claims.PersonID)
	httpx.JSON(w, http.StatusOK, bson.M{"person": memberEditablePerson(person, h.Cfg.CloudinaryCloudName), "emergencyContact": emergencyFrom(person.CustomFields), "directoryVisibility": defaultStringValue(person.CustomFields["member.directoryVisibility"], "hidden"), "consents": consents})
}

func (h *Handler) UpdateMemberProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	var input memberProfileInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	repository, _ := people.NewMongoRepository(h.DB)
	organizationID, personID := platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(claims.PersonID)
	current, err := repository.FindByID(r.Context(), organizationID, personID)
	if err != nil || current == nil || current.ArchivedAt != nil {
		httpx.Error(w, http.StatusNotFound, "member profile not found")
		return
	}
	input.DirectoryVisibility = strings.ToLower(strings.TrimSpace(input.DirectoryVisibility))
	if input.DirectoryVisibility == "groups" {
		input.DirectoryVisibility = "members"
	}
	if input.DirectoryVisibility != "hidden" && input.DirectoryVisibility != "members" && input.DirectoryVisibility != "branch" {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "directoryVisibility", Code: "invalid", Message: "Choose private, groups or branch."}))
		return
	}
	input.EmergencyContact.Name = strings.TrimSpace(input.EmergencyContact.Name)
	input.EmergencyContact.Relationship = strings.TrimSpace(input.EmergencyContact.Relationship)
	input.EmergencyContact.Phone = strings.TrimSpace(input.EmergencyContact.Phone)
	if (input.EmergencyContact.Name != "" || input.EmergencyContact.Relationship != "" || input.EmergencyContact.Phone != "") && (input.EmergencyContact.Name == "" || input.EmergencyContact.Relationship == "" || input.EmergencyContact.Phone == "") {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "emergencyContact", Code: "incomplete", Message: "Emergency contact requires name, relationship and phone."}))
		return
	}
	custom := cloneStrings(current.CustomFields)
	custom["member.emergency.name"], custom["member.emergency.relationship"], custom["member.emergency.phone"] = input.EmergencyContact.Name, input.EmergencyContact.Relationship, input.EmergencyContact.Phone
	custom["member.directoryVisibility"] = input.DirectoryVisibility
	candidate := people.CreateInput{OrganizationID: organizationID, HomeBranchID: current.HomeBranchID, Names: input.Names, Aliases: current.Aliases, PhotoAssetID: input.PhotoAssetID, DateOfBirth: input.DateOfBirth, Gender: input.Gender, ContactPoints: input.ContactPoints, Addresses: input.Addresses, MembershipStage: current.MembershipStage, Tags: current.Tags, CustomFields: custom, CommunicationPreferences: input.CommunicationPreferences, Source: current.Source}
	if err := candidate.NormalizeAndValidate(); err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_profile", Message: err.Error()}))
		return
	}
	var account bson.M
	if err := h.DB.Collection(memberAccounts).FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "status": "active"}, options.FindOne().SetProjection(bson.M{"emailNormalized": 1})).Decode(&account); err != nil || !containsNormalizedEmail(candidate.ContactPoints, stringValue(account["emailNormalized"])) {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "contactPoints", Code: "login_email_required", Message: "Your verified sign-in email must remain on the profile. Contact an administrator to change it."}))
		return
	}
	now := time.Now().UTC()
	consentEvents := communicationConsentEvents(current.CommunicationPreferences, candidate.CommunicationPreferences, organizationID, personID, platform.RequestIDFrom(r.Context()), now)
	store, _ := platform.NewMongoPlatformStore(h.DB)
	service := people.Service{Repository: repository, Platform: memberProfileUOW{base: store, db: h.DB, events: consentEvents}}
	updated, err := service.Update(r.Context(), organizationID, personID, people.UpdateInput{ExpectedVersion: input.ExpectedVersion, HomeBranchID: current.HomeBranchID, Names: candidate.Names, Aliases: current.Aliases, PhotoAssetID: candidate.PhotoAssetID, DateOfBirth: candidate.DateOfBirth, Gender: candidate.Gender, ContactPoints: candidate.ContactPoints, Addresses: candidate.Addresses, MembershipStage: current.MembershipStage, Tags: current.Tags, CustomFields: candidate.CustomFields, CommunicationPreferences: candidate.CommunicationPreferences, Source: current.Source}, platform.Actor{Type: platform.ActorMember, ID: personID}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"person": memberEditablePerson(updated, h.Cfg.CloudinaryCloudName), "emergencyContact": emergencyFrom(updated.CustomFields), "directoryVisibility": defaultStringValue(updated.CustomFields["member.directoryVisibility"], "hidden"), "consents": h.memberConsentProjection(r.Context(), claims.PersonID)})
}

func (h *Handler) GetMemberHousehold(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	householdID, membership := h.resolveMemberHousehold(r.Context(), claims.PersonID, claims.HouseholdID)
	if householdID == "" {
		httpx.JSON(w, http.StatusOK, bson.M{"household": nil, "editable": false})
		return
	}
	repository, _ := people.NewHouseholdRepository(h.DB)
	household, err := repository.FindHouseholdByID(r.Context(), platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(householdID))
	profile, profileErr := repository.FindProfileByHouseholdID(r.Context(), platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(householdID))
	if err != nil || profileErr != nil || household == nil {
		httpx.Error(w, http.StatusNotFound, "household not found")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"household": household, "members": profile.Members, "editable": memberCanEditHousehold(membership)})
}

func (h *Handler) UpdateMemberHousehold(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	householdID, membership := h.resolveMemberHousehold(r.Context(), claims.PersonID, claims.HouseholdID)
	if householdID == "" || !memberCanEditHousehold(membership) {
		httpx.Error(w, http.StatusForbidden, "only a household primary contact can update shared details")
		return
	}
	var input people.UpdateHouseholdInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	repository, _ := people.NewHouseholdRepository(h.DB)
	current, err := repository.FindHouseholdByID(r.Context(), platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(householdID))
	if err != nil || current == nil {
		httpx.Error(w, http.StatusNotFound, "household not found")
		return
	}
	input.HomeBranchID, input.PrimaryContactPersonID, input.StatementPreference = current.HomeBranchID, current.PrimaryContactPersonID, current.StatementPreference
	store, _ := platform.NewMongoPlatformStore(h.DB)
	service := people.HouseholdService{Repository: repository, Platform: store}
	updated, err := service.Update(r.Context(), platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(householdID), input, platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (h *Handler) CreateMemberDataRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	var input struct {
		Type    string `json:"type"`
		Details string `json:"details"`
	}
	if err := platform.DecodeJSON(w, r, &input, 1<<16); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Type, input.Details = strings.ToLower(strings.TrimSpace(input.Type)), strings.TrimSpace(input.Details)
	allowed := map[string]bool{"access": true, "correction": true, "portability": true, "deletion": true, "objection": true}
	if !allowed[input.Type] || len(input.Details) > 2000 {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_request", Message: "Choose a supported request and keep details under 2,000 characters."}))
		return
	}
	now, id := time.Now().UTC(), platform.ID(bson.NewObjectID().Hex())
	var person bson.M
	if err := h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": claims.PersonID, "organizationId": h.Cfg.CHMSOrganizationID}, options.FindOne().SetProjection(bson.M{"homeBranchId": 1})).Decode(&person); err != nil {
		httpx.Error(w, http.StatusNotFound, "member profile not found")
		return
	}
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	value := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "branchId": person["homeBranchId"], "type": input.Type, "details": input.Details, "status": "received", "priority": "routine", "identityVerification": bson.M{"method": "active-member-session", "verifiedAt": now}, "dueAt": now.Add(30 * 24 * time.Hour), "version": int64(1), "createdAt": now, "updatedAt": now}
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(tx context.Context) error {
		if _, err := h.DB.Collection(memberDataRequests).InsertOne(tx, value); err != nil {
			return err
		}
		if err := store.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: actor, Action: "governance.data-request.create", ResourceType: "data-request", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"type", "status"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now}); err != nil {
			return err
		}
		return store.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Type: "governance.data-request.received", EventVersion: 1, AggregateType: "data-request", AggregateID: id, AggregateVersion: 1, Actor: actor, RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now, RecordedAt: now, Payload: map[string]any{"requestId": id, "personId": claims.PersonID, "type": input.Type}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, value)
}

func (h *Handler) ListMemberDataRequests(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	cursor, err := h.DB.Collection(memberDataRequests).Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID}, options.Find().SetProjection(bson.M{"type": 1, "status": 1, "memberResponse": 1, "dueAt": 1, "version": 1, "createdAt": 1, "updatedAt": 1}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(50))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "data requests unavailable")
		return
	}
	defer cursor.Close(r.Context())
	var values []bson.M
	if cursor.All(r.Context(), &values) != nil {
		httpx.Error(w, http.StatusInternalServerError, "data requests unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, values)
}

func (h *Handler) WithdrawMemberDataRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	requestID := platform.ID(strings.TrimSpace(chi.URLParam(r, "requestId")))
	var input struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Reason          string `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &input, 1<<14); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ExpectedVersion < 1 || len(input.Reason) < 3 || len(input.Reason) > 500 {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid", Message: "Provide the current version and a short withdrawal reason."}))
		return
	}
	now := time.Now().UTC()
	filter := bson.M{"_id": requestID, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "status": "received", "version": input.ExpectedVersion}
	result, err := h.DB.Collection(memberDataRequests).UpdateOne(r.Context(), filter, bson.M{"$set": bson.M{"status": "withdrawn", "updatedAt": now, "memberResponse": "You withdrew this request."}, "$inc": bson.M{"version": 1}})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if result.ModifiedCount != 1 {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "Only a current, newly received request can be withdrawn."})
		return
	}
	actor := platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}
	_, _ = h.DB.Collection("chms_data_request_history").InsertOne(r.Context(), bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": h.Cfg.CHMSOrganizationID, "requestId": requestID, "from": "received", "to": "withdrawn", "reason": input.Reason, "memberResponse": "You withdrew this request.", "actor": actor, "occurredAt": now})
	store, _ := platform.NewMongoPlatformStore(h.DB)
	_ = store.AppendAudit(r.Context(), platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: actor, Action: "governance.data-request.withdraw", ResourceType: "data-request", ResourceID: requestID, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"status", "version"}, BeforeVersion: input.ExpectedVersion, AfterVersion: input.ExpectedVersion + 1, Outcome: "success", Reason: input.Reason, RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	httpx.JSON(w, http.StatusOK, bson.M{"id": requestID, "status": "withdrawn", "version": input.ExpectedVersion + 1, "updatedAt": now})
}

func (h *Handler) MemberUploadSignature(w http.ResponseWriter, r *http.Request) {
	if _, ok := memberClaims(r); !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	if !h.Cloud.Configured() {
		httpx.Error(w, http.StatusNotImplemented, "profile image upload is not configured")
		return
	}
	params, err := h.Cloud.Signature("remi/members")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "profile image upload is unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, params)
}

func memberClaims(r *http.Request) (*services.Claims, bool) {
	claims := middleware.ClaimsFrom(r)
	return claims, claims != nil && claims.Role == "member" && platform.ID(claims.PersonID).Valid()
}

func memberEditablePerson(person *people.Person, cloudName string) bson.M {
	photoURL := ""
	if person.PhotoAssetID.Valid() && cloudName != "" {
		photoURL = "https://res.cloudinary.com/" + cloudName + "/image/upload/" + string(person.PhotoAssetID)
	}
	return bson.M{"id": person.ID, "version": person.Version, "names": person.Names, "photoAssetId": person.PhotoAssetID, "photoUrl": photoURL, "dateOfBirth": person.DateOfBirth, "gender": person.Gender, "contactPoints": person.ContactPoints, "addresses": person.Addresses, "homeBranchId": person.HomeBranchID, "membershipStage": person.MembershipStage, "communicationPreferences": person.CommunicationPreferences}
}

func emergencyFrom(custom map[string]string) memberEmergencyContact {
	return memberEmergencyContact{Name: custom["member.emergency.name"], Relationship: custom["member.emergency.relationship"], Phone: custom["member.emergency.phone"]}
}

func defaultStringValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cloneStrings(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values)+4)
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func containsNormalizedEmail(points []people.ContactPoint, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, point := range points {
		if point.Type == "email" && point.Normalized == email {
			return true
		}
	}
	return false
}

func communicationConsentEvents(before, after people.CommunicationPreferences, organizationID, personID platform.ID, requestID string, now time.Time) []bson.M {
	oldValues := map[string]bool{"email": before.Email, "sms": before.SMS, "whatsapp": before.WhatsApp, "phone": before.Phone}
	newValues := map[string]bool{"email": after.Email, "sms": after.SMS, "whatsapp": after.WhatsApp, "phone": after.Phone}
	events := []bson.M{}
	for _, channel := range []string{"email", "sms", "whatsapp", "phone"} {
		if oldValues[channel] == newValues[channel] {
			continue
		}
		state := "withdrawn"
		if newValues[channel] {
			state = "granted"
		}
		events = append(events, bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": organizationID, "personId": personID, "purpose": "church-communications", "channel": channel, "state": state, "noticeVersion": "member-profile-2026-01", "source": "member-self-service", "actor": platform.Actor{Type: platform.ActorMember, ID: personID}, "requestId": requestID, "occurredAt": now})
	}
	return events
}

func (h *Handler) memberConsentProjection(ctx context.Context, personID string) []bson.M {
	cursor, err := h.DB.Collection("chms_consent_events").Find(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID}, options.Find().SetProjection(bson.M{"purpose": 1, "channel": 1, "state": 1, "noticeVersion": 1, "source": 1, "occurredAt": 1}).SetSort(bson.D{{Key: "occurredAt", Value: -1}}).SetLimit(50))
	if err != nil {
		return []bson.M{}
	}
	defer cursor.Close(ctx)
	var events []bson.M
	if cursor.All(ctx, &events) != nil {
		return []bson.M{}
	}
	seen, current := map[string]bool{}, []bson.M{}
	for _, event := range events {
		key := stringValue(event["purpose"]) + ":" + stringValue(event["channel"])
		if seen[key] {
			continue
		}
		seen[key] = true
		current = append(current, event)
	}
	return current
}

func (h *Handler) resolveMemberHousehold(ctx context.Context, personID, selectedHouseholdID string) (string, bson.M) {
	filter := bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": personID, "endedAt": nil}
	if selectedHouseholdID != "" {
		filter["householdId"] = selectedHouseholdID
	}
	var membership bson.M
	if h.DB.Collection("chms_household_memberships").FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "startedAt", Value: 1}})).Decode(&membership) != nil {
		return "", nil
	}
	return stringValue(membership["householdId"]), membership
}

func memberCanEditHousehold(membership bson.M) bool {
	role := strings.ToLower(strings.TrimSpace(stringValue(membership["role"])))
	return role == "primary-contact" || role == "head" || role == "household-admin"
}
