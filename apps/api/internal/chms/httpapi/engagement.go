package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/engagement"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listEngagementRules(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Engagement.ListRules(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, map[string]any{"items": values, "activationEnabled": h.services.Engagement.ActivationEnabled(), "metricVersion": "retention-v1"})
}
func (h *Handler) getEngagementRule(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Engagement.GetRule(r.Context(), p, platform.ID(chi.URLParam(r, "ruleId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) createEngagementRule(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input engagement.RuleInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Engagement.CreateRule(r.Context(), p, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusCreated, value)
}
func (h *Handler) updateEngagementRule(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input engagement.RuleInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Engagement.UpdateRule(r.Context(), p, platform.ID(chi.URLParam(r, "ruleId")), input.ExpectedVersion, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) publishEngagementRule(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		ExpectedVersion int64                       `json:"expectedVersion"`
		Approval        engagement.ApprovalEvidence `json:"approval"`
	}
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Engagement.PublishRule(r.Context(), p, platform.ID(chi.URLParam(r, "ruleId")), input.ExpectedVersion, input.Approval, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) generateEngagementSignals(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		AsOf time.Time `json:"asOf"`
	}
	if err := platform.DecodeJSON(w, r, &input, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Engagement.Generate(r.Context(), p, platform.ID(chi.URLParam(r, "ruleId")), input.AsOf, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) listEngagementSignals(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Engagement.ListSignals(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), r.URL.Query().Get("state"), 100)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (h *Handler) getEngagementCohorts(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
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
	value, err := h.services.Engagement.CohortDashboard(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}

func (h *Handler) getEngagementSafetyReadiness(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Engagement.SafetyReadiness(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}

func (h *Handler) submitEngagementSafetyReview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input engagement.SafetyReviewInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Engagement.SubmitSafetyReview(r.Context(), p, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusCreated, value)
}
func (h *Handler) getEngagementSignal(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Engagement.GetSignal(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}

func (h *Handler) getEngagementReview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Engagement.GetReviewDetail(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}

func (h *Handler) assignEngagementSignal(w http.ResponseWriter, r *http.Request) {
	var input engagement.AssignInput
	h.engagementAction(w, r, &input, func(p platform.Principal) (any, error) {
		return h.services.Engagement.Assign(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")), input, platform.RequestIDFrom(r.Context()))
	})
}
func (h *Handler) snoozeEngagementSignal(w http.ResponseWriter, r *http.Request) {
	var input engagement.SnoozeInput
	h.engagementAction(w, r, &input, func(p platform.Principal) (any, error) {
		return h.services.Engagement.Snooze(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")), input, platform.RequestIDFrom(r.Context()))
	})
}
func (h *Handler) suppressEngagementSignal(w http.ResponseWriter, r *http.Request) {
	var input engagement.SuppressInput
	h.engagementAction(w, r, &input, func(p platform.Principal) (any, error) {
		return h.services.Engagement.Suppress(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")), input, platform.RequestIDFrom(r.Context()))
	})
}
func (h *Handler) recordEngagementContact(w http.ResponseWriter, r *http.Request) {
	var input engagement.ContactInput
	h.engagementAction(w, r, &input, func(p platform.Principal) (any, error) {
		return h.services.Engagement.RecordContact(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")), input, platform.RequestIDFrom(r.Context()))
	})
}
func (h *Handler) resolveEngagementSignal(w http.ResponseWriter, r *http.Request) {
	var input engagement.ResolveInput
	h.engagementAction(w, r, &input, func(p platform.Principal) (any, error) {
		return h.services.Engagement.Resolve(r.Context(), p, platform.ID(chi.URLParam(r, "signalId")), input, platform.RequestIDFrom(r.Context()))
	})
}
func (h *Handler) engagementAction(w http.ResponseWriter, r *http.Request, input any, action func(platform.Principal) (any, error)) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	if err := platform.DecodeJSON(w, r, input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := action(p)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func privateJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, status, value)
}
