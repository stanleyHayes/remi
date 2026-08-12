package community

import (
	"context"
	"fmt"
	"sort"
	"time"

	"remi-api/internal/chms/platform"
)

const communityAnalyticsPrivacyThreshold = 5

type CommunityAnalyticsRange struct {
	BranchID platform.ID `json:"branchId"`
	From     time.Time   `json:"from"`
	To       time.Time   `json:"to"`
}

type CommunityAnalyticsSummary struct {
	ActiveGroups              int `json:"activeGroups"`
	ActiveGroupMemberships    int `json:"activeGroupMemberships"`
	UniqueConnectedPeople     int `json:"uniqueConnectedPeople"`
	MeetingsHeld              int `json:"meetingsHeld"`
	ActiveVolunteerTeams      int `json:"activeVolunteerTeams"`
	UniqueScheduledVolunteers int `json:"uniqueScheduledVolunteers"`
	PlannedServingSlots       int `json:"plannedServingSlots"`
	FilledServingSlots        int `json:"filledServingSlots"`
}

type GroupAnalyticsRow struct {
	GroupID                 platform.ID `json:"groupId"`
	Name                    string      `json:"name"`
	Type                    string      `json:"type"`
	Status                  string      `json:"status"`
	Capacity                int         `json:"capacity"`
	CurrentActiveMembers    int         `json:"currentActiveMembers"`
	PendingMembers          int         `json:"pendingMembers"`
	CurrentCapacityPercent  *int        `json:"currentCapacityPercent,omitempty"`
	MeetingsHeld            int         `json:"meetingsHeld"`
	RecordedPresent         int         `json:"recordedPresent"`
	RecordedAbsentOrExcused int         `json:"recordedAbsentOrExcused"`
	RecordedAttendanceRate  *int        `json:"recordedAttendanceRate,omitempty"`
	Suppressed              bool        `json:"suppressed"`
}

type VolunteerAnalyticsRow struct {
	TeamID                    platform.ID `json:"teamId"`
	Name                      string      `json:"name"`
	Status                    string      `json:"status"`
	ActivePositions           int         `json:"activePositions"`
	PlannedSlots              int         `json:"plannedSlots"`
	FilledSlots               int         `json:"filledSlots"`
	UniqueScheduledVolunteers int         `json:"uniqueScheduledVolunteers"`
	Invited                   int         `json:"invited"`
	Accepted                  int         `json:"accepted"`
	Declined                  int         `json:"declined"`
	Completed                 int         `json:"completed"`
	NoShows                   int         `json:"noShows"`
	FillRate                  *int        `json:"fillRate,omitempty"`
	ResponseRate              *int        `json:"responseRate,omitempty"`
	NoShowRate                *int        `json:"noShowRate,omitempty"`
	Suppressed                bool        `json:"suppressed"`
}

type CommunityAnalyticsQuality struct {
	GroupsWithoutCapacity       int        `json:"groupsWithoutCapacity"`
	MeetingsWithoutMarks        int        `json:"meetingsWithoutMarks"`
	PlansWithoutNeeds           int        `json:"plansWithoutNeeds"`
	AssignmentsWithoutTeamMatch int        `json:"assignmentsWithoutTeamMatch"`
	LatestRecordedAt            *time.Time `json:"latestRecordedAt,omitempty"`
	Caveats                     []string   `json:"caveats"`
}

type CommunityAnalyticsDashboard struct {
	Range            CommunityAnalyticsRange   `json:"range"`
	Summary          CommunityAnalyticsSummary `json:"summary"`
	Groups           []GroupAnalyticsRow       `json:"groups"`
	VolunteerTeams   []VolunteerAnalyticsRow   `json:"volunteerTeams"`
	Quality          CommunityAnalyticsQuality `json:"quality"`
	PrivacyThreshold int                       `json:"privacyThreshold"`
	MetricVersion    string                    `json:"metricVersion"`
	GeneratedAt      time.Time                 `json:"generatedAt"`
}

type CommunityAnalyticsDataset struct {
	Groups      []Group
	Memberships []GroupMembership
	Meetings    []GroupMeeting
	Attendance  []GroupMeetingAttendance
	Teams       []VolunteerTeam
	Positions   []VolunteerPosition
	Plans       []ServicePlan
	Assignments []VolunteerAssignment
}

type communityAnalyticsStore interface {
	LoadCommunityAnalytics(context.Context, platform.ID, platform.ID, time.Time, time.Time) (CommunityAnalyticsDataset, error)
}

