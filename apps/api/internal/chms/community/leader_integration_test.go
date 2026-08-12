package community

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

func TestLeaderWorkspaceIsStrictlyOwnershipScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_leader_workspace_test")
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
	envelope := func(id, branch string) platform.ResourceEnvelope {
		return platform.ResourceEnvelope{ID: platform.ID(id), OrganizationID: "org-1", BranchID: platform.ID(branch), Version: 1, CreatedAt: now, UpdatedAt: now}
	}
	owned := Group{ResourceEnvelope: envelope("group-owned", "accra"), Name: "Young Adults", HomeBranchID: "accra", LeaderPersonIDs: []platform.ID{"leader-1"}, Status: "active"}
	unowned := Group{ResourceEnvelope: envelope("group-unowned", "accra"), Name: "Families", HomeBranchID: "accra", LeaderPersonIDs: []platform.ID{"leader-2"}, Status: "active"}
	if err = repo.InsertGroup(ctx, owned); err != nil {
		t.Fatal(err)
	}
	if err = repo.InsertGroup(ctx, unowned); err != nil {
		t.Fatal(err)
	}
	for _, member := range []GroupMembership{
		{ResourceEnvelope: envelope("membership-active", "accra"), GroupID: owned.ID, PersonID: "person-a", Status: "active"},
		{ResourceEnvelope: envelope("membership-wait", "accra"), GroupID: owned.ID, PersonID: "person-b", Status: "waitlisted"},
		{ResourceEnvelope: envelope("membership-hidden", "accra"), GroupID: unowned.ID, PersonID: "person-c", Status: "active", LeaderNote: "must never leak"},
	} {
		if err = repo.InsertMembership(ctx, member); err != nil {
			t.Fatal(err)
		}
	}
	meeting := GroupMeeting{ResourceEnvelope: envelope("meeting-owned", "accra"), GroupID: owned.ID, Topic: "Community night", StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(26 * time.Hour), Timezone: "Africa/Accra", Status: "scheduled"}
	if err = repo.InsertMeeting(ctx, meeting); err != nil {
		t.Fatal(err)
	}
	team := VolunteerTeam{ResourceEnvelope: envelope("team-owned", "accra"), Name: "Welcome", HomeBranchID: "accra", LeaderPersonIDs: []platform.ID{"leader-1"}, Status: "active"}
	hiddenTeam := VolunteerTeam{ResourceEnvelope: envelope("team-unowned", "accra"), Name: "Media", HomeBranchID: "accra", LeaderPersonIDs: []platform.ID{"leader-2"}, Status: "active"}
	if err = repo.InsertVolunteerTeam(ctx, team); err != nil {
		t.Fatal(err)
	}
	if err = repo.InsertVolunteerTeam(ctx, hiddenTeam); err != nil {
		t.Fatal(err)
	}
	position := VolunteerPosition{ResourceEnvelope: envelope("position-1", "accra"), TeamID: team.ID, Name: "Door host", Status: "active"}
	if err = repo.InsertVolunteerPosition(ctx, position); err != nil {
		t.Fatal(err)
	}
	ownedAssignment := VolunteerAssignment{ResourceEnvelope: envelope("assignment-owned", "accra"), PlanID: "plan-1", OccurrenceID: "occurrence-1", TeamID: team.ID, PositionID: position.ID, PersonID: "person-a", Slot: 1, StartsAt: now.Add(72 * time.Hour), EndsAt: now.Add(74 * time.Hour), Status: "invited"}
	hiddenAssignment := VolunteerAssignment{ResourceEnvelope: envelope("assignment-hidden", "accra"), PlanID: "plan-1", OccurrenceID: "occurrence-1", TeamID: hiddenTeam.ID, PositionID: "hidden-position", PersonID: "person-c", Slot: 1, StartsAt: now.Add(72 * time.Hour), EndsAt: now.Add(74 * time.Hour), Status: "invited"}
	if err = repo.InsertAssignment(ctx, ownedAssignment); err != nil {
		t.Fatal(err)
	}
	if err = repo.InsertAssignment(ctx, hiddenAssignment); err != nil {
		t.Fatal(err)
	}
	evidence := &integrationPlatform{}
	service := Service{Store: repo, Platform: evidence, People: integrationPeople{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	leader := platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: "leader-1"}, OrganizationID: "org-1", Roles: []string{"member"}}
	workspace, err := service.GetLeaderWorkspace(ctx, leader, now, now.Add(30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Groups) != 1 || workspace.Groups[0].Group.ID != owned.ID || workspace.Groups[0].ActiveMembers != 1 || workspace.Groups[0].PendingMembers != 1 || len(workspace.Groups[0].UpcomingMeetings) != 1 {
		t.Fatalf("bad owned group projection: %+v", workspace.Groups)
	}
	if len(workspace.Teams) != 1 || workspace.Teams[0].Team.ID != team.ID || len(workspace.Teams[0].Positions) != 1 {
		t.Fatalf("bad owned team projection: %+v", workspace.Teams)
	}
	if members, listErr := service.ListMemberships(ctx, leader, owned.ID, false); listErr != nil || len(members) != 2 {
		t.Fatalf("owned roster unavailable values=%+v err=%v", members, listErr)
	}
	if _, createErr := service.CreateMeeting(ctx, leader, owned.ID, MeetingInput{Topic: "Leader follow-up", StartsAt: now.Add(48 * time.Hour), EndsAt: now.Add(50 * time.Hour), Timezone: "Africa/Accra"}, "leader-meeting"); createErr != nil {
		t.Fatalf("owned meeting action denied: %v", createErr)
	}
	handoff, handoffErr := service.RequestGroupCommunication(ctx, leader, owned.ID, GroupCommunicationInput{Purpose: "group-operations", Channel: "whatsapp", Message: "Community night begins at 6:30 PM."}, "leader-message")
	if handoffErr != nil || handoff.State != "pending-consent-review" || len(handoff.AudiencePersonIDs) != 1 || handoff.AudiencePersonIDs[0] != "person-a" {
		t.Fatalf("bad consent handoff value=%+v err=%v", handoff, handoffErr)
	}
	if len(evidence.audits) != 2 || len(evidence.events) != 2 {
		t.Fatalf("missing communication evidence audits=%+v events=%+v", evidence.audits, evidence.events)
	}
	payload, _ := evidence.events[1].Payload.(map[string]any)
	if payload["audienceCount"] != 1 {
		t.Fatalf("unsafe communication payload: %+v", payload)
	}
	assignments, listErr := service.ListOwnedLeaderTeamAssignments(ctx, leader, team.ID, now, now.Add(30*24*time.Hour))
	if listErr != nil || len(assignments) != 1 || assignments[0].ID != ownedAssignment.ID {
		t.Fatalf("owned team schedule leaked or omitted assignments values=%+v err=%v", assignments, listErr)
	}
	if _, err = service.ListOwnedLeaderTeamAssignments(ctx, leader, hiddenTeam.ID, now, now.Add(30*24*time.Hour)); err == nil {
		t.Fatal("leader listed an unowned team schedule")
	}
	reminded, remindErr := service.RecordAssignmentReminder(ctx, leader, ownedAssignment.ID, ReminderInput{ExpectedVersion: 1}, "leader-reminder")
	if remindErr != nil || reminded.Reminder.SentCount != 1 {
		t.Fatalf("owned assignment reminder denied value=%+v err=%v", reminded, remindErr)
	}
	if _, err = service.RecordAssignmentReminder(ctx, leader, hiddenAssignment.ID, ReminderInput{ExpectedVersion: 1}, "forbidden-reminder"); err == nil {
		t.Fatal("leader changed an unowned team assignment")
	}
	if _, err = service.GetGroup(ctx, leader, unowned.ID); err == nil {
		t.Fatal("leader read an unowned group")
	}
	if _, err = service.CreateMeeting(ctx, leader, unowned.ID, MeetingInput{Topic: "Forbidden meeting", StartsAt: now.Add(48 * time.Hour), EndsAt: now.Add(50 * time.Hour), Timezone: "Africa/Accra"}, "forbidden-meeting"); err == nil {
		t.Fatal("leader changed an unowned group")
	}
	if _, err = service.RequestGroupCommunication(ctx, leader, unowned.ID, GroupCommunicationInput{Purpose: "group-operations", Channel: "email", Message: "This should never be queued."}, "forbidden-message"); err == nil {
		t.Fatal("leader messaged an unowned group")
	}
	if _, err = service.GetVolunteerTeam(ctx, leader, hiddenTeam.ID); err == nil {
		t.Fatal("leader read an unowned team")
	}
	admin := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "admin-1"}, OrganizationID: "org-1", Roles: []string{"super-admin"}, Grants: []platform.Grant{{Action: "read", Resource: "group", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}, {Action: "read", Resource: "volunteer", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}}}
	analytics, analyticsErr := service.CommunityAnalytics(ctx, admin, "accra", now, now.Add(30*24*time.Hour))
	if analyticsErr != nil || analytics.Summary.ActiveGroups != 2 || analytics.Summary.ActiveVolunteerTeams != 2 || analytics.MetricVersion != "community-ops-v1" {
		t.Fatalf("bad community analytics value=%+v err=%v", analytics, analyticsErr)
	}
	if _, err = service.CommunityAnalytics(ctx, leader, "accra", now, now.Add(30*24*time.Hour)); err == nil {
		t.Fatal("owned-unit leader gained branch-wide aggregate analytics")
	}
	staff := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "staff-1"}, OrganizationID: "org-1"}
	if _, err = service.GetLeaderWorkspace(ctx, staff, now, now.Add(24*time.Hour)); err == nil {
		t.Fatal("unlinked staff identity entered member-owned leader workspace")
	}
}
