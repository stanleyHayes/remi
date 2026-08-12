package care

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type ConsentEvidence struct {
	State         string    `json:"state" bson:"state"`
	Source        string    `json:"source" bson:"source"`
	NoticeVersion string    `json:"noticeVersion" bson:"noticeVersion"`
	CapturedAt    time.Time `json:"capturedAt" bson:"capturedAt"`
}
type CareCase struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonID                  platform.ID     `json:"personId,omitempty" bson:"personId,omitempty"`
	HouseholdID               platform.ID     `json:"householdId,omitempty" bson:"householdId,omitempty"`
	MinistryID                platform.ID     `json:"ministryId" bson:"ministryId"`
	Category                  string          `json:"category" bson:"category"`
	Urgency                   string          `json:"urgency" bson:"urgency"`
	Consent                   ConsentEvidence `json:"consent" bson:"consent"`
	AssignedTeamID            platform.ID     `json:"assignedTeamId,omitempty" bson:"assignedTeamId,omitempty"`
	AssignedUserIDs           []platform.ID   `json:"assignedUserIds" bson:"assignedUserIds"`
	WorkflowInstanceID        platform.ID     `json:"workflowInstanceId" bson:"workflowInstanceId"`
	State                     string          `json:"state" bson:"state"`
	DueAt                     *time.Time      `json:"dueAt,omitempty" bson:"dueAt,omitempty"`
	ClosedAt                  *time.Time      `json:"closedAt,omitempty" bson:"closedAt,omitempty"`
	ClosureOutcome            string          `json:"closureOutcome,omitempty" bson:"closureOutcome,omitempty"`
	RetentionReviewAt         time.Time       `json:"retentionReviewAt" bson:"retentionReviewAt"`
}
type CareCaseInput struct {
	BranchID             platform.ID     `json:"branchId"`
	PersonID             platform.ID     `json:"personId"`
	HouseholdID          platform.ID     `json:"householdId"`
	MinistryID           platform.ID     `json:"ministryId"`
	Category             string          `json:"category"`
	Urgency              string          `json:"urgency"`
	Consent              ConsentEvidence `json:"consent"`
	AssignedTeamID       platform.ID     `json:"assignedTeamId"`
	AssignedUserIDs      []platform.ID   `json:"assignedUserIds"`
	WorkflowDefinitionID platform.ID     `json:"workflowDefinitionId"`
	DueAt                *time.Time      `json:"dueAt"`
}

func (in *CareCaseInput) NormalizeAndValidate(now time.Time) error {
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	in.MinistryID = platform.ID(strings.TrimSpace(string(in.MinistryID)))
	in.Urgency = strings.ToLower(strings.TrimSpace(in.Urgency))
	in.Consent.State = strings.ToLower(strings.TrimSpace(in.Consent.State))
	in.Consent.Source = strings.ToLower(strings.TrimSpace(in.Consent.Source))
	in.Consent.NoticeVersion = strings.TrimSpace(in.Consent.NoticeVersion)
	if !in.BranchID.Valid() || !in.MinistryID.Valid() || (!in.PersonID.Valid() && !in.HouseholdID.Valid()) || (in.PersonID.Valid() && in.HouseholdID.Valid()) {
		return errors.New("branch, ministry and exactly one person or household subject are required")
	}
	allowedCategory := map[string]bool{"pastoral-care": true, "bereavement": true, "health-support": true, "family-support": true, "practical-support": true, "spiritual-support": true, "other": true}
	if !allowedCategory[in.Category] {
		return errors.New("choose a supported care category")
	}
	if in.Urgency == "" {
		in.Urgency = "routine"
	}
	if in.Urgency != "routine" && in.Urgency != "priority" && in.Urgency != "urgent" {
		return errors.New("urgency must be routine, priority or urgent")
	}
	if in.Consent.State != "granted" || in.Consent.Source == "" || in.Consent.NoticeVersion == "" {
		return errors.New("explicit granted consent evidence is required")
	}
	if in.Consent.CapturedAt.IsZero() {
		in.Consent.CapturedAt = now
	}
	in.Consent.CapturedAt = in.Consent.CapturedAt.UTC()
	if in.Consent.CapturedAt.After(now.Add(5 * time.Minute)) {
		return errors.New("consent time cannot be in the future")
	}
	if len(in.AssignedUserIDs) == 0 || !in.WorkflowDefinitionID.Valid() {
		return errors.New("a published care workflow and at least one assigned care user are required")
	}
	seen := map[platform.ID]bool{}
	for _, id := range in.AssignedUserIDs {
		if !id.Valid() || seen[id] {
			return errors.New("assigned care users must be unique")
		}
		seen[id] = true
	}
	if in.DueAt != nil {
		v := in.DueAt.UTC()
		in.DueAt = &v
		if v.Before(now) {
			return errors.New("due time cannot be in the past")
		}
	}
	return nil
}

