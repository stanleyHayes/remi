package participation

import (
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func TestCompileAttendanceAnalyticsKeepsMeasuresSeparateAndClassifiesGuests(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstService := from.Add(3 * 24 * time.Hour)
	secondService := firstService.Add(7 * 24 * time.Hour)
	now := from.Add(120 * 24 * time.Hour)
	occurrences := []Occurrence{
		{ResourceEnvelope: analyticsEnvelope("occ-1", "branch-1", firstService), HomeBranchID: "branch-1", Name: "Sunday Gathering", StartsAt: firstService, EndsAt: firstService.Add(2 * time.Hour), Status: "scheduled"},
		{ResourceEnvelope: analyticsEnvelope("occ-2", "branch-1", secondService), HomeBranchID: "branch-1", Name: "Sunday Gathering", StartsAt: secondService, EndsAt: secondService.Add(2 * time.Hour), Status: "scheduled"},
	}
	attendance := []AttendanceFact{}
	guestVisits := map[platform.ID][]time.Time{}
	for index := 1; index <= 6; index++ {
		id := platform.ID("guest-" + string(rune('0'+index)))
		attendance = append(attendance, AttendanceFact{ResourceEnvelope: analyticsEnvelope(platform.ID("a-"+string(rune('0'+index))), "branch-1", firstService), OccurrenceID: "occ-1", PersonID: id, Status: AttendancePresent, Guest: true, Confidence: 100})
		guestVisits[id] = []time.Time{firstService}
	}
	attendance = append(attendance,
		AttendanceFact{ResourceEnvelope: analyticsEnvelope("a-return", "branch-1", secondService), OccurrenceID: "occ-2", PersonID: "guest-1", Status: AttendancePresent, Guest: true, Confidence: 80},
		AttendanceFact{ResourceEnvelope: analyticsEnvelope("a-member", "branch-1", secondService), OccurrenceID: "occ-2", PersonID: "member-1", Status: AttendancePresent, Confidence: 100},
	)
	guestVisits["guest-1"] = append(guestVisits["guest-1"], secondService)
	headcounts := []Headcount{
		{ResourceEnvelope: analyticsEnvelope("h-old", "branch-1", firstService), OccurrenceID: "occ-1", Category: "auditorium", Count: 90, Confidence: 70, ObservedAt: firstService.Add(time.Hour)},
		{ResourceEnvelope: analyticsEnvelope("h-new", "branch-1", firstService.Add(90*time.Minute)), OccurrenceID: "occ-1", Category: "auditorium", Count: 100, Confidence: 90, ObservedAt: firstService.Add(90 * time.Minute)},
		{ResourceEnvelope: analyticsEnvelope("h-overflow", "branch-1", firstService), OccurrenceID: "occ-1", Category: "overflow", Count: 20, Confidence: 100, ObservedAt: firstService.Add(time.Hour)},
	}
	result := compileAttendanceAnalytics("branch-1", from, from.Add(60*24*time.Hour), AnalyticsDataset{Occurrences: occurrences, Attendance: attendance, Headcounts: headcounts, GuestVisits: guestVisits}, now)
	if result.Summary.UniqueNamedPeople != 7 || result.Summary.NamedAttendances != 8 {
		t.Fatalf("named measures = %+v", result.Summary)
	}
	if result.Summary.AnonymousHeadcount != 120 {
		t.Fatalf("anonymous headcount = %d, want latest categories summed to 120", result.Summary.AnonymousHeadcount)
	}
	if result.Summary.FirstTimeGuests != 6 || result.Summary.ReturningGuests != 1 {
		t.Fatalf("guest classification = %+v", result.Summary)
	}
	if len(result.Cohorts) != 1 || result.Cohorts[0].Suppressed || result.Cohorts[0].ReturnRate30 == nil || *result.Cohorts[0].ReturnRate30 != 17 {
		t.Fatalf("cohort = %+v", result.Cohorts)
	}
	if result.Quality.NamedCoveragePercent != 100 || result.Quality.HeadcountCoveragePercent != 50 {
		t.Fatalf("quality = %+v", result.Quality)
	}
}

func TestCompileAttendanceAnalyticsSuppressesSmallOrImmatureCohorts(t *testing.T) {
	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	visit := from.Add(24 * time.Hour)
	data := AnalyticsDataset{
		Occurrences: []Occurrence{{ResourceEnvelope: analyticsEnvelope("occ", "branch-1", visit), HomeBranchID: "branch-1", Name: "Gathering", StartsAt: visit, EndsAt: visit.Add(time.Hour), Status: "scheduled"}},
		Attendance:  []AttendanceFact{{ResourceEnvelope: analyticsEnvelope("a", "branch-1", visit), OccurrenceID: "occ", PersonID: "guest", Status: AttendancePresent, Guest: true, Confidence: 100}},
		GuestVisits: map[platform.ID][]time.Time{"guest": {visit}},
	}
	result := compileAttendanceAnalytics("branch-1", from, from.Add(7*24*time.Hour), data, from.Add(10*24*time.Hour))
	if len(result.Cohorts) != 1 || !result.Cohorts[0].Suppressed || result.Cohorts[0].ReturnRate30 != nil || result.Cohorts[0].Eligible30 != 0 {
		t.Fatalf("small immature cohort must stay suppressed: %+v", result.Cohorts)
	}
}

func analyticsEnvelope(id, branch platform.ID, updated time.Time) platform.ResourceEnvelope {
	return platform.ResourceEnvelope{ID: id, OrganizationID: "org-1", BranchID: branch, Version: 1, CreatedAt: updated, UpdatedAt: updated}
}
