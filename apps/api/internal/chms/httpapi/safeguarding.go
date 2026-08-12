package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/participation"
	"remi-api/internal/chms/platform"
)

func (h *Handler) createGuardianAuthorization(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.GuardianAuthorizationInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.CreateGuardianAuthorization(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) checkinChild(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.ChildCheckinInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	checkin, label, err := h.services.Participation.CheckinChild(r.Context(), principal, platform.ID(chi.URLParam(r, "sessionId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	childName := "Child"
	if child, findErr := h.services.Repository.FindByID(r.Context(), principal.OrganizationID, input.ChildPersonID); findErr == nil && child != nil {
		childName = child.Names.Preferred
		if childName == "" {
			childName = child.Names.Given
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"checkin": checkin, "label": map[string]any{"checkinId": label.CheckinID, "attendanceId": label.AttendanceID, "securityCode": label.SecurityCode, "expiresAt": label.ExpiresAt, "childDisplayName": childName}})
}

func (h *Handler) pickupChild(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.PickupInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.PickupChild(r.Context(), principal, platform.ID(chi.URLParam(r, "attendanceId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) createSafeguardingIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		Category string `json:"category"`
		Summary  string `json:"summary"`
	}
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.CreateSafeguardingIncident(r.Context(), principal, platform.ID(chi.URLParam(r, "checkinId")), input.Category, input.Summary, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
