package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/community"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Community.ListGroups(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), r.URL.Query().Get("includeClosed") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getLeaderWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Use RFC3339 from and to timestamps."}))
		return
	}
	result, err := h.services.Community.GetLeaderWorkspace(r.Context(), principal, from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getCommunityAnalytics(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Use RFC3339 from and to timestamps."}))
		return
	}
	result, err := h.services.Community.CommunityAnalytics(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) requestGroupCommunication(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.GroupCommunicationInput
	if err := platform.DecodeJSON(w, r, &input, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.RequestGroupCommunication(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, value)
}

func (h *Handler) getLeaderGroupWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	groupID := platform.ID(chi.URLParam(r, "groupId"))
	group, err := h.services.Community.GetOwnedLeaderGroup(r.Context(), principal, groupID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	members, err := h.services.Community.ListMemberships(r.Context(), principal, groupID, true)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Use RFC3339 from and to timestamps."}))
		return
	}
	meetings, err := h.services.Community.ListMeetings(r.Context(), principal, groupID, from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	roster := make([]map[string]any, 0, len(members))
	for _, membership := range members {
		name := "Group member"
		if person, findErr := h.services.Repository.FindByID(r.Context(), principal.OrganizationID, membership.PersonID); findErr == nil && person != nil {
			name = strings.TrimSpace(person.Names.Given + " " + person.Names.Family)
			if name == "" {
				name = "Group member"
			}
		}
		roster = append(roster, map[string]any{"membership": membership, "name": name})
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"group": group, "roster": roster, "meetings": meetings, "generatedAt": time.Now().UTC()})
}

func (h *Handler) getLeaderTeamWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	teamID := platform.ID(chi.URLParam(r, "teamId"))
	team, err := h.services.Community.GetOwnedLeaderTeam(r.Context(), principal, teamID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Use RFC3339 from and to timestamps."}))
		return
	}
	assignments, err := h.services.Community.ListOwnedLeaderTeamAssignments(r.Context(), principal, teamID, from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	positions, err := h.services.Community.ListVolunteerPositions(r.Context(), principal, teamID, false)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	positionNames := map[platform.ID]string{}
	for _, position := range positions {
		positionNames[position.ID] = position.Name
	}
	roster := make([]map[string]any, 0, len(assignments))
	for _, assignment := range assignments {
		name := "Volunteer"
		if person, findErr := h.services.Repository.FindByID(r.Context(), principal.OrganizationID, assignment.PersonID); findErr == nil && person != nil {
			name = strings.TrimSpace(person.Names.Given + " " + person.Names.Family)
			if name == "" {
				name = "Volunteer"
			}
		}
		roster = append(roster, map[string]any{"assignment": assignment, "name": name, "positionName": positionNames[assignment.PositionID]})
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"team": team, "positions": positions, "assignments": roster, "generatedAt": time.Now().UTC()})
}

func (h *Handler) listGroupMembers(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Community.ListMemberships(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) listGroupMemberHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Community.ListMembershipHistory(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), platform.ID(chi.URLParam(r, "personId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createGroupMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.MembershipInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateMembership(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

type membershipTransitionRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	community.MembershipTransition
}

func (h *Handler) transitionGroupMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input membershipTransitionRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	groupID, personID := platform.ID(chi.URLParam(r, "groupId")), platform.ID(chi.URLParam(r, "personId"))
	membership, err := h.services.Community.Store.FindMembership(r.Context(), principal.OrganizationID, groupID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if membership == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Group membership not found."})
		return
	}
	value, err := h.services.Community.TransitionMembership(r.Context(), principal, groupID, membership.ID, input.ExpectedVersion, input.MembershipTransition, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

type endMembershipRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) endGroupMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input endMembershipRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	groupID, personID := platform.ID(chi.URLParam(r, "groupId")), platform.ID(chi.URLParam(r, "personId"))
	membership, err := h.services.Community.Store.FindMembership(r.Context(), principal.OrganizationID, groupID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if membership == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Group membership not found."})
		return
	}
	value, err := h.services.Community.TransitionMembership(r.Context(), principal, groupID, membership.ID, input.ExpectedVersion, community.MembershipTransition{Status: "ended", Role: membership.Role, LeaderNote: membership.LeaderNote, DirectoryVisibility: membership.DirectoryVisibility, Reason: input.Reason}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (h *Handler) listGroupMeetings(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	from, fromErr := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, toErr := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "range", Code: "invalid_range", Message: "Use RFC3339 from and to timestamps."}))
		return
	}
	items, err := h.services.Community.ListMeetings(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createGroupMeeting(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.MeetingInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateMeeting(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (h *Handler) listGroupMeetingAttendance(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Community.ListMeetingAttendance(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), platform.ID(chi.URLParam(r, "meetingId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type groupAttendanceRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	community.MeetingAttendanceInput
}

func (h *Handler) recordGroupMeetingAttendance(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input groupAttendanceRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.RecordMeetingAttendance(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), platform.ID(chi.URLParam(r, "meetingId")), platform.ID(chi.URLParam(r, "personId")), input.ExpectedVersion, input.MeetingAttendanceInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	status := http.StatusOK
	if input.ExpectedVersion == 0 {
		status = http.StatusCreated
	}
	setVersion(w, value.Version)
	writeJSON(w, status, value)
}
func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Community.GetGroup(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}
func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.GroupInput
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateGroup(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

type groupUpdateRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	community.GroupInput
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input groupUpdateRequest
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.UpdateGroup(r.Context(), principal, platform.ID(chi.URLParam(r, "groupId")), input.ExpectedVersion, input.GroupInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}
