package people

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type HouseholdService struct {
	Repository HouseholdStore
	Platform   UnitOfWork
	Now        func() time.Time
}

func (s HouseholdService) Update(ctx context.Context, organizationID, householdID platform.ID, input UpdateHouseholdInput, actor platform.Actor, requestID string) (*Household, error) {
	if err := input.normalizeAndValidate(organizationID); err != nil {
		if _, ok := err.(*platform.DomainError); ok {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_household", Message: err.Error()})
	}
	current, err := s.Repository.FindHouseholdByID(ctx, organizationID, householdID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Household not found."}
	}
	if input.PrimaryContactPersonID.Valid() {
		membership, membershipErr := s.Repository.FindActiveMembership(ctx, organizationID, householdID, input.PrimaryContactPersonID)
		if membershipErr != nil {
			return nil, membershipErr
		}
		if membership == nil {
			return nil, platform.ValidationError(platform.FieldError{Path: "primaryContactPersonId", Code: "not_household_member", Message: "Primary contact must be an active household member."})
		}
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.UpdateHousehold(tx, organizationID, householdID, input.ExpectedVersion, input, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, s.audit(organizationID, input.HomeBranchID, actor, "people.household.update", "household", householdID, []string{"name", "branch", "sharedContacts", "sharedAddresses", "primaryContact", "statementPreference"}, "", requestID, now)); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, s.event(organizationID, input.HomeBranchID, actor, "people.household.updated", "household", householdID, input.ExpectedVersion+1, requestID, now, map[string]any{"householdId": householdID, "previousBranchId": current.HomeBranchID}))
	}); err != nil {
		return nil, fmt.Errorf("update household: %w", err)
	}
	updated, err := s.Repository.FindHouseholdByID(ctx, organizationID, householdID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s HouseholdService) Create(ctx context.Context, input CreateHouseholdInput, actor platform.Actor, requestID string) (*Household, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_household", Message: err.Error()})
	}
	if s.Repository == nil || s.Platform == nil {
		return nil, fmt.Errorf("household service is not configured")
	}
	for _, member := range input.Members {
		exists, err := s.Repository.PersonExists(ctx, input.OrganizationID, member.PersonID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &platform.DomainError{Code: "not_found", Message: "An initial household member was not found."}
		}
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	household := Household{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: input.OrganizationID, BranchID: input.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: actor, UpdatedAt: now, UpdatedBy: actor}, Name: input.Name, HomeBranchID: input.HomeBranchID, SharedContactPoints: input.SharedContactPoints, SharedAddresses: input.SharedAddresses, PrimaryContactPersonID: input.PrimaryContactPersonID, StatementPreference: input.StatementPreference}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertHousehold(tx, household); err != nil {
			return err
		}
		for _, member := range input.Members {
			membership := HouseholdMembership{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: input.OrganizationID, HouseholdID: id, PersonID: member.PersonID, Role: member.Role, StartedAt: now, CreatedAt: now, CreatedBy: actor}
			if err := s.Repository.InsertMembership(tx, membership); err != nil {
				return err
			}
		}
		if err := s.Platform.AppendAudit(tx, s.audit(input.OrganizationID, input.HomeBranchID, actor, "people.household.create", "household", id, []string{"name", "sharedContacts", "sharedAddresses", "members"}, "", requestID, now)); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, s.event(input.OrganizationID, input.HomeBranchID, actor, "people.household.created", "household", id, 1, requestID, now, map[string]any{"householdId": id, "memberCount": len(input.Members)}))
	})
	if err != nil {
		return nil, fmt.Errorf("create household: %w", err)
	}
	return &household, nil
}

func (s HouseholdService) AddMember(ctx context.Context, organizationID, householdID platform.ID, member HouseholdMemberInput, startedAt time.Time, actor platform.Actor, requestID string) (*HouseholdMembership, error) {
	if err := ValidateHouseholdMembership(householdID, member.PersonID, member.Role); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_membership", Message: err.Error()})
	}
	exists, err := s.Repository.HouseholdExists(ctx, organizationID, householdID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &platform.DomainError{Code: "not_found", Message: "Household not found."}
	}
	exists, err = s.Repository.PersonExists(ctx, organizationID, member.PersonID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	now := s.now()
	if startedAt.IsZero() {
		startedAt = now
	}
	membership := HouseholdMembership{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, HouseholdID: householdID, PersonID: member.PersonID, Role: member.Role, StartedAt: startedAt.UTC(), CreatedAt: now, CreatedBy: actor}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertMembership(tx, membership); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, s.audit(organizationID, "", actor, "people.household.member.add", "household", householdID, []string{"members"}, "", requestID, now)); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, s.event(organizationID, "", actor, "people.household.member.added", "household", householdID, 0, requestID, now, map[string]any{"householdId": householdID, "personId": member.PersonID, "role": member.Role}))
	})
	if err != nil {
		return nil, fmt.Errorf("add household member: %w", err)
	}
	return &membership, nil
}

