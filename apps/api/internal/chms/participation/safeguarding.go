package participation

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

const pickupAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

type PickupCodeManager struct{ secret []byte }

func NewPickupCodeManager(secret []byte) (*PickupCodeManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("pickup code secret must contain at least 32 bytes")
	}
	return &PickupCodeManager{secret: append([]byte(nil), secret...)}, nil
}

func (m *PickupCodeManager) Generate() (string, string, error) {
	if m == nil {
		return "", "", errors.New("pickup code manager is unavailable")
	}
	buffer := make([]byte, 6)
	for i := range buffer {
		var sample [1]byte
		for {
			if _, err := rand.Read(sample[:]); err != nil {
				return "", "", err
			}
			limit := 256 - (256 % len(pickupAlphabet))
			if int(sample[0]) < limit {
				buffer[i] = pickupAlphabet[int(sample[0])%len(pickupAlphabet)]
				break
			}
		}
	}
	code := string(buffer)
	return code, m.Hash(code), nil
}

func (m *PickupCodeManager) Hash(code string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(mac.Sum(nil))
}

func (m *PickupCodeManager) Verify(code, expectedHash string) bool {
	actual, errA := hex.DecodeString(m.Hash(code))
	expected, errB := hex.DecodeString(expectedHash)
	return errA == nil && errB == nil && len(actual) == len(expected) && subtle.ConstantTimeCompare(actual, expected) == 1
}

type GuardianAuthorization struct {
	platform.ResourceEnvelope `bson:",inline"`
	ChildPersonID             platform.ID `json:"childPersonId" bson:"childPersonId"`
	GuardianPersonID          platform.ID `json:"guardianPersonId" bson:"guardianPersonId"`
	Relationship              string      `json:"relationship" bson:"relationship"`
	ValidFrom                 time.Time   `json:"validFrom" bson:"validFrom"`
	ValidUntil                *time.Time  `json:"validUntil,omitempty" bson:"validUntil,omitempty"`
	Status                    string      `json:"status" bson:"status"`
	Source                    string      `json:"source" bson:"source"`
}

type GuardianAuthorizationInput struct {
	BranchID         platform.ID `json:"branchId"`
	ChildPersonID    platform.ID `json:"childPersonId"`
	GuardianPersonID platform.ID `json:"guardianPersonId"`
	Relationship     string      `json:"relationship"`
	ValidFrom        time.Time   `json:"validFrom"`
	ValidUntil       *time.Time  `json:"validUntil"`
	Source           string      `json:"source"`
}

type ChildCheckin struct {
	platform.ResourceEnvelope `bson:",inline"`
	SessionID                 platform.ID     `json:"sessionId" bson:"sessionId"`
	OccurrenceID              platform.ID     `json:"occurrenceId" bson:"occurrenceId"`
	AttendanceID              platform.ID     `json:"attendanceId" bson:"attendanceId"`
	ChildPersonID             platform.ID     `json:"childPersonId" bson:"childPersonId"`
	GuardianPersonID          platform.ID     `json:"guardianPersonId" bson:"guardianPersonId"`
	AuthorizationID           platform.ID     `json:"authorizationId" bson:"authorizationId"`
	CodeHash                  string          `json:"-" bson:"codeHash"`
	CodeExpiresAt             time.Time       `json:"codeExpiresAt" bson:"codeExpiresAt"`
	PickedUpAt                *time.Time      `json:"pickedUpAt,omitempty" bson:"pickedUpAt,omitempty"`
	PickedUpByGuardianID      platform.ID     `json:"pickedUpByGuardianId,omitempty" bson:"pickedUpByGuardianId,omitempty"`
	ReleasedBy                *platform.Actor `json:"releasedBy,omitempty" bson:"releasedBy,omitempty"`
}

type ChildCheckinInput struct {
	OccurrenceID     platform.ID `json:"occurrenceId"`
	ChildPersonID    platform.ID `json:"childPersonId"`
	GuardianPersonID platform.ID `json:"guardianPersonId"`
	CapturedAt       time.Time   `json:"capturedAt"`
}

