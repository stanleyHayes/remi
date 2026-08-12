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

func volunteerPrincipal(fields ...platform.FieldClass) platform.Principal {
	grants := make([]platform.Grant, 0, 3)
	for _, action := range []string{"create", "read", "update"} {
		grants = append(grants, platform.Grant{Action: action, Resource: "volunteer", BranchIDs: []platform.ID{"accra"}, FieldClasses: fields})
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "coordinator-1"}, OrganizationID: "org-1", Roles: []string{"volunteer-coordinator"}, Grants: grants}
}

func TestVolunteerLifecycleAgainstMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_volunteer_test")
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
	principal := volunteerPrincipal(platform.FieldOperational, platform.FieldSensitiveMinistry)
	team, err := service.CreateVolunteerTeam(ctx, principal, VolunteerTeamInput{Name: "Hospitality", Description: "Welcome and hosting", HomeBranchID: "accra", LeaderPersonIDs: []platform.ID{"leader-1"}, Status: "active"}, "create-team")
	if err != nil || team.Version != 1 {
		t.Fatalf("create team=%+v err=%v", team, err)
	}
	team, err = service.UpdateVolunteerTeam(ctx, principal, team.ID, 1, VolunteerTeamInput{Name: "Hospitality & Welcome", Description: "Welcome and hosting", HomeBranchID: "accra", LeaderPersonIDs: []platform.ID{"leader-1"}, Status: "active"}, "update-team")
	if err != nil || team.Version != 2 {
		t.Fatalf("update team=%+v err=%v", team, err)
	}
	position, err := service.CreateVolunteerPosition(ctx, principal, team.ID, VolunteerPositionInput{Name: "Children host", Description: "Welcome families", Status: "active", Eligibility: PositionEligibility{MinimumAgeYears: 18, RequiredSkills: []string{"Hospitality", "hospitality"}, BackgroundCheckRequired: true, BackgroundCheckMaxAgeDays: 365, SafeguardingTrainingRequired: true}}, "create-position")
	if err != nil || len(position.Eligibility.RequiredSkills) != 1 {
		t.Fatalf("create position=%+v err=%v", position, err)
	}
	position, err = service.UpdateVolunteerPosition(ctx, principal, team.ID, position.ID, 1, VolunteerPositionInput{Name: "Children welcome host", Description: "Welcome families and coordinate check-in", Status: "active", Eligibility: PositionEligibility{MinimumAgeYears: 18, RequiredSkills: []string{"hospitality"}, BackgroundCheckRequired: true, BackgroundCheckMaxAgeDays: 365, SafeguardingTrainingRequired: true}}, "update-position")
	if err != nil || position.Version != 2 {
		t.Fatalf("update position=%+v err=%v", position, err)
	}
	checkedAt, expiresAt := now.Add(-24*time.Hour), now.Add(364*24*time.Hour)
	profile, err := service.PutVolunteerProfile(ctx, principal, "person-2", 0, VolunteerProfileInput{Skills: []string{"Hospitality", "First Aid"}, PreferredTeamIDs: []platform.ID{team.ID}, PreferredPositionIDs: []platform.ID{position.ID}, Status: "active", Eligibility: ScreeningMetadata{BackgroundCheckStatus: "cleared", BackgroundCheckedAt: &checkedAt, BackgroundCheckExpiresAt: &expiresAt, BackgroundCheckReference: "CHECK-REF-104"}}, "put-profile")
	if err != nil || profile.Version != 1 || len(profile.Skills) != 2 {
		t.Fatalf("put profile=%+v err=%v", profile, err)
	}
	if _, err = service.GetVolunteerProfile(ctx, volunteerPrincipal(platform.FieldOperational), "person-2"); err == nil {
		t.Fatal("operational-only reader accessed restricted screening metadata")
	}
	availability, err := service.AddAvailability(ctx, principal, "person-2", AvailabilityInput{StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(3 * 24 * time.Hour), State: "preferred", Source: "member"}, "availability-request-1", "availability-key-1")
	if err != nil || availability.State != "preferred" {
		t.Fatalf("availability=%+v err=%v", availability, err)
	}
	replayed, err := service.AddAvailability(ctx, principal, "person-2", AvailabilityInput{StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(3 * 24 * time.Hour), State: "preferred", Source: "member"}, "availability-request-2", "availability-key-1")
	if err != nil || replayed.ID != availability.ID {
		t.Fatalf("availability replay=%+v err=%v", replayed, err)
	}
	if _, err = service.AddAvailability(ctx, principal, "person-2", AvailabilityInput{StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(4 * 24 * time.Hour), State: "unavailable", Source: "member"}, "availability-request-3", "availability-key-1"); err == nil {
		t.Fatal("changed availability reused an idempotency key")
	}
	windows, err := service.ListAvailability(ctx, principal, "person-2", now, now.Add(7*24*time.Hour))
	if err != nil || len(windows) != 1 {
		t.Fatalf("availability list=%+v err=%v", windows, err)
	}
	if len(evidence.audits) != 6 || len(evidence.events) != 6 {
		t.Fatalf("evidence audits=%d events=%d", len(evidence.audits), len(evidence.events))
	}
}

func TestClearedScreeningRequiresBoundedMetadata(t *testing.T) {
	input := VolunteerProfileInput{Status: "active", Eligibility: ScreeningMetadata{BackgroundCheckStatus: "cleared"}}
	if err := input.normalize(); err == nil {
		t.Fatal("cleared screening accepted without dates and reference")
	}
}
