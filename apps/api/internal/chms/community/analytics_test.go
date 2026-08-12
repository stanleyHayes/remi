package community

import (
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func TestCompileCommunityAnalyticsUsesHonestDenominatorsAndSuppressesSmallRates(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	envelope := func(id string) platform.ResourceEnvelope {
		return platform.ResourceEnvelope{ID: platform.ID(id), OrganizationID: "org-1", BranchID: "accra", Version: 1, UpdatedAt: now.Add(-time.Hour)}
	}
	data := CommunityAnalyticsDataset{
		Groups: []Group{{ResourceEnvelope: envelope("group-1"), Name: "Community", Type: "community", HomeBranchID: "accra", Capacity: 10, Status: "active"}},
		Memberships: []GroupMembership{
			{ResourceEnvelope: envelope("membership-1"), GroupID: "group-1", PersonID: "person-1", Status: "active"},
			{ResourceEnvelope: envelope("membership-2"), GroupID: "group-1", PersonID: "person-2", Status: "active"},
			{ResourceEnvelope: envelope("membership-3"), GroupID: "group-1", PersonID: "person-3", Status: "waitlisted"},
		},
		Meetings: []GroupMeeting{{ResourceEnvelope: envelope("meeting-1"), GroupID: "group-1", StartsAt: now.Add(-3 * time.Hour), EndsAt: now.Add(-2 * time.Hour), Status: "scheduled"}},
		Attendance: []GroupMeetingAttendance{
			{ResourceEnvelope: envelope("mark-1"), GroupID: "group-1", MeetingID: "meeting-1", PersonID: "person-1", Status: "present"},
			{ResourceEnvelope: envelope("mark-2"), GroupID: "group-1", MeetingID: "meeting-1", PersonID: "person-2", Status: "absent"},
		},
		Teams:     []VolunteerTeam{{ResourceEnvelope: envelope("team-1"), Name: "Welcome", HomeBranchID: "accra", Status: "active"}},
		Positions: []VolunteerPosition{{ResourceEnvelope: envelope("position-1"), TeamID: "team-1", Name: "Host", Status: "active"}},
		Plans:     []ServicePlan{{ResourceEnvelope: envelope("plan-1"), StartsAt: now.Add(-3 * time.Hour), EndsAt: now.Add(-2 * time.Hour), Status: "completed", Needs: []PlanNeed{{PositionID: "position-1", Slots: 8}}}},
		Assignments: []VolunteerAssignment{
			{ResourceEnvelope: envelope("assignment-1"), TeamID: "team-1", PositionID: "position-1", PersonID: "person-1", Status: "completed"},
			{ResourceEnvelope: envelope("assignment-2"), TeamID: "team-1", PositionID: "position-1", PersonID: "person-2", Status: "no-show"},
			{ResourceEnvelope: envelope("assignment-3"), TeamID: "team-1", PositionID: "position-1", PersonID: "person-3", Status: "accepted"},
			{ResourceEnvelope: envelope("assignment-4"), TeamID: "team-1", PositionID: "position-1", PersonID: "person-4", Status: "declined"},
			{ResourceEnvelope: envelope("assignment-5"), TeamID: "team-1", PositionID: "position-1", PersonID: "person-5", Status: "invited"},
		},
	}
	result := compileCommunityAnalytics("accra", now.Add(-24*time.Hour), now.Add(time.Hour), data, now)
	if result.Summary.ActiveGroupMemberships != 2 || result.Summary.UniqueConnectedPeople != 2 || result.Groups[0].CurrentCapacityPercent == nil || *result.Groups[0].CurrentCapacityPercent != 20 {
		t.Fatalf("bad group summary: %+v", result)
	}
	if result.Groups[0].RecordedAttendanceRate != nil || !result.Groups[0].Suppressed {
		t.Fatalf("small group attendance rate was not suppressed: %+v", result.Groups[0])
	}
	team := result.VolunteerTeams[0]
	if team.PlannedSlots != 8 || team.FilledSlots != 4 || team.FillRate == nil || *team.FillRate != 50 || team.ResponseRate == nil || *team.ResponseRate != 80 {
		t.Fatalf("bad volunteer denominator handling: %+v", team)
	}
	if team.NoShowRate != nil {
		t.Fatalf("small completed-duty no-show rate must be suppressed: %+v", team)
	}
	if len(result.Quality.Caveats) < 4 || result.MetricVersion != "community-ops-v1" || result.PrivacyThreshold != 5 {
		t.Fatalf("missing dashboard provenance: %+v", result)
	}
}

func TestCompileCommunityAnalyticsDoesNotTreatFutureMeetingAsHeld(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	data := CommunityAnalyticsDataset{Groups: []Group{{ResourceEnvelope: platform.ResourceEnvelope{ID: "group-1"}, Name: "Future", Status: "active"}}, Meetings: []GroupMeeting{{ResourceEnvelope: platform.ResourceEnvelope{ID: "meeting-1"}, GroupID: "group-1", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), Status: "scheduled"}}}
	result := compileCommunityAnalytics("accra", now.Add(-time.Hour), now.Add(24*time.Hour), data, now)
	if result.Summary.MeetingsHeld != 0 || result.Groups[0].MeetingsHeld != 0 {
		t.Fatalf("future meeting counted as held: %+v", result)
	}
}
