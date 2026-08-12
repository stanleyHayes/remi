package community

import (
	"context"
	"fmt"
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
type VolunteerEligibilityResolver interface {
	ResolveVolunteerEligibility(context.Context, platform.ID, platform.ID) (platform.ID, bool, string, string, string, error)
}
type OccurrenceReferenceResolver interface {
	ResolveOccurrenceReference(context.Context, platform.ID, platform.ID) (platform.ID, time.Time, time.Time, string, error)
}
type Service struct {
	Store           Store
	Platform        UnitOfWork
	People          PersonReferenceResolver
	VolunteerPeople VolunteerEligibilityResolver
	Occurrences     OccurrenceReferenceResolver
	Authorizer      platform.Authorizer
	Now             func() time.Time
}

func (s Service) CreateGroup(ctx context.Context, principal platform.Principal, input GroupInput, requestID string) (*Group, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_group", Message: err.Error()})
	}
	if !s.allowed(principal, "create", input.HomeBranchID, input.MinistryID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create groups in this branch."}
	}
	if err := s.validateLeaders(ctx, principal.OrganizationID, input.LeaderPersonIDs); err != nil {
		return nil, err
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	value := Group{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: principal.OrganizationID, BranchID: input.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, Name: input.Name, Type: input.Type, Description: input.Description, HomeBranchID: input.HomeBranchID, MinistryID: input.MinistryID, LeaderPersonIDs: input.LeaderPersonIDs, Capacity: input.Capacity, MeetingPattern: input.MeetingPattern, Privacy: input.Privacy, Discoverability: input.Discoverability, Status: input.Status}
	if input.Status == "closed" {
		value.ClosedAt = &now
		value.ClosureReason = input.Reason
	}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertGroup(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, principal, value, "community.group.created", []string{"name", "type", "branch", "ministry", "leaders", "capacity", "meetingPattern", "privacy", "discoverability", "status"}, input.Reason, requestID)
	}); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return &value, nil
}
func (s Service) UpdateGroup(ctx context.Context, principal platform.Principal, id platform.ID, expectedVersion int64, input GroupInput, requestID string) (*Group, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_group", Message: err.Error()})
	}
	current, err := s.Store.FindGroup(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || !s.allowed(principal, "update", current.HomeBranchID, current.MinistryID) || !s.allowed(principal, "update", input.HomeBranchID, input.MinistryID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	if err = validateTransition(current.Status, input.Status, input.Reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "status", Code: "invalid_transition", Message: err.Error()})
	}
	if err = s.validateLeaders(ctx, principal.OrganizationID, input.LeaderPersonIDs); err != nil {
		return nil, err
	}
	now := s.now()
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateGroup(tx, principal.OrganizationID, id, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		updated := *current
		updated.HomeBranchID = input.HomeBranchID
		updated.MinistryID = input.MinistryID
		updated.Status = input.Status
		updated.Version = expectedVersion + 1
		return s.evidence(tx, principal, updated, "community.group.updated", []string{"name", "type", "branch", "ministry", "leaders", "capacity", "meetingPattern", "privacy", "discoverability", "status"}, input.Reason, requestID)
	}); err != nil {
		return nil, fmt.Errorf("update group: %w", err)
	}
	return s.Store.FindGroup(ctx, principal.OrganizationID, id)
}
func (s Service) GetGroup(ctx context.Context, principal platform.Principal, id platform.ID) (*Group, error) {
	value, err := s.Store.FindGroup(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.canLeadGroup(principal, *value, "read") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Group not found."}
	}
	return value, nil
}
func (s Service) ListGroups(ctx context.Context, principal platform.Principal, branchID platform.ID, includeClosed bool) ([]Group, error) {
	if !branchID.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "Choose a branch."})
	}
	values, err := s.Store.ListGroups(ctx, principal.OrganizationID, branchID, includeClosed)
	if err != nil {
		return nil, err
	}
	visible := make([]Group, 0, len(values))
	for _, value := range values {
		if s.allowed(principal, "read", value.HomeBranchID, value.MinistryID) {
			visible = append(visible, value)
		}
	}
	return visible, nil
}
func (s Service) validateLeaders(ctx context.Context, organizationID platform.ID, ids []platform.ID) error {
	if s.People == nil && len(ids) > 0 {
		return fmt.Errorf("person reference resolver is unavailable")
	}
	for _, id := range ids {
		_, active, err := s.People.ResolvePersonReference(ctx, organizationID, id)
		if err != nil {
			return err
		}
		if !active {
			return platform.ValidationError(platform.FieldError{Path: "leaderPersonIds", Code: "invalid_leader", Message: "Every leader must reference an active person."})
		}
	}
	return nil
}
func (s Service) allowed(principal platform.Principal, action string, branch, ministry platform.ID) bool {
	return s.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: "group", OrganizationID: principal.OrganizationID, BranchID: branch, MinistryID: ministry, FieldClasses: []platform.FieldClass{platform.FieldOperational}, Now: s.now()}).Allowed
}
func (s Service) evidence(ctx context.Context, principal platform.Principal, value Group, eventType string, fields []string, reason, requestID string) error {
	now := s.now()
	audit := platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: value.HomeBranchID, Actor: principal.Actor, Action: eventType, ResourceType: "group", ResourceID: value.ID, ChangedFields: fields, Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: now}
	if err := s.Platform.AppendAudit(ctx, audit); err != nil {
		return err
	}
	return s.Platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: value.HomeBranchID, Type: eventType, EventVersion: 1, AggregateType: "group", AggregateID: value.ID, AggregateVersion: value.Version, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"groupId": value.ID, "ministryId": value.MinistryID, "status": value.Status}}, State: "pending", AvailableAt: now})
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
