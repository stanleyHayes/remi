package community

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

var activeAssignmentStatuses = map[string]bool{"invited": true, "accepted": true, "substitute-requested": true, "no-show": true, "completed": true}

type PlanNeed struct {
	PositionID platform.ID `json:"positionId" bson:"positionId"`
	Slots      int         `json:"slots" bson:"slots"`
}

type ServicePlan struct {
	platform.ResourceEnvelope `bson:",inline"`
	OccurrenceID              platform.ID `json:"occurrenceId" bson:"occurrenceId"`
	Name                      string      `json:"name" bson:"name"`
	StartsAt                  time.Time   `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time   `json:"endsAt" bson:"endsAt"`
	Needs                     []PlanNeed  `json:"needs" bson:"needs"`
	Status                    string      `json:"status" bson:"status"`
	PublishedAt               *time.Time  `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
	LockedAt                  *time.Time  `json:"lockedAt,omitempty" bson:"lockedAt,omitempty"`
	CompletedAt               *time.Time  `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	CancelledAt               *time.Time  `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
	LastReason                string      `json:"lastReason,omitempty" bson:"lastReason,omitempty"`
}

type ServicePlanInput struct {
	OccurrenceID platform.ID `json:"occurrenceId"`
	Name         string      `json:"name"`
	Needs        []PlanNeed  `json:"needs"`
}

type ServicePlanTransition struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type VolunteerRotation struct {
	platform.ResourceEnvelope `bson:",inline"`
	TeamID                    platform.ID   `json:"teamId" bson:"teamId"`
	PositionID                platform.ID   `json:"positionId" bson:"positionId"`
	Name                      string        `json:"name" bson:"name"`
	PersonIDs                 []platform.ID `json:"personIds" bson:"personIds"`
	CadenceWeeks              int           `json:"cadenceWeeks" bson:"cadenceWeeks"`
	AnchorDate                string        `json:"anchorDate" bson:"anchorDate"`
	Status                    string        `json:"status" bson:"status"`
}

type VolunteerRotationInput struct {
	TeamID       platform.ID   `json:"teamId"`
	PositionID   platform.ID   `json:"positionId"`
	Name         string        `json:"name"`
	PersonIDs    []platform.ID `json:"personIds"`
	CadenceWeeks int           `json:"cadenceWeeks"`
	AnchorDate   string        `json:"anchorDate"`
	Status       string        `json:"status"`
}

type ReminderState struct {
	SentCount  int        `json:"sentCount" bson:"sentCount"`
	LastSentAt *time.Time `json:"lastSentAt,omitempty" bson:"lastSentAt,omitempty"`
	NextDueAt  *time.Time `json:"nextDueAt,omitempty" bson:"nextDueAt,omitempty"`
}

