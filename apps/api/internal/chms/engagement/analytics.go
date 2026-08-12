package engagement

import (
	"context"
	"sort"
	"time"

	"remi-api/internal/chms/platform"
)

const cohortPrivacyThreshold = 5

type CohortRange struct {
	BranchID platform.ID `json:"branchId"`
	From     time.Time   `json:"from"`
	To       time.Time   `json:"to"`
	Timezone string      `json:"timezone"`
}

type CohortConfiguration struct {
	PrivacyThreshold      int   `json:"privacyThreshold"`
	ReturnWindowsDays     []int `json:"returnWindowsDays"`
	GroupConnectionDays   int   `json:"groupConnectionDays"`
	ServingConnectionDays int   `json:"servingConnectionDays"`
}

type CohortMetric struct {
	State       string `json:"state"`
	Eligible    *int   `json:"eligible,omitempty"`
	Achieved    *int   `json:"achieved,omitempty"`
	RatePercent *int   `json:"ratePercent,omitempty"`
}

type CombinedConnectionMetric struct {
	State       string `json:"state"`
	Eligible    *int   `json:"eligible,omitempty"`
	Connected   *int   `json:"connected,omitempty"`
	RatePercent *int   `json:"ratePercent,omitempty"`
	GroupOnly   *int   `json:"groupOnly,omitempty"`
	ServingOnly *int   `json:"servingOnly,omitempty"`
	Both        *int   `json:"both,omitempty"`
	Neither     *int   `json:"neither,omitempty"`
}

type RetentionCohort struct {
	CohortWeek         string                   `json:"cohortWeek"`
	CohortSize         *int                     `json:"cohortSize,omitempty"`
	CohortState        string                   `json:"cohortState"`
	Return30           CohortMetric             `json:"return30"`
	Return60           CohortMetric             `json:"return60"`
	Return90           CohortMetric             `json:"return90"`
	GroupConnection    CohortMetric             `json:"groupConnection"`
	ServingConnection  CohortMetric             `json:"servingConnection"`
	CombinedConnection CombinedConnectionMetric `json:"combinedConnection"`
}

type CohortQuality struct {
	SourceWatermark *time.Time `json:"sourceWatermark,omitempty"`
	Caveats         []string   `json:"caveats"`
}

type CohortDashboard struct {
	MetricVersion string              `json:"metricVersion"`
	Range         CohortRange         `json:"range"`
	Configuration CohortConfiguration `json:"configuration"`
	Cohorts       []RetentionCohort   `json:"cohorts"`
	Quality       CohortQuality       `json:"quality"`
	GeneratedAt   time.Time           `json:"generatedAt"`
}

type cohortPerson struct {
	first       time.Time
	visits      []time.Time
	connections []ConnectionFact
}

func (s Service) CohortDashboard(ctx context.Context, p platform.Principal, branch platform.ID, from, to time.Time) (*CohortDashboard, error) {
	from, to = from.UTC(), to.UTC()
	if !branch.Valid() || from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour || to.After(s.now().Add(time.Minute)) {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a branch and a past reporting range no longer than 366 days."})
	}
	if !s.allowed(p, "read", branch) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read retention analytics for this branch."}
	}
	rules, err := s.Store.ListRules(ctx, p.OrganizationID, branch)
	if err != nil {
		return nil, err
	}
	timezone := ""
	for _, rule := range rules {
		if rule.Status == "published" {
			if timezone == "" {
				timezone = rule.Timezone
			} else if timezone != rule.Timezone {
				return nil, &platform.DomainError{Code: "conflict", Message: "Published branch retention policies use inconsistent timezones; align them before reporting cohorts."}
			}
		}
	}
	if timezone == "" {
		return nil, &platform.DomainError{Code: "conflict", Message: "Publish an approved branch retention policy before reporting cohorts."}
	}
	if _, err = time.LoadLocation(timezone); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "timezone", Code: "invalid", Message: "The branch retention timezone is invalid."})
	}
	data, err := s.Store.LoadDataset(ctx, p.OrganizationID, branch, time.Time{}, to)
	if err != nil {
		return nil, err
	}
	result := compileCohortDashboard(branch, timezone, from, to, data, s.now())
	return &result, nil
}

func compileCohortDashboard(branch platform.ID, timezone string, from, to time.Time, data Dataset, generatedAt time.Time) CohortDashboard {
	config := CohortConfiguration{PrivacyThreshold: cohortPrivacyThreshold, ReturnWindowsDays: []int{30, 60, 90}, GroupConnectionDays: 90, ServingConnectionDays: 120}
	result := CohortDashboard{MetricVersion: "retention-v1", Range: CohortRange{BranchID: branch, From: from, To: to, Timezone: timezone}, Configuration: config, Cohorts: []RetentionCohort{}, Quality: CohortQuality{SourceWatermark: data.LatestRecordedAt, Caveats: append([]string{}, data.Caveats...)}, GeneratedAt: generatedAt}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		result.Quality.Caveats = append(result.Quality.Caveats, "The configured branch timezone is invalid; cohort computation is unavailable.")
		return result
	}
	visits := map[platform.ID][]VisitFact{}
	for _, visit := range data.Visits {
		if data.ActivePeople[visit.PersonID] {
			visits[visit.PersonID] = append(visits[visit.PersonID], visit)
		}
	}
	connections := map[platform.ID][]ConnectionFact{}
	for _, connection := range data.Connections {
		connections[connection.PersonID] = append(connections[connection.PersonID], connection)
	}
	weeks := map[string][]cohortPerson{}
	for personID, facts := range visits {
		sort.Slice(facts, func(i, j int) bool { return facts[i].StartsAt.Before(facts[j].StartsAt) })
		if len(facts) == 0 || !facts[0].Guest {
			continue
		}
		first := facts[0].StartsAt
		localFirst := first.In(location)
		if localFirst.Before(from.In(location)) || !localFirst.Before(to.In(location)) {
			continue
		}
		person := cohortPerson{first: first, connections: connections[personID]}
		for _, fact := range facts {
			person.visits = append(person.visits, fact.StartsAt)
		}
		week := localWeekStart(localFirst).Format("2006-01-02")
		weeks[week] = append(weeks[week], person)
	}
	keys := make([]string, 0, len(weeks))
	for week := range weeks {
		keys = append(keys, week)
	}
	sort.Strings(keys)
	for _, week := range keys {
		result.Cohorts = append(result.Cohorts, compileCohort(week, weeks[week], timezone, to, config))
	}
	if len(result.Cohorts) == 0 && len(result.Quality.Caveats) == 0 {
		result.Quality.Caveats = append(result.Quality.Caveats, "No eligible first-visit cohorts fall inside the selected range; this is an empty result, not a retention conclusion.")
	}
	return result
}

