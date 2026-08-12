package engagement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type testPlatform struct {
	mu     sync.Mutex
	audits []platform.AuditEvent
	events []platform.OutboxRecord
}

func (p *testPlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (p *testPlatform) AppendAudit(_ context.Context, value platform.AuditEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.audits = append(p.audits, value)
	return nil
}
func (p *testPlatform) EnqueueEvent(_ context.Context, value platform.OutboxRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, value)
	return nil
}

func TestSignalGenerationIsEvidenceLinkedIdempotentAndActivationGated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_engagement_test")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	repo, _ := NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	actor := platform.Actor{Type: platform.ActorStaff, ID: "reviewer-1"}
	envelope := platform.ResourceEnvelope{ID: "rule-1", OrganizationID: "org-1", BranchID: "accra", SchemaVersion: 1, Version: 1, CreatedAt: now.AddDate(0, 0, -60), CreatedBy: actor, UpdatedAt: now.AddDate(0, 0, -60), UpdatedBy: actor}
	approvalTime := now.AddDate(0, 0, -61)
	rule := Rule{ResourceEnvelope: envelope, Name: "First visit return evidence", Kind: "first-visit-no-return", Timezone: "Africa/Accra", WindowDays: 30, LookbackDays: 180, ExpiresAfterDays: 14, Status: "published", MetricVersion: "retention-v1", Approval: ApprovalEvidence{ProductOwnerID: "owner-1", PastoralApproverID: "pastor-1", PrivacyApproverID: "privacy-1", ApprovedAt: &approvalTime, ReviewCadenceDays: 30}}
	if err = repo.InsertRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "person-1", "organizationId": "org-1", "homeBranchId": "accra", "personNumber": "P-0001", "names": bson.M{"given": "Ama", "family": "Mensah"}, "archivedAt": nil})
	if err != nil {
		t.Fatal(err)
	}
	visitAt := now.AddDate(0, 0, -45)
	_, err = db.Collection("chms_occurrences").InsertOne(ctx, bson.M{"_id": "occurrence-1", "organizationId": "org-1", "homeBranchId": "accra", "startsAt": visitAt, "endsAt": visitAt.Add(2 * time.Hour), "status": "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("chms_attendance").InsertOne(ctx, bson.M{"_id": "attendance-1", "organizationId": "org-1", "branchId": "accra", "occurrenceId": "occurrence-1", "personId": "person-1", "status": "present", "guest": true, "updatedAt": visitAt.Add(3 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	p := platform.Principal{Actor: actor, OrganizationID: "org-1", Grants: []platform.Grant{{Action: "*", Resource: "engagement", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}}}}
	evidence := &testPlatform{}
	service := Service{Store: repo, Platform: evidence, Authorizer: platform.GrantAuthorizer{}, Flags: platform.NewFeatureFlags(1, map[string]bool{"retention-individual-signals": true}), Now: func() time.Time { return now }}
	generated, err := service.Generate(ctx, p, rule.ID, now, "generate-1")
	if err != nil || generated.Created != 1 || generated.Candidates != 1 {
		t.Fatalf("generation=%+v err=%v", generated, err)
	}
	again, err := service.Generate(ctx, p, rule.ID, now, "generate-2")
	if err != nil || again.Created != 0 || again.Existing != 1 {
		t.Fatalf("idempotent generation=%+v err=%v", again, err)
	}
	signals, err := service.ListSignals(ctx, p, "accra", "open", 100)
	if err != nil || len(signals) != 1 || len(signals[0].Evidence) != 1 || signals[0].Evidence[0].SourceID != "attendance-1" || signals[0].PersonID != "person-1" {
		t.Fatalf("signals=%+v err=%v", signals, err)
	}
	if len(evidence.audits) != 2 {
		t.Fatalf("generation audit missing: %+v", evidence.audits)
	}
	dashboard, err := service.CohortDashboard(ctx, p, "accra", now.AddDate(0, 0, -180), now)
	if err != nil || dashboard.MetricVersion != "retention-v1" || len(dashboard.Cohorts) != 1 || dashboard.Cohorts[0].CohortState != "suppressed" || dashboard.Cohorts[0].CohortSize != nil {
		t.Fatalf("privacy-safe cohort dashboard=%+v err=%v", dashboard, err)
	}
	checklist := SafetyChecklist{ReasonUnderstood: true, CaveatsVisible: true, NoDiagnosis: true, NoAutomaticAction: true, ConsentBoundaryClear: true, SparseDataProtected: true, NoFinancialInference: true, LanguageIsPastoral: true}
	if _, err = service.SubmitSafetyReview(ctx, p, SafetyReviewInput{BranchID: "accra", Role: "pastoral", Decision: "approved", Findings: "The named reviewer must submit this decision.", Checklist: checklist}, "wrong-reviewer"); err == nil {
		t.Fatal("unconfigured reviewer submitted pastoral UAT")
	}
	for _, review := range []struct {
		role  string
		actor platform.ID
	}{{"product", "owner-1"}, {"pastoral", "pastor-1"}, {"privacy", "privacy-1"}} {
		reviewer := p
		reviewer.Actor.ID = review.actor
		readiness, reviewErr := service.SubmitSafetyReview(ctx, reviewer, SafetyReviewInput{BranchID: "accra", Role: review.role, Decision: "approved", Findings: "Reviewed the exact policy version and confirmed every safety statement.", Checklist: checklist}, "uat-"+review.role)
		if reviewErr != nil {
			t.Fatalf("%s safety review: %v", review.role, reviewErr)
		}
		if review.role == "privacy" && readiness.ReleaseState != "approved" {
			t.Fatalf("release not approved after independent reviews: %+v", readiness)
		}
	}
	changedPolicySet := rule
	changedPolicySet.ID, changedPolicySet.Name = "rule-2", "Group connection evidence"
	changedPolicySet.Kind, changedPolicySet.Version = "first-visit-no-group", 1
	if err = repo.InsertRule(ctx, changedPolicySet); err != nil {
		t.Fatal(err)
	}
	staleReadiness, err := service.SafetyReadiness(ctx, p, "accra")
	if err != nil || staleReadiness.ReleaseState != "awaiting-human-review" || staleReadiness.RoleStatus["pastoral"] != "pending" {
		t.Fatalf("changed policy set reused stale approvals: %+v err=%v", staleReadiness, err)
	}
	if _, err = db.Collection("users").InsertOne(ctx, bson.M{"_id": "reviewer-1", "name": "Review Pastor", "email": "review@remi.test", "role": "super-admin", "invitationStatus": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Collection("users").InsertOne(ctx, bson.M{"_id": "reviewer-2", "name": "Second Reviewer", "email": "review2@remi.test", "role": "super-admin", "invitationStatus": "accepted"}); err != nil {
		t.Fatal(err)
	}
	consentEligible := false
	service.Consent = func(_ context.Context, _ platform.Principal, _, _ platform.ID, purpose, channel string) (ConsentDecision, error) {
		decision := "no-grant"
		if consentEligible {
			decision = "eligible"
		}
		return ConsentDecision{Eligible: consentEligible, Decision: decision, EvaluatedAt: now, ProjectionID: "consent-projection-1", ProjectionVersion: 3}, nil
	}
	detail, err := service.GetReviewDetail(ctx, p, signals[0].ID)
	if err != nil || detail.Person.DisplayName != "Ama Mensah" || len(detail.Assignees) != 2 {
		t.Fatalf("review detail=%+v err=%v", detail, err)
	}
	assigned, err := service.Assign(ctx, p, signals[0].ID, AssignInput{ExpectedVersion: 1, AssigneeID: "reviewer-1", Reason: "Review the observable attendance evidence."}, "assign-1")
	if err != nil || assigned.State != "in-review" || assigned.Version != 2 || assigned.AssigneeID != "reviewer-1" {
		t.Fatalf("assigned=%+v err=%v", assigned, err)
	}
	otherReviewer := p
	otherReviewer.Actor.ID = "reviewer-2"
	if _, err = service.Resolve(ctx, otherReviewer, assigned.ID, ResolveInput{ExpectedVersion: 2, Outcome: "no-action-needed", Reason: "An unrelated reviewer must not close this observation."}, "wrong-reviewer"); err == nil {
		t.Fatal("unassigned reviewer changed the observation")
	} else {
		var domain *platform.DomainError
		if !errors.As(err, &domain) || domain.Code != "forbidden" {
			t.Fatalf("wrong reviewer error=%v", err)
		}
	}
	if _, err = service.RecordContact(ctx, p, assigned.ID, ContactInput{ExpectedVersion: 2, Channel: "phone", Outcome: "connected", Summary: "Confirmed current context."}, "contact-denied"); err == nil {
		t.Fatal("contact recorded without current consent")
	}
	consentEligible = true
	contacted, err := service.RecordContact(ctx, p, assigned.ID, ContactInput{ExpectedVersion: 2, Channel: "phone", Outcome: "connected", Summary: "Confirmed current context."}, "contact-1")
	if err != nil || contacted.Version != 3 || contacted.State != "in-review" {
		t.Fatalf("contacted=%+v err=%v", contacted, err)
	}
	snoozed, err := service.Snooze(ctx, p, contacted.ID, SnoozeInput{ExpectedVersion: 3, Until: now.Add(48 * time.Hour), Reason: "Member requested a follow-up later this week."}, "snooze-1")
	if err != nil || snoozed.State != "snoozed" || snoozed.Version != 4 {
		t.Fatalf("snoozed=%+v err=%v", snoozed, err)
	}
	active, err := service.ListSignals(ctx, p, "accra", "active", 100)
	if err != nil || len(active) != 0 {
		t.Fatalf("future snooze leaked into active queue: %+v err=%v", active, err)
	}
	resolved, err := service.Resolve(ctx, p, snoozed.ID, ResolveInput{ExpectedVersion: 4, Outcome: "reconnected", Reason: "Human review completed with consent.", FalsePositive: false}, "resolve-1")
	if err != nil || resolved.State != "resolved" || resolved.Version != 5 || resolved.ResolutionOutcome != "reconnected" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	events, err := repo.ListReviewEvents(ctx, "org-1", resolved.ID)
	if err != nil || len(events) != 4 || events[1].ConsentDecision != "eligible" || events[1].ConsentProjectionID != "consent-projection-1" || events[1].ConsentProjectionVersion != 3 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	second := signals[0]
	second.ID, second.SourceKey, second.Version, second.State, second.AssigneeID = "signal-2", "source-2", 1, "open", ""
	if _, err = repo.InsertSignal(ctx, second); err != nil {
		t.Fatal(err)
	}
	secondAssigned, err := service.Assign(ctx, p, second.ID, AssignInput{ExpectedVersion: 1, AssigneeID: "reviewer-1", Reason: "Review possible source-data issue."}, "assign-2")
	if err != nil {
		t.Fatal(err)
	}
	suppressed, err := service.Suppress(ctx, p, second.ID, SuppressInput{ExpectedVersion: secondAssigned.Version, FeedbackCode: "false-positive", Reason: "Source attendance was entered against the wrong person."}, "suppress-2")
	if err != nil || suppressed.State != "suppressed" || suppressed.FalsePositive == nil || !*suppressed.FalsePositive {
		t.Fatalf("suppressed=%+v err=%v", suppressed, err)
	}
	third := signals[0]
	third.ID, third.SourceKey, third.Version, third.State, third.AssigneeID = "signal-3", "source-3", 1, "open", ""
	if _, err = repo.InsertSignal(ctx, third); err != nil {
		t.Fatal(err)
	}
	p2 := p
	p2.Actor.ID = "reviewer-2"
	type assignmentResult struct{ err error }
	results := make(chan assignmentResult, 2)
	var wg sync.WaitGroup
	for _, attempt := range []struct {
		principal platform.Principal
		assignee  platform.ID
	}{{p, "reviewer-1"}, {p2, "reviewer-2"}} {
		wg.Add(1)
		go func(value struct {
			principal platform.Principal
			assignee  platform.ID
		}) {
			defer wg.Done()
			_, assignErr := service.Assign(ctx, value.principal, third.ID, AssignInput{ExpectedVersion: 1, AssigneeID: value.assignee, Reason: "Concurrent ownership claim for review."}, "concurrent-assign")
			results <- assignmentResult{err: assignErr}
		}(attempt)
	}
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for result := range results {
		if result.err == nil {
			succeeded++
		} else {
			var domain *platform.DomainError
			if errors.As(result.err, &domain) && domain.Code == "version_conflict" {
				conflicted++
			}
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent assignment success=%d conflict=%d", succeeded, conflicted)
	}
	thirdEvents, err := repo.ListReviewEvents(ctx, "org-1", third.ID)
	if err != nil || len(thirdEvents) != 1 {
		t.Fatalf("concurrent review events=%+v err=%v", thirdEvents, err)
	}
	draft := rule
	draft.ID = "rule-draft"
	draft.Status = "draft"
	draft.Version = 1
	draft.PublishedAt = nil
	if err = repo.InsertRule(ctx, draft); err != nil {
		t.Fatal(err)
	}
	disabled := service
	disabled.Flags = platform.NewFeatureFlags(1, map[string]bool{"retention-individual-signals": false})
	if _, err = disabled.PublishRule(ctx, p, draft.ID, 1, draft.Approval, "publish-disabled"); err == nil {
		t.Fatal("rule published while activation flag was disabled")
	}
}

func TestDatasetQueryHasNoFinanceDependency(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(file), "repository.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(source))
	for _, forbidden := range []string{"chms_finance", "contribution", "pledge", "fundid", "giving"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("engagement dataset loader contains prohibited finance dependency %q", forbidden)
		}
	}
}
