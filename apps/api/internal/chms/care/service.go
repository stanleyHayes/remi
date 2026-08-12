package care

import (
	"context"
	"crypto/sha256"
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
	Store      Store
	Platform   UnitOfWork
	Authorizer platform.Authorizer
	Now        func() time.Time
	Attendance AttendanceEvidenceResolver
	Cases      CaseStore
	Cipher     NoteCipher
}

func (s Service) ListDefinitions(ctx context.Context, principal platform.Principal, branchID platform.ID) ([]Definition, error) {
	if !branchID.Valid() {
		return nil, platform.ValidationError(platform.FieldError{Path: "branchId", Code: "required", Message: "Choose a branch."})
	}
	if !s.allowed(principal, "read", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read workflows in this branch."}
	}
	return s.Store.ListDefinitions(ctx, principal.OrganizationID, branchID)
}

func (s Service) GetDefinition(ctx context.Context, principal platform.Principal, id platform.ID) (*Definition, error) {
	value, err := s.Store.FindDefinition(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.allowed(principal, "read", value.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Workflow definition not found."}
	}
	return value, nil
}

func (s Service) ListInstances(ctx context.Context, principal platform.Principal, branchID platform.ID, state string) ([]Instance, error) {
	if !branchID.Valid() || !s.allowed(principal, "read", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read workflows in this branch."}
	}
	state = strings.ToLower(strings.TrimSpace(state))
	if state != "" && state != "active" && state != "completed" && state != "cancelled" {
		return nil, platform.ValidationError(platform.FieldError{Path: "state", Code: "invalid", Message: "Choose active, completed or cancelled."})
	}
	return s.Store.ListInstances(ctx, principal.OrganizationID, branchID, state)
}

func (s Service) GetInstance(ctx context.Context, principal platform.Principal, id platform.ID) (*Instance, []Task, error) {
	value, err := s.Store.FindInstance(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, nil, err
	}
	if value == nil || !s.allowed(principal, "read", value.BranchID) {
		return nil, nil, &platform.DomainError{Code: "not_found", Message: "Workflow instance not found."}
	}
	tasks, err := s.Store.ListTasks(ctx, principal.OrganizationID, id)
	return value, tasks, err
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) allowed(p platform.Principal, action string, b platform.ID) bool {
	return s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "workflow", OrganizationID: p.OrganizationID, BranchID: b, FieldClasses: []platform.FieldClass{platform.FieldOperational}, Now: s.now()}).Allowed
}
func (s Service) CreateDefinition(c context.Context, p platform.Principal, in DefinitionInput, requestID string) (*Definition, error) {
	if e := in.NormalizeAndValidate(); e != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_workflow", Message: e.Error()})
	}
	if !s.allowed(p, "create", in.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create workflows in this branch."}
	}
	now := s.now()
	definitionVersion := 1
	if in.SupersedesDefinitionID.Valid() {
		previous, lookupErr := s.Store.FindDefinition(c, p.OrganizationID, in.SupersedesDefinitionID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if previous == nil || previous.Status != "published" || previous.BranchID != in.BranchID || previous.Purpose != in.Purpose {
			return nil, platform.ValidationError(platform.FieldError{Path: "supersedesDefinitionId", Code: "invalid_revision", Message: "A revision must supersede a published workflow with the same branch and purpose."})
		}
		definitionVersion = previous.DefinitionVersion + 1
	}
	v := Definition{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: in.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, Name: in.Name, Description: in.Description, Purpose: in.Purpose, DefinitionVersion: definitionVersion, SupersedesDefinitionID: in.SupersedesDefinitionID, Status: "draft", InitialStageKey: in.InitialStageKey, Stages: in.Stages, Transitions: in.Transitions, Tasks: in.Tasks}
	if e := s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.InsertDefinition(tx, v); e != nil {
			return e
		}
		return s.evidence(tx, p, v.BranchID, "care.workflow-definition.created", "workflow-definition", v.ID, v.Version, requestID)
	}); e != nil {
		return nil, e
	}
	return &v, nil
}
func (s Service) UpdateDefinition(c context.Context, p platform.Principal, id platform.ID, ver int64, in DefinitionInput, requestID string) (*Definition, error) {
	if e := platform.RequireExpectedVersion(ver); e != nil {
		return nil, e
	}
	if e := in.NormalizeAndValidate(); e != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_workflow", Message: e.Error()})
	}
	cur, e := s.Store.FindDefinition(c, p.OrganizationID, id)
	if e != nil {
		return nil, e
	}
	if cur == nil || cur.Status != "draft" || !s.allowed(p, "update", cur.BranchID) || !s.allowed(p, "update", in.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Editable workflow definition not found."}
	}
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.UpdateDefinition(tx, p.OrganizationID, id, ver, in, s.now(), p.Actor); e != nil {
			return e
		}
		return s.evidence(tx, p, in.BranchID, "care.workflow-definition.updated", "workflow-definition", id, ver+1, requestID)
	}); e != nil {
		return nil, e
	}
	return s.Store.FindDefinition(c, p.OrganizationID, id)
}
func (s Service) PublishDefinition(c context.Context, p platform.Principal, id platform.ID, ver int64, requestID string) (*Definition, error) {
	cur, e := s.Store.FindDefinition(c, p.OrganizationID, id)
	if e != nil {
		return nil, e
	}
	if cur == nil || cur.Status != "draft" || !s.allowed(p, "update", cur.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Draft workflow definition not found."}
	}
	now := s.now()
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.PublishDefinition(tx, p.OrganizationID, id, ver, now, p.Actor); e != nil {
			return e
		}
		return s.evidence(tx, p, cur.BranchID, "care.workflow-definition.published", "workflow-definition", id, ver+1, requestID)
	}); e != nil {
		return nil, e
	}
	return s.Store.FindDefinition(c, p.OrganizationID, id)
}
func (s Service) Start(c context.Context, p platform.Principal, in StartInput, requestID string) (*Instance, error) {
	d, e := s.Store.FindDefinition(c, p.OrganizationID, in.DefinitionID)
	if e != nil {
		return nil, e
	}
	in.SubjectType = strings.ToLower(strings.TrimSpace(in.SubjectType))
	if d == nil || d.Status != "published" || !s.allowed(p, "create", d.BranchID) || !in.SubjectID.Valid() || !in.OwnerID.Valid() || (in.SubjectType != "person" && in.SubjectType != "household") {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_instance", Message: "A published definition, person or household subject, and owner are required."})
	}
	now := s.now()
	due := stageDue(d, d.InitialStageKey, now)
	v := Instance{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: d.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, DefinitionID: d.ID, DefinitionVersion: d.DefinitionVersion, SubjectType: in.SubjectType, SubjectID: in.SubjectID, StageKey: d.InitialStageKey, OwnerID: in.OwnerID, DueAt: due, State: "active"}
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.InsertInstance(tx, v); e != nil {
			return e
		}
		if e := s.createStageTasks(tx, p, v, *d); e != nil {
			return e
		}
		if e := s.Store.InsertEvent(tx, Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, InstanceID: v.ID, Type: "started", ToStage: v.StageKey, Actor: p.Actor, OccurredAt: now}); e != nil {
			return e
		}
		return s.evidence(tx, p, v.BranchID, "care.workflow.started", "workflow-instance", v.ID, v.Version, requestID)
	}); e != nil {
		return nil, e
	}
	return &v, nil
}
func (s Service) Transition(c context.Context, p platform.Principal, id platform.ID, in TransitionInput, requestID string) (*Instance, bool, error) {
	cur, e := s.Store.FindInstance(c, p.OrganizationID, id)
	if e != nil || cur == nil {
		return nil, false, &platform.DomainError{Code: "not_found", Message: "Workflow instance not found."}
	}
	if !s.allowed(p, "update", cur.BranchID) {
		return nil, false, &platform.DomainError{Code: "not_found", Message: "Workflow instance not found."}
	}
	d, e := s.Store.FindDefinition(c, p.OrganizationID, cur.DefinitionID)
	if e != nil || d == nil {
		return nil, false, &platform.DomainError{Code: "conflict", Message: "Pinned workflow definition is unavailable."}
	}
	to := strings.ToLower(strings.TrimSpace(in.ToStageKey))
	automationPayloadHash := fmt.Sprintf("%x", sha256.Sum256([]byte(string(id)+"|"+to+"|"+strings.TrimSpace(in.OutcomeCode)+"|"+strings.TrimSpace(in.OutcomeSummary))))
	if in.AutomationKey != "" {
		hash, instanceID, exists, lookupErr := s.Store.FindAutomation(c, p.OrganizationID, strings.TrimSpace(in.AutomationKey))
		if lookupErr != nil {
			return nil, false, lookupErr
		}
		if exists {
			if hash != automationPayloadHash || instanceID != id {
				return nil, false, &platform.DomainError{Code: "idempotency_conflict", Message: "That automation key was already used for different workflow content."}
			}
			return cur, true, nil
		}
	}
	if !allows(d.Transitions, cur.StageKey, to) {
		return nil, false, platform.ValidationError(platform.FieldError{Path: "toStageKey", Code: "invalid_transition", Message: "That stage transition is not allowed."})
	}
	terminal, requiresOutcome := stageFlags(d.Stages, to)
	in.OutcomeCode, in.OutcomeSummary = strings.TrimSpace(in.OutcomeCode), strings.TrimSpace(in.OutcomeSummary)
	if requiresOutcome && in.OutcomeCode == "" {
		return nil, false, platform.ValidationError(platform.FieldError{Path: "outcomeCode", Code: "required", Message: "Choose an outcome before completing this stage."})
	}
	now := s.now()
	state := "active"
	var completed *time.Time
	if terminal {
		state = "completed"
		completed = &now
	}
	due := stageDue(d, to, now)
	replayed := false
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if in.AutomationKey != "" {
			claimed, claimErr := s.Store.ClaimAutomation(tx, p.OrganizationID, strings.TrimSpace(in.AutomationKey), automationPayloadHash, id, now)
			if claimErr != nil {
				return claimErr
			}
			if !claimed {
				replayed = true
				return nil
			}
		}
		if e := s.Store.UpdateInstance(tx, p.OrganizationID, id, in.ExpectedVersion, to, cur.OwnerID, due, state, in.OutcomeCode, in.OutcomeSummary, completed, now, p.Actor); e != nil {
			return e
		}
		if e := s.createStageTasks(tx, p, Instance{ResourceEnvelope: platform.ResourceEnvelope{ID: id, OrganizationID: p.OrganizationID, BranchID: cur.BranchID, Version: in.ExpectedVersion + 1, CreatedAt: cur.CreatedAt}, StageKey: to, OwnerID: cur.OwnerID}, *d); e != nil {
			return e
		}
		if e := s.Store.InsertEvent(tx, Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, InstanceID: id, Type: "stage-changed", FromStage: cur.StageKey, ToStage: to, Actor: p.Actor, Reason: in.OutcomeCode, OccurredAt: now}); e != nil {
			return e
		}
		return s.evidence(tx, p, cur.BranchID, "care.workflow.stage_changed", "workflow-instance", id, in.ExpectedVersion+1, requestID)
	}); e != nil {
		if in.AutomationKey != "" {
			// A concurrent winner can commit just after Mongo reports our
			// duplicate-key loser. Briefly wait for that idempotency claim to
			// become visible before surfacing a false failure to the caller.
			for attempt := 0; attempt < 20; attempt++ {
				hash, instanceID, exists, lookupErr := s.Store.FindAutomation(c, p.OrganizationID, strings.TrimSpace(in.AutomationKey))
				if lookupErr == nil && exists && hash == automationPayloadHash && instanceID == id {
					latest, findErr := s.Store.FindInstance(c, p.OrganizationID, id)
					if findErr == nil && latest != nil && latest.StageKey == to {
						return latest, true, nil
					}
				}
				if lookupErr != nil || (exists && (hash != automationPayloadHash || instanceID != id)) {
					break
				}
				select {
				case <-c.Done():
					return nil, false, c.Err()
				case <-time.After(25 * time.Millisecond):
				}
			}
		}
		return nil, false, e
	}
	if replayed {
		return cur, true, nil
	}
	v, e := s.Store.FindInstance(c, p.OrganizationID, id)
	return v, false, e
}
func (s Service) Reassign(c context.Context, p platform.Principal, id platform.ID, ver int64, owner platform.ID, reason, requestID string) (*Instance, error) {
	cur, e := s.Store.FindInstance(c, p.OrganizationID, id)
	if e != nil || cur == nil || !s.allowed(p, "update", cur.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Workflow instance not found."}
	}
	if !owner.Valid() || platform.ValidateReason(reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid", Message: "Choose an owner and provide a reason."})
	}
	now := s.now()
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.UpdateInstance(tx, p.OrganizationID, id, ver, cur.StageKey, owner, cur.DueAt, cur.State, cur.OutcomeCode, cur.OutcomeSummary, cur.CompletedAt, now, p.Actor); e != nil {
			return e
		}
		if e := s.Store.ReassignOpenTasks(tx, p.OrganizationID, id, owner, now, p.Actor); e != nil {
			return e
		}
		if e := s.Store.InsertEvent(tx, Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, InstanceID: id, Type: "reassigned", Actor: p.Actor, Reason: reason, OccurredAt: now}); e != nil {
			return e
		}
		return s.evidence(tx, p, cur.BranchID, "care.workflow.reassigned", "workflow-instance", id, ver+1, requestID)
	}); e != nil {
		return nil, e
	}
	return s.Store.FindInstance(c, p.OrganizationID, id)
}
func (s Service) CompleteTask(c context.Context, p platform.Principal, id platform.ID, ver int64, outcome, requestID string) (*Task, error) {
	v, e := s.Store.FindTask(c, p.OrganizationID, id)
	if e != nil || v == nil || !s.allowed(p, "update", v.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Task not found."}
	}
	if strings.TrimSpace(outcome) == "" {
		return nil, platform.ValidationError(platform.FieldError{Path: "outcome", Code: "required", Message: "Record the task outcome."})
	}
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.CompleteTask(tx, p.OrganizationID, id, ver, strings.TrimSpace(outcome), s.now(), p.Actor); e != nil {
			return e
		}
		return s.evidence(tx, p, v.BranchID, "care.task.completed", "task", id, ver+1, requestID)
	}); e != nil {
		return nil, e
	}
	return s.Store.FindTask(c, p.OrganizationID, id)
}
func (s Service) RemindTask(c context.Context, p platform.Principal, id platform.ID, requestID string) (*Task, error) {
	v, e := s.Store.FindTask(c, p.OrganizationID, id)
	if e != nil || v == nil || !s.allowed(p, "update", v.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Task not found."}
	}
	now := s.now()
	if v.LastReminderAt != nil && now.Sub(*v.LastReminderAt) < 24*time.Hour {
		return nil, &platform.DomainError{Code: "conflict", Message: "A reminder was already recorded in the last 24 hours."}
	}
	if e = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if e := s.Store.RemindTask(tx, p.OrganizationID, id, now, p.Actor); e != nil {
			return e
		}
		return s.evidence(tx, p, v.BranchID, "care.task.reminded", "task", id, v.Version+1, requestID)
	}); e != nil {
		return nil, e
	}
	return s.Store.FindTask(c, p.OrganizationID, id)
}
func (s Service) createStageTasks(c context.Context, p platform.Principal, i Instance, d Definition) error {
	now := s.now()
	for _, t := range d.Tasks {
		if t.StageKey != i.StageKey {
			continue
		}
		var due *time.Time
		if t.DueMinutes > 0 {
			v := now.Add(time.Duration(t.DueMinutes) * time.Minute)
			due = &v
		}
		v := Task{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: i.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, InstanceID: i.ID, TemplateKey: t.Key, StageKey: t.StageKey, Title: t.Title, AssigneeID: i.OwnerID, DueAt: due, Status: "open"}
		if e := s.Store.InsertTask(c, v); e != nil {
			return e
		}
	}
	return nil
}
func (s Service) evidence(c context.Context, p platform.Principal, b platform.ID, event, resource string, id platform.ID, version int64, requestID string) error {
	now := s.now()
	if e := s.Platform.AppendAudit(c, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: b, Actor: p.Actor, Action: event, ResourceType: resource, ResourceID: id, ChangedFields: []string{"state", "stage", "owner", "dueAt"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); e != nil {
		return e
	}
	return s.Platform.EnqueueEvent(c, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: b, Type: event, EventVersion: 1, AggregateType: resource, AggregateID: id, AggregateVersion: version, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"id": id}}, State: "pending", AvailableAt: now})
}
func stageDue(d *Definition, key string, now time.Time) *time.Time {
	for _, s := range d.Stages {
		if s.Key == key && s.SLAMinutes > 0 {
			v := now.Add(time.Duration(s.SLAMinutes) * time.Minute)
			return &v
		}
	}
	return nil
}
func stageFlags(stages []Stage, key string) (bool, bool) {
	for _, s := range stages {
		if s.Key == key {
			return s.Terminal, s.OutcomeRequired
		}
	}
	return false, false
}
func allows(ts []Transition, from, to string) bool {
	for _, t := range ts {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}
