package engagement

import (
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

var ruleKinds = map[string]bool{"attendance-gap": true, "first-visit-no-return": true, "first-visit-no-group": true, "first-visit-no-serving": true}

type ApprovalEvidence struct {
	ProductOwnerID     platform.ID `json:"productOwnerId" bson:"productOwnerId"`
	PastoralApproverID platform.ID `json:"pastoralApproverId" bson:"pastoralApproverId"`
	PrivacyApproverID  platform.ID `json:"privacyApproverId" bson:"privacyApproverId"`
	ApprovedAt         *time.Time  `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	ReviewCadenceDays  int         `json:"reviewCadenceDays" bson:"reviewCadenceDays"`
}

type Rule struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string           `json:"name" bson:"name"`
	Kind                      string           `json:"kind" bson:"kind"`
	Description               string           `json:"description" bson:"description"`
	Timezone                  string           `json:"timezone" bson:"timezone"`
	WindowDays                int              `json:"windowDays" bson:"windowDays"`
	LookbackDays              int              `json:"lookbackDays" bson:"lookbackDays"`
	ExpiresAfterDays          int              `json:"expiresAfterDays" bson:"expiresAfterDays"`
	Status                    string           `json:"status" bson:"status"`
	MetricVersion             string           `json:"metricVersion" bson:"metricVersion"`
	Approval                  ApprovalEvidence `json:"approval" bson:"approval"`
	PublishedAt               *time.Time       `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
}

type RuleInput struct {
	ExpectedVersion  int64            `json:"expectedVersion"`
	BranchID         platform.ID      `json:"branchId"`
	Name             string           `json:"name"`
	Kind             string           `json:"kind"`
	Description      string           `json:"description"`
	Timezone         string           `json:"timezone"`
	WindowDays       int              `json:"windowDays"`
	LookbackDays     int              `json:"lookbackDays"`
	ExpiresAfterDays int              `json:"expiresAfterDays"`
	Approval         ApprovalEvidence `json:"approval"`
}

func (input *RuleInput) normalize() error {
	input.Name, input.Kind, input.Description, input.Timezone = strings.TrimSpace(input.Name), strings.ToLower(strings.TrimSpace(input.Kind)), strings.TrimSpace(input.Description), strings.TrimSpace(input.Timezone)
	if !input.BranchID.Valid() || len(input.Name) < 3 || len(input.Name) > 150 || len(input.Description) > 1000 {
		return errors.New("branch, a 3 to 150 character name and a concise description are required")
	}
	if !ruleKinds[input.Kind] {
		return errors.New("choose a supported evidence rule")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return errors.New("timezone must be a valid IANA timezone")
	}
	if input.WindowDays < 7 || input.WindowDays > 180 || input.LookbackDays < input.WindowDays || input.LookbackDays > 730 {
		return errors.New("window must be 7 to 180 days and lookback must cover it without exceeding 730 days")
	}
	if input.ExpiresAfterDays < 1 || input.ExpiresAfterDays > 180 {
		return errors.New("signal expiry must be 1 to 180 days")
	}
	if input.Approval.ReviewCadenceDays < 1 || input.Approval.ReviewCadenceDays > 365 {
		return errors.New("review cadence must be 1 to 365 days")
	}
	return nil
}

func (a ApprovalEvidence) complete() bool {
	return a.ProductOwnerID.Valid() && a.PastoralApproverID.Valid() && a.PrivacyApproverID.Valid() && a.ApprovedAt != nil && a.ReviewCadenceDays > 0
}

type Evidence struct {
	SourceType string      `json:"sourceType" bson:"sourceType"`
	SourceID   platform.ID `json:"sourceId" bson:"sourceId"`
	OccurredAt time.Time   `json:"occurredAt" bson:"occurredAt"`
	Fact       string      `json:"fact" bson:"fact"`
}

type Signal struct {
	platform.ResourceEnvelope `bson:",inline"`
	RuleID                    platform.ID `json:"ruleId" bson:"ruleId"`
	RuleVersion               int64       `json:"ruleVersion" bson:"ruleVersion"`
	RuleName                  string      `json:"ruleName" bson:"ruleName"`
	RuleKind                  string      `json:"ruleKind" bson:"ruleKind"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	ObservedFrom              time.Time   `json:"observedFrom" bson:"observedFrom"`
	ObservedThrough           time.Time   `json:"observedThrough" bson:"observedThrough"`
	Evidence                  []Evidence  `json:"evidence" bson:"evidence"`
	Caveats                   []string    `json:"caveats" bson:"caveats"`
	State                     string      `json:"state" bson:"state"`
	AssigneeID                platform.ID `json:"assigneeId,omitempty" bson:"assigneeId,omitempty"`
	SnoozedUntil              *time.Time  `json:"snoozedUntil,omitempty" bson:"snoozedUntil,omitempty"`
	ResolutionOutcome         string      `json:"resolutionOutcome,omitempty" bson:"resolutionOutcome,omitempty"`
	FalsePositive             *bool       `json:"falsePositive,omitempty" bson:"falsePositive,omitempty"`
	ResolvedAt                *time.Time  `json:"resolvedAt,omitempty" bson:"resolvedAt,omitempty"`
	SourceKey                 string      `json:"-" bson:"sourceKey"`
	ExpiresAt                 time.Time   `json:"expiresAt" bson:"expiresAt"`
}

type ReviewEvent struct {
	ID                       platform.ID    `json:"id" bson:"_id"`
	OrganizationID           platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID                 platform.ID    `json:"branchId" bson:"branchId"`
	SignalID                 platform.ID    `json:"signalId" bson:"signalId"`
	Type                     string         `json:"type" bson:"type"`
	FromState                string         `json:"fromState" bson:"fromState"`
	ToState                  string         `json:"toState" bson:"toState"`
	AssigneeID               platform.ID    `json:"assigneeId,omitempty" bson:"assigneeId,omitempty"`
	Reason                   string         `json:"reason" bson:"reason"`
	Outcome                  string         `json:"outcome,omitempty" bson:"outcome,omitempty"`
	FalsePositive            *bool          `json:"falsePositive,omitempty" bson:"falsePositive,omitempty"`
	SnoozedUntil             *time.Time     `json:"snoozedUntil,omitempty" bson:"snoozedUntil,omitempty"`
	Channel                  string         `json:"channel,omitempty" bson:"channel,omitempty"`
	ConsentDecision          string         `json:"consentDecision,omitempty" bson:"consentDecision,omitempty"`
	ConsentEvaluatedAt       *time.Time     `json:"consentEvaluatedAt,omitempty" bson:"consentEvaluatedAt,omitempty"`
	ConsentProjectionID      platform.ID    `json:"consentProjectionId,omitempty" bson:"consentProjectionId,omitempty"`
	ConsentProjectionVersion int64          `json:"consentProjectionVersion,omitempty" bson:"consentProjectionVersion,omitempty"`
	BlockingSuppressionIDs   []platform.ID  `json:"blockingSuppressionIds,omitempty" bson:"blockingSuppressionIds,omitempty"`
	Actor                    platform.Actor `json:"actor" bson:"actor"`
	RequestID                string         `json:"requestId" bson:"requestId"`
	OccurredAt               time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type PersonSummary struct {
	ID           platform.ID `json:"id" bson:"_id"`
	DisplayName  string      `json:"displayName"`
	PersonNumber string      `json:"personNumber,omitempty" bson:"personNumber,omitempty"`
	Archived     bool        `json:"archived"`
}

type StaffOption struct {
	ID    platform.ID `json:"id"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
	Role  string      `json:"role"`
}

type ReviewDetail struct {
	Signal    *Signal        `json:"signal"`
	Person    *PersonSummary `json:"person"`
	Events    []ReviewEvent  `json:"events"`
	Assignees []StaffOption  `json:"assignees"`
}

type AssignInput struct {
	ExpectedVersion int64       `json:"expectedVersion"`
	AssigneeID      platform.ID `json:"assigneeId"`
	Reason          string      `json:"reason"`
}
type SnoozeInput struct {
	ExpectedVersion int64     `json:"expectedVersion"`
	Until           time.Time `json:"until"`
	Reason          string    `json:"reason"`
}
type SuppressInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
	FeedbackCode    string `json:"feedbackCode"`
}
type ContactInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Channel         string `json:"channel"`
	Outcome         string `json:"outcome"`
	Summary         string `json:"summary"`
}
type ResolveInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Outcome         string `json:"outcome"`
	Reason          string `json:"reason"`
	FalsePositive   bool   `json:"falsePositive"`
}

