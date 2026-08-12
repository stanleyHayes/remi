package reporting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

var runnableMetrics = map[string]bool{
	"attendance.confirmed_people": true, "attendance.headcount": true,
	"groups.active_connections": true, "serving.fill_rate": true,
	"care.open_cases": true, "giving.net_posted": true,
	"giving.unexplained_variance": true,
}

type ReportQuery struct {
	BranchID   platform.ID `json:"branchId" bson:"branchId"`
	StartsAt   time.Time   `json:"startsAt" bson:"startsAt"`
	EndsAt     time.Time   `json:"endsAt" bson:"endsAt"`
	Timezone   string      `json:"timezone" bson:"timezone"`
	MetricIDs  []string    `json:"metricIds" bson:"metricIds"`
	Dimensions []string    `json:"dimensions" bson:"dimensions"`
}

type ReportCell struct {
	MetricID        string    `json:"metricId" bson:"metricId"`
	Domain          string    `json:"domain" bson:"domain"`
	Unit            string    `json:"unit" bson:"unit"`
	State           string    `json:"state" bson:"state"`
	Value           *float64  `json:"value,omitempty" bson:"value,omitempty"`
	Denominator     *int64    `json:"denominator,omitempty" bson:"denominator,omitempty"`
	AsOf            time.Time `json:"asOf" bson:"asOf"`
	SourceWatermark string    `json:"sourceWatermark,omitempty" bson:"sourceWatermark,omitempty"`
	Caveats         []string  `json:"caveats" bson:"caveats"`
}

type ReportResult struct {
	DictionaryVersion string       `json:"dictionaryVersion" bson:"dictionaryVersion"`
	Query             ReportQuery  `json:"query" bson:"query"`
	Cells             []ReportCell `json:"cells" bson:"cells"`
	GeneratedAt       time.Time    `json:"generatedAt" bson:"generatedAt"`
}

type SavedView struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	OwnerID        platform.ID `json:"ownerId" bson:"ownerId"`
	Name           string      `json:"name" bson:"name"`
	Query          ReportQuery `json:"query" bson:"query"`
	Version        int64       `json:"version" bson:"version"`
	CreatedAt      time.Time   `json:"createdAt" bson:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt" bson:"updatedAt"`
}

type SavedViewInput struct {
	Name            string      `json:"name"`
	Query           ReportQuery `json:"query"`
	ExpectedVersion int64       `json:"expectedVersion,omitempty"`
}

