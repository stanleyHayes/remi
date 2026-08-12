package engagement

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type Store interface {
	InsertRule(context.Context, Rule) error
	FindRule(context.Context, platform.ID, platform.ID) (*Rule, error)
	ListRules(context.Context, platform.ID, platform.ID) ([]Rule, error)
	UpdateRule(context.Context, platform.ID, platform.ID, int64, RuleInput, time.Time, platform.Actor) error
	PublishRule(context.Context, platform.ID, platform.ID, int64, ApprovalEvidence, time.Time, platform.Actor) error
	LoadDataset(context.Context, platform.ID, platform.ID, time.Time, time.Time) (Dataset, error)
	InsertSignal(context.Context, Signal) (bool, error)
	FindSignal(context.Context, platform.ID, platform.ID) (*Signal, error)
	ListSignals(context.Context, platform.ID, platform.ID, string, time.Time, int) ([]Signal, error)
	ApplyReview(context.Context, platform.ID, platform.ID, int64, ReviewEvent, time.Time, platform.Actor) error
	InsertReviewEvent(context.Context, ReviewEvent) error
	ListReviewEvents(context.Context, platform.ID, platform.ID) ([]ReviewEvent, error)
	FindPersonSummary(context.Context, platform.ID, platform.ID) (*PersonSummary, error)
	ListCareStaff(context.Context) ([]StaffOption, error)
	CareStaffExists(context.Context, platform.ID) (bool, error)
	InsertSafetyReview(context.Context, SafetyReview) error
	ListSafetyReviews(context.Context, platform.ID, platform.ID) ([]SafetyReview, error)
}
type UnitOfWork interface {
	WithTransaction(context.Context, func(context.Context) error) error
	AppendAudit(context.Context, platform.AuditEvent) error
	EnqueueEvent(context.Context, platform.OutboxRecord) error
}
type Service struct {
	Store      Store
	Platform   UnitOfWork
	Authorizer platform.Authorizer
	Flags      *platform.FeatureFlags
	Consent    func(context.Context, platform.Principal, platform.ID, platform.ID, string, string) (ConsentDecision, error)
	Now        func() time.Time
}

func (s Service) ActivationEnabled() bool {
	return s.Flags != nil && s.Flags.Enabled("retention-individual-signals")
}