func (s HouseholdService) EndMember(ctx context.Context, organizationID, householdID, membershipID platform.ID, reason string, actor platform.Actor, requestID string) error {
	if !organizationID.Valid() || !householdID.Valid() || !membershipID.Valid() {
		return platform.ValidationError(platform.FieldError{Path: "$", Code: "required", Message: "Household membership is required."})
	}
	if err := platform.ValidateReason(reason); err != nil {
		return platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	now := s.now()
	return s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.EndMembership(tx, organizationID, membershipID, now, reason); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, s.audit(organizationID, "", actor, "people.household.member.end", "household", householdID, []string{"members"}, reason, requestID, now)); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, s.event(organizationID, "", actor, "people.household.member.ended", "household", householdID, 0, requestID, now, map[string]any{"householdId": householdID, "membershipId": membershipID}))
	})
}

func (s HouseholdService) CreateRelationship(ctx context.Context, organizationID platform.ID, input Relationship, actor platform.Actor, requestID string) (*Relationship, error) {
	input.Type = NormalizeRelationshipType(input.Type)
	if input.Visibility == "" {
		input.Visibility = "staff"
	}
	if err := ValidateRelationship(input.FromPersonID, input.ToPersonID, input.Type, input.Visibility); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_relationship", Message: err.Error()})
	}
	for _, id := range []platform.ID{input.FromPersonID, input.ToPersonID} {
		exists, err := s.Repository.PersonExists(ctx, organizationID, id)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &platform.DomainError{Code: "not_found", Message: "A related person was not found."}
		}
	}
	if input.HouseholdID.Valid() {
		exists, err := s.Repository.HouseholdExists(ctx, organizationID, input.HouseholdID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, &platform.DomainError{Code: "not_found", Message: "Household not found."}
		}
	}
	now := s.now()
	input.ResourceEnvelope = platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: actor, UpdatedAt: now, UpdatedBy: actor}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertRelationship(tx, input); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, s.audit(organizationID, "", actor, "people.relationship.create", "relationship", input.ID, []string{"people", "type", "visibility"}, "", requestID, now)); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, s.event(organizationID, "", actor, "people.relationship.created", "relationship", input.ID, 1, requestID, now, map[string]any{"relationshipId": input.ID, "fromPersonId": input.FromPersonID, "toPersonId": input.ToPersonID, "type": input.Type}))
	})
	if err != nil {
		return nil, fmt.Errorf("create relationship: %w", err)
	}
	return &input, nil
}

func (s HouseholdService) EndRelationship(ctx context.Context, organizationID, relationshipID platform.ID, expectedVersion int64, reason string, actor platform.Actor, requestID string) (*Relationship, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := platform.ValidateReason(reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	relationship, err := s.Repository.FindRelationship(ctx, organizationID, relationshipID)
	if err != nil {
		return nil, err
	}
	if relationship == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Relationship not found."}
	}
	if relationship.EndedAt != nil {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Relationship has already ended."}
	}
	now := s.now()
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.EndRelationship(tx, organizationID, relationshipID, expectedVersion, now, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, s.audit(organizationID, "", actor, "people.relationship.end", "relationship", relationshipID, []string{"endedAt"}, reason, requestID, now)); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, s.event(organizationID, "", actor, "people.relationship.ended", "relationship", relationshipID, expectedVersion+1, requestID, now, map[string]any{"relationshipId": relationshipID}))
	})
	if err != nil {
		return nil, fmt.Errorf("end relationship: %w", err)
	}
	relationship.EndedAt = &now
	relationship.Version = expectedVersion + 1
	relationship.UpdatedAt = now
	relationship.UpdatedBy = actor
	return relationship, nil
}

func (s HouseholdService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s HouseholdService) audit(org, branch platform.ID, actor platform.Actor, action, resource string, id platform.ID, fields []string, reason, requestID string, now time.Time) platform.AuditEvent {
	return platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, BranchID: branch, Actor: actor, Action: action, ResourceType: resource, ResourceID: id, ChangedFields: fields, Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: now}
}
func (s HouseholdService) event(org, branch platform.ID, actor platform.Actor, eventType, aggregate string, id platform.ID, version int64, requestID string, now time.Time, payload any) platform.OutboxRecord {
	return platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, BranchID: branch, Type: eventType, EventVersion: 1, AggregateType: aggregate, AggregateID: id, AggregateVersion: version, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: payload}, State: "pending", AvailableAt: now}
}
