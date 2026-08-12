package care

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type AssimilationEvidence struct {
	Type       string      `json:"type" bson:"type"`
	SourceID   platform.ID `json:"sourceId" bson:"sourceId"`
	OccurredAt time.Time   `json:"occurredAt" bson:"occurredAt"`
}
type AssimilationProgress struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonID                  platform.ID            `json:"personId" bson:"personId"`
	DefinitionID              platform.ID            `json:"definitionId" bson:"definitionId"`
	WorkflowInstanceID        platform.ID            `json:"workflowInstanceId" bson:"workflowInstanceId"`
	Pathway                   string                 `json:"pathway" bson:"pathway"`
	VisitCount                int                    `json:"visitCount" bson:"visitCount"`
	LastVisitAt               time.Time              `json:"lastVisitAt" bson:"lastVisitAt"`
	Evidence                  []AssimilationEvidence `json:"evidence" bson:"evidence"`
}
type AttendanceEvidenceResolver interface {
	ResolveAttendanceEvidence(context.Context, platform.ID, platform.ID, platform.ID) (platform.ID, string, time.Time, error)
}
type AssimilationInput struct {
	DefinitionID platform.ID `json:"definitionId"`
	OccurrenceID platform.ID `json:"occurrenceId"`
	PersonID     platform.ID `json:"personId"`
	OwnerID      platform.ID `json:"ownerId"`
}

func (s Service) RecordAssimilationVisit(ctx context.Context, principal platform.Principal, input AssimilationInput, requestID string) (*AssimilationProgress, bool, error) {
	if s.Attendance == nil || !input.DefinitionID.Valid() || !input.OccurrenceID.Valid() || !input.PersonID.Valid() || !input.OwnerID.Valid() {
		return nil, false, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_trigger", Message: "Definition, occurrence, person and owner are required."})
	}
	branch, attendanceStatus, occurredAt, err := s.Attendance.ResolveAttendanceEvidence(ctx, principal.OrganizationID, input.OccurrenceID, input.PersonID)
	if err != nil {
		return nil, false, err
	}
	if attendanceStatus != "present" {
		return nil, false, platform.ValidationError(platform.FieldError{Path: "occurrenceId", Code: "attendance_required", Message: "A named present attendance fact is required."})
	}
	definition, err := s.Store.FindDefinition(ctx, principal.OrganizationID, input.DefinitionID)
	if err != nil {
		return nil, false, err
	}
	if definition == nil || definition.Status != "published" || definition.BranchID != branch || !s.allowed(principal, "create", branch) {
		return nil, false, &platform.DomainError{Code: "not_found", Message: "Published assimilation workflow not found."}
	}
	if definition.InitialStageKey != "first-visit" || !allows(definition.Transitions, "first-visit", "second-visit") {
		return nil, false, platform.ValidationError(platform.FieldError{Path: "definitionId", Code: "invalid_assimilation_template", Message: "Assimilation workflows must begin at first-visit and allow second-visit."})
	}
	existing, err := s.Store.FindAssimilationProgress(ctx, principal.OrganizationID, input.PersonID, definition.ID)
	if err != nil {
		return nil, false, err
	}
	for _, evidence := range evidenceOf(existing) {
		if evidence.Type == "attendance" && evidence.SourceID == input.OccurrenceID {
			return existing, true, nil
		}
	}
	now := s.now()
	if occurredAt.IsZero() {
		occurredAt = now
	}
	evidence := AssimilationEvidence{Type: "attendance", SourceID: input.OccurrenceID, OccurredAt: occurredAt}
	if existing == nil {
		return s.startAssimilation(ctx, principal, input, *definition, evidence, requestID)
	}
	return s.advanceAssimilationVisit(ctx, principal, existing, *definition, evidence, requestID)
}

