package people

import (
	"context"
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
type Service struct {
	Repository Repository
	Platform   UnitOfWork
	Now        func() time.Time
}

func (s Service) Update(ctx context.Context, organizationID, personID platform.ID, input UpdateInput, actor platform.Actor, requestID string) (*Person, error) {
	if !organizationID.Valid() || !personID.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "personId", Code: "required", Message: "A valid person is required."})
	}
	if err := input.normalizeAndValidate(organizationID); err != nil {
		if _, ok := err.(*platform.DomainError); ok {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_person", Message: err.Error()})
	}
	current, err := s.Repository.FindByID(ctx, organizationID, personID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.Update(tx, organizationID, personID, input.ExpectedVersion, input, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: input.HomeBranchID, Actor: actor, Action: "people.person.update", ResourceType: "person", ResourceID: personID, ChangedFields: []string{"identity", "contacts", "address", "branch", "membershipStage", "tags", "preferences"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: input.HomeBranchID, Type: "people.person.updated", EventVersion: 1, AggregateType: "person", AggregateID: personID, AggregateVersion: input.ExpectedVersion + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"personId": personID, "previousBranchId": current.HomeBranchID}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("update person: %w", err)
	}
	updated, err := s.Repository.FindByID(ctx, organizationID, personID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s Service) Create(ctx context.Context, input CreateInput, actor platform.Actor, requestID string) (*Person, []platform.ID, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_person", Message: err.Error()})
	}
	duplicates, err := s.Repository.FindDuplicateIDs(ctx, input.OrganizationID, input.ContactPoints)
	if err != nil {
		return nil, nil, err
	}
	if len(duplicates) > 0 {
		return nil, duplicates, &platform.DomainError{Code: "duplicate_candidate", Message: "A possible matching person needs review."}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	id := platform.ID(bson.NewObjectID().Hex())
	personNumber := "P-" + bson.NewObjectID().Hex()[12:]
	person := Person{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: input.OrganizationID, BranchID: input.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: actor, UpdatedAt: now, UpdatedBy: actor}, PersonNumber: personNumber, Names: input.Names, Aliases: input.Aliases, PhotoAssetID: input.PhotoAssetID, DateOfBirth: input.DateOfBirth, Gender: input.Gender, ContactPoints: input.ContactPoints, Addresses: input.Addresses, HomeBranchID: input.HomeBranchID, MembershipStage: input.MembershipStage, Tags: input.Tags, CustomFields: input.CustomFields, CommunicationPreferences: input.CommunicationPreferences, Source: input.Source}
	eventID := platform.ID(bson.NewObjectID().Hex())
	auditID := platform.ID(bson.NewObjectID().Hex())
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.Insert(tx, person); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: auditID, OrganizationID: input.OrganizationID, BranchID: input.HomeBranchID, Actor: actor, Action: "people.person.create", ResourceType: "person", ResourceID: id, ChangedFields: []string{"identity", "contacts", "membershipStage"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: eventID, OrganizationID: input.OrganizationID, BranchID: input.HomeBranchID, Type: "people.person.created", EventVersion: 1, AggregateType: "person", AggregateID: id, AggregateVersion: 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"personId": id, "membershipStage": person.MembershipStage}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create person: %w", err)
	}
	return &person, nil, nil
}

func (s Service) SetArchived(ctx context.Context, organizationID, personID platform.ID, expectedVersion int64, archived bool, reason string, actor platform.Actor, requestID string) (*Person, error) {
	if !organizationID.Valid() || !personID.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "personId", Code: "required", Message: "A valid person is required."})
	}
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := platform.ValidateReason(reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	current, err := s.Repository.FindByID(ctx, organizationID, personID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	if archived == (current.ArchivedAt != nil) {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "The person is already in that archive state."}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	var archivedAt *time.Time
	var archivedBy *platform.Actor
	action := "people.person.restore"
	eventType := "people.person.restored"
	if archived {
		archivedAt = &now
		archivedBy = &actor
		action = "people.person.archive"
		eventType = "people.person.archived"
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.SetArchive(tx, organizationID, personID, expectedVersion, archivedAt, archivedBy, reason, now, actor); err != nil {
			return err
		}
		auditID := platform.ID(bson.NewObjectID().Hex())
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: auditID, OrganizationID: organizationID, BranchID: current.HomeBranchID, Actor: actor, Action: action, ResourceType: "person", ResourceID: personID, ChangedFields: []string{"archivedAt", "archiveReason"}, Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		eventID := platform.ID(bson.NewObjectID().Hex())
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: eventID, OrganizationID: organizationID, BranchID: current.HomeBranchID, Type: eventType, EventVersion: 1, AggregateType: "person", AggregateID: personID, AggregateVersion: expectedVersion + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"personId": personID}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("change person archive state: %w", err)
	}
	current.Version = expectedVersion + 1
	current.UpdatedAt = now
	current.UpdatedBy = actor
	current.ArchivedAt = archivedAt
	current.ArchivedBy = archivedBy
	current.ArchiveReason = reason
	return current, nil
}