type ChildLabel struct {
	CheckinID    platform.ID `json:"checkinId"`
	AttendanceID platform.ID `json:"attendanceId"`
	SecurityCode string      `json:"securityCode"`
	ExpiresAt    time.Time   `json:"expiresAt"`
}

type PickupEvent struct {
	ID               platform.ID    `json:"id" bson:"_id"`
	OrganizationID   platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID         platform.ID    `json:"branchId" bson:"branchId"`
	ChildCheckinID   platform.ID    `json:"childCheckinId" bson:"childCheckinId"`
	AttendanceID     platform.ID    `json:"attendanceId" bson:"attendanceId"`
	GuardianPersonID platform.ID    `json:"guardianPersonId" bson:"guardianPersonId"`
	Operator         platform.Actor `json:"operator" bson:"operator"`
	Outcome          string         `json:"outcome" bson:"outcome"`
	Reason           string         `json:"reason,omitempty" bson:"reason,omitempty"`
	OccurredAt       time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type PickupInput struct {
	GuardianPersonID platform.ID `json:"guardianPersonId"`
	SecurityCode     string      `json:"securityCode"`
}

type PickupReceipt struct {
	ChildCheckinID   platform.ID `json:"childCheckinId"`
	AttendanceID     platform.ID `json:"attendanceId"`
	GuardianPersonID platform.ID `json:"guardianPersonId"`
	ReleasedAt       time.Time   `json:"releasedAt"`
}

type SafeguardingIncident struct {
	platform.ResourceEnvelope `bson:",inline"`
	ChildCheckinID            platform.ID `json:"childCheckinId" bson:"childCheckinId"`
	Category                  string      `json:"category" bson:"category"`
	Summary                   string      `json:"summary" bson:"summary"`
	Status                    string      `json:"status" bson:"status"`
	OccurredAt                time.Time   `json:"occurredAt" bson:"occurredAt"`
}

func (in *GuardianAuthorizationInput) normalize(now time.Time) error {
	in.Relationship = strings.ToLower(strings.TrimSpace(in.Relationship))
	in.Source = strings.TrimSpace(in.Source)
	if !in.BranchID.Valid() || !in.ChildPersonID.Valid() || !in.GuardianPersonID.Valid() || in.ChildPersonID == in.GuardianPersonID {
		return errors.New("branch, distinct child and guardian are required")
	}
	if in.Relationship == "" || len(in.Relationship) > 80 {
		return errors.New("relationship must be 1 to 80 characters")
	}
	if in.ValidFrom.IsZero() {
		in.ValidFrom = now
	}
	in.ValidFrom = in.ValidFrom.UTC()
	normalizeOptionalTime(&in.ValidUntil)
	if in.ValidUntil != nil && !in.ValidUntil.After(in.ValidFrom) {
		return errors.New("authorization expiry must follow its start")
	}
	if in.Source == "" {
		in.Source = "staff-confirmed"
	}
	return nil
}

func (s Service) CreateGuardianAuthorization(ctx context.Context, principal platform.Principal, input GuardianAuthorizationInput, requestID string) (*GuardianAuthorization, error) {
	now := s.now()
	if err := input.normalize(now); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_guardian_authorization", Message: err.Error()})
	}
	if !s.allowedFields(principal, "create", "guardian-authorization", input.BranchID, platform.FieldChildSafeguarding) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot manage guardian authorization in this branch."}
	}
	for _, personID := range []platform.ID{input.ChildPersonID, input.GuardianPersonID} {
		branch, active, err := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID)
		if err != nil {
			return nil, err
		}
		if !active || branch != input.BranchID {
			return nil, platform.ValidationError(platform.FieldError{Path: "person", Code: "invalid_person", Message: "Child and guardian must be active people in the selected branch."})
		}
	}
	id := platform.ID(bson.NewObjectID().Hex())
	value := GuardianAuthorization{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: input.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, ChildPersonID: input.ChildPersonID, GuardianPersonID: input.GuardianPersonID, Relationship: input.Relationship, ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil, Status: "active", Source: input.Source}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertGuardianAuthorization(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, input.BranchID, "participation.guardian-authorization.created", "guardian-authorization", id, 1, []string{"childPersonId", "guardianPersonId", "relationship", "validity"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create guardian authorization: %w", err)
	}
	return &value, nil
}