func (s Service) startAssimilation(ctx context.Context, principal platform.Principal, input AssimilationInput, definition Definition, evidence AssimilationEvidence, requestID string) (*AssimilationProgress, bool, error) {
	now := s.now()
	instance := Instance{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: definition.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, DefinitionID: definition.ID, DefinitionVersion: definition.DefinitionVersion, SubjectType: "person", SubjectID: input.PersonID, StageKey: "first-visit", OwnerID: input.OwnerID, DueAt: stageDue(&definition, "first-visit", now), State: "active"}
	progress := AssimilationProgress{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: definition.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, PersonID: input.PersonID, DefinitionID: definition.ID, WorkflowInstanceID: instance.ID, Pathway: "visitor", VisitCount: 1, LastVisitAt: evidence.OccurredAt, Evidence: []AssimilationEvidence{evidence}}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertAssimilationProgress(tx, progress); err != nil {
			return err
		}
		if err := s.Store.InsertInstance(tx, instance); err != nil {
			return err
		}
		if err := s.createStageTasks(tx, principal, instance, definition); err != nil {
			return err
		}
		if err := s.Store.InsertEvent(tx, Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, InstanceID: instance.ID, Type: "started", ToStage: "first-visit", Actor: principal.Actor, Reason: "verified-first-attendance", OccurredAt: now}); err != nil {
			return err
		}
		return s.evidence(tx, principal, definition.BranchID, "care.assimilation.started", "assimilation-progress", progress.ID, 1, requestID)
	})
	if err != nil {
		latest, _ := s.Store.FindAssimilationProgress(ctx, principal.OrganizationID, input.PersonID, definition.ID)
		if latest != nil {
			return latest, true, nil
		}
		return nil, false, err
	}
	return &progress, false, nil
}

func (s Service) advanceAssimilationVisit(ctx context.Context, principal platform.Principal, progress *AssimilationProgress, definition Definition, evidence AssimilationEvidence, requestID string) (*AssimilationProgress, bool, error) {
	instance, err := s.Store.FindInstance(ctx, principal.OrganizationID, progress.WorkflowInstanceID)
	if err != nil || instance == nil {
		return nil, false, &platform.DomainError{Code: "conflict", Message: "Assimilation workflow instance is unavailable."}
	}
	now := s.now()
	advance := progress.VisitCount == 1 && instance.StageKey == "first-visit"
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.AppendAssimilationVisit(tx, principal.OrganizationID, progress.ID, progress.Version, evidence, now, principal.Actor); err != nil {
			return err
		}
		if !advance {
			return s.evidence(tx, principal, progress.BranchID, "care.assimilation.visit_recorded", "assimilation-progress", progress.ID, progress.Version+1, requestID)
		}
		due := stageDue(&definition, "second-visit", now)
		if err := s.Store.UpdateInstance(tx, principal.OrganizationID, instance.ID, instance.Version, "second-visit", instance.OwnerID, due, "active", "", "", nil, now, principal.Actor); err != nil {
			return err
		}
		advanced := *instance
		advanced.StageKey = "second-visit"
		advanced.Version++
		if err := s.createStageTasks(tx, principal, advanced, definition); err != nil {
			return err
		}
		if err := s.Store.InsertEvent(tx, Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, InstanceID: instance.ID, Type: "stage-changed", FromStage: "first-visit", ToStage: "second-visit", Actor: principal.Actor, Reason: "verified-second-attendance", OccurredAt: now}); err != nil {
			return err
		}
		return s.evidence(tx, principal, progress.BranchID, "care.assimilation.second_visit", "assimilation-progress", progress.ID, progress.Version+1, requestID)
	})
	if err != nil {
		latest, _ := s.Store.FindAssimilationProgress(ctx, principal.OrganizationID, progress.PersonID, definition.ID)
		for _, item := range evidenceOf(latest) {
			if item.Type == evidence.Type && item.SourceID == evidence.SourceID {
				return latest, true, nil
			}
		}
		return nil, false, err
	}
	value, err := s.Store.FindAssimilationProgress(ctx, principal.OrganizationID, progress.PersonID, definition.ID)
	return value, false, err
}

func (s Service) ListAssimilationProgress(ctx context.Context, principal platform.Principal, branchID platform.ID) ([]AssimilationProgress, error) {
	if !branchID.Valid() || !s.allowed(principal, "read", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read assimilation progress in this branch."}
	}
	return s.Store.ListAssimilationProgress(ctx, principal.OrganizationID, branchID, 200)
}
func evidenceOf(value *AssimilationProgress) []AssimilationEvidence {
	if value == nil {
		return nil
	}
	return value.Evidence
}
