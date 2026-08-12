package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/platform"
	"remi-api/internal/chms/reporting"
)

func (h *Handler) getReportingMetrics(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(r)
	if !ok || principal.Actor.Type != platform.ActorStaff {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Reporting catalog not found."})
		return
	}
	branchID := platform.ID(strings.TrimSpace(r.URL.Query().Get("branchId")))
	privateJSON(w, http.StatusOK, h.services.Reporting.Catalog(principal, h.organizationID, branchID))
}

type domainReportRunner struct{ services Services }

func (runner domainReportRunner) Run(ctx context.Context, p platform.Principal, query reporting.ReportQuery) (*reporting.ReportResult, error) {
	result := &reporting.ReportResult{GeneratedAt: time.Now().UTC(), Cells: []reporting.ReportCell{}}
	wanted := map[string]bool{}
	for _, id := range query.MetricIDs {
		wanted[id] = true
	}
	if wanted["attendance.confirmed_people"] || wanted["attendance.headcount"] {
		dashboard, err := runner.services.Participation.AttendanceDashboard(ctx, p, query.BranchID, query.StartsAt, query.EndsAt)
		if err != nil {
			return nil, err
		}
		watermark := ""
		if dashboard.Quality.LatestRecordedAt != nil {
			watermark = dashboard.Quality.LatestRecordedAt.UTC().Format(time.RFC3339Nano)
		}
		if wanted["attendance.confirmed_people"] {
			value := float64(dashboard.Summary.UniqueNamedPeople)
			state := "available"
			if dashboard.Summary.UniqueNamedPeople < 5 {
				state = "suppressed"
			}
			result.Cells = append(result.Cells, reporting.ReportCell{MetricID: "attendance.confirmed_people", Domain: "attendance", Unit: "people", State: state, Value: &value, AsOf: dashboard.GeneratedAt, SourceWatermark: watermark, Caveats: dashboard.Quality.Caveats})
		}
		if wanted["attendance.headcount"] {
			value := float64(dashboard.Summary.AnonymousHeadcount)
			result.Cells = append(result.Cells, reporting.ReportCell{MetricID: "attendance.headcount", Domain: "attendance", Unit: "attendances", State: "available", Value: &value, AsOf: dashboard.GeneratedAt, SourceWatermark: watermark, Caveats: dashboard.Quality.Caveats})
		}
	}
	if wanted["groups.active_connections"] || wanted["serving.fill_rate"] {
		dashboard, err := runner.services.Community.CommunityAnalytics(ctx, p, query.BranchID, query.StartsAt, query.EndsAt)
		if err != nil {
			return nil, err
		}
		watermark := ""
		if dashboard.Quality.LatestRecordedAt != nil {
			watermark = dashboard.Quality.LatestRecordedAt.UTC().Format(time.RFC3339Nano)
		}
		if wanted["groups.active_connections"] {
			value := float64(dashboard.Summary.UniqueConnectedPeople)
			state := "available"
			if dashboard.Summary.UniqueConnectedPeople < 5 {
				state = "suppressed"
			}
			result.Cells = append(result.Cells, reporting.ReportCell{MetricID: "groups.active_connections", Domain: "groups", Unit: "connections", State: state, Value: &value, AsOf: dashboard.GeneratedAt, SourceWatermark: watermark, Caveats: dashboard.Quality.Caveats})
		}
		if wanted["serving.fill_rate"] {
			cell := reporting.ReportCell{MetricID: "serving.fill_rate", Domain: "serving", Unit: "percent", State: "unavailable", AsOf: dashboard.GeneratedAt, SourceWatermark: watermark, Caveats: dashboard.Quality.Caveats}
			if dashboard.Summary.PlannedServingSlots > 0 {
				value := 100 * float64(dashboard.Summary.FilledServingSlots) / float64(dashboard.Summary.PlannedServingSlots)
				denominator := int64(dashboard.Summary.PlannedServingSlots)
				cell.State = "available"
				cell.Value = &value
				cell.Denominator = &denominator
			}
			result.Cells = append(result.Cells, cell)
		}
	}
	if wanted["care.open_cases"] {
		cases, err := runner.services.Care.ListCareCases(ctx, p, query.BranchID, false)
		if err != nil {
			return nil, err
		}
		value := float64(len(cases))
		result.Cells = append(result.Cells, reporting.ReportCell{MetricID: "care.open_cases", Domain: "care", Unit: "cases", State: "available", Value: &value, AsOf: result.GeneratedAt, Caveats: []string{"Counts only open cases assigned to the signed-in care worker."}})
	}
	if wanted["giving.net_posted"] || wanted["giving.unexplained_variance"] {
		report, err := runner.services.Finance.BuildFinanceReport(ctx, p, finance.FinanceReportInput{BranchID: query.BranchID, StartsAt: query.StartsAt, EndsAt: query.EndsAt})
		if err != nil {
			return nil, err
		}
		if wanted["giving.net_posted"] {
			value := float64(report.Totals.NetContributionMinor)
			result.Cells = append(result.Cells, reporting.ReportCell{MetricID: "giving.net_posted", Domain: "finance", Unit: "minor currency units", State: "available", Value: &value, AsOf: report.Meta.AsOf, SourceWatermark: report.Meta.SourceWatermark, Caveats: report.Meta.Caveats})
		}
		if wanted["giving.unexplained_variance"] {
			value := float64(report.Totals.UnexplainedMinor)
			result.Cells = append(result.Cells, reporting.ReportCell{MetricID: "giving.unexplained_variance", Domain: "finance", Unit: "minor currency units", State: "available", Value: &value, AsOf: report.Meta.AsOf, SourceWatermark: report.Meta.SourceWatermark, Caveats: report.Meta.Caveats})
		}
	}
	return result, nil
}