type SafetyChecklist struct {
	ReasonUnderstood     bool `json:"reasonUnderstood" bson:"reasonUnderstood"`
	CaveatsVisible       bool `json:"caveatsVisible" bson:"caveatsVisible"`
	NoDiagnosis          bool `json:"noDiagnosis" bson:"noDiagnosis"`
	NoAutomaticAction    bool `json:"noAutomaticAction" bson:"noAutomaticAction"`
	ConsentBoundaryClear bool `json:"consentBoundaryClear" bson:"consentBoundaryClear"`
	SparseDataProtected  bool `json:"sparseDataProtected" bson:"sparseDataProtected"`
	NoFinancialInference bool `json:"noFinancialInference" bson:"noFinancialInference"`
	LanguageIsPastoral   bool `json:"languageIsPastoral" bson:"languageIsPastoral"`
}

func (value SafetyChecklist) complete() bool {
	return value.ReasonUnderstood && value.CaveatsVisible && value.NoDiagnosis && value.NoAutomaticAction && value.ConsentBoundaryClear && value.SparseDataProtected && value.NoFinancialInference && value.LanguageIsPastoral
}

type SafetyReviewInput struct {
	BranchID  platform.ID     `json:"branchId"`
	Role      string          `json:"role"`
	Decision  string          `json:"decision"`
	Findings  string          `json:"findings"`
	Checklist SafetyChecklist `json:"checklist"`
}

