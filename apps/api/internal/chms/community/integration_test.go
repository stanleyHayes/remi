package community

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type integrationPlatform struct {
	audits []platform.AuditEvent
	events []platform.OutboxRecord
}

func (p *integrationPlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (p *integrationPlatform) AppendAudit(_ context.Context, value platform.AuditEvent) error {
	p.audits = append(p.audits, value)
	return nil
}
func (p *integrationPlatform) EnqueueEvent(_ context.Context, value platform.OutboxRecord) error {
	p.events = append(p.events, value)
	return nil
}

type integrationPeople struct{}

func (integrationPeople) ResolvePersonReference(_ context.Context, org, id platform.ID) (platform.ID, bool, error) {
	return "accra", org == "org-1" && (id == "leader-1" || id == "person-2"), nil
}

func TestGroupLifecycleAgainstMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_community_test")
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
	evidence := &integrationPlatform{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	service := Service{Store: repo, Platform: evidence, People: integrationPeople{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := groupPrincipal()
	input := GroupInput{HomeBranchID: "accra", Name: "Young Adults", Type: "community", LeaderPersonIDs: []platform.ID{"leader-1"}, Capacity: 1, Privacy: "request", Discoverability: "members", Status: "draft", MeetingPattern: &MeetingPattern{Frequency: "weekly", Weekday: "friday", LocalStart: "18:30", DurationMinutes: 90, Timezone: "Africa/Accra", Location: "Fellowship hall"}}
	created, err := service.CreateGroup(ctx, principal, input, "create-request")
	if err != nil || created.Version != 1 {
		t.Fatalf("create value=%+v err=%v", created, err)
	}
	input.Status = "active"
	updated, err := service.UpdateGroup(ctx, principal, created.ID, 1, input, "activate-request")
	if err != nil || updated.Status != "active" || updated.Version != 2 {
		t.Fatalf("activate value=%+v err=%v", updated, err)
	}
	firstMember, err := service.CreateMembership(ctx, principal, created.ID, MembershipInput{PersonID: "leader-1", Mode: "add", Role: "facilitator", Source: "staff"}, "add-first-member")
	if err != nil || firstMember.Status != "active" {
		t.Fatalf("first member value=%+v err=%v", firstMember, err)
	}
	secondMember, err := service.CreateMembership(ctx, principal, created.ID, MembershipInput{PersonID: "person-2", Mode: "add", Role: "member", Source: "staff"}, "add-second-member")
	if err != nil || secondMember.Status != "waitlisted" {
		t.Fatalf("capacity waitlist value=%+v err=%v", secondMember, err)
	}
	firstMember, err = service.TransitionMembership(ctx, principal, created.ID, firstMember.ID, 1, MembershipTransition{Status: "ended", Role: firstMember.Role, Reason: "Moved to another group"}, "end-first-member")
	if err != nil || firstMember.Status != "ended" {
		t.Fatalf("end first member value=%+v err=%v", firstMember, err)
	}
	secondMember, err = service.TransitionMembership(ctx, principal, created.ID, secondMember.ID, 1, MembershipTransition{Status: "active", Role: secondMember.Role}, "activate-waitlist")
	if err != nil || secondMember.Status != "active" {
		t.Fatalf("activate waitlist value=%+v err=%v", secondMember, err)
	}
	meeting, err := service.CreateMeeting(ctx, principal, created.ID, MeetingInput{Topic: "Community dinner", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), Timezone: "Africa/Accra", Location: "Fellowship hall"}, "create-meeting")
	if err != nil {
		t.Fatalf("create meeting value=%+v err=%v", meeting, err)
	}
	attendance, err := service.RecordMeetingAttendance(ctx, principal, created.ID, meeting.ID, "person-2", 0, MeetingAttendanceInput{Status: "present", Confidence: 100}, "record-meeting-attendance")
	if err != nil || attendance.Version != 1 {
		t.Fatalf("meeting attendance value=%+v err=%v", attendance, err)
	}
	roster, err := service.ListMemberships(ctx, principal, created.ID, true)
	if err != nil || len(roster) != 2 {
		t.Fatalf("roster values=%+v err=%v", roster, err)
	}
	history, err := service.ListMembershipHistory(ctx, principal, created.ID, "person-2")
	if err != nil || len(history) != 2 || history[0].ToStatus != "waitlisted" || history[1].ToStatus != "active" {
		t.Fatalf("membership history values=%+v err=%v", history, err)
	}
	input.Status = "paused"
	input.Reason = "Leader sabbatical"
	paused, err := service.UpdateGroup(ctx, principal, created.ID, 2, input, "pause-request")
	if err != nil || paused.Status != "paused" {
		t.Fatalf("pause value=%+v err=%v", paused, err)
	}
	input.Status = "closed"
	input.Reason = "Ministry season completed"
	closed, err := service.UpdateGroup(ctx, principal, created.ID, 3, input, "close-request")
	if err != nil || closed.ClosedAt == nil || closed.Version != 4 {
		t.Fatalf("close value=%+v err=%v", closed, err)
	}
	input.Status = "active"
	input.Reason = ""
	if _, err = service.UpdateGroup(ctx, principal, created.ID, 4, input, "reopen-request"); err == nil {
		t.Fatal("closed group reopened")
	}
	items, err := service.ListGroups(ctx, principal, "accra", false)
	if err != nil || len(items) != 0 {
		t.Fatalf("closed filter values=%+v err=%v", items, err)
	}
	items, err = service.ListGroups(ctx, principal, "accra", true)
	if err != nil || len(items) != 1 {
		t.Fatalf("closed list values=%+v err=%v", items, err)
	}
	if len(evidence.audits) != 10 || len(evidence.events) != 10 {
		t.Fatalf("evidence audits=%d events=%d", len(evidence.audits), len(evidence.events))
	}
}

type concurrencyPlatform struct{}

func (concurrencyPlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (concurrencyPlatform) AppendAudit(context.Context, platform.AuditEvent) error    { return nil }
func (concurrencyPlatform) EnqueueEvent(context.Context, platform.OutboxRecord) error { return nil }

type concurrencyPeople struct{}

func (concurrencyPeople) ResolvePersonReference(_ context.Context, org, id platform.ID) (platform.ID, bool, error) {
	return "accra", org == "org-1" && id.Valid(), nil
}

func TestConcurrentRosterAddsNeverExceedCapacity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_community_capacity_test")
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
	service := Service{Store: repo, Platform: concurrencyPlatform{}, People: concurrencyPeople{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := groupPrincipal()
	group, err := service.CreateGroup(ctx, principal, GroupInput{HomeBranchID: "accra", Name: "Capacity race", Type: "community", Capacity: 5, Privacy: "request", Discoverability: "members", Status: "active"}, "create-capacity-group")
	if err != nil {
		t.Fatal(err)
	}
	const attempts = 24
	results := make(chan *GroupMembership, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			value, createErr := service.CreateMembership(ctx, principal, group.ID, MembershipInput{PersonID: platform.ID(fmt.Sprintf("person-%02d", index)), Mode: "add", Role: "member", Source: "staff"}, fmt.Sprintf("capacity-%02d", index))
			if createErr != nil {
				errs <- createErr
				return
			}
			results <- value
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for createErr := range errs {
		t.Fatalf("concurrent add failed: %v", createErr)
	}
	active, waitlisted := 0, 0
	for value := range results {
		switch value.Status {
		case "active":
			active++
		case "waitlisted":
			waitlisted++
		default:
			t.Fatalf("unexpected concurrent membership status %q", value.Status)
		}
	}
	if active != 5 || waitlisted != attempts-5 {
		t.Fatalf("capacity exceeded: active=%d waitlisted=%d", active, waitlisted)
	}
	stored, err := repo.FindGroup(ctx, principal.OrganizationID, group.ID)
	if err != nil || stored == nil || stored.ActiveMemberCount != 5 {
		t.Fatalf("seat projection=%+v err=%v", stored, err)
	}
}
func groupPrincipal() platform.Principal {
	grants := []platform.Grant{}
	for _, action := range []string{"create", "read", "update"} {
		grants = append(grants, platform.Grant{Action: action, Resource: "group", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}})
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "staff-1"}, OrganizationID: "org-1", Roles: []string{"group-admin"}, Grants: grants}
}

func TestGroupMinistryScopeFiltersCreateReadListAndUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_community_ministry_scope_test")
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = db.Drop(cleanup)
		_ = client.Disconnect(cleanup)
	})
	repo, _ := NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	service := Service{Store: repo, Platform: &integrationPlatform{}, People: integrationPeople{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	all := groupPrincipal()
	youthInput := GroupInput{HomeBranchID: "accra", MinistryID: "youth", Name: "Youth Circle", Type: "youth", Privacy: "request", Discoverability: "members", Status: "active"}
	worshipInput := GroupInput{HomeBranchID: "accra", MinistryID: "worship", Name: "Worship Team", Type: "ministry", Privacy: "request", Discoverability: "members", Status: "active"}
	youth, err := service.CreateGroup(ctx, all, youthInput, "create-youth")
	if err != nil {
		t.Fatal(err)
	}
	worship, err := service.CreateGroup(ctx, all, worshipInput, "create-worship")
	if err != nil {
		t.Fatal(err)
	}

	scoped := groupPrincipal()
	for index := range scoped.Grants {
		scoped.Grants[index].MinistryIDs = []platform.ID{"youth"}
	}
	items, err := service.ListGroups(ctx, scoped, "accra", false)
	if err != nil || len(items) != 1 || items[0].ID != youth.ID {
		t.Fatalf("scoped list=%+v err=%v", items, err)
	}
	if _, err = service.GetGroup(ctx, scoped, worship.ID); err == nil {
		t.Fatal("cross-ministry group was readable")
	}
	if _, err = service.CreateGroup(ctx, scoped, worshipInput, "cross-ministry-create"); err == nil {
		t.Fatal("cross-ministry group was created")
	}
	worshipInput.Name = "Changed by youth operator"
	if _, err = service.UpdateGroup(ctx, scoped, worship.ID, worship.Version, worshipInput, "cross-ministry-update"); err == nil {
		t.Fatal("cross-ministry group was updated")
	}
	if _, err = service.GetGroup(ctx, scoped, youth.ID); err != nil {
		t.Fatalf("own ministry group unavailable: %v", err)
	}
}