type VolunteerAssignment struct {
	platform.ResourceEnvelope `bson:",inline"`
	PlanID                    platform.ID   `json:"planId" bson:"planId"`
	OccurrenceID              platform.ID   `json:"occurrenceId" bson:"occurrenceId"`
	TeamID                    platform.ID   `json:"teamId" bson:"teamId"`
	PositionID                platform.ID   `json:"positionId" bson:"positionId"`
	PersonID                  platform.ID   `json:"personId" bson:"personId"`
	Slot                      int           `json:"slot" bson:"slot"`
	StartsAt                  time.Time     `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time     `json:"endsAt" bson:"endsAt"`
	Status                    string        `json:"status" bson:"status"`
	ResponseReasonCode        string        `json:"responseReasonCode,omitempty" bson:"responseReasonCode,omitempty"`
	RespondedAt               *time.Time    `json:"respondedAt,omitempty" bson:"respondedAt,omitempty"`
	SubstitutesAssignmentID   platform.ID   `json:"substitutesAssignmentId,omitempty" bson:"substitutesAssignmentId,omitempty"`
	ReplacedByAssignmentID    platform.ID   `json:"replacedByAssignmentId,omitempty" bson:"replacedByAssignmentId,omitempty"`
	Reminder                  ReminderState `json:"reminder" bson:"reminder"`
	NoShowReason              string        `json:"noShowReason,omitempty" bson:"noShowReason,omitempty"`
	OverrideReason            string        `json:"overrideReason,omitempty" bson:"overrideReason,omitempty"`
}

type AssignmentInput struct {
	PositionID     platform.ID `json:"positionId"`
	PersonID       platform.ID `json:"personId"`
	Slot           int         `json:"slot"`
	OverrideReason string      `json:"overrideReason"`
}

type AssignmentResponseInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Response        string `json:"response"`
	ReasonCode      string `json:"reasonCode"`
}

type SubstituteInput struct {
	ExpectedVersion    int64       `json:"expectedVersion"`
	SubstitutePersonID platform.ID `json:"substitutePersonId"`
	Reason             string      `json:"reason"`
	OverrideReason     string      `json:"overrideReason"`
}

type ReminderInput struct {
	ExpectedVersion int64      `json:"expectedVersion"`
	NextDueAt       *time.Time `json:"nextDueAt"`
}

type NoShowInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

type AssignmentUpdate struct {
	Status                 string
	ResponseReasonCode     string
	RespondedAt            *time.Time
	ReplacedByAssignmentID platform.ID
	Reminder               ReminderState
	NoShowReason           string
}

type AssignmentEvent struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID    `json:"branchId" bson:"branchId"`
	AssignmentID   platform.ID    `json:"assignmentId" bson:"assignmentId"`
	PlanID         platform.ID    `json:"planId" bson:"planId"`
	FromStatus     string         `json:"fromStatus,omitempty" bson:"fromStatus,omitempty"`
	ToStatus       string         `json:"toStatus" bson:"toStatus"`
	ReasonCode     string         `json:"reasonCode,omitempty" bson:"reasonCode,omitempty"`
	Actor          platform.Actor `json:"actor" bson:"actor"`
	RequestID      string         `json:"requestId,omitempty" bson:"requestId,omitempty"`
	OccurredAt     time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type AssignmentConflict struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type AssignmentPreview struct {
	Eligible  bool                 `json:"eligible"`
	Conflicts []AssignmentConflict `json:"conflicts"`
}

func (input *ServicePlanInput) normalize() error {
	input.Name = strings.TrimSpace(input.Name)
	if !input.OccurrenceID.Valid() || len(input.Name) < 2 || len(input.Name) > 150 {
		return errors.New("occurrence and a 2 to 150 character plan name are required")
	}
	seen := map[platform.ID]bool{}
	for index := range input.Needs {
		need := &input.Needs[index]
		if !need.PositionID.Valid() || need.Slots < 1 || need.Slots > 100 || seen[need.PositionID] {
			return errors.New("plan needs require unique positions and 1 to 100 slots")
		}
		seen[need.PositionID] = true
	}
	if len(input.Needs) == 0 || len(input.Needs) > 50 {
		return errors.New("a plan needs 1 to 50 positions")
	}
	sort.Slice(input.Needs, func(i, j int) bool { return input.Needs[i].PositionID < input.Needs[j].PositionID })
	return nil
}

func (input *VolunteerRotationInput) normalize() error {
	input.Name = strings.TrimSpace(input.Name)
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.PersonIDs = normalizeIDs(input.PersonIDs)
	if !input.TeamID.Valid() || !input.PositionID.Valid() || len(input.Name) < 2 || len(input.Name) > 150 {
		return errors.New("team, position and rotation name are required")
	}
	if len(input.PersonIDs) < 2 || len(input.PersonIDs) > 100 {
		return errors.New("a rotation requires 2 to 100 people")
	}
	if input.CadenceWeeks < 1 || input.CadenceWeeks > 52 {
		return errors.New("rotation cadence must be 1 to 52 weeks")
	}
	if _, err := time.Parse("2006-01-02", input.AnchorDate); err != nil {
		return errors.New("rotation anchor date must use YYYY-MM-DD")
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Status != "active" && input.Status != "inactive" {
		return errors.New("rotation status must be active or inactive")
	}
	return nil
}

func (s Service) CreateServicePlan(ctx context.Context, principal platform.Principal, input ServicePlanInput, requestID string) (*ServicePlan, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_plan", Message: err.Error()})
	}
	if s.Occurrences == nil {
		return nil, errors.New("occurrence resolver is unavailable")
	}
	branchID, startsAt, endsAt, occurrenceStatus, err := s.Occurrences.ResolveOccurrenceReference(ctx, principal.OrganizationID, input.OccurrenceID)
	if err != nil {
		return nil, err
	}
	if !branchID.Valid() || occurrenceStatus == "cancelled" || !s.allowedVolunteer(principal, "create", branchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service occurrence not found."}
	}
	for _, need := range input.Needs {
		position, findErr := s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, need.PositionID)
		if findErr != nil {
			return nil, findErr
		}
		if position == nil || position.BranchID != branchID || position.Status != "active" {
			return nil, platform.ValidationError(platform.FieldError{Path: "needs", Code: "invalid_position", Message: "Every needed position must be active in the occurrence branch."})
		}
	}
	now := s.now()
	value := ServicePlan{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: branchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, OccurrenceID: input.OccurrenceID, Name: input.Name, StartsAt: startsAt, EndsAt: endsAt, Needs: input.Needs, Status: "draft"}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertServicePlan(tx, value); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, branchID, value.ID, value.Version, "community.service-plan.created", requestID)
	}); err != nil {
		return nil, fmt.Errorf("create service plan: %w", err)
	}
	return &value, nil
}

func (s Service) GetServicePlan(ctx context.Context, principal platform.Principal, id platform.ID) (*ServicePlan, error) {
	value, err := s.Store.FindServicePlan(ctx, principal.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.allowedVolunteer(principal, "read", value.BranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service plan not found."}
	}
	return value, nil
}

func (s Service) ListServicePlans(ctx context.Context, principal platform.Principal, branchID platform.ID, from, to time.Time) ([]ServicePlan, error) {
	if !branchID.Valid() || from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 2*366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a branch and a valid range no longer than two years."})
	}
	if !s.allowedVolunteer(principal, "read", branchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read service plans in this branch."}
	}
	return s.Store.ListServicePlans(ctx, principal.OrganizationID, branchID, from.UTC(), to.UTC())
}

func (s Service) TransitionServicePlan(ctx context.Context, principal platform.Principal, id platform.ID, expectedVersion int64, input ServicePlanTransition, requestID string) (*ServicePlan, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	input.Status, input.Reason = strings.ToLower(strings.TrimSpace(input.Status)), strings.TrimSpace(input.Reason)
	current, err := s.GetServicePlan(ctx, principal, id)
	if err != nil {
		return nil, err
	}
	if !s.allowedVolunteer(principal, "update", current.BranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service plan not found."}
	}
	allowed := map[string]map[string]bool{"draft": {"published": true, "cancelled": true}, "published": {"locked": true, "cancelled": true}, "locked": {"completed": true, "cancelled": true}}
	if !allowed[current.Status][input.Status] || ((input.Status == "cancelled" || input.Status == "completed") && len(input.Reason) < 3) {
		return nil, platform.ValidationError(platform.FieldError{Path: "status", Code: "invalid_transition", Message: "This plan transition is not allowed or needs a reason."})
	}
	if input.Status == "completed" && s.now().Before(current.EndsAt) {
		return nil, &platform.DomainError{Code: "conflict", Message: "A plan cannot be completed before the occurrence ends."}
	}
	now := s.now()
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateServicePlan(tx, principal.OrganizationID, id, expectedVersion, input, now, principal.Actor); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, current.BranchID, id, expectedVersion+1, "community.service-plan.transitioned", requestID)
	}); err != nil {
		return nil, fmt.Errorf("transition service plan: %w", err)
	}
	return s.Store.FindServicePlan(ctx, principal.OrganizationID, id)
}

func (s Service) CreateVolunteerRotation(ctx context.Context, principal platform.Principal, input VolunteerRotationInput, requestID string) (*VolunteerRotation, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_rotation", Message: err.Error()})
	}
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, input.TeamID)
	if err != nil {
		return nil, err
	}
	position, err := s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, input.PositionID)
	if err != nil {
		return nil, err
	}
	if team == nil || position == nil || position.TeamID != team.ID || !s.canLeadTeam(principal, *team, "create") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team or position not found."}
	}
	for _, personID := range input.PersonIDs {
		if _, active, resolveErr := s.People.ResolvePersonReference(ctx, principal.OrganizationID, personID); resolveErr != nil {
			return nil, resolveErr
		} else if !active {
			return nil, platform.ValidationError(platform.FieldError{Path: "personIds", Code: "invalid_person", Message: "Every rotation member must be active."})
		}
	}
	now := s.now()
	value := VolunteerRotation{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: team.HomeBranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, TeamID: input.TeamID, PositionID: input.PositionID, Name: input.Name, PersonIDs: input.PersonIDs, CadenceWeeks: input.CadenceWeeks, AnchorDate: input.AnchorDate, Status: input.Status}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertVolunteerRotation(tx, value); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, value.BranchID, value.ID, value.Version, "community.volunteer-rotation.created", requestID)
	}); err != nil {
		return nil, fmt.Errorf("create volunteer rotation: %w", err)
	}
	return &value, nil
}

func (s Service) ListVolunteerRotations(ctx context.Context, principal platform.Principal, teamID platform.ID, includeInactive bool) ([]VolunteerRotation, error) {
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, teamID)
	if err != nil {
		return nil, err
	}
	if team == nil || !s.canLeadTeam(principal, *team, "read") {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer team not found."}
	}
	return s.Store.ListVolunteerRotations(ctx, principal.OrganizationID, teamID, includeInactive)
}

func (s Service) PreviewAssignment(ctx context.Context, principal platform.Principal, planID platform.ID, input AssignmentInput) (*AssignmentPreview, error) {
	plan, err := s.GetServicePlan(ctx, principal, planID)
	if err != nil {
		return nil, err
	}
	if !input.PersonID.Valid() || !input.PositionID.Valid() || input.Slot < 1 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_assignment", Message: "Person, position and a positive slot are required."})
	}
	conflicts, err := s.assignmentConflicts(ctx, principal, *plan, input.PositionID, input.PersonID, "")
	if err != nil {
		return nil, err
	}
	return &AssignmentPreview{Eligible: len(conflicts) == 0, Conflicts: conflicts}, nil
}

func (s Service) CreateAssignment(ctx context.Context, principal platform.Principal, planID platform.ID, input AssignmentInput, requestID string) (*VolunteerAssignment, error) {
	plan, err := s.GetServicePlan(ctx, principal, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status != "draft" && plan.Status != "published" {
		return nil, &platform.DomainError{Code: "conflict", Message: "Assignments can only be added to draft or published plans."}
	}
	if !s.allowedVolunteer(principal, "update", plan.BranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Service plan not found."}
	}
	if !planAllowsSlot(*plan, input.PositionID, input.Slot) {
		return nil, platform.ValidationError(platform.FieldError{Path: "slot", Code: "invalid_slot", Message: "Choose a slot defined by this plan."})
	}
	conflicts, err := s.assignmentConflicts(ctx, principal, *plan, input.PositionID, input.PersonID, "")
	if err != nil {
		return nil, err
	}
	input.OverrideReason = strings.TrimSpace(input.OverrideReason)
	if len(conflicts) > 0 && (len(input.OverrideReason) < 3 || !s.allowedVolunteer(principal, "approve", plan.BranchID, platform.FieldOperational)) {
		return nil, &platform.DomainError{Code: "assignment_conflict", Message: "Resolve the assignment conflicts or use an authorized, reasoned override.", Details: map[string]any{"conflicts": conflicts}}
	}
	position, _ := s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, input.PositionID)
	now := s.now()
	value := VolunteerAssignment{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: plan.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, PlanID: plan.ID, OccurrenceID: plan.OccurrenceID, TeamID: position.TeamID, PositionID: input.PositionID, PersonID: input.PersonID, Slot: input.Slot, StartsAt: plan.StartsAt, EndsAt: plan.EndsAt, Status: "invited", OverrideReason: input.OverrideReason}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertAssignment(tx, value); err != nil {
			return err
		}
		if err := s.Store.InsertAssignmentEvent(tx, s.assignmentEvent(principal, value, "", "invited", "", requestID, now)); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, plan.BranchID, value.ID, value.Version, "community.volunteer-assignment.created", requestID)
	}); err != nil {
		return nil, fmt.Errorf("create volunteer assignment: %w", err)
	}
	return &value, nil
}

func (s Service) RespondAssignment(ctx context.Context, principal platform.Principal, assignmentID platform.ID, input AssignmentResponseInput, requestID string) (*VolunteerAssignment, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	input.Response, input.ReasonCode = strings.ToLower(strings.TrimSpace(input.Response)), strings.ToLower(strings.TrimSpace(input.ReasonCode))
	if input.Response != "accepted" && input.Response != "declined" {
		return nil, platform.ValidationError(platform.FieldError{Path: "response", Code: "invalid_response", Message: "Response must be accepted or declined."})
	}
	if input.Response == "declined" && len(input.ReasonCode) < 3 {
		return nil, platform.ValidationError(platform.FieldError{Path: "reasonCode", Code: "required", Message: "Declining requires a reason code."})
	}
	current, err := s.Store.FindAssignment(ctx, principal.OrganizationID, assignmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status != "invited" || (!s.allowedVolunteer(principal, "update", current.BranchID, platform.FieldOperational) && principal.Actor.ID != current.PersonID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer assignment not found."}
	}
	now := s.now()
	update := AssignmentUpdate{Status: input.Response, ResponseReasonCode: input.ReasonCode, RespondedAt: &now, Reminder: current.Reminder}
	return s.applyAssignmentUpdate(ctx, principal, *current, input.ExpectedVersion, update, "community.volunteer-assignment.responded", input.ReasonCode, requestID)
}

func (s Service) SubstituteAssignment(ctx context.Context, principal platform.Principal, assignmentID platform.ID, input SubstituteInput, requestID string) (*VolunteerAssignment, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	input.Reason, input.OverrideReason = strings.TrimSpace(input.Reason), strings.TrimSpace(input.OverrideReason)
	if !input.SubstitutePersonID.Valid() || len(input.Reason) < 3 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_substitute", Message: "Choose a substitute and provide a reason."})
	}
	current, err := s.Store.FindAssignment(ctx, principal.OrganizationID, assignmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || (current.Status != "invited" && current.Status != "accepted" && current.Status != "substitute-requested") || !s.allowedVolunteer(principal, "update", current.BranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer assignment not found."}
	}
	plan, err := s.GetServicePlan(ctx, principal, current.PlanID)
	if err != nil {
		return nil, err
	}
	conflicts, err := s.assignmentConflicts(ctx, principal, *plan, current.PositionID, input.SubstitutePersonID, current.ID)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 && (len(input.OverrideReason) < 3 || !s.allowedVolunteer(principal, "approve", current.BranchID, platform.FieldOperational)) {
		return nil, &platform.DomainError{Code: "assignment_conflict", Message: "The substitute has unresolved conflicts.", Details: map[string]any{"conflicts": conflicts}}
	}
	now := s.now()
	replacement := VolunteerAssignment{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: current.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor}, PlanID: current.PlanID, OccurrenceID: current.OccurrenceID, TeamID: current.TeamID, PositionID: current.PositionID, PersonID: input.SubstitutePersonID, Slot: current.Slot, StartsAt: current.StartsAt, EndsAt: current.EndsAt, Status: "invited", SubstitutesAssignmentID: current.ID, OverrideReason: input.OverrideReason}
	update := AssignmentUpdate{Status: "replaced", ReplacedByAssignmentID: replacement.ID, Reminder: current.Reminder}
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateAssignment(tx, principal.OrganizationID, current.ID, input.ExpectedVersion, update, now, principal.Actor); err != nil {
			return err
		}
		if err := s.Store.InsertAssignmentEvent(tx, s.assignmentEvent(principal, *current, current.Status, "replaced", input.Reason, requestID, now)); err != nil {
			return err
		}
		if err := s.Store.InsertAssignment(tx, replacement); err != nil {
			return err
		}
		if err := s.Store.InsertAssignmentEvent(tx, s.assignmentEvent(principal, replacement, "", "invited", input.Reason, requestID, now)); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, current.BranchID, replacement.ID, replacement.Version, "community.volunteer-assignment.substituted", requestID)
	}); err != nil {
		return nil, fmt.Errorf("substitute volunteer assignment: %w", err)
	}
	return &replacement, nil
}

func (s Service) RecordAssignmentReminder(ctx context.Context, principal platform.Principal, assignmentID platform.ID, input ReminderInput, requestID string) (*VolunteerAssignment, error) {
	current, err := s.Store.FindAssignment(ctx, principal.OrganizationID, assignmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || !activeAssignmentStatuses[current.Status] || !s.canManageAssignment(ctx, principal, current) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer assignment not found."}
	}
	now := s.now()
	reminder := current.Reminder
	reminder.SentCount++
	reminder.LastSentAt = &now
	reminder.NextDueAt = input.NextDueAt
	return s.applyAssignmentUpdate(ctx, principal, *current, input.ExpectedVersion, AssignmentUpdate{Status: current.Status, ResponseReasonCode: current.ResponseReasonCode, RespondedAt: current.RespondedAt, Reminder: reminder}, "community.volunteer-assignment.reminded", "", requestID)
}

func (s Service) MarkAssignmentNoShow(ctx context.Context, principal platform.Principal, assignmentID platform.ID, input NoShowInput, requestID string) (*VolunteerAssignment, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	current, err := s.Store.FindAssignment(ctx, principal.OrganizationID, assignmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status != "accepted" || !s.canManageAssignment(ctx, principal, current) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer assignment not found."}
	}
	if s.now().Before(current.EndsAt) || len(input.Reason) < 3 {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_no_show", Message: "No-shows require a reason after the occurrence ends."})
	}
	return s.applyAssignmentUpdate(ctx, principal, *current, input.ExpectedVersion, AssignmentUpdate{Status: "no-show", ResponseReasonCode: current.ResponseReasonCode, RespondedAt: current.RespondedAt, Reminder: current.Reminder, NoShowReason: input.Reason}, "community.volunteer-assignment.no-show", input.Reason, requestID)
}

func (s Service) canManageAssignment(ctx context.Context, principal platform.Principal, assignment *VolunteerAssignment) bool {
	if s.allowedVolunteer(principal, "update", assignment.BranchID, platform.FieldOperational) {
		return true
	}
	if principal.Actor.Type != platform.ActorMember || !principal.Actor.ID.Valid() {
		return false
	}
	team, err := s.Store.FindVolunteerTeam(ctx, principal.OrganizationID, assignment.TeamID)
	return err == nil && team != nil && ownsID(team.LeaderPersonIDs, principal.Actor.ID)
}

func (s Service) ListAssignments(ctx context.Context, principal platform.Principal, planID platform.ID) ([]VolunteerAssignment, error) {
	plan, err := s.GetServicePlan(ctx, principal, planID)
	if err != nil {
		return nil, err
	}
	return s.Store.ListAssignments(ctx, principal.OrganizationID, plan.ID)
}

// ListOwnAssignments is deliberately narrower than the coordinator roster:
// members can only read their own serving schedule and cannot enumerate plans.
func (s Service) ListOwnAssignments(ctx context.Context, principal platform.Principal, from, to time.Time) ([]VolunteerAssignment, error) {
	if principal.Actor.Type != platform.ActorMember || !principal.Actor.ID.Valid() {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Member access is required."}
	}
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a valid range no longer than one year."})
	}
	return s.Store.ListPersonAssignments(ctx, principal.OrganizationID, principal.Actor.ID, from.UTC(), to.UTC())
}

func (s Service) ListAssignmentEvents(ctx context.Context, principal platform.Principal, assignmentID platform.ID) ([]AssignmentEvent, error) {
	assignment, err := s.Store.FindAssignment(ctx, principal.OrganizationID, assignmentID)
	if err != nil {
		return nil, err
	}
	if assignment == nil || !s.allowedVolunteer(principal, "read", assignment.BranchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Volunteer assignment not found."}
	}
	return s.Store.ListAssignmentEvents(ctx, principal.OrganizationID, assignmentID)
}

func (s Service) assignmentConflicts(ctx context.Context, principal platform.Principal, plan ServicePlan, positionID, personID, excludeAssignmentID platform.ID) ([]AssignmentConflict, error) {
	position, err := s.Store.FindVolunteerPosition(ctx, principal.OrganizationID, positionID)
	if err != nil {
		return nil, err
	}
	if position == nil || position.Status != "active" || position.BranchID != plan.BranchID {
		return []AssignmentConflict{{Code: "position-unavailable", Severity: "blocker", Message: "The position is not active in this branch."}}, nil
	}
	if s.VolunteerPeople == nil {
		return nil, errors.New("volunteer eligibility resolver is unavailable")
	}
	branchID, active, stage, birthValue, birthPrecision, err := s.VolunteerPeople.ResolveVolunteerEligibility(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	conflicts := []AssignmentConflict{}
	if !active || branchID != plan.BranchID {
		conflicts = append(conflicts, AssignmentConflict{Code: "person-unavailable", Severity: "blocker", Message: "The person is not active in this branch."})
		return conflicts, nil
	}
	profile, err := s.Store.FindVolunteerProfile(ctx, principal.OrganizationID, personID)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.Status != "active" {
		conflicts = append(conflicts, AssignmentConflict{Code: "volunteer-profile-inactive", Severity: "blocker", Message: "An active volunteer profile is required."})
		return conflicts, nil
	}
	for _, required := range position.Eligibility.RequiredSkills {
		if !containsString(profile.Skills, required) {
			conflicts = append(conflicts, AssignmentConflict{Code: "missing-skill", Severity: "blocker", Message: "A required skill is missing."})
			break
		}
	}
	if len(position.Eligibility.MembershipStages) > 0 && !containsString(position.Eligibility.MembershipStages, stage) {
		conflicts = append(conflicts, AssignmentConflict{Code: "membership-stage", Severity: "blocker", Message: "The membership-stage requirement is not met."})
	}
	if position.Eligibility.MinimumAgeYears > 0 {
		if ok, known := meetsMinimumAge(birthValue, birthPrecision, position.Eligibility.MinimumAgeYears, plan.StartsAt); !known {
			conflicts = append(conflicts, AssignmentConflict{Code: "age-unverified", Severity: "blocker", Message: "Age eligibility cannot be verified from the recorded birth-date precision."})
		} else if !ok {
			conflicts = append(conflicts, AssignmentConflict{Code: "minimum-age", Severity: "blocker", Message: "The minimum-age requirement is not met."})
		}
	}
	if position.Eligibility.BackgroundCheckRequired {
		metadata := profile.Eligibility
		if metadata.BackgroundCheckStatus != "cleared" || metadata.BackgroundCheckedAt == nil || metadata.BackgroundCheckExpiresAt == nil || metadata.BackgroundCheckedAt.After(plan.StartsAt) || !metadata.BackgroundCheckExpiresAt.After(plan.EndsAt) {
			conflicts = append(conflicts, AssignmentConflict{Code: "background-check", Severity: "blocker", Message: "Current background-check clearance is required."})
		}
	}
	if position.Eligibility.SafeguardingTrainingRequired {
		metadata := profile.Eligibility
		if metadata.SafeguardingTrainingAt == nil || metadata.SafeguardingTrainingExpiresAt == nil || metadata.SafeguardingTrainingAt.After(plan.StartsAt) || !metadata.SafeguardingTrainingExpiresAt.After(plan.EndsAt) {
			conflicts = append(conflicts, AssignmentConflict{Code: "safeguarding-training", Severity: "blocker", Message: "Current safeguarding training is required."})
		}
	}
	windows, err := s.Store.ListAvailability(ctx, principal.OrganizationID, personID, plan.StartsAt, plan.EndsAt)
	if err != nil {
		return nil, err
	}
	for _, window := range windows {
		if window.State == "unavailable" {
			conflicts = append(conflicts, AssignmentConflict{Code: "unavailable", Severity: "blocker", Message: "The volunteer marked this time unavailable."})
			break
		}
	}
	overlaps, err := s.Store.ListPersonAssignmentsOverlapping(ctx, principal.OrganizationID, personID, plan.StartsAt, plan.EndsAt, excludeAssignmentID)
	if err != nil {
		return nil, err
	}
	if len(overlaps) > 0 {
		conflicts = append(conflicts, AssignmentConflict{Code: "schedule-overlap", Severity: "blocker", Message: "The volunteer already has an overlapping active assignment."})
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Code < conflicts[j].Code })
	return conflicts, nil
}

func (s Service) applyAssignmentUpdate(ctx context.Context, principal platform.Principal, current VolunteerAssignment, expectedVersion int64, update AssignmentUpdate, eventType, reason, requestID string) (*VolunteerAssignment, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.UpdateAssignment(tx, principal.OrganizationID, current.ID, expectedVersion, update, now, principal.Actor); err != nil {
			return err
		}
		updated := current
		updated.Status, updated.Version = update.Status, expectedVersion+1
		if err := s.Store.InsertAssignmentEvent(tx, s.assignmentEvent(principal, updated, current.Status, update.Status, reason, requestID, now)); err != nil {
			return err
		}
		return s.volunteerEvidence(tx, principal, current.BranchID, current.ID, updated.Version, eventType, requestID)
	}); err != nil {
		return nil, fmt.Errorf("update volunteer assignment: %w", err)
	}
	return s.Store.FindAssignment(ctx, principal.OrganizationID, current.ID)
}

func (s Service) assignmentEvent(principal platform.Principal, assignment VolunteerAssignment, fromStatus, toStatus, reason, requestID string, occurredAt time.Time) AssignmentEvent {
	return AssignmentEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: assignment.BranchID, AssignmentID: assignment.ID, PlanID: assignment.PlanID, FromStatus: fromStatus, ToStatus: toStatus, ReasonCode: reason, Actor: principal.Actor, RequestID: requestID, OccurredAt: occurredAt}
}

func planAllowsSlot(plan ServicePlan, positionID platform.ID, slot int) bool {
	for _, need := range plan.Needs {
		if need.PositionID == positionID {
			return slot >= 1 && slot <= need.Slots
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func meetsMinimumAge(value, precision string, minimum int, at time.Time) (bool, bool) {
	if value == "" {
		return false, false
	}
	formats := map[string]string{"year": "2006", "month": "2006-01", "day": "2006-01-02"}
	format, ok := formats[precision]
	if !ok {
		return false, false
	}
	birth, err := time.Parse(format, value)
	if err != nil {
		return false, false
	}
	if precision == "year" {
		ageAtYearEnd := at.Year() - birth.Year()
		ageAtYearStart := ageAtYearEnd - 1
		if ageAtYearStart >= minimum {
			return true, true
		}
		if ageAtYearEnd < minimum {
			return false, true
		}
		return false, false
	}
	if precision == "month" {
		birth = time.Date(birth.Year(), birth.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	age := at.Year() - birth.Year()
	anniversary := birth.AddDate(age, 0, 0)
	if at.Before(anniversary) {
		age--
	}
	return age >= minimum, true
}
