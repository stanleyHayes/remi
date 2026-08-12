package participation

import (
	"context"
	"fmt"
	"sort"
	"time"

	"remi-api/internal/chms/platform"
)

const attendanceCohortPrivacyThreshold = 5

type AttendanceDashboard struct {
	Range       AnalyticsRange        `json:"range"`
	Summary     AttendanceSummary     `json:"summary"`
	Trends      []AttendanceTrend     `json:"trends"`
	Cohorts     []GuestReturnCohort   `json:"cohorts"`
	Quality     AttendanceDataQuality `json:"quality"`
	MetricNotes []MetricNote          `json:"metricNotes"`
	GeneratedAt time.Time             `json:"generatedAt"`
}

type AnalyticsRange struct {
	BranchID platform.ID `json:"branchId"`
	From     time.Time   `json:"from"`
	To       time.Time   `json:"to"`
}

type AttendanceSummary struct {
	CompletedOccurrences int `json:"completedOccurrences"`
	UniqueNamedPeople    int `json:"uniqueNamedPeople"`
	NamedAttendances     int `json:"namedAttendances"`
	AnonymousHeadcount   int `json:"anonymousHeadcount"`
	FirstTimeGuests      int `json:"firstTimeGuests"`
	ReturningGuests      int `json:"returningGuests"`
}

type AttendanceTrend struct {
	OccurrenceID        platform.ID `json:"occurrenceId"`
	ServiceDefinitionID platform.ID `json:"serviceDefinitionId,omitempty"`
	Name                string      `json:"name"`
	StartsAt            time.Time   `json:"startsAt"`
	NamedPresent        int         `json:"namedPresent"`
	AnonymousHeadcount  int         `json:"anonymousHeadcount"`
	FirstTimeGuests     int         `json:"firstTimeGuests"`
	ReturningGuests     int         `json:"returningGuests"`
	AverageConfidence   int         `json:"averageConfidence"`
}

type GuestReturnCohort struct {
	CohortWeek      string `json:"cohortWeek"`
	FirstTimeGuests int    `json:"firstTimeGuests"`
	Eligible30      int    `json:"eligible30"`
	Returned30      int    `json:"returned30"`
	ReturnRate30    *int   `json:"returnRate30,omitempty"`
	Eligible60      int    `json:"eligible60"`
	Returned60      int    `json:"returned60"`
	ReturnRate60    *int   `json:"returnRate60,omitempty"`
	Eligible90      int    `json:"eligible90"`
	Returned90      int    `json:"returned90"`
	ReturnRate90    *int   `json:"returnRate90,omitempty"`
	Suppressed      bool   `json:"suppressed"`
}

type AttendanceDataQuality struct {
	OccurrencesWithNamedData int        `json:"occurrencesWithNamedData"`
	OccurrencesWithHeadcount int        `json:"occurrencesWithHeadcount"`
	NamedCoveragePercent     int        `json:"namedCoveragePercent"`
	HeadcountCoveragePercent int        `json:"headcountCoveragePercent"`
	AverageConfidence        int        `json:"averageConfidence"`
	LatestRecordedAt         *time.Time `json:"latestRecordedAt,omitempty"`
	Caveats                  []string   `json:"caveats"`
}

type MetricNote struct {
	Metric     string `json:"metric"`
	Definition string `json:"definition"`
	Grain      string `json:"grain"`
}

type AnalyticsDataset struct {
	Occurrences []Occurrence
	Attendance  []AttendanceFact
	Headcounts  []Headcount
	GuestVisits map[platform.ID][]time.Time
}

type attendanceAnalyticsStore interface {
	LoadAttendanceAnalytics(context.Context, platform.ID, platform.ID, time.Time, time.Time) (AnalyticsDataset, error)
}

func (s Service) AttendanceDashboard(ctx context.Context, principal platform.Principal, branchID platform.ID, from, to time.Time) (*AttendanceDashboard, error) {
	from = from.UTC()
	to = to.UTC()
	if !branchID.Valid() || from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a branch and a valid range no longer than 366 days."})
	}
	if !s.allowed(principal, "read", "attendance", branchID) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read attendance analytics for this branch."}
	}
	store, ok := s.Store.(attendanceAnalyticsStore)
	if !ok {
		return nil, fmt.Errorf("attendance analytics store is unavailable")
	}
	dataset, err := store.LoadAttendanceAnalytics(ctx, principal.OrganizationID, branchID, from, to)
	if err != nil {
		return nil, err
	}
	result := compileAttendanceAnalytics(branchID, from, to, dataset, s.now())
	return &result, nil
}

