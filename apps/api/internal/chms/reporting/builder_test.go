package reporting

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

type builderRunner struct{ result ReportResult }

func (r builderRunner) Run(context.Context, platform.Principal, ReportQuery) (*ReportResult, error) {
	copy := r.result
	return &copy, nil
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ platform.Principal, _ ReportQuery) (*ReportResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type builderRepo struct {
	views      []SavedView
	exports    []ExportRun
	schedules  []Schedule
	runs       []BoardPackRun
	deliveries []Delivery
	recipients []ApprovedRecipient
}

func (r *builderRepo) InsertSavedView(_ context.Context, v SavedView) error {
	r.views = append(r.views, v)
	return nil
}
func (r *builderRepo) UpdateSavedView(_ context.Context, org, owner, id platform.ID, version int64, in SavedViewInput, now time.Time) error {
	for i := range r.views {
		if r.views[i].ID == id && r.views[i].OrganizationID == org && r.views[i].OwnerID == owner && r.views[i].Version == version {
			r.views[i].Name = in.Name
			r.views[i].Query = in.Query
			r.views[i].Version++
			r.views[i].UpdatedAt = now
			return nil
		}
	}
	return platform.VersionConflict(version)
}
func (r *builderRepo) FindSavedView(_ context.Context, org, owner, id platform.ID) (*SavedView, error) {
	for _, v := range r.views {
		if v.ID == id && v.OrganizationID == org && v.OwnerID == owner {
			copy := v
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *builderRepo) ListSavedViews(_ context.Context, org, owner platform.ID) ([]SavedView, error) {
	out := []SavedView{}
	for _, v := range r.views {
		if v.OrganizationID == org && v.OwnerID == owner {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *builderRepo) InsertExport(_ context.Context, v ExportRun) error {
	r.exports = append(r.exports, v)
	return nil
}
func (r *builderRepo) FindExport(_ context.Context, org, owner, id platform.ID) (*ExportRun, error) {
	for _, v := range r.exports {
		if v.ID == id && v.OrganizationID == org && v.OwnerID == owner {
			copy := v
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *builderRepo) ListExports(_ context.Context, org, owner platform.ID) ([]ExportRun, error) {
	out := []ExportRun{}
	for _, v := range r.exports {
		if v.OrganizationID == org && v.OwnerID == owner {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *builderRepo) ClaimPendingExport(_ context.Context, _ time.Time) (*ExportRun, error) {
	for i := range r.exports {
		if r.exports[i].State == "pending" {
			r.exports[i].State = "processing"
			copy := r.exports[i]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *builderRepo) CompleteExport(_ context.Context, id platform.ID, artifact []byte, hash, name string, rows int, now time.Time) error {
	for i := range r.exports {
		if r.exports[i].ID == id {
			r.exports[i].State = "completed"
			r.exports[i].Artifact = artifact
			r.exports[i].ArtifactHash = hash
			r.exports[i].FileName = name
			r.exports[i].RowCount = rows
			r.exports[i].CompletedAt = &now
		}
	}
	return nil
}
func (r *builderRepo) FailExport(context.Context, platform.ID, string, time.Time) error { return nil }

func (r *builderRepo) FindStaffRecipients(_ context.Context, _ platform.ID, ids []platform.ID) ([]ApprovedRecipient, error) {
	wanted := map[platform.ID]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	out := []ApprovedRecipient{}
	for _, recipient := range r.recipients {
		if wanted[recipient.UserID] {
			out = append(out, recipient)
		}
	}
	return out, nil
}
func (r *builderRepo) InsertSchedule(_ context.Context, value Schedule) error {
	r.schedules = append(r.schedules, value)
	return nil
}
func (r *builderRepo) ListSchedules(_ context.Context, org, owner platform.ID) ([]Schedule, error) {
	out := []Schedule{}
	for _, value := range r.schedules {
		if value.OrganizationID == org && value.OwnerID == owner {
			out = append(out, value)
		}
	}
	return out, nil
}
func (r *builderRepo) ClaimDueSchedule(_ context.Context, now time.Time) (*Schedule, error) {
	for i := range r.schedules {
		if r.schedules[i].State == "active" && !r.schedules[i].NextRunAt.After(now) {
			copy := r.schedules[i]
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *builderRepo) CompleteSchedule(_ context.Context, id platform.ID, last, next time.Time) error {
	for i := range r.schedules {
		if r.schedules[i].ID == id {
			r.schedules[i].LastRunAt = &last
			r.schedules[i].NextRunAt = next
			r.schedules[i].Version++
		}
	}
	return nil
}
func (r *builderRepo) PauseSchedule(_ context.Context, id platform.ID, code string, _ time.Time) error {
	for i := range r.schedules {
		if r.schedules[i].ID == id {
			r.schedules[i].State = "paused"
			r.schedules[i].LastErrorCode = code
		}
	}
	return nil
}
func (r *builderRepo) InsertBoardPackRun(_ context.Context, value BoardPackRun) error {
	r.runs = append(r.runs, value)
	return nil
}
func (r *builderRepo) InsertDelivery(_ context.Context, value Delivery) error {
	r.deliveries = append(r.deliveries, value)
	return nil
}
func (r *builderRepo) MarkDeliverySent(_ context.Context, id platform.ID, now time.Time) error {
	for i := range r.deliveries {
		if r.deliveries[i].ID == id {
			r.deliveries[i].State = "delivered"
			r.deliveries[i].DeliveredAt = &now
		}
	}
	return nil
}
func (r *builderRepo) MarkDeliveryFailed(_ context.Context, id platform.ID, now time.Time) error {
	for i := range r.deliveries {
		if r.deliveries[i].ID == id {
			r.deliveries[i].State = "failed"
			r.deliveries[i].DeliveredAt = &now
		}
	}
	return nil
}
func (r *builderRepo) FindDelivery(_ context.Context, org, recipient, id platform.ID) (*Delivery, error) {
	for _, value := range r.deliveries {
		if value.ID == id && value.OrganizationID == org && value.RecipientUserID == recipient {
			copy := value
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *builderRepo) FindBoardPackRun(_ context.Context, org, id platform.ID) (*BoardPackRun, error) {
	for _, value := range r.runs {
		if value.ID == id && value.OrganizationID == org {
			copy := value
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *builderRepo) MarkDeliveryOpened(_ context.Context, id platform.ID, now time.Time) error {
	for i := range r.deliveries {
		if r.deliveries[i].ID == id {
			r.deliveries[i].State = "opened"
			r.deliveries[i].OpenedAt = &now
		}
	}
	return nil
}
func (r *builderRepo) MarkDeliveryExpired(_ context.Context, id platform.ID, _ time.Time) error {
	for i := range r.deliveries {
		if r.deliveries[i].ID == id {
			r.deliveries[i].State = "expired"
		}
	}
	return nil
}

type builderPrincipals struct {
	principal platform.Principal
	active    bool
}

func (r builderPrincipals) ResolveReportingPrincipal(context.Context, platform.ID, platform.ID) (platform.Principal, bool, error) {
	return r.principal, r.active, nil
}

type builderSender struct{ sent []string }

func (s *builderSender) Send(to, _, _ string) error { s.sent = append(s.sent, to); return nil }

type builderEvidence struct{ audits []platform.AuditEvent }

func (e *builderEvidence) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (e *builderEvidence) AppendAudit(_ context.Context, event platform.AuditEvent) error {
	e.audits = append(e.audits, event)
	return nil
}

func builderPrincipal(actor string) platform.Principal {
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: platform.ID(actor)}, OrganizationID: "org-1", Grants: []platform.Grant{{Action: "read", Resource: "attendance", BranchIDs: []platform.ID{"branch-1"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}}}
}
func builderQuery() ReportQuery {
	return ReportQuery{BranchID: "branch-1", StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Timezone: "Africa/Accra", MetricIDs: []string{"attendance.confirmed_people"}, Dimensions: []string{"metric"}}
}

func TestBuilderRejectsArbitraryDimensionsMetricsAndPermissions(t *testing.T) {
	service := Service{Authorizer: platform.GrantAuthorizer{}}
	p := builderPrincipal("owner-1")
	query := builderQuery()
	query.Dimensions = []string{"person"}
	if service.validateQuery(p, "org-1", query) == nil {
		t.Fatal("arbitrary person dimension accepted")
	}
	query = builderQuery()
	query.MetricIDs = []string{"people.active"}
	if service.validateQuery(p, "org-1", query) == nil {
		t.Fatal("unsupported engine metric accepted")
	}
	query = builderQuery()
	query.MetricIDs = []string{"giving.net_posted"}
	if service.validateQuery(p, "org-1", query) == nil {
		t.Fatal("financial metric disclosed")
	}
}
func TestRunReportStripsSuppressedValues(t *testing.T) {
	value := float64(2)
	denominator := int64(3)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	service := Service{Authorizer: platform.GrantAuthorizer{}, Runner: builderRunner{result: ReportResult{GeneratedAt: now, Cells: []ReportCell{{MetricID: "attendance.confirmed_people", Domain: "attendance", Unit: "people", State: "suppressed", Value: &value, Denominator: &denominator, AsOf: now}}}}}
	result, err := service.RunReport(context.Background(), builderPrincipal("owner-1"), "org-1", builderQuery())
	if err != nil {
		t.Fatal(err)
	}
	if result.Cells[0].Value != nil || result.Cells[0].Denominator != nil {
		t.Fatal("suppressed cell retained private values")
	}
}

func TestReportQueryTimezoneRangeAndPerformanceBudget(t *testing.T) {
	service := Service{Authorizer: platform.GrantAuthorizer{}}
	principal := builderPrincipal("owner-1")
	query := builderQuery()
	query.Timezone = "Africa/Accra"
	query.EndsAt = query.StartsAt.Add(366 * 24 * time.Hour)
	if err := service.validateQuery(principal, "org-1", query); err != nil {
		t.Fatalf("exact 366-day Accra range rejected: %v", err)
	}
	query.EndsAt = query.EndsAt.Add(time.Nanosecond)
	if err := service.validateQuery(principal, "org-1", query); err == nil {
		t.Fatal("over-budget range accepted")
	}
	query = builderQuery()
	query.Timezone = "Africa/Not-A-Place"
	if err := service.validateQuery(principal, "org-1", query); err == nil {
		t.Fatal("invalid IANA timezone accepted")
	}
	query = builderQuery()
	started := time.Now()
	service.Runner = blockingRunner{}
	service.QueryTimeout = 20 * time.Millisecond
	if _, err := service.RunReport(context.Background(), principal, "org-1", query); err == nil {
		t.Fatal("report runner exceeded budget without cancellation")
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("query budget cancellation took %s", elapsed)
	}
}

func TestReportResultReconcilesCanonicalCellsAndEmptyStates(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	principal := builderPrincipal("owner-1")
	query := builderQuery()
	value := float64(5)
	cases := []struct {
		name   string
		result ReportResult
	}{
		{name: "omitted metric", result: ReportResult{GeneratedAt: now, Cells: []ReportCell{}}},
		{name: "duplicate metric", result: ReportResult{GeneratedAt: now, Cells: []ReportCell{{MetricID: query.MetricIDs[0], Domain: "attendance", Unit: "people", State: "available", Value: &value, AsOf: now}, {MetricID: query.MetricIDs[0], Domain: "attendance", Unit: "people", State: "available", Value: &value, AsOf: now}}}},
		{name: "wrong unit", result: ReportResult{GeneratedAt: now, Cells: []ReportCell{{MetricID: query.MetricIDs[0], Domain: "attendance", Unit: "currency", State: "available", Value: &value, AsOf: now}}}},
		{name: "negative count", result: ReportResult{GeneratedAt: now, Cells: []ReportCell{{MetricID: query.MetricIDs[0], Domain: "attendance", Unit: "people", State: "available", Value: floatPointer(-1), AsOf: now}}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			service := Service{Authorizer: platform.GrantAuthorizer{}, Runner: builderRunner{result: test.result}}
			if _, err := service.RunReport(context.Background(), principal, "org-1", query); err == nil {
				t.Fatal("unreconciled result accepted")
			}
		})
	}
	service := Service{Authorizer: platform.GrantAuthorizer{}, Runner: builderRunner{result: ReportResult{GeneratedAt: now, Cells: []ReportCell{{MetricID: query.MetricIDs[0], Domain: "attendance", Unit: "people", State: "unavailable", AsOf: now}}}}}
	result, err := service.RunReport(context.Background(), principal, "org-1", query)
	if err != nil || result.Cells[0].Value != nil || result.Cells[0].State != "unavailable" {
		t.Fatalf("valid empty state rejected: result=%+v err=%v", result, err)
	}
}

func floatPointer(value float64) *float64 { return &value }
func TestSavedViewsAndExportsAreOwnerScopedVersionedAndAudited(t *testing.T) {
	repo := &builderRepo{}
	evidence := &builderEvidence{}
	value := float64(7)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	service := Service{Authorizer: platform.GrantAuthorizer{}, Repository: repo, Evidence: evidence, Runner: builderRunner{result: ReportResult{GeneratedAt: now, Cells: []ReportCell{{MetricID: "attendance.confirmed_people", Domain: "attendance", Unit: "people", State: "available", Value: &value, AsOf: now}}}}, Now: func() time.Time { return now }}
	owner := builderPrincipal("owner-1")
	view, err := service.SaveView(context.Background(), owner, "org-1", SavedViewInput{Name: "Sunday pulse", Query: builderQuery()})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.UpdateView(context.Background(), owner, "org-1", view.ID, SavedViewInput{Name: "Weekend pulse", Query: builderQuery(), ExpectedVersion: 1})
	if err != nil || view.Version != 2 {
		t.Fatalf("update=%+v err=%v", view, err)
	}
	other := builderPrincipal("owner-2")
	views, _ := service.ListViews(context.Background(), other, "org-1")
	if len(views) != 0 {
		t.Fatal("saved view crossed owner boundary")
	}
	run, err := service.RequestExport(context.Background(), owner, "org-1", builderQuery(), "request-1")
	if err != nil || run.State != "pending" || len(evidence.audits) != 1 {
		t.Fatalf("run=%+v audits=%d err=%v", run, len(evidence.audits), err)
	}
	processed, err := service.ProcessPendingExport(context.Background())
	if err != nil || !processed || repo.exports[0].State != "completed" || repo.exports[0].ArtifactHash == "" {
		t.Fatalf("processed=%v run=%+v err=%v", processed, repo.exports[0], err)
	}
	if !strings.HasPrefix(string(repo.exports[0].Artifact), "\ufeffmetric_id") {
		t.Fatal("CSV header missing")
	}
	if _, err = service.GetExport(context.Background(), other, "org-1", repo.exports[0].ID); err == nil {
		t.Fatal("export crossed owner boundary")
	}
	revoked := owner
	revoked.Grants = nil
	if _, err = service.GetExport(context.Background(), revoked, "org-1", repo.exports[0].ID); err == nil {
		t.Fatal("export remained downloadable after metric permission revocation")
	}
}
func TestReportCSVPreventsFormulaInjection(t *testing.T) {
	value := float64(1)
	body, _, err := reportCSV(ReportResult{Cells: []ReportCell{{MetricID: "=CMD()", Domain: "+evil", State: "available", Value: &value, Unit: "people", AsOf: time.Now()}}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "\n=CMD") || strings.Contains(text, ",+evil,") {
		t.Fatalf("unsafe CSV: %q", text)
	}
}

func TestScheduledBoardPackReauthorizesDeliversAndExpires(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	owner := builderPrincipal("owner-1")
	recipient := builderPrincipal("recipient-1")
	repo := &builderRepo{recipients: []ApprovedRecipient{{UserID: recipient.Actor.ID, Email: "board@remi.church", Name: "Board member"}}}
	evidence := &builderEvidence{}
	sender := &builderSender{}
	value := float64(8)
	service := Service{Authorizer: platform.GrantAuthorizer{}, Repository: repo, Evidence: evidence, Runner: builderRunner{result: ReportResult{DictionaryVersion: "reporting-dictionary-v1", GeneratedAt: now, Query: builderQuery(), Cells: []ReportCell{{MetricID: "attendance.confirmed_people", Domain: "attendance", Unit: "people", State: "available", Value: &value, AsOf: now}}}}, Principals: builderPrincipals{principal: owner, active: true}, Sender: sender, AdminAppURL: "https://admin.remi.church", Now: func() time.Time { return now }}
	view, err := service.SaveView(context.Background(), owner, "org-1", SavedViewInput{Name: "Sunday pulse", Query: builderQuery()})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := service.CreateSchedule(context.Background(), owner, "org-1", ScheduleInput{SavedViewID: view.ID, SavedViewVersion: view.Version, Name: "Leadership pack", Cadence: "weekly", Format: "board-pack", RecipientUserIDs: []platform.ID{recipient.Actor.ID}, NextRunAt: now.Add(time.Minute)}, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	repo.schedules[0].NextRunAt = now.Add(-time.Minute)
	processed, err := service.ProcessDueSchedule(context.Background())
	if err != nil || !processed || len(repo.runs) != 1 || len(repo.deliveries) != 1 || len(sender.sent) != 1 {
		t.Fatalf("processed=%v runs=%d deliveries=%d sent=%d err=%v", processed, len(repo.runs), len(repo.deliveries), len(sender.sent), err)
	}
	run := repo.runs[0]
	if run.SavedViewVersion != view.Version || run.DictionaryVersion != "reporting-dictionary-v1" || run.ArtifactHash == "" || !strings.HasSuffix(run.FileName, ".zip") || repo.schedules[0].LastRunAt == nil {
		t.Fatalf("incomplete provenance: %+v", run)
	}
	reader, err := zip.NewReader(bytes.NewReader(run.Artifact), int64(len(run.Artifact)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range reader.File {
		handle, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		body, readErr := io.ReadAll(handle)
		_ = handle.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[file.Name] = body
	}
	if len(files["report.csv"]) == 0 {
		t.Fatal("board pack report missing")
	}
	var manifest map[string]any
	if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil || manifest["dictionaryVersion"] != "reporting-dictionary-v1" {
		t.Fatalf("manifest=%v err=%v", manifest, err)
	}
	delivery, retrieved, err := service.GetDeliveryArtifact(context.Background(), recipient, "org-1", repo.deliveries[0].ID)
	if err != nil || delivery.ID == "" || retrieved.ArtifactHash != run.ArtifactHash || repo.deliveries[0].OpenedAt == nil {
		t.Fatalf("delivery=%+v run=%+v err=%v", delivery, retrieved, err)
	}
	if _, _, err = service.GetDeliveryArtifact(context.Background(), builderPrincipal("stranger"), "org-1", repo.deliveries[0].ID); err == nil {
		t.Fatal("cross-recipient delivery access accepted")
	}
	expired := repo.deliveries[0]
	expired.ID = "expired-delivery"
	expired.ExpiresAt = now.Add(-time.Second)
	repo.deliveries = append(repo.deliveries, expired)
	if _, _, err = service.GetDeliveryArtifact(context.Background(), recipient, "org-1", expired.ID); err == nil || repo.deliveries[1].State != "expired" {
		t.Fatalf("expired delivery state=%s err=%v", repo.deliveries[1].State, err)
	}
	if schedule.State != "active" || len(evidence.audits) < 5 {
		t.Fatalf("schedule=%+v audits=%d", schedule, len(evidence.audits))
	}
}

func TestScheduleRejectsUnapprovedRecipientAndPausesWhenOwnerIsInactive(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	owner := builderPrincipal("owner-1")
	repo := &builderRepo{}
	service := Service{Authorizer: platform.GrantAuthorizer{}, Repository: repo, Evidence: &builderEvidence{}, Runner: builderRunner{}, Principals: builderPrincipals{principal: owner, active: false}, Now: func() time.Time { return now }}
	view, err := service.SaveView(context.Background(), owner, "org-1", SavedViewInput{Name: "Pulse", Query: builderQuery()})
	if err != nil {
		t.Fatal(err)
	}
	input := ScheduleInput{SavedViewID: view.ID, SavedViewVersion: view.Version, Name: "Weekly", Cadence: "weekly", Format: "csv", RecipientUserIDs: []platform.ID{"recipient-1"}, NextRunAt: now.Add(time.Minute)}
	if _, err = service.CreateSchedule(context.Background(), owner, "org-1", input, "request-1"); err == nil {
		t.Fatal("unapproved recipient accepted")
	}
	repo.recipients = []ApprovedRecipient{{UserID: "recipient-1", Email: "staff@remi.church"}}
	if _, err = service.CreateSchedule(context.Background(), owner, "org-1", input, "request-2"); err != nil {
		t.Fatal(err)
	}
	repo.schedules[0].NextRunAt = now.Add(-time.Minute)
	processed, err := service.ProcessDueSchedule(context.Background())
	if err != nil || !processed || repo.schedules[0].State != "paused" || repo.schedules[0].LastErrorCode != "owner_inactive" {
		t.Fatalf("processed=%v schedule=%+v err=%v", processed, repo.schedules[0], err)
	}
}
