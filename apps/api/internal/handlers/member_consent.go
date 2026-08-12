package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/consent"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/middleware"
)

func (h *Handler) memberConsentService() (consent.Service, *consent.Repository, error) {
	repository, err := consent.NewRepository(h.DB)
	if err != nil {
		return consent.Service{}, nil, err
	}
	store, err := platform.NewMongoPlatformStore(h.DB)
	if err != nil {
		return consent.Service{}, nil, err
	}
	return consent.Service{Repository: repository, Platform: store, Authorizer: platform.GrantAuthorizer{}}, repository, nil
}

func (h *Handler) staffConsentContext(r *http.Request) (platform.Principal, platform.ID, platform.ID, bool) {
	claims := middleware.ClaimsFrom(r)
	if claims == nil || claims.Role == "member" {
		return platform.Principal{}, "", "", false
	}
	personID := platform.ID(chi.URLParam(r, "personId"))
	organizationID := platform.ID(h.Cfg.CHMSOrganizationID)
	var person bson.M
	if h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": personID, "organizationId": organizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"homeBranchId": 1})).Decode(&person) != nil {
		return platform.Principal{}, "", "", false
	}
	branchID := platform.ID(stringValue(person["homeBranchId"]))
	action := "read"
	if claims.Role == "editor" || claims.Role == "super-admin" {
		action = "*"
	}
	principal := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: platform.ID(claims.UserID)}, OrganizationID: organizationID, Grants: []platform.Grant{{Action: action, Resource: "consent", BranchIDs: []platform.ID{branchID}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}}}}
	return principal, branchID, personID, true
}

func (h *Handler) AdminListPersonConsents(w http.ResponseWriter, r *http.Request) {
	p, b, personID, ok := h.staffConsentContext(r)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "person not found")
		return
	}
	service, repository, err := h.memberConsentService()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "consent preferences unavailable")
		return
	}
	items, err := service.List(r.Context(), p, b, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	suppressions, err := repository.ListSuppressions(r.Context(), p.OrganizationID, personID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "consent preferences unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"items": items, "suppressions": suppressions})
}
func (h *Handler) AdminEvaluateCommunication(w http.ResponseWriter, r *http.Request) {
	p, b, personID, ok := h.staffConsentContext(r)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "person not found")
		return
	}
	service, _, _ := h.memberConsentService()
	value, err := service.Evaluate(r.Context(), p, b, personID, r.URL.Query().Get("purpose"), r.URL.Query().Get("channel"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}
func (h *Handler) AdminApplySuppression(w http.ResponseWriter, r *http.Request) {
	p, b, personID, ok := h.staffConsentContext(r)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "person not found")
		return
	}
	var input consent.SuppressionInput
	if err := platform.DecodeJSON(w, r, &input, 1<<16); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	service, _, _ := h.memberConsentService()
	value, err := service.ApplySuppression(r.Context(), p, b, personID, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}

func (h *Handler) memberConsentContext(r *http.Request) (platform.Principal, platform.ID, platform.ID, bool) {
	claims, ok := memberClaims(r)
	if !ok {
		return platform.Principal{}, "", "", false
	}
	organizationID, personID := platform.ID(h.Cfg.CHMSOrganizationID), platform.ID(claims.PersonID)
	var person bson.M
	if h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": personID, "organizationId": organizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"homeBranchId": 1})).Decode(&person) != nil {
		return platform.Principal{}, "", "", false
	}
	branchID := platform.ID(stringValue(person["homeBranchId"]))
	if !branchID.Valid() {
		return platform.Principal{}, "", "", false
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorMember, ID: personID}, OrganizationID: organizationID}, branchID, personID, true
}

func (h *Handler) ListMemberConsents(w http.ResponseWriter, r *http.Request) {
	principal, branchID, personID, ok := h.memberConsentContext(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	service, repository, err := h.memberConsentService()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "consent preferences unavailable")
		return
	}
	items, err := service.List(r.Context(), principal, branchID, personID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	suppressions, err := repository.ListSuppressions(r.Context(), principal.OrganizationID, personID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "consent preferences unavailable")
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{"items": items, "suppressions": suppressions, "noticeVersion": "communications-2026-01"})
}

func (h *Handler) UpdateMemberConsent(w http.ResponseWriter, r *http.Request) {
	principal, branchID, personID, ok := h.memberConsentContext(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	var input consent.Choice
	if err := platform.DecodeJSON(w, r, &input, 1<<16); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	input.Source = "member-self-service"
	if input.NoticeVersion == "" {
		input.NoticeVersion = "communications-2026-01"
	}
	service, _, err := h.memberConsentService()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "consent preferences unavailable")
		return
	}
	value, err := service.Apply(r.Context(), principal, branchID, personID, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}

func (h *Handler) GetMemberCommunicationEligibility(w http.ResponseWriter, r *http.Request) {
	principal, branchID, personID, ok := h.memberConsentContext(r)
	if !ok {
		httpx.Error(w, http.StatusForbidden, "member access required")
		return
	}
	service, _, err := h.memberConsentService()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "consent preferences unavailable")
		return
	}
	value, err := service.Evaluate(r.Context(), principal, branchID, personID, r.URL.Query().Get("purpose"), r.URL.Query().Get("channel"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}