type SafetyReview struct {
	ID                platform.ID     `json:"id" bson:"_id"`
	OrganizationID    platform.ID     `json:"organizationId" bson:"organizationId"`
	BranchID          platform.ID     `json:"branchId" bson:"branchId"`
	MetricVersion     string          `json:"metricVersion" bson:"metricVersion"`
	PolicyFingerprint string          `json:"policyFingerprint" bson:"policyFingerprint"`
	Role              string          `json:"role" bson:"role"`
	Decision          string          `json:"decision" bson:"decision"`
	Findings          string          `json:"findings" bson:"findings"`
	Checklist         SafetyChecklist `json:"checklist" bson:"checklist"`
	Actor             platform.Actor  `json:"actor" bson:"actor"`
	RequestID         string          `json:"requestId" bson:"requestId"`
	ReviewedAt        time.Time       `json:"reviewedAt" bson:"reviewedAt"`
}

type SafetyPolicyReference struct {
	ID          platform.ID `json:"id"`
	Version     int64       `json:"version"`
	Name        string      `json:"name"`
	Kind        string      `json:"kind"`
	PublishedAt *time.Time  `json:"publishedAt,omitempty"`
}

type SafetyReadiness struct {
	MetricVersion     string                  `json:"metricVersion"`
	BranchID          platform.ID             `json:"branchId"`
	PolicyFingerprint string                  `json:"policyFingerprint"`
	Policies          []SafetyPolicyReference `json:"policies"`
	RequiredRoles     []string                `json:"requiredRoles"`
	EligibleRoles     []string                `json:"eligibleRoles"`
	RoleStatus        map[string]string       `json:"roleStatus"`
	ReleaseState      string                  `json:"releaseState"`
	Reviews           []SafetyReview          `json:"reviews"`
	GeneratedAt       time.Time               `json:"generatedAt"`
}

type ConsentDecision struct {
	Eligible               bool
	Decision               string
	EvaluatedAt            time.Time
	ProjectionID           platform.ID
	ProjectionVersion      int64
	BlockingSuppressionIDs []platform.ID
}

type VisitFact struct {
	AttendanceID, OccurrenceID, PersonID platform.ID
	StartsAt, RecordedAt                 time.Time
	Guest                                bool
}
type ConnectionFact struct {
	ID, PersonID platform.ID
	OccurredAt   time.Time
	Kind         string
}
type Dataset struct {
	Visits           []VisitFact
	Connections      []ConnectionFact
	ActivePeople     map[platform.ID]bool
	LatestRecordedAt *time.Time
	Caveats          []string
}

type GenerationResult struct {
	RuleID          platform.ID `json:"ruleId"`
	Created         int         `json:"created"`
	Existing        int         `json:"existing"`
	Candidates      int         `json:"candidates"`
	SourceWatermark *time.Time  `json:"sourceWatermark,omitempty"`
	Caveats         []string    `json:"caveats"`
	GeneratedAt     time.Time   `json:"generatedAt"`
}