func compileCohort(week string, people []cohortPerson, timezone string, asOf time.Time, config CohortConfiguration) RetentionCohort {
	row := RetentionCohort{CohortWeek: week, CohortState: "available"}
	if len(people) >= config.PrivacyThreshold {
		row.CohortSize = integer(len(people))
	} else {
		row.CohortState = "suppressed"
	}
	row.Return30 = compileMetric(people, timezone, asOf, 30, func(person cohortPerson, deadline time.Time) bool { return returnedBy(person, deadline, timezone) })
	row.Return60 = compileMetric(people, timezone, asOf, 60, func(person cohortPerson, deadline time.Time) bool { return returnedBy(person, deadline, timezone) })
	row.Return90 = compileMetric(people, timezone, asOf, 90, func(person cohortPerson, deadline time.Time) bool { return returnedBy(person, deadline, timezone) })
	row.GroupConnection = compileMetric(people, timezone, asOf, config.GroupConnectionDays, func(person cohortPerson, deadline time.Time) bool { return connectedBy(person, "group", deadline) })
	row.ServingConnection = compileMetric(people, timezone, asOf, config.ServingConnectionDays, func(person cohortPerson, deadline time.Time) bool { return connectedBy(person, "serving", deadline) })
	row.CombinedConnection = compileCombined(people, timezone, asOf, config)
	return row
}

func compileMetric(people []cohortPerson, timezone string, asOf time.Time, days int, achieved func(cohortPerson, time.Time) bool) CohortMetric {
	eligible, count := 0, 0
	for _, person := range people {
		deadline := localDeadlineExclusive(person.first, timezone, days)
		if asOf.Before(deadline) {
			continue
		}
		eligible++
		if achieved(person, deadline) {
			count++
		}
	}
	if eligible == 0 {
		return CohortMetric{State: "pending"}
	}
	if eligible < cohortPrivacyThreshold {
		return CohortMetric{State: "suppressed"}
	}
	return CohortMetric{State: "available", Eligible: integer(eligible), Achieved: integer(count), RatePercent: integer(percentRounded(count, eligible))}
}

func compileCombined(people []cohortPerson, timezone string, asOf time.Time, config CohortConfiguration) CombinedConnectionMetric {
	eligible, groupOnly, servingOnly, both, neither := 0, 0, 0, 0, 0
	for _, person := range people {
		groupDeadline := localDeadlineExclusive(person.first, timezone, config.GroupConnectionDays)
		servingDeadline := localDeadlineExclusive(person.first, timezone, config.ServingConnectionDays)
		if asOf.Before(groupDeadline) || asOf.Before(servingDeadline) {
			continue
		}
		eligible++
		group, serving := connectedBy(person, "group", groupDeadline), connectedBy(person, "serving", servingDeadline)
		switch {
		case group && serving:
			both++
		case group:
			groupOnly++
		case serving:
			servingOnly++
		default:
			neither++
		}
	}
	if eligible == 0 {
		return CombinedConnectionMetric{State: "pending"}
	}
	if eligible < cohortPrivacyThreshold {
		return CombinedConnectionMetric{State: "suppressed"}
	}
	connected := groupOnly + servingOnly + both
	return CombinedConnectionMetric{State: "available", Eligible: integer(eligible), Connected: integer(connected), RatePercent: integer(percentRounded(connected, eligible)), GroupOnly: integer(groupOnly), ServingOnly: integer(servingOnly), Both: integer(both), Neither: integer(neither)}
}

func returnedBy(person cohortPerson, deadline time.Time, timezone string) bool {
	for _, visit := range person.visits {
		if laterLocalDate(person.first, visit, timezone) && visit.Before(deadline) {
			return true
		}
	}
	return false
}

func connectedBy(person cohortPerson, kind string, deadline time.Time) bool {
	for _, connection := range person.connections {
		if connection.Kind == kind && connection.OccurredAt.After(person.first) && connection.OccurredAt.Before(deadline) {
			return true
		}
	}
	return false
}

func localWeekStart(value time.Time) time.Time {
	day := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func percentRounded(numerator, denominator int) int {
	if denominator == 0 {
		return 0
	}
	return (numerator*100 + denominator/2) / denominator
}

func integer(value int) *int { return &value }