type ExportRun struct {
	ID             platform.ID  `json:"id" bson:"_id"`
	OrganizationID platform.ID  `json:"organizationId" bson:"organizationId"`
	OwnerID        platform.ID  `json:"ownerId" bson:"ownerId"`
	BranchID       platform.ID  `json:"branchId" bson:"branchId"`
	State          string       `json:"state" bson:"state"`
	Result         ReportResult `json:"result" bson:"result"`
	FileName       string       `json:"fileName,omitempty" bson:"fileName,omitempty"`
	ContentType    string       `json:"contentType,omitempty" bson:"contentType,omitempty"`
	Artifact       []byte       `json:"-" bson:"artifact,omitempty"`
	ArtifactHash   string       `json:"artifactHash,omitempty" bson:"artifactHash,omitempty"`
	RowCount       int          `json:"rowCount" bson:"rowCount"`
	ErrorCode      string       `json:"errorCode,omitempty" bson:"errorCode,omitempty"`
	CreatedAt      time.Time    `json:"createdAt" bson:"createdAt"`
	CompletedAt    *time.Time   `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
}

type BuilderRepository interface {
	InsertSavedView(context.Context, SavedView) error
	UpdateSavedView(context.Context, platform.ID, platform.ID, platform.ID, int64, SavedViewInput, time.Time) error
	FindSavedView(context.Context, platform.ID, platform.ID, platform.ID) (*SavedView, error)
	ListSavedViews(context.Context, platform.ID, platform.ID) ([]SavedView, error)
	InsertExport(context.Context, ExportRun) error
	FindExport(context.Context, platform.ID, platform.ID, platform.ID) (*ExportRun, error)
	ListExports(context.Context, platform.ID, platform.ID) ([]ExportRun, error)
	ClaimPendingExport(context.Context, time.Time) (*ExportRun, error)
	CompleteExport(context.Context, platform.ID, []byte, string, string, int, time.Time) error
	FailExport(context.Context, platform.ID, string, time.Time) error
}

type ReportRunner interface {
	Run(context.Context, platform.Principal, ReportQuery) (*ReportResult, error)
}
type EvidenceStore interface {
	WithTransaction(context.Context, func(context.Context) error) error
	AppendAudit(context.Context, platform.AuditEvent) error
}

func (s Service) validateQuery(p platform.Principal, organizationID platform.ID, query ReportQuery) error {
	if !query.BranchID.Valid() || query.StartsAt.IsZero() || query.EndsAt.IsZero() || !query.EndsAt.After(query.StartsAt) || query.EndsAt.Sub(query.StartsAt) > 366*24*time.Hour {
		return platform.ValidationError(platform.FieldError{Path: "query.range", Code: "invalid_range", Message: "Choose a branch and a reporting range no longer than 366 days."})
	}
	if _, err := time.LoadLocation(query.Timezone); err != nil {
		return platform.ValidationError(platform.FieldError{Path: "query.timezone", Code: "invalid_timezone", Message: "Choose a valid IANA timezone."})
	}
	if len(query.MetricIDs) == 0 || len(query.MetricIDs) > 12 {
		return platform.ValidationError(platform.FieldError{Path: "query.metricIds", Code: "invalid_count", Message: "Choose between one and twelve approved measures."})
	}
	if len(query.Dimensions) == 0 || len(query.Dimensions) > 2 {
		return platform.ValidationError(platform.FieldError{Path: "query.dimensions", Code: "invalid_count", Message: "Choose metric and optionally domain dimensions."})
	}
	allowedDimensions := map[string]bool{"metric": true, "domain": true}
	seenDimensions := map[string]bool{}
	for _, dimension := range query.Dimensions {
		if !allowedDimensions[dimension] || seenDimensions[dimension] {
			return platform.ValidationError(platform.FieldError{Path: "query.dimensions", Code: "unsupported", Message: "Only the approved metric and domain dimensions are available."})
		}
		seenDimensions[dimension] = true
	}
	visible := map[string]Definition{}
	for _, item := range s.Catalog(p, organizationID, query.BranchID).Items {
		visible[item.ID] = item
	}
	seen := map[string]bool{}
	for _, id := range query.MetricIDs {
		if seen[id] || !runnableMetrics[id] {
			return platform.ValidationError(platform.FieldError{Path: "query.metricIds", Code: "unsupported", Message: "Choose unique measures supported by the governed report engine."})
		}
		if _, ok := visible[id]; !ok {
			return &platform.DomainError{Code: "forbidden", Message: "A selected measure is outside your reporting access."}
		}
		seen[id] = true
	}
	return nil
}

func (s Service) RunReport(ctx context.Context, p platform.Principal, organizationID platform.ID, query ReportQuery) (*ReportResult, error) {
	if err := s.validateQuery(p, organizationID, query); err != nil {
		return nil, err
	}
	if s.Runner == nil {
		return nil, fmt.Errorf("report runner is unavailable")
	}
	timeout := s.QueryTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := s.Runner.Run(runContext, p, query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("report runner returned no result")
	}
	result.DictionaryVersion = "reporting-dictionary-v1"
	result.Query = query
	if result.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("report runner returned no generation timestamp")
	}
	wanted := make(map[string]Definition, len(query.MetricIDs))
	for _, id := range query.MetricIDs {
		definition, ok := definitionByID(id)
		if !ok {
			return nil, fmt.Errorf("metric %s has no canonical definition", id)
		}
		wanted[id] = definition
	}
	seen := make(map[string]bool, len(result.Cells))
	for i := range result.Cells {
		cell := &result.Cells[i]
		definition, ok := wanted[cell.MetricID]
		if !ok || seen[cell.MetricID] {
			return nil, fmt.Errorf("report runner returned an unexpected or duplicate metric %s", cell.MetricID)
		}
		seen[cell.MetricID] = true
		if cell.Domain != definition.Domain || cell.Unit != definition.Unit || cell.AsOf.IsZero() {
			return nil, fmt.Errorf("metric %s did not reconcile to its canonical domain, unit and timestamp", cell.MetricID)
		}
		if cell.State != "available" && cell.State != "suppressed" && cell.State != "unavailable" {
			return nil, fmt.Errorf("metric %s returned an invalid availability state", cell.MetricID)
		}
		if cell.State != "available" {
			cell.Value = nil
			cell.Denominator = nil
		}
		if cell.State == "available" && (cell.Value == nil || math.IsNaN(*cell.Value) || math.IsInf(*cell.Value, 0)) {
			return nil, fmt.Errorf("metric %s returned an empty available cell", cell.MetricID)
		}
		if cell.Denominator != nil && *cell.Denominator <= 0 {
			return nil, fmt.Errorf("metric %s returned a non-positive denominator", cell.MetricID)
		}
		if cell.Unit == "percent" && cell.Value != nil && (*cell.Value < 0 || *cell.Value > 100) {
			return nil, fmt.Errorf("metric %s returned an out-of-range percentage", cell.MetricID)
		}
		if (cell.Unit == "people" || cell.Unit == "attendances" || cell.Unit == "connections" || cell.Unit == "cases") && cell.Value != nil && (*cell.Value < 0 || math.Trunc(*cell.Value) != *cell.Value) {
			return nil, fmt.Errorf("metric %s returned an invalid count", cell.MetricID)
		}
	}
	for id := range wanted {
		if !seen[id] {
			return nil, fmt.Errorf("report runner omitted requested metric %s", id)
		}
	}
	sort.Slice(result.Cells, func(i, j int) bool { return result.Cells[i].MetricID < result.Cells[j].MetricID })
	return result, nil
}

func definitionByID(id string) (Definition, bool) {
	for _, definition := range Definitions() {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

func (s Service) SaveView(ctx context.Context, p platform.Principal, organizationID platform.ID, input SavedViewInput) (*SavedView, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 {
		return nil, platform.ValidationError(platform.FieldError{Path: "name", Code: "invalid", Message: "Enter a view name up to 100 characters."})
	}
	if err := s.validateQuery(p, organizationID, input.Query); err != nil {
		return nil, err
	}
	now := s.now()
	view := SavedView{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, OwnerID: p.Actor.ID, Name: input.Name, Query: input.Query, Version: 1, CreatedAt: now, UpdatedAt: now}
	if s.Repository == nil {
		return nil, fmt.Errorf("report repository is unavailable")
	}
	if err := s.Repository.InsertSavedView(ctx, view); err != nil {
		return nil, err
	}
	return &view, nil
}

func (s Service) UpdateView(ctx context.Context, p platform.Principal, organizationID, id platform.ID, input SavedViewInput) (*SavedView, error) {
	if input.ExpectedVersion < 1 {
		return nil, platform.ValidationError(platform.FieldError{Path: "expectedVersion", Code: "required", Message: "The current saved-view version is required."})
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 {
		return nil, platform.ValidationError(platform.FieldError{Path: "name", Code: "invalid", Message: "Enter a view name up to 100 characters."})
	}
	if err := s.validateQuery(p, organizationID, input.Query); err != nil {
		return nil, err
	}
	if err := s.Repository.UpdateSavedView(ctx, organizationID, p.Actor.ID, id, input.ExpectedVersion, input, s.now()); err != nil {
		return nil, err
	}
	return s.Repository.FindSavedView(ctx, organizationID, p.Actor.ID, id)
}

func (s Service) ListViews(ctx context.Context, p platform.Principal, organizationID platform.ID) ([]SavedView, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("report repository is unavailable")
	}
	return s.Repository.ListSavedViews(ctx, organizationID, p.Actor.ID)
}

func (s Service) RequestExport(ctx context.Context, p platform.Principal, organizationID platform.ID, query ReportQuery, requestID string) (*ExportRun, error) {
	result, err := s.RunReport(ctx, p, organizationID, query)
	if err != nil {
		return nil, err
	}
	now := s.now()
	run := ExportRun{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, OwnerID: p.Actor.ID, BranchID: query.BranchID, State: "pending", Result: *result, CreatedAt: now}
	if s.Repository == nil || s.Evidence == nil {
		return nil, fmt.Errorf("report export persistence is unavailable")
	}
	err = s.Evidence.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertExport(tx, run); err != nil {
			return err
		}
		return s.Evidence.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: query.BranchID, Actor: p.Actor, Action: "report.export.request", ResourceType: "report-export", ResourceID: run.ID, ChangedFields: []string{"query", "snapshot", "state"}, Outcome: "success", RequestID: requestID, OccurredAt: now})
	})
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s Service) ProcessPendingExport(ctx context.Context) (bool, error) {
	if s.Repository == nil {
		return false, nil
	}
	run, err := s.Repository.ClaimPendingExport(ctx, s.now())
	if err != nil || run == nil {
		return false, err
	}
	artifact, rowCount, err := reportCSV(run.Result)
	now := s.now()
	if err != nil {
		_ = s.Repository.FailExport(ctx, run.ID, "artifact_generation_failed", now)
		return true, err
	}
	digest := sha256.Sum256(artifact)
	name := fmt.Sprintf("remi-report-%s.csv", now.Format("20060102-150405"))
	return true, s.Repository.CompleteExport(ctx, run.ID, artifact, hex.EncodeToString(digest[:]), name, rowCount, now)
}

func reportCSV(result ReportResult) ([]byte, int, error) {
	buffer := &bytes.Buffer{}
	buffer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"metric_id", "domain", "state", "value", "unit", "as_of", "source_watermark"}); err != nil {
		return nil, 0, err
	}
	rows := 0
	for _, cell := range result.Cells {
		value := ""
		if cell.Value != nil {
			value = strconv.FormatFloat(*cell.Value, 'f', -1, 64)
		}
		values := []string{cell.MetricID, cell.Domain, cell.State, value, cell.Unit, cell.AsOf.UTC().Format(time.RFC3339), cell.SourceWatermark}
		for i := range values {
			values[i] = safeReportCSV(values[i])
		}
		if err := writer.Write(values); err != nil {
			return nil, 0, err
		}
		rows++
	}
	writer.Flush()
	return buffer.Bytes(), rows, writer.Error()
}
func safeReportCSV(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
