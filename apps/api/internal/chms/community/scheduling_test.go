package community

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type schedulingPeople struct{}

func (schedulingPeople) ResolvePersonReference(_ context.Context, organizationID, personID platform.ID) (platform.ID, bool, error) {
	return "accra", organizationID == "org-1" && (personID == "person-2" || personID == "person-3"), nil
}

func (schedulingPeople) ResolveVolunteerEligibility(_ context.Context, organizationID, personID platform.ID) (platform.ID, bool, string, string, string, error) {
	return "accra", organizationID == "org-1" && (personID == "person-2" || personID == "person-3"), "member", "1990-01-01", "day", nil
}

type schedulingOccurrences struct {
	startsAt time.Time
	endsAt   time.Time
}

func (o schedulingOccurrences) ResolveOccurrenceReference(_ context.Context, organizationID, occurrenceID platform.ID) (platform.ID, time.Time, time.Time, string, error) {
	if organizationID != "org-1" || occurrenceID != "occurrence-1" {
		return "", time.Time{}, time.Time{}, "", nil
	}
	return "accra", o.startsAt, o.endsAt, "scheduled", nil
}

func schedulingPrincipal() platform.Principal {
	grants := make([]platform.Grant, 0, 4)
	for _, action := range []string{"create", "read", "update", "approve"} {
		grants = append(grants, platform.Grant{Action: action, Resource: "volunteer", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldSensitiveMinistry}})
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "scheduler-1"}, OrganizationID: "org-1", Roles: []string{"volunteer-coordinator"}, Grants: grants}
}