func compileAttendanceAnalytics(branchID platform.ID, from, to time.Time, data AnalyticsDataset, now time.Time) AttendanceDashboard {
	result := AttendanceDashboard{
		Range:       AnalyticsRange{BranchID: branchID, From: from, To: to},
		GeneratedAt: now,
		Trends:      []AttendanceTrend{}, Cohorts: []GuestReturnCohort{},
		MetricNotes: []MetricNote{
			{Metric: "uniqueNamedPeople", Definition: "Distinct people with current status present in a completed, non-cancelled occurrence in the selected range.", Grain: "person"},
			{Metric: "anonymousHeadcount", Definition: "Sum of the latest observation in each category for each occurrence; never deduplicated with named attendance.", Grain: "occurrence and category"},
			{Metric: "firstTimeGuests", Definition: "Guest attendance whose occurrence is the person's earliest recorded guest visit.", Grain: "person"},
			{Metric: "returnRate30", Definition: "First-time guests with another recorded present visit within 30 days, divided by guests old enough to observe the full window.", Grain: "weekly cohort"},
		},
	}
	occurrences := map[platform.ID]Occurrence{}
	for _, occurrence := range data.Occurrences {
		if occurrence.Status == "cancelled" || occurrence.EndsAt.After(now) || occurrence.StartsAt.Before(from) || !occurrence.StartsAt.Before(to) {
			continue
		}
		occurrences[occurrence.ID] = occurrence
		result.Summary.CompletedOccurrences++
	}
	attendanceByOccurrence := map[platform.ID][]AttendanceFact{}
	uniquePeople := map[platform.ID]bool{}
	for _, fact := range data.Attendance {
		if _, ok := occurrences[fact.OccurrenceID]; !ok || fact.Status != AttendancePresent {
			continue
		}
		attendanceByOccurrence[fact.OccurrenceID] = append(attendanceByOccurrence[fact.OccurrenceID], fact)
		uniquePeople[fact.PersonID] = true
		result.Summary.NamedAttendances++
	}
	result.Summary.UniqueNamedPeople = len(uniquePeople)
	headcountsByOccurrence := latestHeadcounts(data.Headcounts, occurrences)
	cohortPeople := map[string][]platform.ID{}
	firstVisit := map[platform.ID]time.Time{}
	for personID, visits := range data.GuestVisits {
		if len(visits) == 0 {
			continue
		}
		sort.Slice(visits, func(i, j int) bool { return visits[i].Before(visits[j]) })
		firstVisit[personID] = visits[0]
		if !visits[0].Before(from) && visits[0].Before(to) {
			week := weekStart(visits[0]).Format("2006-01-02")
			cohortPeople[week] = append(cohortPeople[week], personID)
		}
	}
	trendIDs := make([]platform.ID, 0, len(occurrences))
	for id := range occurrences {
		trendIDs = append(trendIDs, id)
	}
	sort.Slice(trendIDs, func(i, j int) bool {
		return occurrences[trendIDs[i]].StartsAt.Before(occurrences[trendIDs[j]].StartsAt)
	})
	totalConfidence, confidenceSamples := 0, 0
	var latest *time.Time
	for _, id := range trendIDs {
		occurrence := occurrences[id]
		trend := AttendanceTrend{OccurrenceID: id, ServiceDefinitionID: occurrence.ServiceDefinitionID, Name: occurrence.Name, StartsAt: occurrence.StartsAt}
		for _, fact := range attendanceByOccurrence[id] {
			trend.NamedPresent++
			totalConfidence += fact.Confidence
			confidenceSamples++
			if fact.UpdatedAt.After(timeValue(latest)) {
				value := fact.UpdatedAt
				latest = &value
			}
			if fact.Guest {
				if firstVisit[fact.PersonID].Equal(occurrence.StartsAt) {
					trend.FirstTimeGuests++
					result.Summary.FirstTimeGuests++
				} else {
					trend.ReturningGuests++
					result.Summary.ReturningGuests++
				}
			}
		}
		for _, count := range headcountsByOccurrence[id] {
			trend.AnonymousHeadcount += count.Count
			totalConfidence += count.Confidence
			confidenceSamples++
			if count.UpdatedAt.After(timeValue(latest)) {
				value := count.UpdatedAt
				latest = &value
			}
		}
		trend.AverageConfidence = averageConfidence(attendanceByOccurrence[id], headcountsByOccurrence[id])
		result.Summary.AnonymousHeadcount += trend.AnonymousHeadcount
		if trend.NamedPresent > 0 {
			result.Quality.OccurrencesWithNamedData++
		}
		if len(headcountsByOccurrence[id]) > 0 {
			result.Quality.OccurrencesWithHeadcount++
		}
		result.Trends = append(result.Trends, trend)
	}
	if result.Summary.CompletedOccurrences > 0 {
		result.Quality.NamedCoveragePercent = percent(result.Quality.OccurrencesWithNamedData, result.Summary.CompletedOccurrences)
		result.Quality.HeadcountCoveragePercent = percent(result.Quality.OccurrencesWithHeadcount, result.Summary.CompletedOccurrences)
	}
	if confidenceSamples > 0 {
		result.Quality.AverageConfidence = totalConfidence / confidenceSamples
	}
	result.Quality.LatestRecordedAt = latest
	result.Quality.Caveats = qualityCaveats(result)
	weeks := make([]string, 0, len(cohortPeople))
	for week := range cohortPeople {
		weeks = append(weeks, week)
	}
	sort.Strings(weeks)
	for _, week := range weeks {
		result.Cohorts = append(result.Cohorts, buildCohort(week, cohortPeople[week], firstVisit, data.GuestVisits, to))
	}
	return result
}