func (s Service) CreateRule(ctx context.Context, p platform.Principal, input RuleInput, requestID string) (*Rule, error) {
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_rule", Message: err.Error()})
	}
	if !s.allowed(p, "create", input.BranchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot create engagement rules in this branch."}
	}
	now := s.now()
	value := Rule{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: input.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: p.Actor, UpdatedAt: now, UpdatedBy: p.Actor}, Name: input.Name, Kind: input.Kind, Description: input.Description, Timezone: input.Timezone, WindowDays: input.WindowDays, LookbackDays: input.LookbackDays, ExpiresAfterDays: input.ExpiresAfterDays, Status: "draft", MetricVersion: "retention-v1", Approval: input.Approval}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertRule(tx, value); err != nil {
			return err
		}
		return s.evidence(tx, p, value.BranchID, "engagement.rule.created", "engagement-rule", value.ID, value.Version, requestID, now)
	}); err != nil {
		return nil, fmt.Errorf("create engagement rule: %w", err)
	}
	return &value, nil
}
func (s Service) UpdateRule(ctx context.Context, p platform.Principal, id platform.ID, expected int64, input RuleInput, requestID string) (*Rule, error) {
	if err := platform.RequireExpectedVersion(expected); err != nil {
		return nil, err
	}
	if err := input.normalize(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_rule", Message: err.Error()})
	}
	current, err := s.Store.FindRule(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status != "draft" || !s.allowed(p, "update", current.BranchID) || !s.allowed(p, "update", input.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Editable engagement rule not found."}
	}
	now := s.now()
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Store.UpdateRule(tx, p.OrganizationID, id, expected, input, now, p.Actor); e != nil {
			return e
		}
		return s.evidence(tx, p, input.BranchID, "engagement.rule.updated", "engagement-rule", id, expected+1, requestID, now)
	}); err != nil {
		return nil, err
	}
	return s.Store.FindRule(ctx, p.OrganizationID, id)
}
func (s Service) PublishRule(ctx context.Context, p platform.Principal, id platform.ID, expected int64, approval ApprovalEvidence, requestID string) (*Rule, error) {
	if err := platform.RequireExpectedVersion(expected); err != nil {
		return nil, err
	}
	current, err := s.Store.FindRule(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status != "draft" || !s.allowed(p, "approve", current.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Draft engagement rule not found."}
	}
	if !approval.complete() {
		return nil, platform.ValidationError(platform.FieldError{Path: "approval", Code: "approval_required", Message: "Product, pastoral and privacy approvals with date and cadence are required."})
	}
	if approval.ProductOwnerID == approval.PastoralApproverID || approval.ProductOwnerID == approval.PrivacyApproverID || approval.PastoralApproverID == approval.PrivacyApproverID {
		return nil, platform.ValidationError(platform.FieldError{Path: "approval", Code: "independent_approvals_required", Message: "Product, pastoral and privacy approvals must be recorded by different people."})
	}
	if approval.ApprovedAt.After(s.now().Add(time.Minute)) {
		return nil, platform.ValidationError(platform.FieldError{Path: "approval.approvedAt", Code: "invalid", Message: "Approval cannot be dated in the future."})
	}
	if s.Flags == nil || !s.Flags.Enabled("retention-individual-signals") {
		return nil, &platform.DomainError{Code: "feature_disabled", Message: "Individual retention signals remain disabled until the approved production activation flag is enabled."}
	}
	now := s.now()
	if err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if e := s.Store.PublishRule(tx, p.OrganizationID, id, expected, approval, now, p.Actor); e != nil {
			return e
		}
		return s.evidence(tx, p, current.BranchID, "engagement.rule.published", "engagement-rule", id, expected+1, requestID, now)
	}); err != nil {
		return nil, err
	}
	return s.Store.FindRule(ctx, p.OrganizationID, id)
}
func (s Service) ListRules(ctx context.Context, p platform.Principal, branch platform.ID) ([]Rule, error) {
	if !s.allowed(p, "read", branch) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read engagement rules in this branch."}
	}
	return s.Store.ListRules(ctx, p.OrganizationID, branch)
}
func (s Service) GetRule(ctx context.Context, p platform.Principal, id platform.ID) (*Rule, error) {
	value, err := s.Store.FindRule(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.allowed(p, "read", value.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Engagement rule not found."}
	}
	return value, nil
}
func (s Service) GetSignal(ctx context.Context, p platform.Principal, id platform.ID) (*Signal, error) {
	value, err := s.Store.FindSignal(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || !s.allowed(p, "read", value.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Engagement signal not found."}
	}
	return value, nil
}
func (s Service) ListSignals(ctx context.Context, p platform.Principal, branch platform.ID, state string, limit int) ([]Signal, error) {
	if !s.allowed(p, "read", branch) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read engagement signals in this branch."}
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	return s.Store.ListSignals(ctx, p.OrganizationID, branch, state, s.now(), limit)
}

func (s Service) GetReviewDetail(ctx context.Context, p platform.Principal, id platform.ID) (*ReviewDetail, error) {
	signal, err := s.GetSignal(ctx, p, id)
	if err != nil {
		return nil, err
	}
	person, err := s.Store.FindPersonSummary(ctx, p.OrganizationID, signal.PersonID)
	if err != nil {
		return nil, err
	}
	if person == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "The person linked to this observation is unavailable."}
	}
	events, err := s.Store.ListReviewEvents(ctx, p.OrganizationID, signal.ID)
	if err != nil {
		return nil, err
	}
	assignees, err := s.Store.ListCareStaff(ctx)
	if err != nil {
		return nil, err
	}
	return &ReviewDetail{Signal: signal, Person: person, Events: events, Assignees: assignees}, nil
}

func (s Service) Assign(ctx context.Context, p platform.Principal, id platform.ID, input AssignInput, requestID string) (*Signal, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if !input.AssigneeID.Valid() || platform.ValidateReason(input.Reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_assignment", Message: "Choose an active care reviewer and record a reason."})
	}
	eligible, err := s.Store.CareStaffExists(ctx, input.AssigneeID)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, platform.ValidationError(platform.FieldError{Path: "assigneeId", Code: "invalid", Message: "Choose an active reviewer with restricted ministry access."})
	}
	return s.review(ctx, p, id, input.ExpectedVersion, ReviewEvent{Type: "assigned", ToState: "in-review", AssigneeID: input.AssigneeID, Reason: input.Reason}, requestID)
}

func (s Service) Snooze(ctx context.Context, p platform.Principal, id platform.ID, input SnoozeInput, requestID string) (*Signal, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	now, reason := s.now(), strings.TrimSpace(input.Reason)
	if input.Until.IsZero() || !input.Until.After(now) || input.Until.After(now.AddDate(0, 0, 90)) || platform.ValidateReason(reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_snooze", Message: "Choose a future snooze within 90 days and record a reason."})
	}
	until := input.Until.UTC()
	return s.review(ctx, p, id, input.ExpectedVersion, ReviewEvent{Type: "snoozed", ToState: "snoozed", SnoozedUntil: &until, Reason: reason}, requestID)
}