func TestSchedulingLifecycleConflictsAndConcurrencyAgainstMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_scheduling_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	startsAt, endsAt := now.Add(5*24*time.Hour), now.Add(5*24*time.Hour+2*time.Hour)
	evidence := &integrationPlatform{}
	people := schedulingPeople{}
	service := Service{Store: repo, Platform: evidence, People: people, VolunteerPeople: people, Occurrences: schedulingOccurrences{startsAt: startsAt, endsAt: endsAt}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := schedulingPrincipal()

	team, err := service.CreateVolunteerTeam(ctx, principal, VolunteerTeamInput{Name: "Hospitality", HomeBranchID: "accra", Status: "active"}, "team")
	if err != nil {
		t.Fatal(err)
	}
	position, err := service.CreateVolunteerPosition(ctx, principal, team.ID, VolunteerPositionInput{Name: "Welcome host", Status: "active", Eligibility: PositionEligibility{MinimumAgeYears: 18, MembershipStages: []string{"member"}, RequiredSkills: []string{"hospitality"}}}, "position")
	if err != nil {
		t.Fatal(err)
	}
	for _, personID := range []platform.ID{"person-2", "person-3"} {
		if _, err = service.PutVolunteerProfile(ctx, principal, personID, 0, VolunteerProfileInput{Skills: []string{"hospitality"}, Status: "active", Eligibility: ScreeningMetadata{BackgroundCheckStatus: "not-required"}}, "profile-"+string(personID)); err != nil {
			t.Fatal(err)
		}
	}
	rotation, err := service.CreateVolunteerRotation(ctx, principal, VolunteerRotationInput{TeamID: team.ID, PositionID: position.ID, Name: "Every other Sunday", PersonIDs: []platform.ID{"person-2", "person-3"}, CadenceWeeks: 2, AnchorDate: "2026-08-16", Status: "active"}, "rotation")
	if err != nil || rotation.Version != 1 {
		t.Fatalf("rotation=%+v err=%v", rotation, err)
	}

	plan, err := service.CreateServicePlan(ctx, principal, ServicePlanInput{OccurrenceID: "occurrence-1", Name: "Sunday celebration", Needs: []PlanNeed{{PositionID: position.ID, Slots: 2}}}, "plan")
	if err != nil || plan.Status != "draft" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if _, err = service.CreateServicePlan(ctx, principal, ServicePlanInput{OccurrenceID: "occurrence-1", Name: "Duplicate", Needs: []PlanNeed{{PositionID: position.ID, Slots: 1}}}, "duplicate-plan"); err == nil {
		t.Fatal("duplicate occurrence plan was accepted")
	}
	assignment, err := service.CreateAssignment(ctx, principal, plan.ID, AssignmentInput{PositionID: position.ID, PersonID: "person-2", Slot: 1}, "assignment")
	if err != nil || assignment.Status != "invited" {
		t.Fatalf("assignment=%+v err=%v", assignment, err)
	}
	memberPrincipal := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "person-2"}, OrganizationID: "org-1"}
	own, err := service.ListOwnAssignments(ctx, memberPrincipal, now, now.Add(30*24*time.Hour))
	if err != nil || len(own) != 1 || own[0].PersonID != "person-2" {
		t.Fatalf("member assignments=%+v err=%v", own, err)
	}
	if _, err = service.ListOwnAssignments(ctx, principal, now, now.Add(30*24*time.Hour)); err == nil {
		t.Fatal("staff principal entered member-only assignment surface")
	}
	preview, err := service.PreviewAssignment(ctx, principal, plan.ID, AssignmentInput{PositionID: position.ID, PersonID: "person-2", Slot: 2})
	if err != nil || preview.Eligible || len(preview.Conflicts) != 1 || preview.Conflicts[0].Code != "schedule-overlap" {
		t.Fatalf("overlap preview=%+v err=%v", preview, err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, personID := range []platform.ID{"person-2", "person-3"} {
		wg.Add(1)
		go func(personID platform.ID) {
			defer wg.Done()
			_, createErr := service.CreateAssignment(ctx, principal, plan.ID, AssignmentInput{PositionID: position.ID, PersonID: personID, Slot: 2, OverrideReason: "Coordinator approved coverage"}, "slot-race-"+string(personID))
			results <- createErr
		}(personID)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for createErr := range results {
		if createErr == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("unique serving slot admitted %d concurrent assignments", succeeded)
	}

	assignment, err = service.RespondAssignment(ctx, memberPrincipal, assignment.ID, AssignmentResponseInput{ExpectedVersion: 1, Response: "accepted"}, "member-accept")
	if err != nil || assignment.Status != "accepted" || assignment.Version != 2 {
		t.Fatalf("accept=%+v err=%v", assignment, err)
	}
	if _, err = service.RespondAssignment(ctx, principal, assignment.ID, AssignmentResponseInput{ExpectedVersion: 1, Response: "declined", ReasonCode: "away"}, "stale-response"); err == nil {
		t.Fatal("stale assignment version was accepted")
	}
	assignment, err = service.RecordAssignmentReminder(ctx, principal, assignment.ID, ReminderInput{ExpectedVersion: 2}, "reminder")
	if err != nil || assignment.Reminder.SentCount != 1 || assignment.Version != 3 {
		t.Fatalf("reminder=%+v err=%v", assignment, err)
	}
	now = endsAt.Add(time.Hour)
	assignment, err = service.MarkAssignmentNoShow(ctx, principal, assignment.ID, NoShowInput{ExpectedVersion: 3, Reason: "Volunteer did not arrive"}, "no-show")
	if err != nil || assignment.Status != "no-show" || assignment.Version != 4 {
		t.Fatalf("no-show=%+v err=%v", assignment, err)
	}
	events, err := service.ListAssignmentEvents(ctx, principal, assignment.ID)
	if err != nil || len(events) != 4 || events[0].ToStatus != "invited" || events[3].ToStatus != "no-show" {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	allAssignments, err := service.ListAssignments(ctx, principal, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	var secondSlot VolunteerAssignment
	for _, candidate := range allAssignments {
		if candidate.Slot == 2 {
			secondSlot = candidate
			break
		}
	}
	if !secondSlot.ID.Valid() {
		t.Fatal("concurrent slot winner was not persisted")
	}
	substitutePersonID := platform.ID("person-2")
	if secondSlot.PersonID == substitutePersonID {
		substitutePersonID = "person-3"
	}
	replacement, err := service.SubstituteAssignment(ctx, principal, secondSlot.ID, SubstituteInput{ExpectedVersion: secondSlot.Version, SubstitutePersonID: substitutePersonID, Reason: "Roster coverage changed", OverrideReason: "Coordinator approved coverage"}, "substitute")
	if err != nil || replacement.SubstitutesAssignmentID != secondSlot.ID || replacement.Status != "invited" {
		t.Fatalf("replacement=%+v err=%v", replacement, err)
	}
	secondSlotEvents, err := service.ListAssignmentEvents(ctx, principal, secondSlot.ID)
	if err != nil || len(secondSlotEvents) != 2 || secondSlotEvents[1].ToStatus != "replaced" {
		t.Fatalf("substitution history=%+v err=%v", secondSlotEvents, err)
	}

	plan, err = service.TransitionServicePlan(ctx, principal, plan.ID, 1, ServicePlanTransition{Status: "published"}, "publish")
	if err != nil || plan.Status != "published" {
		t.Fatalf("publish=%+v err=%v", plan, err)
	}
	plan, err = service.TransitionServicePlan(ctx, principal, plan.ID, 2, ServicePlanTransition{Status: "locked"}, "lock")
	if err != nil || plan.Status != "locked" {
		t.Fatalf("lock=%+v err=%v", plan, err)
	}
	plan, err = service.TransitionServicePlan(ctx, principal, plan.ID, 3, ServicePlanTransition{Status: "completed", Reason: "Service concluded"}, "complete")
	if err != nil || plan.Status != "completed" {
		t.Fatalf("complete=%+v err=%v", plan, err)
	}
}
