package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/participation"
	"remi-api/internal/chms/platform"
)

func (h *Handler) listServiceDefinitions(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Participation.ListDefinitions(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), r.URL.Query().Get("includeInactive") == "true")
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) createServiceDefinition(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.DefinitionInput
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	created, err := h.services.Participation.CreateDefinition(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, created.Version)
	writeJSON(w, http.StatusCreated, created)
}
func (h *Handler) getServiceDefinition(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Participation.Store.FindDefinition(r.Context(), h.organizationID, platform.ID(chi.URLParam(r, "definitionId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if value == nil || !h.allowed(principal, "read", "service-definition", value.HomeBranchID, platform.FieldOperational) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Service definition not found."})
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

type definitionUpdateRequest struct {
	ExpectedVersion        int64                            `json:"expectedVersion"`
	HomeBranchID           platform.ID                      `json:"homeBranchId"`
	Name                   string                           `json:"name"`
	Description            string                           `json:"description"`
	Timezone               string                           `json:"timezone"`
	DefaultDurationMinutes int                              `json:"defaultDurationMinutes"`
	DefaultRoomIDs         []platform.ID                    `json:"defaultRoomIds"`
	DefaultCapacity        int                              `json:"defaultCapacity"`
	Recurrence             *participation.RecurrencePattern `json:"recurrence"`
	Status                 string                           `json:"status"`
}

func (h *Handler) updateServiceDefinition(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input definitionUpdateRequest
	if err := platform.DecodeJSON(w, r, &input, 256<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	updated, err := h.services.Participation.UpdateDefinition(r.Context(), principal, platform.ID(chi.URLParam(r, "definitionId")), input.ExpectedVersion, participation.DefinitionInput{HomeBranchID: input.HomeBranchID, Name: input.Name, Description: input.Description, Timezone: input.Timezone, DefaultDurationMinutes: input.DefaultDurationMinutes, DefaultRoomIDs: input.DefaultRoomIDs, DefaultCapacity: input.DefaultCapacity, Recurrence: input.Recurrence, Status: input.Status}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, updated.Version)
	writeJSON(w, http.StatusOK, updated)
}

type generationRequest struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func (h *Handler) generateOccurrences(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input generationRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	items, err := h.services.Participation.GenerateOccurrences(r.Context(), principal, platform.ID(chi.URLParam(r, "definitionId")), input.From, input.To, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "generatedOrExisting": len(items)})
}
func (h *Handler) listOccurrences(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.services.Participation.ListOccurrences(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getAttendanceAnalytics(w http.ResponseWriter, r *http.Request) {
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
	result, err := h.services.Participation.AttendanceDashboard(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")), from, to)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) createOccurrence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.OccurrenceInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	created, err := h.services.Participation.CreateOccurrence(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, created.Version)
	writeJSON(w, http.StatusCreated, created)
}
func (h *Handler) getOccurrence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Participation.Store.FindOccurrence(r.Context(), h.organizationID, platform.ID(chi.URLParam(r, "occurrenceId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if value == nil || !h.allowed(principal, "read", "occurrence", value.HomeBranchID, platform.FieldOperational) {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Occurrence not found."})
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

type occurrenceUpdateRequest struct {
	ExpectedVersion     int64         `json:"expectedVersion"`
	HomeBranchID        platform.ID   `json:"homeBranchId"`
	ServiceDefinitionID platform.ID   `json:"serviceDefinitionId"`
	Name                string        `json:"name"`
	StartsAt            time.Time     `json:"startsAt"`
	EndsAt              time.Time     `json:"endsAt"`
	Timezone            string        `json:"timezone"`
	RoomIDs             []platform.ID `json:"roomIds"`
	Capacity            int           `json:"capacity"`
}

func (h *Handler) updateOccurrence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input occurrenceUpdateRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	updated, err := h.services.Participation.UpdateOccurrence(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), input.ExpectedVersion, participation.OccurrenceInput{HomeBranchID: input.HomeBranchID, ServiceDefinitionID: input.ServiceDefinitionID, Name: input.Name, StartsAt: input.StartsAt, EndsAt: input.EndsAt, Timezone: input.Timezone, RoomIDs: input.RoomIDs, Capacity: input.Capacity}, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, updated.Version)
	writeJSON(w, http.StatusOK, updated)
}

type cancelOccurrenceRequest struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) cancelOccurrence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input cancelOccurrenceRequest
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	updated, err := h.services.Participation.CancelOccurrence(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), input.ExpectedVersion, input.Reason, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, updated.Version)
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) listAttendance(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Participation.ListAttendance(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type attendanceRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	participation.AttendanceInput
}

func (h *Handler) recordAttendance(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input attendanceRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Participation.RecordAttendance(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), platform.ID(chi.URLParam(r, "personId")), input.ExpectedVersion, input.AttendanceInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	status := http.StatusOK
	if value.Version == 1 {
		status = http.StatusCreated
	}
	writeJSON(w, status, value)
}

type attendanceCorrectionRequest struct {
	PersonID        platform.ID `json:"personId"`
	ExpectedVersion int64       `json:"expectedVersion"`
	participation.AttendanceInput
}

func (h *Handler) correctAttendance(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input attendanceCorrectionRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.CorrectAttendance(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), input.PersonID, input.ExpectedVersion, input.AttendanceInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) listAttendanceEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Participation.ListAttendanceEvents(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), platform.ID(chi.URLParam(r, "personId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listHeadcounts(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Participation.ListHeadcounts(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createHeadcount(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.HeadcountInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.CreateHeadcount(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusCreated, value)
}

type headcountUpdateRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	participation.HeadcountInput
}

func (h *Handler) updateHeadcount(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input headcountUpdateRequest
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if input.ExpectedVersion == 0 {
		input.ExpectedVersion = parseIfMatch(r.Header.Get("If-Match"))
	}
	value, err := h.services.Participation.UpdateHeadcount(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), platform.ID(chi.URLParam(r, "headcountId")), input.ExpectedVersion, input.HeadcountInput, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	setVersion(w, value.Version)
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) lockAttendance(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.LockAttendance(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")), input.Reason, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) getAttendanceControl(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	value, err := h.services.Participation.GetAttendanceControl(r.Context(), principal, platform.ID(chi.URLParam(r, "occurrenceId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *Handler) closeAttendancePeriod(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input participation.PeriodCloseInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.services.Participation.CloseAttendancePeriod(r.Context(), principal, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *Handler) listAttendancePeriods(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	items, err := h.services.Participation.ListAttendancePeriodCloses(r.Context(), principal, platform.ID(r.URL.Query().Get("branchId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func setVersion(w http.ResponseWriter, version int64) {
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(version, 10)))
}
