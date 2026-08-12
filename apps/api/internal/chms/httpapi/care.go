package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/care"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listWorkflowDefinitions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Care.ListDefinitions(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) createWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in care.DefinitionInput
	if err := platform.DecodeJSON(w, r, &in, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Care.CreateDefinition(r.Context(), p, in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusCreated, v)
}
func (h *Handler) getWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	v, err := h.services.Care.GetDefinition(r.Context(), p, platform.ID(chi.URLParam(r, "definitionId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) updateWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in care.DefinitionInput
	if err := platform.DecodeJSON(w, r, &in, 1<<20); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	ver := parseIfMatch(r.Header.Get("If-Match"))
	v, err := h.services.Care.UpdateDefinition(r.Context(), p, platform.ID(chi.URLParam(r, "definitionId")), ver, in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) publishWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if err := platform.DecodeJSON(w, r, &in, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Care.PublishDefinition(r.Context(), p, platform.ID(chi.URLParam(r, "definitionId")), in.ExpectedVersion, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) listWorkflowInstances(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	v, err := h.services.Care.ListInstances(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), r.URL.Query().Get("state"))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": v})
}
func (h *Handler) startWorkflowInstance(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in care.StartInput
	if err := platform.DecodeJSON(w, r, &in, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Care.Start(r.Context(), p, in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusCreated, v)
}
func (h *Handler) getWorkflowInstance(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	v, tasks, err := h.services.Care.GetInstance(r.Context(), p, platform.ID(chi.URLParam(r, "instanceId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, map[string]any{"instance": v, "tasks": tasks})
}
func (h *Handler) transitionWorkflowInstance(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in care.TransitionInput
	if err := platform.DecodeJSON(w, r, &in, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, replayed, err := h.services.Care.Transition(r.Context(), p, platform.ID(chi.URLParam(r, "instanceId")), in, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, map[string]any{"instance": v, "replayed": replayed})
}
func (h *Handler) reassignWorkflowInstance(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in struct {
		ExpectedVersion int64       `json:"expectedVersion"`
		OwnerID         platform.ID `json:"ownerId"`
		Reason          string      `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &in, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Care.Reassign(r.Context(), p, platform.ID(chi.URLParam(r, "instanceId")), in.ExpectedVersion, in.OwnerID, in.Reason, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) completeWorkflowTask(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var in struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Outcome         string `json:"outcome"`
	}
	if err := platform.DecodeJSON(w, r, &in, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	v, err := h.services.Care.CompleteTask(r.Context(), p, platform.ID(chi.URLParam(r, "taskId")), in.ExpectedVersion, in.Outcome, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}
func (h *Handler) remindWorkflowTask(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	v, err := h.services.Care.RemindTask(r.Context(), p, platform.ID(chi.URLParam(r, "taskId")), platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, v.Version)
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) listAssimilationProgress(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Care.ListAssimilationProgress(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) recordAssimilationAttendance(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input care.AssimilationInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, replayed, err := h.services.Care.RecordAssimilationVisit(r.Context(), p, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": value, "replayed": replayed})
}

func (h *Handler) listCareCases(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Care.ListCareCases(r.Context(), p, platform.ID(r.URL.Query().Get("branchId")), r.URL.Query().Get("includeClosed") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) createCareCase(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input care.CareCaseInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Care.CreateCareCase(r.Context(), p, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}
func (h *Handler) getCareCase(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, contacts, err := h.services.Care.GetCareCase(r.Context(), p, platform.ID(chi.URLParam(r, "caseId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, map[string]any{"case": value, "contactEvents": contacts})
}
func (h *Handler) assignCareCase(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input care.CareAssignmentInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Care.AssignCareCase(r.Context(), p, platform.ID(chi.URLParam(r, "caseId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}
func (h *Handler) closeCareCase(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input care.CareCloseInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Care.CloseCareCase(r.Context(), p, platform.ID(chi.URLParam(r, "caseId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}
func (h *Handler) addCareNote(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		Content         string `json:"content"`
		Classification  string `json:"classification"`
		AuthorizedGroup string `json:"authorizedGroup"`
	}
	if err := platform.DecodeJSON(w, r, &input, 32<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Care.AddRestrictedNote(r.Context(), p, platform.ID(chi.URLParam(r, "caseId")), input.Content, input.Classification, input.AuthorizedGroup, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (h *Handler) listCareNotes(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	values, err := h.services.Care.ListRestrictedNotes(r.Context(), p, platform.ID(chi.URLParam(r, "caseId")), platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) addCareContactEvent(w http.ResponseWriter, r *http.Request) {
	p, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		Channel string `json:"channel"`
		Purpose string `json:"purpose"`
		Outcome string `json:"outcome"`
		Summary string `json:"summary"`
	}
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Care.AddContactEvent(r.Context(), p, platform.ID(chi.URLParam(r, "caseId")), input.Channel, input.Purpose, input.Outcome, input.Summary, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
