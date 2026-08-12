package participation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type UnitOfWork interface {
	WithTransaction(context.Context, func(context.Context) error) error
	AppendAudit(context.Context, platform.AuditEvent) error
	EnqueueEvent(context.Context, platform.OutboxRecord) error
}
type PersonReferenceResolver interface {
	ResolvePersonReference(context.Context, platform.ID, platform.ID) (platform.ID, bool, error)
}
type Service struct {
	Store       Store
	Platform    UnitOfWork
	People      PersonReferenceResolver
	PickupCodes *PickupCodeManager
	Authorizer  platform.Authorizer
	Now         func() time.Time
}

func (s Service) CreateDefinition(ctx context.Context, principal platform.Principal, input DefinitionInput, requestID string) (*ServiceDefinition, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_service_definition", Message: err.Error()})
	}
	if !s.allowed(principal, "create", "service-definition", input.HomeBranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create services in this branch."}
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	value := ServiceDefinition{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: input.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, Name: input.Name, Description: input.Description, HomeBranchID: input.HomeBranchID, Timezone: input.Timezone, DefaultDurationMinutes: input.DefaultDurationMinutes, DefaultRoomIDs: input.DefaultRoomIDs, DefaultCapacity: input.DefaultCapacity, Recurrence: input.Recurrence, Status: input.Status}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertDefinition(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, value.BranchID, "participation.service-definition.created", "service-definition", id, 1, []string{"name", "branch", "timezone", "recurrence", "rooms", "capacity"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create service definition: %w", err)
	}
	return &value, nil
}
func (s Service) UpdateDefinition(ctx context.Context, principal platform.Principal, id platform.ID, expectedVersion int64, input DefinitionInput, requestID string) (*ServiceDefinition, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_service_definition", Message: err.Error()})
	}
	current, err := s.Store.FindDefinition(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service definition not found."}
	}
	if !s.allowed(principal, "update", "service-definition", current.HomeBranchID) || !s.allowed(principal, "update", "service-definition", input.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service definition not found."}
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateDefinition(tx, principal.OrganizationID, id, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		return s.evidence(tx, principal, input.HomeBranchID, "participation.service-definition.updated", "service-definition", id, expectedVersion+1, []string{"name", "branch", "timezone", "recurrence", "rooms", "capacity", "status"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("update service definition: %w", err)
	}
	updated, err := s.Store.FindDefinition(ctx, principal.OrganizationID, id)
	return updated, err
}
func (s Service) ListDefinitions(ctx context.Context, principal platform.Principal, branchID platform.ID, includeInactive bool) ([]ServiceDefinition, error) {
	if !s.allowed(principal, "read", "service-definition", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read services in this branch."}
	}
	return s.Store.ListDefinitions(ctx, principal.OrganizationID, branchID, includeInactive)
}
func (s Service) CreateOccurrence(ctx context.Context, principal platform.Principal, input OccurrenceInput, requestID string) (*Occurrence, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_occurrence", Message: err.Error()})
	}
	if !s.allowed(principal, "create", "occurrence", input.HomeBranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create occurrences in this branch."}
	}
	if input.ServiceDefinitionID.Valid() {
		definition, err := s.Store.FindDefinition(ctx, principal.OrganizationID, input.ServiceDefinitionID)
		if err != nil {
			return nil, err
		}
		if definition == nil || definition.Status != "active" || definition.HomeBranchID != input.HomeBranchID {
			return nil, platform.ValidationError(platform.FieldError{Path: "serviceDefinitionId", Code: "invalid_definition", Message: "Choose an active service definition in the same branch."})
		}
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	keySource := input.ServiceDefinitionID
	if !keySource.Valid() {
		keySource = platform.ID("manual:" + string(id))
	}
	value := Occurrence{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: input.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, ServiceDefinitionID: input.ServiceDefinitionID, HomeBranchID: input.HomeBranchID, OccurrenceKey: OccurrenceKey(principal.OrganizationID, keySource, input.StartsAt), Name: input.Name, StartsAt: input.StartsAt, EndsAt: input.EndsAt, Timezone: input.Timezone, RoomIDs: input.RoomIDs, Capacity: input.Capacity, Status: "scheduled"}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertOccurrence(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, input.HomeBranchID, "participation.occurrence.created", "occurrence", id, 1, []string{"definition", "startsAt", "endsAt", "rooms", "capacity"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create occurrence: %w", err)
	}
	return &value, nil
}
func (s Service) UpdateOccurrence(ctx context.Context, principal platform.Principal, id platform.ID, expectedVersion int64, input OccurrenceInput, requestID string) (*Occurrence, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_occurrence", Message: err.Error()})
	}
	current, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status == "cancelled" {
		return nil, &platform.DomainError{Code: "not_found", Message: "Active occurrence not found."}
	}
	if input.ServiceDefinitionID != current.ServiceDefinitionID {
		return nil, platform.ValidationError(platform.FieldError{Path: "serviceDefinitionId", Code: "immutable", Message: "An occurrence cannot change its service definition."})
	}
	if !s.allowed(principal, "update", "occurrence", current.HomeBranchID) || !s.allowed(principal, "update", "occurrence", input.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateOccurrence(tx, principal.OrganizationID, id, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		return s.evidence(tx, principal, input.HomeBranchID, "participation.occurrence.updated", "occurrence", id, expectedVersion+1, []string{"branch", "name", "startsAt", "endsAt", "timezone", "rooms", "capacity"}, "", requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("update occurrence: %w", err)
	}
	updated, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, id)
	return updated, err
}
func (s Service) CancelOccurrence(ctx context.Context, principal platform.Principal, id platform.ID, expectedVersion int64, reason, requestID string) (*Occurrence, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := platform.ValidateReason(reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	current, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status == "cancelled" {
		return nil, &platform.DomainError{Code: "not_found", Message: "Active occurrence not found."}
	}
	if !s.allowed(principal, "cancel", "occurrence", current.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.CancelOccurrence(tx, principal.OrganizationID, id, expectedVersion, now, reason, principal.Actor); err != nil {
			return err
		}
		return s.evidence(tx, principal, current.HomeBranchID, "participation.occurrence.cancelled", "occurrence", id, expectedVersion+1, []string{"status", "cancelledAt"}, reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("cancel occurrence: %w", err)
	}
	updated, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, id)
	return updated, err
}
func (s Service) ListOccurrences(ctx context.Context, principal platform.Principal, branchID platform.ID, from, to time.Time) ([]Occurrence, error) {
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a valid occurrence range no longer than 366 days."})
	}
	if !s.allowed(principal, "read", "occurrence", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read occurrences in this branch."}
	}
	return s.Store.ListOccurrences(ctx, principal.OrganizationID, branchID, from, to)
}
func (s Service) GenerateOccurrences(ctx context.Context, principal platform.Principal, definitionID platform.ID, from, to time.Time, requestID string) ([]Occurrence, error) {
	definition, err := s.Store.FindDefinition(ctx, principal.OrganizationID, definitionID)
	if err != nil {
		return nil, err
	}
	if definition == nil || definition.Status != "active" {
		return nil, &platform.DomainError{Code: "not_found", Message: "Active service definition not found."}
	}
	if !s.allowed(principal, "create", "occurrence", definition.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service definition not found."}
	}
	inputs, err := ExpandRecurrence(*definition, from, to)
	if err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_generation", Message: err.Error()})
	}
	results := make([]Occurrence, 0, len(inputs))
	for _, input := range inputs {
		key := OccurrenceKey(principal.OrganizationID, definition.ID, input.StartsAt)
		existing, findErr := s.Store.FindOccurrenceByKey(ctx, principal.OrganizationID, key)
		if findErr != nil {
			return nil, findErr
		}
		if existing != nil {
			results = append(results, *existing)
			continue
		}
		created, createErr := s.CreateOccurrence(ctx, principal, input, requestID)
		if createErr != nil {
			existing, findErr = s.Store.FindOccurrenceByKey(ctx, principal.OrganizationID, key)
			if findErr == nil && existing != nil {
				results = append(results, *existing)
				continue
			}
			return nil, createErr
		}
		results = append(results, *created)
	}
	return results, nil
}

func (s Service) RecordAttendance(ctx context.Context, principal platform.Principal, occurrenceID, personID platform.ID, expectedVersion int64, input AttendanceInput, requestID string) (*AttendanceFact, error) {
	return s.recordAttendance(ctx, principal, occurrenceID, personID, expectedVersion, input, requestID, false)
}

func (s Service) recordAttendance(ctx context.Context, principal platform.Principal, occurrenceID, personID platform.ID, expectedVersion int64, input AttendanceInput, requestID string, allowClosed bool) (*AttendanceFact, error) {
	if !personID.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "personId", Code: "required", Message: "Choose a person."})
	}
	if s.People == nil {
		return nil, errors.New("person reference resolver is unavailable")
	}
	personBranch, active, err := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_attendance", Message: err.Error()})
	}
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || occurrence.Status == "cancelled" {
		return nil, &platform.DomainError{Code: "not_found", Message: "Active occurrence not found."}
	}
	if !s.allowed(principal, "record", "attendance", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	if !s.allowed(principal, "read", "person", personBranch) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	if !allowClosed {
		closed, err := s.attendanceClosed(ctx, principal.OrganizationID, *occurrence)
		if err != nil {
			return nil, err
		}
		if closed {
			return nil, &platform.DomainError{Code: "attendance_locked", Message: "Attendance is closed. Submit an approved correction instead."}
		}
	}
	if input.CheckedInAt != nil && (input.CheckedInAt.Before(occurrence.StartsAt.Add(-24*time.Hour)) || input.CheckedInAt.After(occurrence.EndsAt.Add(24*time.Hour))) {
		return nil, platform.ValidationError(platform.FieldError{Path: "checkedInAt", Code: "outside_occurrence_window", Message: "Check-in must be within 24 hours of the occurrence."})
	}
	if input.CheckedOutAt != nil && (input.CheckedOutAt.Before(occurrence.StartsAt.Add(-24*time.Hour)) || input.CheckedOutAt.After(occurrence.EndsAt.Add(24*time.Hour))) {
		return nil, platform.ValidationError(platform.FieldError{Path: "checkedOutAt", Code: "outside_occurrence_window", Message: "Check-out must be within 24 hours of the occurrence."})
	}
	current, err := s.Store.FindAttendance(ctx, principal.OrganizationID, occurrenceID, personID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if current == nil && expectedVersion > 0 {
		return nil, platform.VersionConflict(0)
	}
	if current != nil {
		if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
			return nil, err
		}
		if expectedVersion != current.Version {
			return nil, platform.VersionConflict(current.Version)
		}
	}
	late := now.After(occurrence.EndsAt)
	if late {
		if err := platform.ValidateReason(input.Reason); err != nil {
			return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "late_correction_reason_required", Message: "A reason is required after the service has ended."})
		}
	}
	id := platform.ID(bson.NewObjectID().Hex())
	version := int64(1)
	eventType := "recorded"
	var before *AttendanceSnapshot
	if current != nil {
		id = current.ID
		version = current.Version + 1
		eventType = "updated"
		snapshot := attendanceSnapshot(*current)
		before = &snapshot
		if late {
			eventType = "corrected"
		}
	}
	value := AttendanceFact{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: occurrence.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: version, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, OccurrenceID: occurrenceID, PersonID: personID, Status: input.Status, Source: input.Source, Confidence: input.Confidence, CheckedInAt: cloneTime(input.CheckedInAt), CheckedOutAt: cloneTime(input.CheckedOutAt), StationID: input.StationID, OperatorID: principal.Actor.ID, Guest: input.Guest, SyncCommandID: input.SyncCommandID}
	if current != nil {
		value.CreatedAt = current.CreatedAt
		value.CreatedBy = current.CreatedBy
	}
	event := AttendanceEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: occurrence.HomeBranchID, AttendanceID: id, OccurrenceID: occurrenceID, PersonID: personID, Type: eventType, Before: before, After: attendanceSnapshot(value), Reason: input.Reason, Actor: principal.Actor, RequestID: requestID, OccurredAt: now}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if current == nil {
			if err := s.Store.InsertAttendance(tx, value); err != nil {
				return err
			}
		} else if err := s.Store.UpdateAttendance(tx, principal.OrganizationID, occurrenceID, personID, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		if err := s.Store.InsertAttendanceEvent(tx, event); err != nil {
			return err
		}
		return s.evidence(tx, principal, occurrence.HomeBranchID, "participation.attendance."+eventType, "attendance", id, version, []string{"status", "source", "confidence", "checkIn", "checkOut", "guest"}, input.Reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("record attendance: %w", err)
	}
	return s.Store.FindAttendance(ctx, principal.OrganizationID, occurrenceID, personID)
}