func (s Service) CheckinChild(ctx context.Context, principal platform.Principal, sessionID platform.ID, input ChildCheckinInput, requestID string) (*ChildCheckin, *ChildLabel, error) {
	if s.PickupCodes == nil {
		return nil, nil, errors.New("pickup code manager is unavailable")
	}
	session, err := s.GetCheckinSession(ctx, principal, sessionID)
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	if session.State != "active" || !session.ExpiresAt.After(now) {
		return nil, nil, &platform.DomainError{Code: "checkin_session_locked", Message: "This check-in station is locked or expired."}
	}
	if !s.allowedFields(principal, "create", "child-checkin", session.BranchID, platform.FieldChildSafeguarding) {
		return nil, nil, &platform.DomainError{Code: "forbidden", Message: "Safeguarding access is required."}
	}
	authorizedOccurrence := false
	for _, id := range session.OccurrenceIDs {
		if id == input.OccurrenceID {
			authorizedOccurrence = true
			break
		}
	}
	if !authorizedOccurrence {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "occurrenceId", Code: "occurrence_not_authorized", Message: "Occurrence is not authorized for this station."})
	}
	authorization, err := s.Store.FindActiveGuardianAuthorization(ctx, principal.OrganizationID, session.BranchID, input.ChildPersonID, input.GuardianPersonID, now)
	if err != nil {
		return nil, nil, err
	}
	if authorization == nil {
		return nil, nil, &platform.DomainError{Code: "pickup_denied", Message: "Guardian authorization could not be verified."}
	}
	captured := input.CapturedAt.UTC()
	if captured.IsZero() {
		captured = now
	}
	attendance, err := s.RecordAttendance(ctx, principal, input.OccurrenceID, input.ChildPersonID, 0, AttendanceInput{Status: AttendancePresent, Source: "kiosk", Confidence: 100, CheckedInAt: &captured, StationID: sessionID}, requestID)
	if err != nil {
		return nil, nil, err
	}
	code, codeHash, err := s.PickupCodes.Generate()
	if err != nil {
		return nil, nil, err
	}
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, input.OccurrenceID)
	if err != nil {
		return nil, nil, err
	}
	expiresAt := occurrence.EndsAt.Add(6 * time.Hour)
	if expiresAt.Before(now.Add(time.Hour)) {
		expiresAt = now.Add(time.Hour)
	}
	id := platform.ID(bson.NewObjectID().Hex())
	value := ChildCheckin{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: session.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, SessionID: sessionID, OccurrenceID: input.OccurrenceID, AttendanceID: attendance.ID, ChildPersonID: input.ChildPersonID, GuardianPersonID: input.GuardianPersonID, AuthorizationID: authorization.ID, CodeHash: codeHash, CodeExpiresAt: expiresAt}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertChildCheckin(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, session.BranchID, "participation.child.checked-in", "child-checkin", id, 1, []string{"occurrenceId", "attendanceId", "authorizationId", "codeExpiresAt"}, "", requestID, now)
	}); err != nil {
		return nil, nil, fmt.Errorf("check in child: %w", err)
	}
	return &value, &ChildLabel{CheckinID: id, AttendanceID: attendance.ID, SecurityCode: code, ExpiresAt: expiresAt}, nil
}