func (s Service) Suppress(ctx context.Context, p platform.Principal, id platform.ID, input SuppressInput, requestID string) (*Signal, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	input.Reason, input.FeedbackCode = strings.TrimSpace(input.Reason), strings.ToLower(strings.TrimSpace(input.FeedbackCode))
	allowed := map[string]bool{"false-positive": true, "duplicate-evidence": true, "stale-data": true, "known-context": true, "person-request": true, "other": true}
	if !allowed[input.FeedbackCode] || platform.ValidateReason(input.Reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_suppression", Message: "Choose a feedback category and record why this observation should leave the queue."})
	}
	falsePositive := map[string]bool{"false-positive": true, "duplicate-evidence": true, "stale-data": true}[input.FeedbackCode]
	return s.review(ctx, p, id, input.ExpectedVersion, ReviewEvent{Type: "suppressed", ToState: "suppressed", Outcome: input.FeedbackCode, Reason: input.Reason, FalsePositive: &falsePositive}, requestID)
}

func (s Service) RecordContact(ctx context.Context, p platform.Principal, id platform.ID, input ContactInput, requestID string) (*Signal, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	input.Channel, input.Outcome, input.Summary = strings.ToLower(strings.TrimSpace(input.Channel)), strings.ToLower(strings.TrimSpace(input.Outcome)), strings.TrimSpace(input.Summary)
	channels := map[string]bool{"phone": true, "email": true, "sms": true, "whatsapp": true, "push": true}
	outcomes := map[string]bool{"connected": true, "no-answer": true, "message-left": true, "declined": true, "follow-up-requested": true}
	if !channels[input.Channel] || !outcomes[input.Outcome] || input.Summary == "" || len(input.Summary) > 500 {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_contact", Message: "Choose a supported channel and outcome, and record a concise summary."})
	}
	signal, err := s.GetSignal(ctx, p, id)
	if err != nil {
		return nil, err
	}
	person, err := s.Store.FindPersonSummary(ctx, p.OrganizationID, signal.PersonID)
	if err != nil {
		return nil, err
	}
	if person == nil || person.Archived {
		return nil, &platform.DomainError{Code: "conflict", Message: "The linked person is inactive, so outreach cannot be recorded."}
	}
	if signal.AssigneeID != p.Actor.ID {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Only the assigned reviewer can record outreach."}
	}
	if s.Consent == nil {
		return nil, &platform.DomainError{Code: "consent_unavailable", Message: "Current consent could not be verified, so outreach was not recorded."}
	}
	decision, err := s.Consent(ctx, p, signal.BranchID, signal.PersonID, "pastoral-care", input.Channel)
	if err != nil {
		return nil, err
	}
	if !decision.Eligible {
		return nil, &platform.DomainError{Code: "consent_required", Message: "Current pastoral-care consent does not permit this channel."}
	}
	evaluated := decision.EvaluatedAt.UTC()
	return s.reviewWithCurrent(ctx, p, signal, input.ExpectedVersion, ReviewEvent{Type: "contact-recorded", ToState: signal.State, Channel: input.Channel, Outcome: input.Outcome, Reason: input.Summary, ConsentDecision: decision.Decision, ConsentEvaluatedAt: &evaluated, ConsentProjectionID: decision.ProjectionID, ConsentProjectionVersion: decision.ProjectionVersion, BlockingSuppressionIDs: append([]platform.ID{}, decision.BlockingSuppressionIDs...)}, requestID)
}

func (s Service) Resolve(ctx context.Context, p platform.Principal, id platform.ID, input ResolveInput, requestID string) (*Signal, error) {
	if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
		return nil, err
	}
	input.Outcome, input.Reason = strings.ToLower(strings.TrimSpace(input.Outcome)), strings.TrimSpace(input.Reason)
	allowed := map[string]bool{"reconnected": true, "already-connected": true, "care-offered": true, "declined": true, "unreachable": true, "no-action-needed": true, "referred": true, "other": true}
	if !allowed[input.Outcome] || platform.ValidateReason(input.Reason) != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_resolution", Message: "Choose an outcome and record the review context."})
	}
	falsePositive := input.FalsePositive
	return s.review(ctx, p, id, input.ExpectedVersion, ReviewEvent{Type: "resolved", ToState: "resolved", Outcome: input.Outcome, Reason: input.Reason, FalsePositive: &falsePositive}, requestID)
}

