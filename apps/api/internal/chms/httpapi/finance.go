package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/middleware"
)

func (h *Handler) financePrincipal(w http.ResponseWriter, r *http.Request) (platform.Principal, bool) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorStaff {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Finance configuration not found."})
		return platform.Principal{}, false
	}
	return p, true
}
func decodeFinance[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var input T
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return input, false
	}
	return input, true
}
func respondFinance(w http.ResponseWriter, r *http.Request, value any, err error, status int) {
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, status, value)
}
func respondFinanceList(w http.ResponseWriter, r *http.Request, value any, err error) {
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func statementFilter(r *http.Request) (string, platform.ID, platform.ID, int) {
	year, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	return strings.ToLower(strings.TrimSpace(r.URL.Query().Get("subjectType"))), platform.ID(strings.TrimSpace(r.URL.Query().Get("subjectId"))), platform.ID(strings.TrimSpace(r.URL.Query().Get("periodId"))), year
}

func writePrivatePDF(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *Handler) memberStatementPrincipal(w http.ResponseWriter, r *http.Request) (platform.Principal, string, platform.ID, bool) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorMember {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return platform.Principal{}, "", "", false
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" || scope == "individual" || scope == "person" {
		return p, "person", p.Actor.ID, true
	}
	claims := middleware.ClaimsFrom(r)
	if scope != "household" || claims == nil || !platform.ID(claims.HouseholdID).Valid() {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "scope", Code: "invalid", Message: "A linked household is required for household statements."}))
		return platform.Principal{}, "", "", false
	}
	return p, "household", platform.ID(claims.HouseholdID), true
}

func (h *Handler) listFinanceStatements(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	subjectType, subjectID, _, _ := statementFilter(r)
	v, e := h.services.Finance.ListStatements(r.Context(), p, subjectType, subjectID)
	respondFinanceList(w, r, v, e)
}

func (h *Handler) searchFinanceStatementSubjects(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SearchStatementSubjects(r.Context(), p, r.URL.Query().Get("q"))
	respondFinanceList(w, r, v, e)
}

func (h *Handler) generateFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.StatementInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GenerateStatement(r.Context(), p, i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}

func (h *Handler) getFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetStatement(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) downloadFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	pdf, run, err := h.services.Finance.StatementPDF(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writePrivatePDF(w, fmt.Sprintf("REMI-statement-%d-v%d.pdf", run.Year, run.StatementVersion), pdf)
}

func (h *Handler) deliverFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.DeliverStatement(r.Context(), p, platform.ID(chi.URLParam(r, "id")), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) listFinanceReceipts(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	subjectType, subjectID, periodID, year := statementFilter(r)
	v, e := h.services.Finance.ListReceipts(r.Context(), p, subjectType, subjectID, periodID, year)
	respondFinanceList(w, r, v, e)
}