func latestHeadcounts(values []Headcount, occurrences map[platform.ID]Occurrence) map[platform.ID][]Headcount {
	latest := map[platform.ID]map[string]Headcount{}
	for _, value := range values {
		if _, ok := occurrences[value.OccurrenceID]; !ok {
			continue
		}
		if latest[value.OccurrenceID] == nil {
			latest[value.OccurrenceID] = map[string]Headcount{}
		}
		key := value.Category + ":" + string(value.RoomID)
		current, exists := latest[value.OccurrenceID][key]
		if !exists || value.ObservedAt.After(current.ObservedAt) {
			latest[value.OccurrenceID][key] = value
		}
	}
	result := map[platform.ID][]Headcount{}
	for occurrenceID, categories := range latest {
		for _, value := range categories {
			result[occurrenceID] = append(result[occurrenceID], value)
		}
	}
	return result
}

func buildCohort(week string, people []platform.ID, first map[platform.ID]time.Time, visits map[platform.ID][]time.Time, observedThrough time.Time) GuestReturnCohort {
	cohort := GuestReturnCohort{CohortWeek: week, FirstTimeGuests: len(people), Suppressed: len(people) < attendanceCohortPrivacyThreshold}
	for _, personID := range people {
		start := first[personID]
		for _, window := range []struct {
			days               int
			eligible, returned *int
		}{{30, &cohort.Eligible30, &cohort.Returned30}, {60, &cohort.Eligible60, &cohort.Returned60}, {90, &cohort.Eligible90, &cohort.Returned90}} {
			end := start.Add(time.Duration(window.days) * 24 * time.Hour)
			if observedThrough.Before(end) {
				continue
			}
			*window.eligible++
			for _, visit := range visits[personID] {
				if visit.After(start) && !visit.After(end) {
					*window.returned++
					break
				}
			}
		}
	}
	if !cohort.Suppressed {
		cohort.ReturnRate30 = rate(cohort.Returned30, cohort.Eligible30)
		cohort.ReturnRate60 = rate(cohort.Returned60, cohort.Eligible60)
		cohort.ReturnRate90 = rate(cohort.Returned90, cohort.Eligible90)
	}
	return cohort
}

func qualityCaveats(result AttendanceDashboard) []string {
	caveats := []string{"Named attendance and anonymous headcount are separate measures and must not be added together."}
	if result.Quality.NamedCoveragePercent < 100 {
		caveats = append(caveats, "Some completed occurrences have no named attendance; unique-person and guest metrics may be understated.")
	}
	if result.Quality.HeadcountCoveragePercent < 100 {
		caveats = append(caveats, "Some completed occurrences have no anonymous headcount; aggregate attendance is incomplete.")
	}
	if result.Quality.AverageConfidence > 0 && result.Quality.AverageConfidence < 80 {
		caveats = append(caveats, "Average source confidence is below 80%; reconcile imported or estimated observations before decisions.")
	}
	if len(result.Cohorts) > 0 {
		caveats = append(caveats, "Return rates are hidden for cohorts smaller than five people and until each observation window is complete.")
	}
	return caveats
}

func averageConfidence(attendance []AttendanceFact, counts []Headcount) int {
	total := 0
	for _, v := range attendance {
		total += v.Confidence
	}
	for _, v := range counts {
		total += v.Confidence
	}
	if len(attendance)+len(counts) == 0 {
		return 0
	}
	return total / (len(attendance) + len(counts))
}
func percent(n, d int) int {
	if d == 0 {
		return 0
	}
	return (n*100 + d/2) / d
}
func rate(n, d int) *int {
	if d == 0 {
		return nil
	}
	value := percent(n, d)
	return &value
}
func weekStart(value time.Time) time.Time {
	day := (int(value.Weekday()) + 6) % 7
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location()).AddDate(0, 0, -day)
}
func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
