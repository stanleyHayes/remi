package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

func auditQueryFromRequest(r *http.Request) (platform.AuditQuery, error) {
	values := r.URL.Query()
	startsAt, err := time.Parse(time.RFC3339, values.Get("startsAt"))
	if err != nil {
		return platform.AuditQuery{}, platform.ValidationError(platform.FieldError{Path: "startsAt", Code: "invalid", Message: "Provide an RFC3339 audit start time."})
	}
	endsAt, err := time.Parse(time.RFC3339, values.Get("endsAt"))
	if err != nil {
		return platform.AuditQuery{}, platform.ValidationError(platform.FieldError{Path: "endsAt", Code: "invalid", Message: "Provide an RFC3339 audit end time."})
	}
	limit, _ := strconv.Atoi(values.Get("limit"))
	return platform.AuditQuery{BranchID: platform.ID(values.Get("branchId")), ActorID: platform.ID(values.Get("actorId")), Action: values.Get("action"), ResourceType: values.Get("resourceType"), ResourceID: platform.ID(values.Get("resourceId")), SubjectID: platform.ID(values.Get("subjectId")), RequestID: values.Get("requestId"), Outcome: values.Get("outcome"), StartsAt: startsAt, EndsAt: endsAt, Limit: limit, Cursor: values.Get("cursor")}, nil
}

func (h *Handler) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	query, err := auditQueryFromRequest(r)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	page, err := h.services.Audit.List(r.Context(), principal, query, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, page)
}

type auditExportInput struct {
	Query  platform.AuditQuery `json:"query"`
	Reason string              `json:"reason"`
}

func (h *Handler) exportAuditEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok {
		platform.WriteError(w, r, &platform.DomainError{Code: "unauthenticated", Message: "Sign in to continue."})
		return
	}
	var input auditExportInput
	if err := platform.DecodeJSON(w, r, &input, 64<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	artifact, err := h.services.Audit.Export(r.Context(), principal, input.Query, input.Reason, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(artifact.FileName, `"`, "")+`"`)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("X-Artifact-SHA256", artifact.ArtifactHash)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Row-Count", strconv.Itoa(artifact.RowCount))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(artifact.Bytes)
}