func (h *Handler) downloadFinanceReceipt(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	pdf, receipt, err := h.services.Finance.ReceiptPDF(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writePrivatePDF(w, "REMI-receipt-"+receipt.ReceiptNumber+".pdf", pdf)
}

func (h *Handler) deliverFinanceReceipt(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.DeliverReceipt(r.Context(), p, platform.ID(chi.URLParam(r, "id")), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) listOwnFinanceStatements(w http.ResponseWriter, r *http.Request) {
	p, subjectType, subjectID, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListStatements(r.Context(), p, subjectType, subjectID)
	respondFinanceList(w, r, v, e)
}

func (h *Handler) generateOwnFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, subjectType, subjectID, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	var filter struct {
		PeriodID platform.ID `json:"periodId"`
		Year     int         `json:"year"`
	}
	if err := platform.DecodeJSON(w, r, &filter, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, e := h.services.Finance.GenerateStatement(r.Context(), p, finance.StatementInput{SubjectType: subjectType, SubjectID: subjectID, PeriodID: filter.PeriodID, Year: filter.Year}, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}

func (h *Handler) getOwnFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetStatement(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) downloadOwnFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	pdf, run, err := h.services.Finance.StatementPDF(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writePrivatePDF(w, fmt.Sprintf("REMI-statement-%d-v%d.pdf", run.Year, run.StatementVersion), pdf)
}

func (h *Handler) deliverOwnFinanceStatement(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.DeliverStatement(r.Context(), p, platform.ID(chi.URLParam(r, "id")), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) listOwnFinanceReceipts(w http.ResponseWriter, r *http.Request) {
	p, subjectType, subjectID, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	_, _, periodID, year := statementFilter(r)
	v, e := h.services.Finance.ListReceipts(r.Context(), p, subjectType, subjectID, periodID, year)
	respondFinanceList(w, r, v, e)
}

func (h *Handler) downloadOwnFinanceReceipt(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	pdf, receipt, err := h.services.Finance.ReceiptPDF(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writePrivatePDF(w, "REMI-receipt-"+receipt.ReceiptNumber+".pdf", pdf)
}

func (h *Handler) deliverOwnFinanceReceipt(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberStatementPrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.DeliverReceipt(r.Context(), p, platform.ID(chi.URLParam(r, "id")), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) listFinanceFunds(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListFunds(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) listFinanceCampaigns(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListCampaigns(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinanceCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.CampaignInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveCampaign(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinanceCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.CampaignInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveCampaign(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinancePledges(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListPledges(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.PledgeInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SavePledge(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) getFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetPledge(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) updateFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.PledgeInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SavePledge(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) remindFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.PledgeReminderInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SendPledgeReminder(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusAccepted)
}
func (h *Handler) listFinanceSettlements(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListSettlements(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) importFinanceSettlement(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.SettlementInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ImportSettlement(r.Context(), p, i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) getFinanceSettlement(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetSettlement(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinanceReconciliationItems(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListReconciliationItems(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinanceList(w, r, v, e)
}
func (h *Handler) listFinanceReconciliationOwners(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListReconciliationOwners(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) resolveFinanceReconciliationItem(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.ReconciliationResolutionInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ResolveReconciliationItem(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) closeFinancePeriod(w http.ResponseWriter, r *http.Request) {
	h.controlFinancePeriod(w, r, "close")
}
func (h *Handler) reopenFinancePeriod(w http.ResponseWriter, r *http.Request) {
	h.controlFinancePeriod(w, r, "reopen")
}
func (h *Handler) controlFinancePeriod(w http.ResponseWriter, r *http.Request, action string) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.PeriodControlInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ControlPeriod(r.Context(), p, platform.ID(chi.URLParam(r, "id")), action, i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinancePeriodControlRequests(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListPeriodControlRequests(r.Context(), p, platform.ID(r.URL.Query().Get("periodId")))
	respondFinanceList(w, r, v, e)
}
func (h *Handler) listOwnFinancePledges(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorMember {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	v, e := h.services.Finance.ListPledges(r.Context(), p)
	respondFinanceList(w, r, v, e)
}

type memberGivingIntentInput struct {
	AmountMinor       int64       `json:"amountMinor"`
	FundID            platform.ID `json:"fundId"`
	CampaignID        platform.ID `json:"campaignId,omitempty"`
	PledgeID          platform.ID `json:"pledgeId,omitempty"`
	AttributionScope  string      `json:"attributionScope"`
	SavePaymentMethod bool        `json:"savePaymentMethod,omitempty"`
}

type recurringStateInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	State           string `json:"state"`
}

func (h *Handler) memberGivingContext(w http.ResponseWriter, r *http.Request, scope string) (platform.Principal, *people.Person, finance.DonorAttribution, bool) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorMember || h.services.Repository == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to give."})
		return platform.Principal{}, nil, finance.DonorAttribution{}, false
	}
	person, err := h.services.Repository.FindByID(r.Context(), p.OrganizationID, p.Actor.ID)
	if err != nil || person == nil {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Your member profile is unavailable."})
		return platform.Principal{}, nil, finance.DonorAttribution{}, false
	}
	donor := finance.DonorAttribution{Type: "person", PersonID: p.Actor.ID}
	if strings.EqualFold(strings.TrimSpace(scope), "household") {
		claims := middleware.ClaimsFrom(r)
		if claims == nil || h.services.Households == nil || !platform.ID(claims.HouseholdID).Valid() {
			platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "attributionScope", Code: "household_unavailable", Message: "A linked household is required."}))
			return platform.Principal{}, nil, finance.DonorAttribution{}, false
		}
		profile, profileErr := h.services.Households.FindProfileByHouseholdID(r.Context(), p.OrganizationID, platform.ID(claims.HouseholdID))
		if profileErr != nil || profile == nil || profile.PrimaryContactID != p.Actor.ID {
			platform.WriteError(w, r, &platform.DomainError{Code: "forbidden", Message: "Only the household primary contact can attribute a gift to the household."})
			return platform.Principal{}, nil, finance.DonorAttribution{}, false
		}
		donor = finance.DonorAttribution{Type: "household", HouseholdID: profile.ID}
	}
	return p, person, donor, true
}

func (h *Handler) listOwnFinanceCampaigns(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberGivingContext(w, r, "person")
	if !ok {
		return
	}
	values, err := h.services.Finance.ListPublicCampaigns(r.Context(), p.OrganizationID)
	respondFinanceList(w, r, values, err)
}

func (h *Handler) createOwnGivingIntent(w http.ResponseWriter, r *http.Request) {
	var input memberGivingIntentInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	p, person, donor, ok := h.memberGivingContext(w, r, input.AttributionScope)
	if !ok {
		return
	}
	if input.SavePaymentMethod && donor.Type != "person" {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "savePaymentMethod", Code: "person_only", Message: "Save a payment method with an individual gift."}))
		return
	}
	result, err := h.services.Finance.CreateMemberPaystackIntent(r.Context(), p.OrganizationID, p.Actor.ID, finance.PaymentIntentInput{Email: middleware.ClaimsFrom(r).Email, AmountMinor: input.AmountMinor, Currency: "GHS", BranchID: person.HomeBranchID, FundID: input.FundID, CampaignID: input.CampaignID, PledgeID: input.PledgeID, Donor: donor, SavePaymentMethod: input.SavePaymentMethod, CallbackURL: strings.TrimRight(h.services.Finance.MemberAppURL, "/") + "/giving"}, platform.RequestIDFrom(r.Context()), strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	respondFinance(w, r, result, err, http.StatusCreated)
}

func (h *Handler) listOwnPaymentMethods(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberGivingContext(w, r, "person")
	if !ok {
		return
	}
	values, err := h.services.Finance.ListMemberPaymentMethods(r.Context(), p.OrganizationID, p.Actor.ID)
	respondFinanceList(w, r, values, err)
}

func (h *Handler) disableOwnPaymentMethod(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberGivingContext(w, r, "person")
	if !ok {
		return
	}
	err := h.services.Finance.DisableMemberPaymentMethod(r.Context(), p.OrganizationID, p.Actor.ID, platform.ID(chi.URLParam(r, "id")), platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listOwnRecurringInstructions(w http.ResponseWriter, r *http.Request) {
	p, _, _, ok := h.memberGivingContext(w, r, "person")
	if !ok {
		return
	}
	values, err := h.services.Finance.ListRecurringInstructions(r.Context(), p.OrganizationID, p.Actor.ID)
	respondFinanceList(w, r, values, err)
}

func (h *Handler) createOwnRecurringInstruction(w http.ResponseWriter, r *http.Request) {
	var input finance.RecurringInstructionInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	p, person, donor, ok := h.memberGivingContext(w, r, input.Donor.Type)
	if !ok {
		return
	}
	input.Donor, input.BranchID = donor, person.HomeBranchID
	value, err := h.services.Finance.CreateRecurringInstruction(r.Context(), p.OrganizationID, p.Actor.ID, input, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, value, err, http.StatusCreated)
}

func (h *Handler) updateOwnRecurringInstruction(w http.ResponseWriter, r *http.Request) {
	var input recurringStateInput
	if err := platform.DecodeJSON(w, r, &input, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	p, _, _, ok := h.memberGivingContext(w, r, "person")
	if !ok {
		return
	}
	value, err := h.services.Finance.UpdateRecurringInstructionState(r.Context(), p.OrganizationID, p.Actor.ID, platform.ID(chi.URLParam(r, "id")), input.ExpectedVersion, input.State, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, value, err, http.StatusOK)
}
func (h *Handler) createOwnFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorMember {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	i, ok := decodeFinance[finance.PledgeInput](w, r)
	if !ok {
		return
	}
	i.Donor = finance.DonorAttribution{Type: "person", PersonID: p.Actor.ID}
	if i.Reminder.OptedIn {
		i.Reminder.RecipientPersonID = p.Actor.ID
	}
	v, e := h.services.Finance.SavePledge(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) getOwnFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorMember {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	v, e := h.services.Finance.GetPledge(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) updateOwnFinancePledge(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorMember {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	i, ok := decodeFinance[finance.PledgeInput](w, r)
	if !ok {
		return
	}
	if i.Reminder.OptedIn {
		i.Reminder.RecipientPersonID = p.Actor.ID
	}
	v, e := h.services.Finance.SavePledge(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) createFinanceFund(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.FundInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveFund(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinanceFund(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.FundInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveFund(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) deactivateFinanceFund(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.DeactivateFundInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.DeactivateFund(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinancePaymentMethods(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListPaymentMethods(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinancePaymentMethod(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.PaymentMethodInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SavePaymentMethod(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinancePaymentMethod(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.PaymentMethodInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SavePaymentMethod(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinanceCampuses(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListCampuses(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinanceCampus(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.CampusSettingsInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveCampus(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinanceCampus(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.CampusSettingsInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveCampus(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinancePeriods(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListPeriods(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinancePeriod(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.FiscalPeriodInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SavePeriod(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinancePeriod(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.FiscalPeriodInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SavePeriod(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinanceSequences(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListSequences(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinanceSequence(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.ReceiptSequenceInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveSequence(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinanceSequence(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.ReceiptSequenceInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveSequence(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) listFinanceMappings(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListMappings(r.Context(), p)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinanceMapping(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.AccountMappingInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveMapping(r.Context(), p, "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinanceMapping(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.AccountMappingInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveMapping(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) listFinanceContributions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	v, e := h.services.Finance.ListContributions(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), limit)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) postFinanceContribution(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.ContributionInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.PostContribution(r.Context(), p, i, platform.RequestIDFrom(r.Context()), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) getFinanceContribution(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetContribution(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) adjustFinanceContribution(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.AdjustmentInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.AdjustContribution(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusCreated)
}

type attributionCorrectionInput struct {
	Donor  finance.DonorAttribution `json:"donor"`
	Reason string                   `json:"reason"`
}

func (h *Handler) correctFinanceContributionAttribution(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[attributionCorrectionInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.CorrectAttribution(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i.Donor, i.Reason, platform.RequestIDFrom(r.Context()), r.Header.Get("Idempotency-Key"))
	respondFinance(w, r, v, e, http.StatusCreated)
}

func financeReportInputFromQuery(r *http.Request) (finance.FinanceReportInput, error) {
	input := finance.FinanceReportInput{BranchID: platform.ID(strings.TrimSpace(r.URL.Query().Get("branchId"))), PeriodID: platform.ID(strings.TrimSpace(r.URL.Query().Get("periodId")))}
	if value := strings.TrimSpace(r.URL.Query().Get("startsAt")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return input, fmt.Errorf("startsAt must be RFC3339")
		}
		input.StartsAt = parsed
	}
	if value := strings.TrimSpace(r.URL.Query().Get("endsAt")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return input, fmt.Errorf("endsAt must be RFC3339")
		}
		input.EndsAt = parsed
	}
	return input, nil
}

func (h *Handler) getFinanceReport(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	input, err := financeReportInputFromQuery(r)
	if err != nil {
		platform.WriteError(w, r, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_report_filter", Message: err.Error()}))
		return
	}
	v, e := h.services.Finance.BuildFinanceReport(r.Context(), p, input)
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) createFinanceExportRun(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.FinanceExportInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.CreateFinanceExport(r.Context(), p, i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}

func (h *Handler) listFinanceExportRuns(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ListFinanceExports(r.Context(), p, platform.ID(strings.TrimSpace(r.URL.Query().Get("branchId"))))
	respondFinanceList(w, r, v, e)
}

func (h *Handler) getFinanceExportRun(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetFinanceExport(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	if v != nil {
		v.Artifact = nil
	}
	respondFinance(w, r, v, e, http.StatusOK)
}

func (h *Handler) downloadFinanceExportRun(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, err := h.services.Finance.GetFinanceExport(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", v.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, v.FileName))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(v.Artifact)
}

func (h *Handler) listFinanceBatches(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	v, e := h.services.Finance.ListBatches(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), limit)
	respondFinanceList(w, r, v, e)
}
func (h *Handler) createFinanceBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.CountingBatchInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.CreateBatch(r.Context(), p, i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) getFinanceBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.GetBatch(r.Context(), p, platform.ID(chi.URLParam(r, "id")))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) createFinanceBatchEntry(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.BatchEntryInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveBatchEntry(r.Context(), p, platform.ID(chi.URLParam(r, "id")), "", i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusCreated)
}
func (h *Handler) updateFinanceBatchEntry(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.BatchEntryInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.SaveBatchEntry(r.Context(), p, platform.ID(chi.URLParam(r, "id")), platform.ID(chi.URLParam(r, "entryId")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}
func (h *Handler) confirmFinanceBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[finance.CountConfirmationInput](w, r)
	if !ok {
		return
	}
	v, e := h.services.Finance.ConfirmBatchCount(r.Context(), p, platform.ID(chi.URLParam(r, "id")), i, platform.RequestIDFrom(r.Context()))
	respondFinance(w, r, v, e, http.StatusOK)
}

type batchTransitionRequest struct {
	Action          string `json:"action"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) transitionFinanceBatch(w http.ResponseWriter, r *http.Request) {
	p, ok := h.financePrincipal(w, r)
	if !ok {
		return
	}
	i, ok := decodeFinance[batchTransitionRequest](w, r)
	if !ok {
		return
	}
	input := finance.BatchTransitionInput{ExpectedVersion: i.ExpectedVersion, Reason: i.Reason}
	var value *finance.CountingBatch
	var err error
	switch i.Action {
	case "start-counting":
		value, err = h.services.Finance.StartBatch(r.Context(), p, platform.ID(chi.URLParam(r, "id")), input, platform.RequestIDFrom(r.Context()))
	case "reset-count":
		value, err = h.services.Finance.ResetBatchCount(r.Context(), p, platform.ID(chi.URLParam(r, "id")), input, platform.RequestIDFrom(r.Context()))
	case "approve":
		value, err = h.services.Finance.ApproveBatch(r.Context(), p, platform.ID(chi.URLParam(r, "id")), input, platform.RequestIDFrom(r.Context()))
	case "post":
		value, err = h.services.Finance.PostBatch(r.Context(), p, platform.ID(chi.URLParam(r, "id")), input, platform.RequestIDFrom(r.Context()))
	default:
		err = platform.ValidationError(platform.FieldError{Path: "action", Code: "unsupported", Message: "Unsupported batch transition."})
	}
	respondFinance(w, r, value, err, http.StatusOK)
}