func (s Service) review(ctx context.Context, p platform.Principal, id platform.ID, expected int64, event ReviewEvent, requestID string) (*Signal, error) {
	signal, err := s.GetSignal(ctx, p, id)
	if err != nil {
		return nil, err
	}
	if event.Type != "assigned" && signal.AssigneeID != p.Actor.ID {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Only the assigned reviewer can change this observation."}
	}
	if event.Type == "snoozed" && event.SnoozedUntil != nil && !event.SnoozedUntil.Before(signal.ExpiresAt) {
		return nil, platform.ValidationError(platform.FieldError{Path: "until", Code: "invalid", Message: "Snooze must end before the evidence expires."})
	}
	return s.reviewWithCurrent(ctx, p, signal, expected, event, requestID)
}

func (s Service) reviewWithCurrent(ctx context.Context, p platform.Principal, signal *Signal, expected int64, event ReviewEvent, requestID string) (*Signal, error) {
	if signal.Version != expected {
		return nil, platform.VersionConflict(expected)
	}
	if signal.State == "resolved" || signal.State == "suppressed" || !signal.ExpiresAt.After(s.now()) {
		return nil, &platform.DomainError{Code: "conflict", Message: "This observation is no longer actionable."}
	}
	now := s.now()
	event.ID, event.OrganizationID, event.BranchID, event.SignalID, event.FromState, event.Actor, event.RequestID, event.OccurredAt = platform.ID(bson.NewObjectID().Hex()), p.OrganizationID, signal.BranchID, signal.ID, signal.State, p.Actor, requestID, now
	if event.ToState == "" {
		event.ToState = signal.State
	}
	err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.ApplyReview(tx, p.OrganizationID, signal.ID, expected, event, now, p.Actor); err != nil {
			return err
		}
		if err := s.Store.InsertReviewEvent(tx, event); err != nil {
			return err
		}
		return s.evidence(tx, p, signal.BranchID, "engagement.signal."+event.Type, "engagement-signal", signal.ID, expected+1, requestID, now)
	})
	if err != nil {
		return nil, err
	}
	return s.Store.FindSignal(ctx, p.OrganizationID, signal.ID)
}
func (s Service) Generate(ctx context.Context, p platform.Principal, ruleID platform.ID, asOf time.Time, requestID string) (*GenerationResult, error) {
	rule, err := s.Store.FindRule(ctx, p.OrganizationID, ruleID)
	if err != nil {
		return nil, err
	}
	if rule == nil || rule.Status != "published" || !s.allowed(p, "operate", rule.BranchID) {
		return nil, &platform.DomainError{Code: "not_found", Message: "Published engagement rule not found."}
	}
	if s.Flags == nil || !s.Flags.Enabled("retention-individual-signals") {
		return nil, &platform.DomainError{Code: "feature_disabled", Message: "Individual retention signals are disabled."}
	}
	asOf = asOf.UTC()
	if asOf.IsZero() || asOf.After(s.now().Add(time.Minute)) {
		return nil, platform.ValidationError(platform.FieldError{Path: "asOf", Code: "invalid", Message: "Choose a valid as-of time."})
	}
	from := asOf.AddDate(0, 0, -rule.LookbackDays)
	data, err := s.Store.LoadDataset(ctx, p.OrganizationID, rule.BranchID, from, asOf)
	if err != nil {
		return nil, err
	}
	candidates := evaluate(*rule, data, asOf)
	result := &GenerationResult{RuleID: rule.ID, Candidates: len(candidates), SourceWatermark: data.LatestRecordedAt, Caveats: append([]string{}, data.Caveats...), GeneratedAt: s.now()}
	for _, candidate := range candidates {
		candidate.CreatedBy, candidate.UpdatedBy = p.Actor, p.Actor
		candidate.Caveats = append(candidate.Caveats, data.Caveats...)
		created, insertErr := s.Store.InsertSignal(ctx, candidate)
		if insertErr != nil {
			return nil, insertErr
		}
		if created {
			result.Created++
		} else {
			result.Existing++
		}
	}
	if err = s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: rule.BranchID, Actor: p.Actor, Action: "engagement.signals.generated", ResourceType: "engagement-rule", ResourceID: rule.ID, ChangedFields: []string{"candidates", "created", "existing", "sourceWatermark"}, Outcome: "success", RequestID: requestID, OccurredAt: s.now()}); err != nil {
		return nil, err
	}
	return result, nil
}

