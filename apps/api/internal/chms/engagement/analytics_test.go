package engagement

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func cohortPeople(count int, first time.Time) Dataset {
	data := Dataset{ActivePeople: map[platform.ID]bool{}, Visits: []VisitFact{}, Connections: []ConnectionFact{}}
	for index := 0; index < count; index++ {
		id := platform.ID("person-" + string(rune('a'+index)))
		data.ActivePeople[id] = true
		data.Visits = append(data.Visits, visit("first-"+string(id), string(id), first, true))
	}
	return data
}

func TestCohortDashboardMaturityReturnsAndConnectionPartitions(t *testing.T) {
	first := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	data := cohortPeople(6, first)
	for index := 0; index < 3; index++ {
		id := platform.ID("person-" + string(rune('a'+index)))
		data.Visits = append(data.Visits, visit("return-"+string(id), string(id), first.AddDate(0, 0, 20+index*20), false))
	}
	data.Connections = append(data.Connections,
		ConnectionFact{ID: "g-a", PersonID: "person-a", Kind: "group", OccurredAt: first.AddDate(0, 0, 12)},
		ConnectionFact{ID: "s-b", PersonID: "person-b", Kind: "serving", OccurredAt: first.AddDate(0, 0, 14)},
		ConnectionFact{ID: "g-c", PersonID: "person-c", Kind: "group", OccurredAt: first.AddDate(0, 0, 16)},
		ConnectionFact{ID: "s-c", PersonID: "person-c", Kind: "serving", OccurredAt: first.AddDate(0, 0, 18)},
	)
	result := compileCohortDashboard("accra", "Africa/Accra", first.AddDate(0, 0, -1), first.AddDate(0, 0, 150), data, first.AddDate(0, 0, 150))
	if len(result.Cohorts) != 1 {
		t.Fatalf("cohorts=%+v", result.Cohorts)
	}
	row := result.Cohorts[0]
	if row.CohortSize == nil || *row.CohortSize != 6 || row.Return30.Achieved == nil || *row.Return30.Achieved != 1 || row.Return60.Achieved == nil || *row.Return60.Achieved != 3 {
		t.Fatalf("return metrics=%+v", row)
	}
	combined := row.CombinedConnection
	if combined.GroupOnly == nil || *combined.GroupOnly != 1 || *combined.ServingOnly != 1 || *combined.Both != 1 || *combined.Neither != 3 || *combined.Connected != 3 {
		t.Fatalf("combined partition=%+v", combined)
	}
	if *combined.GroupOnly+*combined.ServingOnly+*combined.Both+*combined.Neither != *combined.Eligible {
		t.Fatal("combined partition does not reconcile")
	}
}

func TestCohortDashboardSuppressesSparseCellsAndKeepsImmaturePending(t *testing.T) {
	first := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	sparse := compileCohortDashboard("accra", "Africa/Accra", first.AddDate(0, 0, -1), first.AddDate(0, 0, 150), cohortPeople(4, first), first.AddDate(0, 0, 150)).Cohorts[0]
	if sparse.CohortState != "suppressed" || sparse.CohortSize != nil || sparse.Return30.Eligible != nil || sparse.CombinedConnection.Neither != nil {
		t.Fatalf("sparse values leaked: %+v", sparse)
	}
	immature := compileCohortDashboard("accra", "Africa/Accra", first.AddDate(0, 0, -1), first.AddDate(0, 0, 20), cohortPeople(5, first), first.AddDate(0, 0, 20)).Cohorts[0]
	if immature.Return30.State != "pending" || immature.Return30.Eligible != nil || immature.Return30.RatePercent != nil {
		t.Fatalf("immature cohort treated as zero: %+v", immature.Return30)
	}
}

func TestCohortDashboardUsesDistinctBranchLocalDates(t *testing.T) {
	first := time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	data := cohortPeople(5, first)
	for index := 0; index < 5; index++ {
		id := platform.ID("person-" + string(rune('a'+index)))
		data.Visits = append(data.Visits, visit("same-"+string(id), string(id), first.Add(20*time.Minute), false))
	}
	row := compileCohortDashboard("accra", "Africa/Accra", first.Add(-24*time.Hour), first.AddDate(0, 0, 40), data, first.AddDate(0, 0, 40)).Cohorts[0]
	if row.Return30.Achieved == nil || *row.Return30.Achieved != 0 {
		t.Fatalf("same local date manufactured a return: %+v", row.Return30)
	}
}

func TestReturnWindowsCountTwentyNineAndFortyFiveButNotNinetyOneDays(t *testing.T) {
	first := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	people := []cohortPerson{
		{first: first, visits: []time.Time{first, first.AddDate(0, 0, 29)}},
		{first: first, visits: []time.Time{first, first.AddDate(0, 0, 45)}},
		{first: first, visits: []time.Time{first, first.AddDate(0, 0, 91)}},
		{first: first, visits: []time.Time{first, first.Add(3 * time.Hour)}},
		{first: first, visits: []time.Time{first}},
	}
	asOf := first.AddDate(0, 0, 130)
	returned := func(person cohortPerson, deadline time.Time) bool {
		return returnedBy(person, deadline, "Africa/Accra")
	}
	metric30 := compileMetric(people, "Africa/Accra", asOf, 30, returned)
	metric60 := compileMetric(people, "Africa/Accra", asOf, 60, returned)
	metric90 := compileMetric(people, "Africa/Accra", asOf, 90, returned)
	if metric30.Achieved == nil || *metric30.Achieved != 1 || *metric60.Achieved != 2 || *metric90.Achieved != 2 {
		t.Fatalf("return windows 30=%+v 60=%+v 90=%+v", metric30, metric60, metric90)
	}
}

func TestCohortOutputIsSymmetricAcrossPersonIdentifiers(t *testing.T) {
	first := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	left := cohortPeople(5, first)
	right := Dataset{ActivePeople: map[platform.ID]bool{}, Visits: []VisitFact{}, Connections: []ConnectionFact{}}
	for index, fact := range left.Visits {
		personID := platform.ID("renamed-" + string(rune('a'+index)))
		right.ActivePeople[personID] = true
		fact.PersonID = personID
		fact.AttendanceID = platform.ID("renamed-attendance-" + string(rune('a'+index)))
		right.Visits = append(right.Visits, fact)
	}
	from, to := first.AddDate(0, 0, -1), first.AddDate(0, 0, 130)
	leftResult := compileCohortDashboard("accra", "Africa/Accra", from, to, left, to)
	rightResult := compileCohortDashboard("accra", "Africa/Accra", from, to, right, to)
	if !reflect.DeepEqual(leftResult, rightResult) {
		t.Fatalf("identifier-only change altered aggregate output\nleft=%+v\nright=%+v", leftResult, rightResult)
	}
}

func TestCohortAnalyticsHasNoFinancialEvidenceDependency(t *testing.T) {
	for _, name := range []string{"analytics.go", "repository.go"} {
		contents, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(contents))
		for _, prohibited := range []string{"chms_contributions", "chms_pledges", "chms_funds", "giving amount", "donation"} {
			if strings.Contains(lower, prohibited) {
				t.Fatalf("%s contains prohibited retention source %q", name, prohibited)
			}
		}
	}
}
