package consent

import (
	"context"
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
	Repository *Repository
	Platform   UnitOfWork
	Authorizer platform.Authorizer
	Now        func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) allowed(p platform.Principal, action string, branchID, personID platform.ID) bool {
	if p.Actor.Type == platform.ActorMember {
		return p.Actor.ID == personID
	}
	return s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "consent", OrganizationID: p.OrganizationID, BranchID: branchID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: s.now()}).Allowed
}
func (s Service) List(ctx context.Context, p platform.Principal, branchID, personID platform.ID) ([]Projection, error) {
	if !s.allowed(p, "read", branchID, personID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Consent preferences not found."}
	}
	return s.Repository.ListProjections(ctx, p.OrganizationID, personID)
}
func (s Service) Apply(ctx context.Context, p platform.Principal, branchID, personID platform.ID, choice Choice, requestID string) (*Projection, error) {
	if err := choice.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_consent", Message: err.Error()})
	}
	if !branchID.Valid() || !personID.Valid() || !s.allowed(p, "update", branchID, personID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Consent preferences not found."}
	}
	now := s.now()
	current, err := s.Repository.FindProjection(ctx, p.OrganizationID, personID, choice.Purpose, choice.Channel)
	if err != nil {
		return nil, err
	}
	if current == nil && choice.ExpectedVersion != 0 {
		return nil, platform.VersionConflict(choice.ExpectedVersion)
	}
	if current != nil && current.Version != choice.ExpectedVersion {
		return nil, platform.VersionConflict(choice.ExpectedVersion)
	}
	event := Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branchID, PersonID: personID, Purpose: choice.Purpose, Channel: choice.Channel, State: choice.State, NoticeVersion: choice.NoticeVersion, EvidenceReference: choice.EvidenceReference, Source: choice.Source, Actor: p.Actor, RequestID: requestID, OccurredAt: now}
	value := Projection{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, PersonID: personID, Purpose: choice.Purpose, Channel: choice.Channel, State: choice.State, NoticeVersion: choice.NoticeVersion, EvidenceReference: choice.EvidenceReference, Source: choice.Source, LastEventID: event.ID}
	if current != nil {
		value.ResourceEnvelope = current.ResourceEnvelope
		value.UpdatedAt = now
		value.UpdatedBy = p.Actor
		value.Version = current.Version + 1
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertEvent(tx, event); err != nil {
			return err
		}
		if current == nil {
			if err := s.Repository.InsertProjection(tx, value); err != nil {
				return err
			}
		} else if err := s.Repository.UpdateProjection(tx, value, current.Version); err != nil {
			return err
		}
		if err := s.Repository.UpsertMemberWithdrawal(tx, value, now, p.Actor); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branchID, Actor: p.Actor, Action: "consent.preference." + choice.State, ResourceType: "consent", ResourceID: value.ID, SubjectIDs: []platform.ID{personID}, ChangedFields: []string{"purpose", "channel", "state", "noticeVersion"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindProjection(ctx, p.OrganizationID, personID, choice.Purpose, choice.Channel)
}
func (s Service) Evaluate(ctx context.Context, p platform.Principal, branchID, personID platform.ID, purpose, channel string) (*Eligibility, error) {
	choice := Choice{Purpose: purpose, Channel: channel, State: "granted", NoticeVersion: "evaluation", Source: "evaluation"}
	if err := choice.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_eligibility", Message: err.Error()})
	}
	if !s.allowed(p, "read", branchID, personID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Consent preferences not found."}
	}
	projection, err := s.Repository.FindProjection(ctx, p.OrganizationID, personID, choice.Purpose, choice.Channel)
	if err != nil {
		return nil, err
	}
	blocks, err := s.Repository.ListActiveSuppressions(ctx, p.OrganizationID, personID, choice.Purpose, choice.Channel)
	if err != nil {
		return nil, err
	}
	decision := "no-grant"
	eligible := projection != nil && projection.State == "granted" && len(blocks) == 0
	if projection != nil && projection.State == "withdrawn" {
		decision = "withdrawn"
	} else if len(blocks) > 0 {
		decision = "suppressed"
	} else if eligible {
		decision = "eligible"
	}
	return &Eligibility{Eligible: eligible, Consent: projection, BlockingReasons: blocks, Decision: decision, EvaluatedAt: s.now()}, nil
}
func (s Service) ApplySuppression(ctx context.Context, p platform.Principal, branchID, personID platform.ID, input SuppressionInput, requestID string) (*Suppression, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_suppression", Message: err.Error()})
	}
	if p.Actor.Type == platform.ActorMember || !s.allowed(p, "update", branchID, personID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Consent preferences not found."}
	}
	now := s.now()
	var current *Suppression
	var err error
	if input.ID.Valid() {
		current, err = s.Repository.FindSuppression(ctx, p.OrganizationID, input.ID)
		if err != nil {
			return nil, err
		}
		if current == nil || current.PersonID != personID || current.Version != input.ExpectedVersion {
			return nil, platform.VersionConflict(input.ExpectedVersion)
		}
	} else if input.ExpectedVersion != 0 {
		return nil, platform.VersionConflict(input.ExpectedVersion)
	}
	value := Suppression{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, PersonID: personID, Purpose: input.Purpose, Channel: input.Channel, Reason: input.Reason, Source: input.Source, State: input.State}
	if current != nil {
		value = *current
		value.State = input.State
		value.Reason = input.Reason
		value.Source = input.Source
		value.UpdatedAt = now
		value.UpdatedBy = p.Actor
		value.Version = current.Version + 1
	}
	if input.State == "released" {
		value.ReleasedAt = &now
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if current == nil {
			if err := s.Repository.InsertSuppression(tx, value); err != nil {
				return err
			}
		} else if err := s.Repository.UpdateSuppression(tx, value, current.Version); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branchID, Actor: p.Actor, Action: "consent.suppression." + input.State, ResourceType: "suppression", ResourceID: value.ID, SubjectIDs: []platform.ID{personID}, ChangedFields: []string{"purpose", "channel", "reason", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	if err != nil {
		return nil, err
	}
	return s.Repository.FindSuppression(ctx, p.OrganizationID, value.ID)
}