type CareAssignmentInput struct {
	ExpectedVersion int64         `json:"expectedVersion"`
	AssignedTeamID  platform.ID   `json:"assignedTeamId"`
	AssignedUserIDs []platform.ID `json:"assignedUserIds"`
	Reason          string        `json:"reason"`
}
type CareCloseInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Outcome         string `json:"outcome"`
	Reason          string `json:"reason"`
}
type RestrictedNote struct {
	ID                platform.ID             `json:"id" bson:"_id"`
	OrganizationID    platform.ID             `json:"-" bson:"organizationId"`
	BranchID          platform.ID             `json:"-" bson:"branchId"`
	CaseID            platform.ID             `json:"caseId" bson:"caseId"`
	Classification    string                  `json:"classification" bson:"classification"`
	AuthorizedGroup   string                  `json:"authorizedGroup" bson:"authorizedGroup"`
	EncryptedContent  platform.EncryptedValue `json:"-" bson:"encryptedContent"`
	Author            platform.Actor          `json:"author" bson:"author"`
	RetentionReviewAt time.Time               `json:"retentionReviewAt" bson:"retentionReviewAt"`
	CreatedAt         time.Time               `json:"createdAt" bson:"createdAt"`
}
type RestrictedNoteView struct {
	ID                platform.ID    `json:"id"`
	CaseID            platform.ID    `json:"caseId"`
	Classification    string         `json:"classification"`
	AuthorizedGroup   string         `json:"authorizedGroup"`
	Content           string         `json:"content"`
	Author            platform.Actor `json:"author"`
	RetentionReviewAt time.Time      `json:"retentionReviewAt"`
	CreatedAt         time.Time      `json:"createdAt"`
}
type ContactEvent struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID    `json:"branchId" bson:"branchId"`
	CaseID         platform.ID    `json:"caseId" bson:"caseId"`
	Channel        string         `json:"channel" bson:"channel"`
	Purpose        string         `json:"purpose" bson:"purpose"`
	Outcome        string         `json:"outcome" bson:"outcome"`
	Summary        string         `json:"summary,omitempty" bson:"summary,omitempty"`
	Actor          platform.Actor `json:"actor" bson:"actor"`
	OccurredAt     time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type CaseStore interface {
	InsertCase(context.Context, CareCase) error
	FindCase(context.Context, platform.ID, platform.ID) (*CareCase, error)
	ListCases(context.Context, platform.ID, platform.ID, []platform.ID, bool) ([]CareCase, error)
	UpdateAssignment(context.Context, platform.ID, platform.ID, int64, platform.ID, []platform.ID, time.Time, platform.Actor) error
	CloseCase(context.Context, platform.ID, platform.ID, int64, string, time.Time, platform.Actor) error
	InsertRestrictedNote(context.Context, RestrictedNote) error
	ListRestrictedNotes(context.Context, platform.ID, platform.ID) ([]RestrictedNote, error)
	InsertContactEvent(context.Context, ContactEvent) error
	ListContactEvents(context.Context, platform.ID, platform.ID) ([]ContactEvent, error)
}
type NoteCipher interface {
	Encrypt([]byte, []byte) (platform.EncryptedValue, error)
	Decrypt(platform.EncryptedValue, []byte) ([]byte, error)
}