func (s Service) CorrectAttendance(ctx context.Context, principal platform.Principal, occurrenceID, personID platform.ID, expectedVersion int64, input AttendanceInput, requestID string) (*AttendanceFact, error) {
	if err := platform.ValidateReason(input.Reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "correction_reason_required", Message: "A correction reason is required."})
	}
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || !s.allowed(principal, "approve", "attendance-correction", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	closed, err := s.attendanceClosed(ctx, principal.OrganizationID, *occurrence)
	if err != nil {
		return nil, err
	}
	if !closed {
		return nil, &platform.DomainError{Code: "conflict", Message: "Use normal attendance recording until attendance is locked."}
	}
	return s.recordAttendance(ctx, principal, occurrenceID, personID, expectedVersion, input, requestID, true)
}

func (s Service) LockAttendance(ctx context.Context, principal platform.Principal, occurrenceID platform.ID, reason, requestID string) (*AttendanceLock, error) {
	if err := platform.ValidateReason(reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "lock_reason_required", Message: err.Error()})
	}
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if occurrence == nil || !s.allowed(principal, "approve", "attendance-lock", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	if now.Before(occurrence.EndsAt) {
		return nil, &platform.DomainError{Code: "conflict", Message: "Attendance can only be locked after the occurrence ends."}
	}
	value := AttendanceLock{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: occurrence.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, OccurrenceID: occurrenceID, Reason: strings.TrimSpace(reason), LockedAt: now, LockedBy: principal.Actor}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertAttendanceLock(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, occurrence.HomeBranchID, "participation.attendance.locked", "attendance-lock", value.ID, 1, []string{"occurrenceId", "lockedAt"}, value.Reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("lock attendance: %w", err)
	}
	return &value, nil
}