func (s Service) TransitionMembership(ctx context.Context, organizationID, personID platform.ID, expectedVersion int64, input MembershipTransitionInput, actor platform.Actor, requestID string) (*MembershipEvent, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	person, err := s.Repository.FindByID(ctx, organizationID, personID)
	if err != nil {
		return nil, err
	}
	if person == nil || person.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	if err := input.NormalizeAndValidate(person.MembershipStage, now); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "toStage", Code: "invalid_transition", Message: err.Error()})
	}
	event := MembershipEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, PersonID: personID, FromStage: person.MembershipStage, ToStage: input.ToStage, EffectiveAt: input.EffectiveAt, ReasonCode: input.ReasonCode, NoteReference: input.NoteReference, CreatedAt: now, CreatedBy: actor, RequestID: requestID}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.TransitionMembership(tx, organizationID, personID, expectedVersion, input.ToStage, event, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: person.HomeBranchID, Actor: actor, Action: "people.membership.transition", ResourceType: "person", ResourceID: personID, ChangedFields: []string{"membershipStage"}, Outcome: "success", Reason: input.ReasonCode, RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: person.HomeBranchID, Type: "people.membership.changed", EventVersion: 1, AggregateType: "person", AggregateID: personID, AggregateVersion: expectedVersion + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"personId": personID, "fromStage": event.FromStage, "toStage": event.ToStage, "effectiveAt": event.EffectiveAt}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("transition membership: %w", err)
	}
	return &event, nil
}

func (s Service) ReverseMembershipTransition(ctx context.Context, organizationID, personID, eventID platform.ID, expectedVersion int64, reasonCode string, actor platform.Actor, requestID string) (*MembershipEvent, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	reasonCode = strings.ToLower(strings.TrimSpace(reasonCode))
	if !reasonCodePattern.MatchString(reasonCode) || len(reasonCode) > 64 {
		return nil, platform.ValidationError(platform.FieldError{Path: "reasonCode", Code: "invalid_reason", Message: "Enter a valid reason code."})
	}
	person, err := s.Repository.FindByID(ctx, organizationID, personID)
	if err != nil {
		return nil, err
	}
	if person == nil || person.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Person not found."}
	}
	original, err := s.Repository.FindMembershipEvent(ctx, organizationID, personID, eventID)
	if err != nil {
		return nil, err
	}
	if original == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Membership transition not found."}
	}
	if original.ReversedByEventID.Valid() {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Membership transition has already been reversed."}
	}
	if person.MembershipStage != original.ToStage {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Only the current membership transition can be reversed."}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	reversal := MembershipEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, PersonID: personID, FromStage: original.ToStage, ToStage: original.FromStage, EffectiveAt: now, ReasonCode: reasonCode, ReversesEventID: original.ID, CreatedAt: now, CreatedBy: actor, RequestID: requestID}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.ReverseMembershipEvent(tx, organizationID, personID, expectedVersion, reversal, original.ID, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: person.HomeBranchID, Actor: actor, Action: "people.membership.reverse", ResourceType: "person", ResourceID: personID, ChangedFields: []string{"membershipStage"}, Outcome: "success", Reason: reasonCode, RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: person.HomeBranchID, Type: "people.membership.reversed", EventVersion: 1, AggregateType: "person", AggregateID: personID, AggregateVersion: expectedVersion + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"personId": personID, "reversedEventId": original.ID, "fromStage": reversal.FromStage, "toStage": reversal.ToStage}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("reverse membership transition: %w", err)
	}
	return &reversal, nil
}
