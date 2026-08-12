package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/communications"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listCommunicationAudiences(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Communications.List(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createCommunicationAudience(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in communications.SaveInput
	if err := platform.DecodeJSON(w, r, &in, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Communications.Create(r.Context(), p, in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusCreated, v)
}
func (h *Handler) updateCommunicationAudience(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in communications.SaveInput
	if err := platform.DecodeJSON(w, r, &in, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if in.ExpectedVersion == 0 {
		in.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	v, err := h.services.Communications.Update(r.Context(), p, platform.ID(chi.URLParam(r, "audienceId")), in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) previewCommunicationAudience(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	v, _, err := h.services.Communications.Preview(r.Context(), p, platform.ID(chi.URLParam(r, "audienceId")), limit)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, v)
}

type audienceExportRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) exportCommunicationAudience(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in audienceExportRequest
	if err := platform.DecodeJSON(w, r, &in, 16<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	var out bytes.Buffer
	record, err := h.services.Communications.Export(r.Context(), p, platform.ID(chi.URLParam(r, "audienceId")), in.Reason, platform.RequestIDFrom(r.Context()), &out)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=audience-%s.csv", record.AudienceID))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Export-Audit-ID", string(record.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
}
func (h *Handler) listCommunicationAudienceExports(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Communications.ListExports(r.Context(), p, platform.ID(chi.URLParam(r, "audienceId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