func (s Service) CommunityAnalytics(ctx context.Context, principal platform.Principal, branchID platform.ID, from, to time.Time) (*CommunityAnalyticsDashboard, error) {
	from, to = from.UTC(), to.UTC()
	if !branchID.Valid() || from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return nil, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Choose a branch and a valid range no longer than 366 days."})
	}
	// Branch-wide analytics intentionally require a grant that is not narrowed to
	// one ministry; ministry-scoped operators use their filtered group directory.
	if !s.allowed(principal, "read", branchID, "") || !s.allowedVolunteer(principal, "read", branchID, platform.FieldOperational) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read ministry analytics for this branch."}
	}
	store, ok := s.Store.(communityAnalyticsStore)
	if !ok {
		return nil, fmt.Errorf("community analytics store is unavailable")
	}
	data, err := store.LoadCommunityAnalytics(ctx, principal.OrganizationID, branchID, from, to)
	if err != nil {
		return nil, err
	}
	result := compileCommunityAnalytics(branchID, from, to, data, s.now())
	return &result, nil
}

func compileCommunityAnalytics(branchID platform.ID, from, to time.Time, data CommunityAnalyticsDataset, now time.Time) CommunityAnalyticsDashboard {
	result := CommunityAnalyticsDashboard{Range: CommunityAnalyticsRange{BranchID: branchID, From: from, To: to}, Groups: []GroupAnalyticsRow{}, VolunteerTeams: []VolunteerAnalyticsRow{}, PrivacyThreshold: communityAnalyticsPrivacyThreshold, MetricVersion: "community-ops-v1", GeneratedAt: now}
	groupRows := map[platform.ID]*GroupAnalyticsRow{}
	groupPeople := map[platform.ID]map[platform.ID]bool{}
	connected := map[platform.ID]bool{}
	for _, group := range data.Groups {
		row := GroupAnalyticsRow{GroupID: group.ID, Name: group.Name, Type: group.Type, Status: group.Status, Capacity: group.Capacity}
		groupRows[group.ID] = &row
		if group.Status == "active" {
			result.Summary.ActiveGroups++
		}
		if group.Capacity == 0 {
			result.Quality.GroupsWithoutCapacity++
		}
	}
	for _, membership := range data.Memberships {
		row := groupRows[membership.GroupID]
		if row == nil {
			continue
		}
		switch membership.Status {
		case "active":
			row.CurrentActiveMembers++
			result.Summary.ActiveGroupMemberships++
			connected[membership.PersonID] = true
			if groupPeople[membership.GroupID] == nil {
				groupPeople[membership.GroupID] = map[platform.ID]bool{}
			}
			groupPeople[membership.GroupID][membership.PersonID] = true
		case "applied", "invited", "waitlisted":
			row.PendingMembers++
		}
	}
	result.Summary.UniqueConnectedPeople = len(connected)
	meetingRows := map[platform.ID]*GroupAnalyticsRow{}
	meetingMarks := map[platform.ID]int{}
	for _, meeting := range data.Meetings {
		if meeting.Status == "cancelled" || meeting.EndsAt.After(now) {
			continue
		}
		if row := groupRows[meeting.GroupID]; row != nil {
			row.MeetingsHeld++
			result.Summary.MeetingsHeld++
			meetingRows[meeting.ID] = row
		}
	}
	for _, mark := range data.Attendance {
		row := meetingRows[mark.MeetingID]
		if row == nil {
			continue
		}
		meetingMarks[mark.MeetingID]++
		if mark.Status == "present" {
			row.RecordedPresent++
		} else if mark.Status == "absent" || mark.Status == "excused" {
			row.RecordedAbsentOrExcused++
		}
		if mark.UpdatedAt.After(timeOrZero(result.Quality.LatestRecordedAt)) {
			value := mark.UpdatedAt
			result.Quality.LatestRecordedAt = &value
		}
	}
	for meetingID := range meetingRows {
		if meetingMarks[meetingID] == 0 {
			result.Quality.MeetingsWithoutMarks++
		}
	}
	for _, row := range groupRows {
		if row.Capacity > 0 {
			value := percentage(row.CurrentActiveMembers, row.Capacity)
			row.CurrentCapacityPercent = &value
		}
		marks := row.RecordedPresent + row.RecordedAbsentOrExcused
		row.Suppressed = marks > 0 && (marks < communityAnalyticsPrivacyThreshold || len(groupPeople[row.GroupID]) < communityAnalyticsPrivacyThreshold)
		if marks >= communityAnalyticsPrivacyThreshold && len(groupPeople[row.GroupID]) >= communityAnalyticsPrivacyThreshold {
			value := percentage(row.RecordedPresent, marks)
			row.RecordedAttendanceRate = &value
		}
		result.Groups = append(result.Groups, *row)
	}
	teamRows := map[platform.ID]*VolunteerAnalyticsRow{}
	for _, team := range data.Teams {
		row := VolunteerAnalyticsRow{TeamID: team.ID, Name: team.Name, Status: team.Status}
		teamRows[team.ID] = &row
		if team.Status == "active" {
			result.Summary.ActiveVolunteerTeams++
		}
	}
	positionTeam := map[platform.ID]platform.ID{}
	for _, position := range data.Positions {
		positionTeam[position.ID] = position.TeamID
		if row := teamRows[position.TeamID]; row != nil && position.Status == "active" {
			row.ActivePositions++
		}
	}
	for _, plan := range data.Plans {
		if plan.Status == "cancelled" {
			continue
		}
		if len(plan.Needs) == 0 {
			result.Quality.PlansWithoutNeeds++
		}
		for _, need := range plan.Needs {
			if row := teamRows[positionTeam[need.PositionID]]; row != nil {
				row.PlannedSlots += need.Slots
				result.Summary.PlannedServingSlots += need.Slots
			}
		}
	}
	globalVolunteers := map[platform.ID]bool{}
	teamVolunteers := map[platform.ID]map[platform.ID]bool{}
	for _, assignment := range data.Assignments {
		row := teamRows[assignment.TeamID]
		if row == nil {
			result.Quality.AssignmentsWithoutTeamMatch++
			continue
		}
		if teamVolunteers[assignment.TeamID] == nil {
			teamVolunteers[assignment.TeamID] = map[platform.ID]bool{}
		}
		teamVolunteers[assignment.TeamID][assignment.PersonID] = true
		globalVolunteers[assignment.PersonID] = true
		switch assignment.Status {
		case "invited":
			row.Invited++
		case "accepted":
			row.Accepted++
		case "declined":
			row.Declined++
		case "completed":
			row.Completed++
		case "no-show":
			row.NoShows++
		}
		if assignment.Status != "declined" && assignment.Status != "replaced" {
			row.FilledSlots++
			result.Summary.FilledServingSlots++
		}
		if assignment.UpdatedAt.After(timeOrZero(result.Quality.LatestRecordedAt)) {
			value := assignment.UpdatedAt
			result.Quality.LatestRecordedAt = &value
		}
	}
	result.Summary.UniqueScheduledVolunteers = len(globalVolunteers)
	for teamID, row := range teamRows {
		row.UniqueScheduledVolunteers = len(teamVolunteers[teamID])
		observations := row.Invited + row.Accepted + row.Declined + row.Completed + row.NoShows
		row.Suppressed = observations > 0 && (observations < communityAnalyticsPrivacyThreshold || row.UniqueScheduledVolunteers < communityAnalyticsPrivacyThreshold)
		if row.PlannedSlots >= communityAnalyticsPrivacyThreshold && row.UniqueScheduledVolunteers >= communityAnalyticsPrivacyThreshold {
			value := percentage(row.FilledSlots, row.PlannedSlots)
			row.FillRate = &value
		}
		responses := row.Accepted + row.Declined + row.Completed + row.NoShows
		if observations >= communityAnalyticsPrivacyThreshold && row.UniqueScheduledVolunteers >= communityAnalyticsPrivacyThreshold {
			value := percentage(responses, observations)
			row.ResponseRate = &value
		}
		outcomes := row.Completed + row.NoShows
		if outcomes >= communityAnalyticsPrivacyThreshold && row.UniqueScheduledVolunteers >= communityAnalyticsPrivacyThreshold {
			value := percentage(row.NoShows, outcomes)
			row.NoShowRate = &value
		}
		result.VolunteerTeams = append(result.VolunteerTeams, *row)
	}
	sort.Slice(result.Groups, func(i, j int) bool { return result.Groups[i].Name < result.Groups[j].Name })
	sort.Slice(result.VolunteerTeams, func(i, j int) bool { return result.VolunteerTeams[i].Name < result.VolunteerTeams[j].Name })
	result.Quality.Caveats = []string{"Group capacity utilization uses the current active roster and current capacity, not a historical snapshot.", "Recorded attendance rate uses only explicit present, absent or excused marks; unmarked roster members are not assumed absent.", "Volunteer fill uses configured plan needs. Rates are suppressed when either observations or distinct people are fewer than five.", "Leader notes, contact details, screening metadata and financial data are excluded."}
	return result
}

func percentage(part, total int) int {
	if total <= 0 {
		return 0
	}
	value := (part*100 + total/2) / total
	if value > 100 {
		return 100
	}
	return value
}
func timeOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