func (s Service) caseAllowed(p platform.Principal, action string, v *CareCase) bool {
	if v == nil || !s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "care-case", OrganizationID: p.OrganizationID, BranchID: v.BranchID, MinistryID: v.MinistryID, ResourceID: v.ID, FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}, Now: s.now()}).Allowed {
		return false
	}
	for _, id := range v.AssignedUserIDs {
		if id == p.Actor.ID {
			return true
		}
	}
	for _, id := range p.AssignedResourceIDs {
		if id == v.ID {
			return true
		}
	}
	return false
}
func (s Service) CreateCareCase(c context.Context, p platform.Principal, in CareCaseInput, requestID string) (*CareCase, error) {
	now := s.now()
	if err := in.NormalizeAndValidate(now); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_case", Message: err.Error()})
	}
	if s.Cases == nil || !s.Authorizer.Authorize(p, platform.AccessRequest{Action: "create", ResourceType: "care-case", OrganizationID: p.OrganizationID, BranchID: in.BranchID, MinistryID: in.MinistryID, FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}, Now: now}).Allowed {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create restricted care cases."}
	}
	definition, err := s.Store.FindDefinition(c, p.OrganizationID, in.WorkflowDefinitionID)
	if err != nil {
		return nil, err
	}
	if definition == nil || definition.Status != "published" || definition.BranchID != in.BranchID || definition.Purpose != "pastoral-care" {
		return nil, platform.ValidationError(platform.FieldError{Path: "workflowDefinitionId", Code: "invalid", Message: "Choose a published pastoral-care workflow for this branch."})
	}
	subjectType, subjectID := "person", in.PersonID
	if in.HouseholdID.Valid() {
		subjectType, subjectID = "household", in.HouseholdID
	}
	instanceDue := stageDue(definition, definition.InitialStageKey, now)
	if in.DueAt != nil {
		instanceDue = in.DueAt
	}
	instance := Instance{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: in.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, DefinitionID: definition.ID, DefinitionVersion: definition.DefinitionVersion, SubjectType: subjectType, SubjectID: subjectID, StageKey: definition.InitialStageKey, OwnerID: in.AssignedUserIDs[0], DueAt: instanceDue, State: "active"}
	v := CareCase{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: in.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, PersonID: in.PersonID, HouseholdID: in.HouseholdID, MinistryID: in.MinistryID, Category: in.Category, Urgency: in.Urgency, Consent: in.Consent, AssignedTeamID: in.AssignedTeamID, AssignedUserIDs: in.AssignedUserIDs, WorkflowInstanceID: instance.ID, State: "open", DueAt: in.DueAt, RetentionReviewAt: now.Add(365 * 24 * time.Hour)}
	err = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if err := s.Cases.InsertCase(tx, v); err != nil {
			return err
		}
		if err := s.Store.InsertInstance(tx, instance); err != nil {
			return err
		}
		if err := s.createStageTasks(tx, p, instance, *definition); err != nil {
			return err
		}
		if err := s.Store.InsertEvent(tx, Event{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, InstanceID: instance.ID, Type: "started", ToStage: instance.StageKey, Actor: p.Actor, Reason: "care-case-created", OccurredAt: now}); err != nil {
			return err
		}
		return s.evidence(tx, p, v.BranchID, "care.case.created", "care-case", v.ID, 1, requestID)
	})
	return &v, err
}
func (s Service) GetCareCase(c context.Context, p platform.Principal, id platform.ID) (*CareCase, []ContactEvent, error) {
	v, err := s.Cases.FindCase(c, p.OrganizationID, id)
	if err != nil {
		return nil, nil, err
	}
	if !s.caseAllowed(p, "read", v) {
		return nil, nil, &platform.DomainError{Code: "not_found", Message: "Care case not found."}
	}
	events, err := s.Cases.ListContactEvents(c, p.OrganizationID, id)
	return v, events, err
}
func (s Service) ListCareCases(c context.Context, p platform.Principal, branch platform.ID, includeClosed bool) ([]CareCase, error) {
	if !branch.Valid() {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read care cases in this branch."}
	}
	values, err := s.Cases.ListCases(c, p.OrganizationID, branch, []platform.ID{p.Actor.ID}, includeClosed)
	if err != nil {
		return nil, err
	}
	visible := make([]CareCase, 0, len(values))
	for index := range values {
		if s.caseAllowed(p, "read", &values[index]) {
			visible = append(visible, values[index])
		}
	}
	return visible, nil
}
func (s Service) AssignCareCase(c context.Context, p platform.Principal, id platform.ID, in CareAssignmentInput, requestID string) (*CareCase, error) {
	v, err := s.Cases.FindCase(c, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !s.caseAllowed(p, "update", v) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Care case not found."}
	}
	if platform.RequireExpectedVersion(in.ExpectedVersion) != nil || len(in.AssignedUserIDs) == 0 || platform.ValidateReason(in.Reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_assignment", Message: "Expected version, assignees and a reason are required."})
	}
	now := s.now()
	instance, err := s.Store.FindInstance(c, p.OrganizationID, v.WorkflowInstanceID)
	if err != nil || instance == nil || instance.State != "active" {
		return nil, &platform.DomainError{Code: "conflict", Message: "The care workflow is unavailable for reassignment."}
	}
	err = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if err := s.Cases.UpdateAssignment(tx, p.OrganizationID, id, in.ExpectedVersion, in.AssignedTeamID, in.AssignedUserIDs, now, p.Actor); err != nil {
			return err
		}
		if err := s.Store.UpdateInstance(tx, p.OrganizationID, instance.ID, instance.Version, instance.StageKey, in.AssignedUserIDs[0], instance.DueAt, instance.State, instance.OutcomeCode, instance.OutcomeSummary, instance.CompletedAt, now, p.Actor); err != nil {
			return err
		}
		if err := s.Store.ReassignOpenTasks(tx, p.OrganizationID, instance.ID, in.AssignedUserIDs[0], now, p.Actor); err != nil {
			return err
		}
		return s.evidence(tx, p, v.BranchID, "care.case.assigned", "care-case", id, in.ExpectedVersion+1, requestID)
	})
	if err != nil {
		return nil, err
	}
	return s.Cases.FindCase(c, p.OrganizationID, id)
}
func (s Service) CloseCareCase(c context.Context, p platform.Principal, id platform.ID, in CareCloseInput, requestID string) (*CareCase, error) {
	v, err := s.Cases.FindCase(c, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !s.caseAllowed(p, "update", v) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Care case not found."}
	}
	in.Outcome, in.Reason = strings.ToLower(strings.TrimSpace(in.Outcome)), strings.TrimSpace(in.Reason)
	if platform.RequireExpectedVersion(in.ExpectedVersion) != nil || in.Outcome == "" || platform.ValidateReason(in.Reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_closure", Message: "Expected version, outcome and reason are required."})
	}
	now := s.now()
	instance, err := s.Store.FindInstance(c, p.OrganizationID, v.WorkflowInstanceID)
	if err != nil || instance == nil || instance.State != "active" {
		return nil, &platform.DomainError{Code: "conflict", Message: "The care workflow is unavailable for closure."}
	}
	err = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if err := s.Cases.CloseCase(tx, p.OrganizationID, id, in.ExpectedVersion, in.Outcome, now, p.Actor); err != nil {
			return err
		}
		if err := s.Store.UpdateInstance(tx, p.OrganizationID, instance.ID, instance.Version, instance.StageKey, instance.OwnerID, instance.DueAt, "completed", in.Outcome, in.Reason, &now, now, p.Actor); err != nil {
			return err
		}
		if err := s.Store.CloseOpenTasks(tx, p.OrganizationID, instance.ID, "care-case-closed", now, p.Actor); err != nil {
			return err
		}
		return s.evidence(tx, p, v.BranchID, "care.case.closed", "care-case", id, in.ExpectedVersion+1, requestID)
	})
	if err != nil {
		return nil, err
	}
	return s.Cases.FindCase(c, p.OrganizationID, id)
}
func (s Service) AddRestrictedNote(c context.Context, p platform.Principal, caseID platform.ID, content, classification, group, requestID string) (*RestrictedNoteView, error) {
	v, err := s.Cases.FindCase(c, p.OrganizationID, caseID)
	if err != nil {
		return nil, err
	}
	if !s.caseAllowed(p, "update", v) || s.Cipher == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Care case not found."}
	}
	content, classification, group = strings.TrimSpace(content), strings.ToLower(strings.TrimSpace(classification)), strings.TrimSpace(group)
	if len(content) < 1 || len(content) > 10000 || classification == "" || group == "" {
		return nil, platform.ValidationError(platform.FieldError{Path: "content", Code: "invalid", Message: "Note content, classification and authorized group are required."})
	}
	now := s.now()
	id := platform.ID(bson.NewObjectID().Hex())
	encrypted, err := s.Cipher.Encrypt([]byte(content), []byte(string(p.OrganizationID)+":"+string(caseID)+":"+string(id)))
	if err != nil {
		return nil, err
	}
	note := RestrictedNote{ID: id, OrganizationID: p.OrganizationID, BranchID: v.BranchID, CaseID: caseID, Classification: classification, AuthorizedGroup: group, EncryptedContent: encrypted, Author: p.Actor, RetentionReviewAt: v.RetentionReviewAt, CreatedAt: now}
	err = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if err := s.Cases.InsertRestrictedNote(tx, note); err != nil {
			return err
		}
		return s.evidence(tx, p, v.BranchID, "care.restricted-note.created", "restricted-note", id, 1, requestID)
	})
	if err != nil {
		return nil, err
	}
	return noteView(note, content), nil
}
func (s Service) ListRestrictedNotes(c context.Context, p platform.Principal, caseID platform.ID, requestID string) ([]RestrictedNoteView, error) {
	v, err := s.Cases.FindCase(c, p.OrganizationID, caseID)
	if err != nil {
		return nil, err
	}
	if !s.caseAllowed(p, "read", v) || s.Cipher == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Care case not found."}
	}
	notes, err := s.Cases.ListRestrictedNotes(c, p.OrganizationID, caseID)
	if err != nil {
		return nil, err
	}
	views := make([]RestrictedNoteView, 0, len(notes))
	for _, note := range notes {
		raw, decryptErr := s.Cipher.Decrypt(note.EncryptedContent, []byte(string(p.OrganizationID)+":"+string(caseID)+":"+string(note.ID)))
		if decryptErr != nil {
			return nil, decryptErr
		}
		views = append(views, *noteView(note, string(raw)))
	}
	audit := platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, Actor: p.Actor, Action: "care.restricted-note.read", ResourceType: "care-case", ResourceID: caseID, SubjectIDs: []platform.ID{v.PersonID, v.HouseholdID}, ChangedFields: []string{"restrictedNotes"}, Outcome: "success", RequestID: requestID, OccurredAt: s.now()}
	if err = s.Platform.AppendAudit(c, audit); err != nil {
		return nil, err
	}
	return views, nil
}
func noteView(v RestrictedNote, content string) *RestrictedNoteView {
	return &RestrictedNoteView{ID: v.ID, CaseID: v.CaseID, Classification: v.Classification, AuthorizedGroup: v.AuthorizedGroup, Content: content, Author: v.Author, RetentionReviewAt: v.RetentionReviewAt, CreatedAt: v.CreatedAt}
}
func (s Service) AddContactEvent(c context.Context, p platform.Principal, caseID platform.ID, channel, purpose, outcome, summary, requestID string) (*ContactEvent, error) {
	v, err := s.Cases.FindCase(c, p.OrganizationID, caseID)
	if err != nil {
		return nil, err
	}
	if !s.caseAllowed(p, "update", v) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Care case not found."}
	}
	channel, purpose, outcome, summary = strings.ToLower(strings.TrimSpace(channel)), strings.TrimSpace(purpose), strings.ToLower(strings.TrimSpace(outcome)), strings.TrimSpace(summary)
	if !map[string]bool{"phone": true, "email": true, "sms": true, "whatsapp": true, "in-person": true}[channel] || purpose == "" || outcome == "" || len(summary) > 500 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_contact", Message: "Choose a channel, purpose and outcome; keep summary under 500 characters."})
	}
	event := ContactEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: v.BranchID, CaseID: caseID, Channel: channel, Purpose: purpose, Outcome: outcome, Summary: summary, Actor: p.Actor, OccurredAt: s.now()}
	err = s.Platform.WithTransaction(c, func(tx context.Context) error {
		if err := s.Cases.InsertContactEvent(tx, event); err != nil {
			return err
		}
		return s.evidence(tx, p, v.BranchID, "care.contact.recorded", "care-case", caseID, v.Version, requestID)
	})
	return &event, err
}
