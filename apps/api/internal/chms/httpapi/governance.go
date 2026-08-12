package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/governance"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listDataRequests(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in again."})
		return
	}
	branch := platform.ID(strings.TrimSpace(r.URL.Query().Get("branchId")))
	values, err := h.services.Governance.List(r.Context(), p, branch, r.URL.Query().Get("status"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, values)
}
func (h *Handler) getDataRequest(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in again."})
		return
	}
	value, history, err := h.services.Governance.Get(r.Context(), p, platform.ID(chi.URLParam(r, "requestId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"request": value, "history": history})
}
func (h *Handler) transitionDataRequest(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in again."})
		return
	}
	var input governance.TransitionInput
	if err := platform.DecodeJSON(w, r, &input, 1<<16); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Governance.Move(r.Context(), p, platform.ID(chi.URLParam(r, "requestId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