func (s Service) PickupChild(ctx context.Context, principal platform.Principal, attendanceID platform.ID, input PickupInput, requestID string) (*PickupReceipt, error) {
	now := s.now()
	checkin, err := s.Store.FindChildCheckinByAttendance(ctx, principal.OrganizationID, attendanceID)
	if err != nil {
		return nil, err
	}
	if checkin == nil || !s.allowedFields(principal, "pickup", "child-checkin", checkin.BranchID, platform.FieldChildSafeguarding) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Child check-in not found."}
	}
	outcome := "released"
	denied := ""
	if checkin.PickedUpAt != nil {
		outcome, denied = "replayed", "Pickup has already been completed."
	} else if !checkin.CodeExpiresAt.After(now) {
		outcome, denied = "expired", "Pickup authorization has expired."
	} else {
		authorization, authErr := s.Store.FindActiveGuardianAuthorization(ctx, principal.OrganizationID, checkin.BranchID, checkin.ChildPersonID, input.GuardianPersonID, now)
		if authErr != nil {
			return nil, authErr
		}
		if authorization == nil {
			outcome, denied = "unauthorized_guardian", "Pickup could not be authorized."
		} else if !s.PickupCodes.Verify(input.SecurityCode, checkin.CodeHash) {
			outcome, denied = "invalid_code", "Pickup could not be authorized."
		}
	}
	event := PickupEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: checkin.BranchID, ChildCheckinID: checkin.ID, AttendanceID: attendanceID, GuardianPersonID: input.GuardianPersonID, Operator: principal.Actor, Outcome: outcome, Reason: denied, OccurredAt: now}
	if outcome != "released" {
		if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
			if err := s.Store.InsertPickupEvent(tx, event); err != nil {
				return err
			}
			return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: checkin.BranchID, Actor: principal.Actor, Action: "participation.child.pickup-denied", ResourceType: "child-checkin", ResourceID: checkin.ID, ChangedFields: []string{"pickupOutcome"}, Outcome: "denied", Reason: outcome, RequestID: requestID, OccurredAt: now})
		}); err != nil {
			return nil, fmt.Errorf("record denied pickup: %w", err)
		}
		return nil, &platform.DomainError{Code: "pickup_denied", Message: denied}
	}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.ReleaseChildCheckin(tx, principal.OrganizationID, checkin.ID, checkin.Version, input.GuardianPersonID, now, principal.Actor); err != nil {
			return err
		}
		if err := s.Store.InsertPickupEvent(tx, event); err != nil {
			return err
		}
		return s.evidence(tx, principal, checkin.BranchID, "participation.child.picked-up", "child-checkin", checkin.ID, checkin.Version+1, []string{"pickedUpAt", "pickedUpByGuardianId", "releasedBy"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("release child: %w", err)
	}
	return &PickupReceipt{ChildCheckinID: checkin.ID, AttendanceID: attendanceID, GuardianPersonID: input.GuardianPersonID, ReleasedAt: now}, nil
}

func (s Service) CreateSafeguardingIncident(ctx context.Context, principal platform.Principal, checkinID platform.ID, category, summary, requestID string) (*SafeguardingIncident, error) {
	category = strings.ToLower(strings.TrimSpace(category))
	summary = strings.TrimSpace(summary)
	if category == "" || len(category) > 80 || len(summary) < 3 || len(summary) > 2000 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_incident", Message: "Category and a 3 to 2000 character summary are required."})
	}
	checkin, err := s.Store.FindChildCheckin(ctx, principal.OrganizationID, checkinID)
	if err != nil {
		return nil, err
	}
	if checkin == nil || !s.allowedFields(principal, "create", "safeguarding-incident", checkin.BranchID, platform.FieldChildSafeguarding) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Child check-in not found."}
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	value := SafeguardingIncident{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: checkin.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, ChildCheckinID: checkinID, Category: category, Summary: summary, Status: "open", OccurredAt: now}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertSafeguardingIncident(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, checkin.BranchID, "participation.safeguarding-incident.created", "safeguarding-incident", id, 1, []string{"childCheckinId", "category", "status"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create safeguarding incident: %w", err)
	}
	return &value, nil
}
