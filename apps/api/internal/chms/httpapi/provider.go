package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/platform"
)

func (h *Handler) ListPublicCampaigns(w http.ResponseWriter, r *http.Request) {
	value, err := h.services.Finance.ListPublicCampaigns(r.Context(), h.organizationID)
	respondFinanceList(w, r, value, err)
}

func (h *Handler) GetPublicCampaign(w http.ResponseWriter, r *http.Request) {
	value, err := h.services.Finance.GetPublicCampaign(r.Context(), h.organizationID, chi.URLParam(r, "slug"))
	respondFinance(w, r, value, err, http.StatusOK)
}

func (h *Handler) CreatePaystackIntent(w http.ResponseWriter, r *http.Request) {
	var input finance.PaymentIntentInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Finance.CreatePaystackIntent(r.Context(), h.organizationID, input, platform.RequestIDFrom(r.Context()), r.Header.Get("Idempotency-Key"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) ReceivePaystackWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "$", Code: "body_too_large", Message: "Webhook body is too large."}))
			return
		}
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_body", Message: "Webhook body could not be read."}))
		return
	}
	value, err := h.services.Finance.ReceivePaystackWebhook(r.Context(), h.organizationID, raw, r.Header.Get("X-Paystack-Signature"), platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listFinancePaymentIntents(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	value, err := h.services.Finance.ListPaymentIntents(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), limit)
	respondFinanceList(w, r, value, err)
}

func (h *Handler) listFinanceProviderExceptions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	value, err := h.services.Finance.ListProviderExceptions(r.Context(), p, limit)
	respondFinanceList(w, r, value, err)
}
