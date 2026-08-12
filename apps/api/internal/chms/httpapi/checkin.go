package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/participation"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
)

func (h *Handler) createCheckinSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.CheckinSessionInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.CreateCheckinSession(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

type checkinHouseholdResult struct {
	ID      platform.ID                     `json:"id,omitempty"`
	Name    string                          `json:"name"`
	Members []people.HouseholdProfileMember `json:"members"`
}

func (h *Handler) lookupCheckinHouseholds(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	session, err := h.services.Participation.GetCheckinSession(r.Context(), principal, platform.ID(chi.URLParam(r, "sessionId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if session.State != "active" || !session.ExpiresAt.After(time.Now().UTC()) {
		platform.WriteError(w, r, &platform.DomainError{Code: "checkin_session_locked", Message: "This check-in station is locked or expired."})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "q", Code: "query_too_short", Message: "Enter at least two characters."}))
		return
	}
	page, err := h.services.Search.Search(r.Context(), principal, people.PersonSearchRequest{Filter: people.PersonSearchFilter{Query: query, BranchID: session.BranchID}, Limit: 20})
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	items := make([]checkinHouseholdResult, 0, len(page.Items))
	seen := map[platform.ID]bool{}
	for _, person := range page.Items {
		profile, profileErr := h.services.Households.FindProfileByPersonID(r.Context(), principal.OrganizationID, person.ID)
		if profileErr != nil {
			platform.WriteError(w, r, profileErr)
			return
		}
		if profile != nil {
			if seen[profile.ID] {
				continue
			}
			seen[profile.ID] = true
			items = append(items, checkinHouseholdResult{ID: profile.ID, Name: profile.Name, Members: profile.Members})
			continue
		}
		items = append(items, checkinHouseholdResult{Name: person.Names.Preferred, Members: []people.HouseholdProfileMember{{ID: person.ID, Name: displayCheckinName(person.Names)}}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type quickGuestRequest struct {
	OccurrenceID  platform.ID           `json:"occurrenceId"`
	Names         people.Names          `json:"names"`
	ContactPoints []people.ContactPoint `json:"contactPoints"`
	CapturedAt    time.Time             `json:"capturedAt"`
}

func (h *Handler) createCheckinGuest(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	session, err := h.services.Participation.GetCheckinSession(r.Context(), principal, platform.ID(chi.URLParam(r, "sessionId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if session.State != "active" || !session.ExpiresAt.After(time.Now().UTC()) {
		platform.WriteError(w, r, &platform.DomainError{Code: "checkin_session_locked", Message: "This check-in station is locked or expired."})
		return
	}
	var input quickGuestRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	authorized := false
	for _, id := range session.OccurrenceIDs {
		if id == input.OccurrenceID {
			authorized = true
			break
		}
	}
	if !authorized {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "occurrenceId", Code: "occurrence_not_authorized", Message: "Choose an occurrence authorized for this station."}))
		return
	}
	created, duplicates, err := h.services.People.Create(r.Context(), people.CreateInput{OrganizationID: principal.OrganizationID, HomeBranchID: session.BranchID, Names: input.Names, ContactPoints: input.ContactPoints, MembershipStage: "guest", Tags: []string{"check-in-guest"}, Source: people.Source{Type: "checkin", Reference: string(session.ID)}}, principal.Actor, platform.RequestIDFrom(r.Context()))
	if err != nil {
		if domain, yes := err.(*platform.DomainError); yes && domain.Code == "duplicate_candidate" {
			domain.Details = map[string]any{"candidateIds": duplicates}
		}
		platform.WriteError(w, r, err)
		return
	}
	captured := input.CapturedAt.UTC()
	if captured.IsZero() {
		captured = time.Now().UTC()
	}
	attendance, err := h.services.Participation.RecordAttendance(r.Context(), principal, input.OccurrenceID, created.ID, 0, participation.AttendanceInput{Status: participation.AttendancePresent, Source: "kiosk", Confidence: 100, CheckedInAt: &captured, StationID: session.ID, Guest: true}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"person": created, "attendance": attendance})
}

func displayCheckinName(names people.Names) string {
	if names.Preferred != "" {
		return names.Preferred
	}
	return strings.TrimSpace(names.Given + " " + names.Family)
}

func (h *Handler) getCheckinSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Participation.GetCheckinSession(r.Context(), principal, platform.ID(chi.URLParam(r, "sessionId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) syncCheckinCommands(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.CheckinSyncRequest
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.SyncCheckinCommands(r.Context(), principal, platform.ID(chi.URLParam(r, "sessionId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) lockCheckinSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.LockCheckinSession(r.Context(), principal, platform.ID(chi.URLParam(r, "sessionId")), input.Reason, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}
