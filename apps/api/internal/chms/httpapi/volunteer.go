package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/community"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listVolunteerTeams(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Community.ListVolunteerTeams(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createVolunteerTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.VolunteerTeamInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateVolunteerTeam(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) getVolunteerTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Community.GetVolunteerTeam(r.Context(), principal, platform.ID(chi.URLParam(r, "teamId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

type volunteerTeamUpdateRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	community.VolunteerTeamInput
}

func (h *Handler) updateVolunteerTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input volunteerTeamUpdateRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.UpdateVolunteerTeam(r.Context(), principal, platform.ID(chi.URLParam(r, "teamId")), input.ExpectedVersion, input.VolunteerTeamInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listVolunteerPositions(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Community.ListVolunteerPositions(r.Context(), principal, platform.ID(chi.URLParam(r, "teamId")), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createVolunteerPosition(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.VolunteerPositionInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.CreateVolunteerPosition(r.Context(), principal, platform.ID(chi.URLParam(r, "teamId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

type volunteerPositionUpdateRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	community.VolunteerPositionInput
}

func (h *Handler) updateVolunteerPosition(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input volunteerPositionUpdateRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Community.UpdateVolunteerPosition(r.Context(), principal, platform.ID(chi.URLParam(r, "teamId")), platform.ID(chi.URLParam(r, "positionId")), input.ExpectedVersion, input.VolunteerPositionInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) getVolunteerProfile(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Community.GetVolunteerProfile(r.Context(), principal, platform.ID(chi.URLParam(r, "personId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

type volunteerProfilePutRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	community.VolunteerProfileInput
}

func (h *Handler) putVolunteerProfile(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input volunteerProfilePutRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.PutVolunteerProfile(r.Context(), principal, platform.ID(chi.URLParam(r, "personId")), input.ExpectedVersion, input.VolunteerProfileInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listVolunteerAvailability(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.services.Community.ListAvailability(r.Context(), principal, platform.ID(chi.URLParam(r, "personId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) addVolunteerAvailability(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input community.AvailabilityInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Community.AddAvailability(r.Context(), principal, platform.ID(chi.URLParam(r, "personId")), input, platform.RequestIDFrom(r.Context()), r.Header.Get("Idempotency-Key"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