func (h *Handler) reportingService() reporting.Service {
	service := h.services.Reporting
	service.Runner = domainReportRunner{services: h.services}
	return service
}
func (h *Handler) reportingPrincipal(w http.ResponseWriter, r *http.Request) (platform.Principal, bool) {
	p, ok := h.principal(r)
	if !ok || p.Actor.Type != platform.ActorStaff {
		platform.WriteError(w, r, &platform.DomainError{Code: "not_found", Message: "Reporting workspace not found."})
		return platform.Principal{}, false
	}
	return p, true
}
func (h *Handler) runReportingQuery(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	var query reporting.ReportQuery
	if err := platform.DecodeJSON(w, r, &query, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.reportingService().RunReport(r.Context(), p, h.organizationID, query)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) listReportingViews(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	values, err := h.reportingService().ListViews(r.Context(), p, h.organizationID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) createReportingView(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	var input reporting.SavedViewInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.reportingService().SaveView(r.Context(), p, h.organizationID, input)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusCreated, value)
}
func (h *Handler) updateReportingView(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	var input reporting.SavedViewInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.reportingService().UpdateView(r.Context(), p, h.organizationID, platform.ID(chi.URLParam(r, "viewId")), input)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) listReportingExports(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	values, err := h.reportingService().ListExports(r.Context(), p, h.organizationID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) createReportingExport(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	var query reporting.ReportQuery
	if err := platform.DecodeJSON(w, r, &query, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.reportingService().RequestExport(r.Context(), p, h.organizationID, query, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusAccepted, value)
}
func (h *Handler) getReportingExport(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	value, err := h.reportingService().GetExport(r.Context(), p, h.organizationID, platform.ID(chi.URLParam(r, "exportId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, value)
}
func (h *Handler) downloadReportingExport(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	value, err := h.reportingService().GetExport(r.Context(), p, h.organizationID, platform.ID(chi.URLParam(r, "exportId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	if value.State != "completed" || len(value.Artifact) == 0 {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "The report export is not ready."})
		return
	}
	digest := sha256.Sum256(value.Artifact)
	if value.ArtifactHash == "" || hex.EncodeToString(digest[:]) != value.ArtifactHash {
		platform.WriteError(w, r, &platform.DomainError{Code: "conflict", Message: "The report artifact failed its integrity check."})
		return
	}
	w.Header().Set("Content-Type", value.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(value.FileName))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(value.Artifact)
}

func (h *Handler) ProcessDueReportingSchedule(ctx context.Context) (bool, error) {
	return h.reportingService().ProcessDueSchedule(ctx)
}

func (h *Handler) listReportingSchedules(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	values, err := h.reportingService().ListSchedules(r.Context(), p, h.organizationID)
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *Handler) createReportingSchedule(w http.ResponseWriter, r *http.Request) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return
	}
	var input reporting.ScheduleInput
	if err := platform.DecodeJSON(w, r, &input, 128<<10); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	value, err := h.reportingService().CreateSchedule(r.Context(), p, h.organizationID, input, platform.RequestIDFrom(r.Context()))
	if err != nil {
		platform.WriteError(w, r, err)
		return
	}
	privateJSON(w, http.StatusCreated, value)
}
func (h *Handler) reportingDelivery(w http.ResponseWriter, r *http.Request) (*reporting.Delivery, *reporting.BoardPackRun, bool) {
	p, ok := h.reportingPrincipal(w, r)
	if !ok {
		return nil, nil, false
	}
	delivery, run, err := h.reportingService().GetDeliveryArtifact(r.Context(), p, h.organizationID, platform.ID(chi.URLParam(r, "deliveryId")))
	if err != nil {
		platform.WriteError(w, r, err)
		return nil, nil, false
	}
	return delivery, run, true
}
func (h *Handler) getReportingDelivery(w http.ResponseWriter, r *http.Request) {
	delivery, run, ok := h.reportingDelivery(w, r)
	if !ok {
		return
	}
	privateJSON(w, http.StatusOK, map[string]any{"delivery": delivery, "run": run})
}
func (h *Handler) downloadReportingDelivery(w http.ResponseWriter, r *http.Request) {
	_, run, ok := h.reportingDelivery(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", run.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(run.FileName))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(run.Artifact)
}