func evaluate(rule Rule, data Dataset, asOf time.Time) []Signal {
	visits := map[platform.ID][]VisitFact{}
	for _, v := range data.Visits {
		if data.ActivePeople[v.PersonID] {
			visits[v.PersonID] = append(visits[v.PersonID], v)
		}
	}
	connections := map[platform.ID][]ConnectionFact{}
	for _, c := range data.Connections {
		connections[c.PersonID] = append(connections[c.PersonID], c)
	}
	result := []Signal{}
	lookbackStart := asOf.AddDate(0, 0, -rule.LookbackDays)
	for personID, values := range visits {
		sort.Slice(values, func(i, j int) bool { return values[i].StartsAt.Before(values[j].StartsAt) })
		first, last := values[0], values[len(values)-1]
		windowEnd := localDeadlineExclusive(first.StartsAt, rule.Timezone, rule.WindowDays)
		eligible, evidence := false, []Evidence{}
		observedFrom := first.StartsAt
		switch rule.Kind {
		case "attendance-gap":
			observedFrom = lookbackStart
			eligible = !last.StartsAt.Before(lookbackStart) && !asOf.Before(localDeadlineExclusive(last.StartsAt, rule.Timezone, rule.WindowDays))
			evidence = []Evidence{{SourceType: "attendance", SourceID: last.AttendanceID, OccurredAt: last.StartsAt, Fact: "last qualifying named attendance"}}
		case "first-visit-no-return":
			if first.Guest && !asOf.Before(windowEnd) {
				eligible = true
				for _, v := range values[1:] {
					if laterLocalDate(first.StartsAt, v.StartsAt, rule.Timezone) && v.StartsAt.Before(windowEnd) {
						eligible = false
						break
					}
				}
			}
			evidence = []Evidence{{SourceType: "attendance", SourceID: first.AttendanceID, OccurredAt: first.StartsAt, Fact: "first qualifying guest visit"}}
		case "first-visit-no-group", "first-visit-no-serving":
			if first.Guest && !asOf.Before(windowEnd) {
				eligible = true
				wanted := "group"
				if rule.Kind == "first-visit-no-serving" {
					wanted = "serving"
				}
				for _, c := range connections[personID] {
					if c.Kind == wanted && c.OccurredAt.After(first.StartsAt) && c.OccurredAt.Before(windowEnd) {
						eligible = false
						break
					}
				}
			}
			evidence = []Evidence{{SourceType: "attendance", SourceID: first.AttendanceID, OccurredAt: first.StartsAt, Fact: "first qualifying guest visit"}}
		}
		if !eligible {
			continue
		}
		now := asOf
		evidenceAnchor := "no-source"
		if len(evidence) > 0 {
			evidenceAnchor = string(evidence[0].SourceID)
		}
		result = append(result, Signal{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: rule.OrganizationID, BranchID: rule.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, UpdatedAt: now}, RuleID: rule.ID, RuleVersion: rule.Version, RuleName: rule.Name, RuleKind: rule.Kind, PersonID: personID, ObservedFrom: observedFrom, ObservedThrough: asOf, Evidence: evidence, Caveats: []string{"This is an observed participation gap, not a judgment of faith, intent, worth or pastoral need.", "A human reviewer must verify context and consent before any action."}, State: "open", SourceKey: fmt.Sprintf("%s:%d:%s:%s", rule.ID, rule.Version, personID, evidenceAnchor), ExpiresAt: asOf.AddDate(0, 0, rule.ExpiresAfterDays)})
	}
	return result
}
func (s Service) allowed(p platform.Principal, action string, branch platform.ID) bool {
	return s.Authorizer.Authorize(p, platform.AccessRequest{Action: action, ResourceType: "engagement", OrganizationID: p.OrganizationID, BranchID: branch, FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}, Now: s.now()}).Allowed
}
func localDeadlineExclusive(value time.Time, timezone string, days int) time.Time {
	location, _ := time.LoadLocation(timezone)
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, days+1).UTC()
}
func laterLocalDate(first, candidate time.Time, timezone string) bool {
	location, _ := time.LoadLocation(timezone)
	a, b := first.In(location), candidate.In(location)
	return time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, location).After(time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, location))
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) evidence(ctx context.Context, p platform.Principal, branch platform.ID, event, resource string, id platform.ID, version int64, requestID string, now time.Time) error {
	if err := s.Platform.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branch, Actor: p.Actor, Action: event, ResourceType: resource, ResourceID: id, ChangedFields: []string{"configuration", "status", "version"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
		return err
	}
	return s.Platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: branch, Type: event, EventVersion: 1, AggregateType: resource, AggregateID: id, AggregateVersion: version, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"resourceId": id, "version": version}}, State: "pending", AvailableAt: now})
}
