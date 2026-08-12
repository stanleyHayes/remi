package engagement

import (
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func testRule(kind string) Rule {
	return Rule{ResourceEnvelope: platform.ResourceEnvelope{ID: "rule-1", OrganizationID: "org-1", BranchID: "accra", Version: 2}, Name: "Observed gap", Kind: kind, Timezone: "Africa/Accra", WindowDays: 30, LookbackDays: 120, ExpiresAfterDays: 14, Status: "published", MetricVersion: "retention-v1"}
}
func visit(id, person string, at time.Time, guest bool) VisitFact {
	return VisitFact{AttendanceID: platform.ID(id), OccurrenceID: platform.ID("occ-" + id), PersonID: platform.ID(person), StartsAt: at, RecordedAt: at.Add(time.Hour), Guest: guest}
}

func TestFirstVisitReturnUsesDistinctLocalDates(t *testing.T) {
	first := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	data := Dataset{ActivePeople: map[platform.ID]bool{"same-day": true, "next-day": true, "late": true}, Visits: []VisitFact{
		visit("a1", "same-day", first, true), visit("a2", "same-day", first.Add(3*time.Hour), true),
		visit("b1", "next-day", first, true), visit("b2", "next-day", first.Add(24*time.Hour), false),
		visit("c1", "late", first, true), visit("c2", "late", first.AddDate(0, 0, 31), false),
	}}
	values := evaluate(testRule("first-visit-no-return"), data, asOf)
	if len(values) != 2 {
		t.Fatalf("expected same-day and late candidates, got %+v", values)
	}
	seen := map[platform.ID]bool{}
	for _, value := range values {
		seen[value.PersonID] = true
	}
	if !seen["same-day"] || !seen["late"] || seen["next-day"] {
		t.Fatalf("wrong local-date return classification: %+v", seen)
	}
}

func TestConnectionRulesUseOnlyPermittedFactsInsideMatureWindow(t *testing.T) {
	first := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	data := Dataset{ActivePeople: map[platform.ID]bool{"connected": true, "invited-only": true}, Visits: []VisitFact{visit("a", "connected", first, true), visit("b", "invited-only", first, true)}, Connections: []ConnectionFact{{ID: "membership-1", PersonID: "connected", OccurredAt: first.AddDate(0, 0, 10), Kind: "group"}}}
	values := evaluate(testRule("first-visit-no-group"), data, asOf)
	if len(values) != 1 || values[0].PersonID != "invited-only" {
		t.Fatalf("wrong group-gap evidence: %+v", values)
	}
	if len(values[0].Evidence) != 1 || values[0].Evidence[0].SourceType != "attendance" || values[0].Caveats[0] == "" {
		t.Fatalf("missing explainability: %+v", values[0])
	}
}

func TestImmatureAndArchivedPeopleNeverProduceSignals(t *testing.T) {
	first := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	data := Dataset{ActivePeople: map[platform.ID]bool{"active": true}, Visits: []VisitFact{visit("a", "active", first, true), visit("b", "archived", first.AddDate(0, 0, -100), true)}}
	if values := evaluate(testRule("first-visit-no-return"), data, asOf); len(values) != 0 {
		t.Fatalf("immature or archived candidate generated: %+v", values)
	}
}

func TestAttendanceGapRequiresRecentHistoryInsideLookback(t *testing.T) {
	asOf := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	recent := asOf.AddDate(0, 0, -45)
	ancient := asOf.AddDate(0, 0, -200)
	data := Dataset{ActivePeople: map[platform.ID]bool{"recent": true, "ancient": true}, Visits: []VisitFact{visit("a", "recent", recent, false), visit("b", "ancient", ancient, false)}}
	values := evaluate(testRule("attendance-gap"), data, asOf)
	if len(values) != 1 || values[0].PersonID != "recent" || values[0].ObservedFrom != asOf.AddDate(0, 0, -120) {
		t.Fatalf("bad attendance-gap lookback: %+v", values)
	}
}