func (s Service) CloseAttendancePeriod(ctx context.Context, principal platform.Principal, input PeriodCloseInput, requestID string) (*AttendancePeriodClose, error) {
	now := s.now()
	if err := input.NormalizeAndValidate(now); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_period_close", Message: err.Error()})
	}
	if !s.allowed(principal, "approve", "attendance-period", input.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot close attendance periods in this branch."}
	}
	value := AttendancePeriodClose{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: input.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, StartsAt: input.StartsAt, EndsAt: input.EndsAt, Reason: input.Reason, ClosedAt: now, ClosedBy: principal.Actor}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertAttendancePeriodClose(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, input.BranchID, "participation.attendance-period.closed", "attendance-period", value.ID, 1, []string{"startsAt", "endsAt", "closedAt"}, value.Reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("close attendance period: %w", err)
	}
	return &value, nil
}

func (s Service) ListAttendancePeriodCloses(ctx context.Context, principal platform.Principal, branchID platform.ID) ([]AttendancePeriodClose, error) {
	if !s.allowed(principal, "read", "attendance-period", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read attendance periods in this branch."}
	}
	return s.Store.ListAttendancePeriodCloses(ctx, principal.OrganizationID, branchID)
}

func (s Service) GetAttendanceControl(ctx context.Context, principal platform.Principal, occurrenceID platform.ID) (*AttendanceControlStatus, error) {
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || !s.allowed(principal, "read", "attendance", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	lock, err := s.Store.FindAttendanceLock(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	period, err := s.Store.FindClosedAttendancePeriod(ctx, principal.OrganizationID, occurrence.HomeBranchID, occurrence.StartsAt)
	if err != nil {
		return nil, err
	}
	return &AttendanceControlStatus{Locked: lock != nil || period != nil, Lock: lock, Period: period}, nil
}

func (s Service) attendanceClosed(ctx context.Context, organizationID platform.ID, occurrence Occurrence) (bool, error) {
	lock, err := s.Store.FindAttendanceLock(ctx, organizationID, occurrence.ID)
	if err != nil || lock != nil {
		return lock != nil, err
	}
	period, err := s.Store.FindClosedAttendancePeriod(ctx, organizationID, occurrence.HomeBranchID, occurrence.StartsAt)
	return period != nil, err
}

func (s Service) ListAttendance(ctx context.Context, principal platform.Principal, occurrenceID platform.ID) ([]AttendanceFact, error) {
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || !s.allowed(principal, "read", "attendance", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	return s.Store.ListAttendance(ctx, principal.OrganizationID, occurrenceID)
}

func (s Service) ListAttendanceEvents(ctx context.Context, principal platform.Principal, occurrenceID, personID platform.ID) ([]AttendanceEvent, error) {
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || !s.allowed(principal, "read", "attendance", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	return s.Store.ListAttendanceEvents(ctx, principal.OrganizationID, occurrenceID, personID, 200)
}

func (s Service) CreateHeadcount(ctx context.Context, principal platform.Principal, occurrenceID platform.ID, input HeadcountInput, requestID string) (*Headcount, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_headcount", Message: err.Error()})
	}
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || occurrence.Status == "cancelled" || !s.allowed(principal, "record", "headcount", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Active occurrence not found."}
	}
	if input.ObservedAt.Before(occurrence.StartsAt.Add(-24*time.Hour)) || input.ObservedAt.After(occurrence.EndsAt.Add(24*time.Hour)) {
		return nil, platform.ValidationError(platform.FieldError{Path: "observedAt", Code: "outside_occurrence_window", Message: "Observation must be within 24 hours of the occurrence."})
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	value := Headcount{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: occurrence.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, OccurrenceID: occurrenceID, Category: input.Category, RoomID: input.RoomID, Count: input.Count, Source: input.Source, Confidence: input.Confidence, ObservedAt: input.ObservedAt, Reason: input.Reason}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertHeadcount(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, occurrence.HomeBranchID, "participation.headcount.recorded", "headcount", id, 1, []string{"category", "room", "count", "source", "confidence", "observedAt"}, input.Reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create headcount: %w", err)
	}
	return &value, nil
}

func (s Service) UpdateHeadcount(ctx context.Context, principal platform.Principal, occurrenceID, headcountID platform.ID, expectedVersion int64, input HeadcountInput, requestID string) (*Headcount, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_headcount", Message: err.Error()})
	}
	if err := platform.ValidateReason(input.Reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "correction_reason_required", Message: "A reason is required to correct a headcount."})
	}
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	current, err := s.Store.FindHeadcount(ctx, principal.OrganizationID, headcountID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || current == nil || current.OccurrenceID != occurrenceID || !s.allowed(principal, "record", "headcount", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Headcount not found."}
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateHeadcount(tx, principal.OrganizationID, headcountID, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		return s.evidence(tx, principal, occurrence.HomeBranchID, "participation.headcount.corrected", "headcount", headcountID, expectedVersion+1, []string{"category", "room", "count", "source", "confidence", "observedAt"}, input.Reason, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("update headcount: %w", err)
	}
	return s.Store.FindHeadcount(ctx, principal.OrganizationID, headcountID)
}

func (s Service) ListHeadcounts(ctx context.Context, principal platform.Principal, occurrenceID platform.ID) ([]Headcount, error) {
	occurrence, err := s.Store.FindOccurrence(ctx, principal.OrganizationID, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil || !s.allowed(principal, "read", "headcount", occurrence.HomeBranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."}
	}
	return s.Store.ListHeadcounts(ctx, principal.OrganizationID, occurrenceID)
}
func (s Service) allowed(principal platform.Principal, action, resource string, branch platform.ID) bool {
	if s.Authorizer == nil {
		return false
	}
	return s.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: resource, OrganizationID: principal.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldOperational}, Now: s.now()}).Allowed
}
func (s Service) allowedFields(principal platform.Principal, action, resource string, branch platform.ID, fields ...platform.FieldClass) bool {
	if s.Authorizer == nil {
		return false
	}
	return s.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: resource, OrganizationID: principal.OrganizationID, BranchID: branch, FieldClasses: fields, Now: s.now()}).Allowed
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) evidence(ctx context.Context, principal platform.Principal, branch platform.ID, eventType, resource string, id platform.ID, version int64, fields []string, reason, requestID string, now time.Time) error {
	action := eventType
	if err := s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branch, Actor: principal.Actor, Action: action, ResourceType: resource, ResourceID: id, ChangedFields: fields, Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: now}); err != nil {
		return err
	}
	return s.Platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branch, Type: eventType, EventVersion: 1, AggregateType: resource, AggregateID: id, AggregateVersion: version, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"id": id}}, State: "pending", AvailableAt: now})
}
