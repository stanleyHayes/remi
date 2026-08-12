package httpapi

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/communications"
	"remi-api/internal/chms/platform"
)

func (h *Handler) communicationPrincipal(w http.ResponseWriter, r *http.Request) (platform.Principal, bool) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
	}
	return p, ok
}
func (h *Handler) listCommunicationTemplates(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	items, err := h.services.Communications.ListTemplates(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createCommunicationTemplate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	var in communications.TemplateInput
	if err := platform.DecodeJSON(w, r, &in, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Communications.CreateTemplate(r.Context(), p, in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusCreated, v)
}
func (h *Handler) updateCommunicationTemplate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	var in communications.TemplateInput
	if err := platform.DecodeJSON(w, r, &in, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if in.ExpectedVersion == 0 {
		in.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	v, err := h.services.Communications.UpdateTemplate(r.Context(), p, platform.ID(chi.URLParam(r, "templateId")), in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}

type versionRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) publishCommunicationTemplate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	var in versionRequest
	if err := platform.DecodeJSON(w, r, &in, 4<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Communications.PublishTemplate(r.Context(), p, platform.ID(chi.URLParam(r, "templateId")), in.ExpectedVersion, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) listCommunicationCampaigns(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	items, err := h.services.Communications.ListCampaigns(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createCommunicationCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	var in communications.CampaignInput
	if err := platform.DecodeJSON(w, r, &in, 16<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Communications.CreateCampaign(r.Context(), p, in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusCreated, v)
}
func (h *Handler) getCommunicationCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	v, deliveries, err := h.services.Communications.GetCampaign(r.Context(), p, platform.ID(chi.URLParam(r, "campaignId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"campaign": v, "deliveries": deliveries})
}
func (h *Handler) submitCommunicationCampaign(w http.ResponseWriter, r *http.Request) {
	h.transitionCommunicationCampaign(w, r, "submit")
}
func (h *Handler) approveCommunicationCampaign(w http.ResponseWriter, r *http.Request) {
	h.transitionCommunicationCampaign(w, r, "approve")
}
func (h *Handler) transitionCommunicationCampaign(w http.ResponseWriter, r *http.Request, action string) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	var in versionRequest
	if err := platform.DecodeJSON(w, r, &in, 4<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	var v *communications.Campaign
	var err error
	if action == "submit" {
		v, err = h.services.Communications.SubmitCampaign(r.Context(), p, platform.ID(chi.URLParam(r, "campaignId")), in.ExpectedVersion, platform.RequestIDFrom(r.Context()))
	} else {
		v, err = h.services.Communications.ApproveCampaign(r.Context(), p, platform.ID(chi.URLParam(r, "campaignId")), in.ExpectedVersion, platform.RequestIDFrom(r.Context()))
	}
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) scheduleCommunicationCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.communicationPrincipal(w, r)
	if !ok {
		return
	}
	var in communications.ScheduleInput
	if err := platform.DecodeJSON(w, r, &in, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Communications.ScheduleCampaign(r.Context(), p, platform.ID(chi.URLParam(r, "campaignId")), in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}

// ReceiveCommunicationOptOut is public by design; authorization is the
// short-lived HMAC token embedded into the recipient's own message.
func (h *Handler) ReceiveCommunicationOptOut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := platform.DecodeJSON(w, r, &in, 8<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if err := h.services.Communications.OptOut(r.Context(), in.Token); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"status": "suppressed"})
}

func (h *Handler) ReceiveCommunicationEvent(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_body", Message: "Delivery event could not be read."}))
		return
	}
	recorded, err := h.services.Communications.RecordProviderEvent(r.Context(), chi.URLParam(r, "provider"), raw, r.Header.Get("X-REMI-Communication-Signature"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": recorded})
}
