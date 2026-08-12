package imports

import (
	"context"
	"strings"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func TestParseCSVNormalizesHeaderRowsAndHashesSource(t *testing.T) {
	source := "\ufeffFirst Name,Email\n Ama , AMA@example.com \n\n Kojo,kojo@example.com\n"
	parsed, err := ParseCSV(strings.NewReader(source), DefaultParseLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Rows) != 2 || parsed.Headers[0] != "First Name" || parsed.Rows[0]["First Name"] != "Ama" || parsed.Hash == "" {
		t.Fatalf("unexpected parsed CSV: %+v", parsed)
	}
	again, err := ParseCSV(strings.NewReader(source), DefaultParseLimits())
	if err != nil || again.Hash != parsed.Hash {
		t.Fatal("source hash is not deterministic")
	}
}
func TestParseCSVRejectsUnsafeOrAmbiguousShapes(t *testing.T) {
	tests := map[string]struct {
		source string
		limits ParseLimits
	}{"duplicate header": {"Name,Name\nA,B\n", DefaultParseLimits()}, "row width": {"Name,Email\nA\n", DefaultParseLimits()}, "too many rows": {"Name\nA\nB\n", ParseLimits{MaxBytes: 100, MaxRows: 1, MaxColumns: 5, MaxCellBytes: 20}}, "cell too large": {"Name\nabcdef\n", ParseLimits{MaxBytes: 100, MaxRows: 5, MaxColumns: 5, MaxCellBytes: 3}}, "empty": {"Name\n\n", DefaultParseLimits()}}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCSV(strings.NewReader(test.source), test.limits); err == nil {
				t.Fatal("expected CSV rejection")
			}
		})
	}
}
func TestMappingRejectsUnknownDuplicateAndUnsupportedTargets(t *testing.T) {
	headers := []string{"Name", "Email"}
	allowed := map[string]bool{"names.given": true, "contact.email": true}
	if err := ValidateMapping(headers, MappingInput{Version: 1, Fields: map[string]string{"Name": "names.given", "Email": "contact.email"}}, allowed); err != nil {
		t.Fatal(err)
	}
	for name, mapping := range map[string]MappingInput{"unknown source": {Version: 1, Fields: map[string]string{"Other": "names.given"}}, "duplicate target": {Version: 1, Fields: map[string]string{"Name": "names.given", "Email": "names.given"}}, "unsupported": {Version: 1, Fields: map[string]string{"Name": "passwordHash"}}} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMapping(headers, mapping, allowed); err == nil {
				t.Fatal("expected mapping rejection")
			}
		})
	}
}

type fakeRepository struct {
	existing *ImportRun
	run      *ImportRun
	rows     []ImportRow
}

func (r *fakeRepository) InsertRunAndRows(_ context.Context, run ImportRun, rows []ImportRow) error {
	r.run = &run
	r.rows = rows
	return nil
}
func (r *fakeRepository) FindBySourceHash(context.Context, platform.ID, string, string) (*ImportRun, error) {
	return r.existing, nil
}
func (r *fakeRepository) FindRun(context.Context, platform.ID, platform.ID) (*ImportRun, error) {
	return r.run, nil
}
func (r *fakeRepository) SetMapping(_ context.Context, _ platform.ID, _ platform.ID, _ int64, mapping MappingInput, _ time.Time, _ platform.Actor) error {
	r.run.Mapping = mapping.Fields
	r.run.MappingVersion = mapping.Version
	r.run.State = "mapped"
	r.run.Version++
	for i := range r.rows {
		r.rows[i].Mapped = nil
		r.rows[i].Errors = nil
		r.rows[i].State = "staged"
	}
	return nil
}
func (r *fakeRepository) SetRunState(_ context.Context, _ platform.ID, _ platform.ID, from, to string, _ time.Time, _ platform.Actor) error {
	if r.run.State != from {
		return &platform.DomainError{Code: "invalid_transition", Message: "state changed"}
	}
	r.run.State = to
	return nil
}
func (r *fakeRepository) ListRowsAfter(_ context.Context, _ platform.ID, _ platform.ID, after, limit int) ([]ImportRow, error) {
	result := []ImportRow{}
	for _, row := range r.rows {
		if row.RowNumber > after && len(result) < limit {
			result = append(result, row)
		}
	}
	return result, nil
}
func (r *fakeRepository) UpdateRowValidation(_ context.Context, _ platform.ID, _ platform.ID, rows []ImportRow) error {
	for _, updated := range rows {
		for i := range r.rows {
			if r.rows[i].ID == updated.ID {
				r.rows[i] = updated
			}
		}
	}
	return nil
}
func (r *fakeRepository) FinishValidation(_ context.Context, _ platform.ID, _ platform.ID, valid, invalid int, _ time.Time, _ platform.Actor) error {
	r.run.State = "validated"
	r.run.ValidRows = valid
	r.run.InvalidRows = invalid
	r.run.Version++
	return nil
}
func (r *fakeRepository) ListRowsByStatesAfter(_ context.Context, _ platform.ID, _ platform.ID, states []string, after, limit int) ([]ImportRow, error) {
	allowed := map[string]bool{}
	for _, state := range states {
		allowed[state] = true
	}
	result := []ImportRow{}
	for _, row := range r.rows {
		if row.RowNumber > after && allowed[row.State] && len(result) < limit {
			result = append(result, row)
		}
	}
	return result, nil
}
func (r *fakeRepository) RecordRowCommitted(_ context.Context, _ platform.ID, _ platform.ID, rowID platform.ID, rowNumber int, resourceID platform.ID, version int64, now time.Time) error {
	for i := range r.rows {
		if r.rows[i].ID == rowID {
			r.rows[i].State = "committed"
			r.rows[i].CanonicalResourceID = resourceID
			r.rows[i].ResourceVersionAtCommit = version
			r.rows[i].CommittedAt = &now
		}
	}
	r.run.CommittedRows++
	if rowNumber > r.run.CommitCursor {
		r.run.CommitCursor = rowNumber
	}
	return nil
}
func (r *fakeRepository) ListRowsByStates(_ context.Context, _ platform.ID, _ platform.ID, states []string) ([]ImportRow, error) {
	allowed := map[string]bool{}
	for _, state := range states {
		allowed[state] = true
	}
	rows := []ImportRow{}
	for _, row := range r.rows {
		if allowed[row.State] {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (r *fakeRepository) CompleteCommit(_ context.Context, _ platform.ID, _ platform.ID, committed int, manifest string, now time.Time, _ platform.Actor) error {
	r.run.State = "completed"
	r.run.CommittedRows = committed
	r.run.ManifestHash = manifest
	r.run.CommittedAt = &now
	r.run.Version++
	return nil
}
func (r *fakeRepository) RecordRowRollback(_ context.Context, _ platform.ID, _ platform.ID, rowID platform.ID, success bool, rowError RowError, now time.Time) error {
	for i := range r.rows {
		if r.rows[i].ID == rowID {
			if success {
				r.rows[i].State = "rolled-back"
				r.rows[i].RolledBackAt = &now
			} else {
				r.rows[i].State = "rollback-blocked"
				r.rows[i].Errors = []RowError{rowError}
			}
		}
	}
	return nil
}
func (r *fakeRepository) FinishRollback(_ context.Context, _ platform.ID, _ platform.ID, blocked int, manifest string, now time.Time, _ platform.Actor) error {
	r.run.State = "rolled-back"
	if blocked > 0 {
		r.run.State = "rollback-blocked"
	}
	r.run.RollbackBlockedRows = blocked
	r.run.RollbackManifestHash = manifest
	r.run.RolledBackAt = &now
	r.run.Version++
	return nil
}

type fakePlatform struct {
	audits []platform.AuditEvent
	events []platform.OutboxRecord
}

func (p *fakePlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (p *fakePlatform) AppendAudit(_ context.Context, event platform.AuditEvent) error {
	p.audits = append(p.audits, event)
	return nil
}
func (p *fakePlatform) EnqueueEvent(_ context.Context, event platform.OutboxRecord) error {
	p.events = append(p.events, event)
	return nil
}

func TestStageCSVStoresRowsProvenanceAuditAndEvent(t *testing.T) {
	now := time.Date(2026, 8, 11, 15, 0, 0, 0, time.UTC)
	repo := &fakeRepository{}
	evidence := &fakePlatform{}
	service := Service{Repository: repo, Platform: evidence, Limits: DefaultParseLimits(), Now: func() time.Time { return now }}
	run, err := service.StageCSV(context.Background(), StageInput{OrganizationID: "org", BranchID: "branch", EntityType: "people", SourceType: "legacy-spreadsheet", SourceName: "members.csv", Reader: strings.NewReader("Name,Email\nAma,ama@example.com\nKojo,kojo@example.com\n")}, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request-import")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != "staged" || run.TotalRows != 2 || len(repo.rows) != 2 || repo.rows[0].RowNumber != 2 || len(evidence.audits) != 1 || evidence.events[0].Type != "governance.import.staged" {
		t.Fatalf("incomplete stage result: run=%+v rows=%+v", run, repo.rows)
	}
}
func TestStageCSVRefusesActiveDuplicateSource(t *testing.T) {
	service := Service{Repository: &fakeRepository{existing: &ImportRun{ResourceEnvelope: platform.ResourceEnvelope{ID: "existing"}}}, Platform: &fakePlatform{}, Limits: DefaultParseLimits()}
	_, err := service.StageCSV(context.Background(), StageInput{OrganizationID: "org", EntityType: "people", SourceType: "csv", SourceName: "members.csv", Reader: strings.NewReader("Name\nAma\n")}, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request")
	if err == nil {
		t.Fatal("expected duplicate-source conflict")
	}
}

func TestConfigureMappingRequiresSupportedAndRequiredTargets(t *testing.T) {
	now := time.Date(2026, 8, 11, 15, 0, 0, 0, time.UTC)
	run := &ImportRun{ResourceEnvelope: platform.ResourceEnvelope{ID: "run", OrganizationID: "org", Version: 1}, Headers: []string{"First Name", "Email"}, State: "staged"}
	repo := &fakeRepository{run: run, rows: []ImportRow{{ID: "row", RowNumber: 2, State: "staged"}}}
	service := Service{Repository: repo, Platform: &fakePlatform{}, Now: func() time.Time { return now }}
	schema := ValidationSchema{AllowedTargets: map[string]bool{"names.given": true, "contact.email": true}, RequiredTargets: []string{"names.given"}}
	if _, err := service.ConfigureMapping(context.Background(), "org", "run", 1, MappingInput{Version: 1, Fields: map[string]string{"Email": "contact.email"}}, schema, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request"); err == nil {
		t.Fatal("expected missing required mapping")
	}
	mapped, err := service.ConfigureMapping(context.Background(), "org", "run", 1, MappingInput{Version: 1, Fields: map[string]string{"First Name": "names.given", "Email": "contact.email"}}, schema, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request")
	if err != nil {
		t.Fatal(err)
	}
	if mapped.State != "mapped" || mapped.Version != 2 || mapped.Mapping["First Name"] != "names.given" {
		t.Fatalf("bad mapped run: %+v", mapped)
	}
}

func TestValidateDryRunMapsEveryRowAndKeepsRowErrors(t *testing.T) {
	now := time.Date(2026, 8, 11, 15, 0, 0, 0, time.UTC)
	run := &ImportRun{ResourceEnvelope: platform.ResourceEnvelope{ID: "run", OrganizationID: "org", Version: 2}, Headers: []string{"First Name", "Email"}, Mapping: map[string]string{"First Name": "names.given", "Email": "contact.email"}, State: "mapped"}
	repo := &fakeRepository{run: run, rows: []ImportRow{{ID: "row-1", RowNumber: 2, Source: map[string]string{"First Name": "Ama", "Email": "ama@example.com"}, State: "staged"}, {ID: "row-2", RowNumber: 3, Source: map[string]string{"First Name": "", "Email": "bad"}, State: "staged"}}}
	evidence := &fakePlatform{}
	service := Service{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	schema := ValidationSchema{RequiredTargets: []string{"names.given", "contact.email"}, Validate: func(_ context.Context, mapped map[string]string) []RowError {
		if !strings.Contains(mapped["contact.email"], "@") {
			return []RowError{{Field: "contact.email", Code: "invalid_email", Message: "Enter a valid email."}}
		}
		return nil
	}}
	validated, err := service.ValidateDryRun(context.Background(), "org", "run", schema, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "validate")
	if err != nil {
		t.Fatal(err)
	}
	if validated.ValidRows != 1 || validated.InvalidRows != 1 || repo.rows[0].State != "valid" || repo.rows[1].State != "invalid" || len(repo.rows[1].Errors) != 2 || evidence.events[0].Type != "governance.import.validated" {
		t.Fatalf("bad dry run: run=%+v rows=%+v", validated, repo.rows)
	}
}

type fakeCommitter struct {
	calls  []platform.ID
	failAt platform.ID
}

func (c *fakeCommitter) Commit(_ context.Context, _ ImportRun, row ImportRow) (platform.ID, int64, error) {
	c.calls = append(c.calls, row.ID)
	if row.ID == c.failAt {
		return "", 0, context.Canceled
	}
	return platform.ID("resource-" + row.ID), 1, nil
}

type fakeRollbacker struct {
	changed map[platform.ID]bool
	calls   []platform.ID
}

func (r *fakeRollbacker) Rollback(_ context.Context, _ ImportRun, row ImportRow, _ string) *RowError {
	r.calls = append(r.calls, row.ID)
	if r.changed[row.ID] {
		return &RowError{Field: "$", Code: "resource_changed", Message: "Resource changed after import."}
	}
	return nil
}

func TestCommitValidatedResumesAndProducesDeterministicManifest(t *testing.T) {
	now := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	run := &ImportRun{ResourceEnvelope: platform.ResourceEnvelope{ID: "run", OrganizationID: "org", Version: 3}, State: "validated", ValidRows: 2, InvalidRows: 1}
	repo := &fakeRepository{run: run, rows: []ImportRow{{ID: "row-1", RowNumber: 2, State: "valid"}, {ID: "row-bad", RowNumber: 3, State: "invalid"}, {ID: "row-2", RowNumber: 4, State: "valid"}}}
	evidence := &fakePlatform{}
	service := Service{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	first := &fakeCommitter{failAt: "row-2"}
	if _, err := service.CommitValidated(context.Background(), "org", "run", first, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "commit-1"); err == nil {
		t.Fatal("expected simulated interrupted commit")
	}
	if repo.run.State != "committing" || repo.run.CommittedRows != 1 || repo.rows[0].State != "committed" || repo.rows[2].State != "valid" {
		t.Fatalf("commit was not resumable: run=%+v rows=%+v", repo.run, repo.rows)
	}
	second := &fakeCommitter{}
	completed, err := service.CommitValidated(context.Background(), "org", "run", second, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "commit-2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != "completed" || completed.CommittedRows != 2 || completed.ManifestHash == "" || len(second.calls) != 1 || second.calls[0] != "row-2" || evidence.events[0].Type != "governance.import.completed" {
		t.Fatalf("bad resumed completion: run=%+v calls=%v", completed, second.calls)
	}
	if manifestForRows([]ImportRow{repo.rows[0], repo.rows[2]}) != completed.ManifestHash {
		t.Fatal("manifest is not deterministic")
	}
}

func TestRollbackOnlyCompletesUnchangedRowsAndRecordsBlockedManifest(t *testing.T) {
	now := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	committedAt := now.Add(-time.Hour)
	run := &ImportRun{ResourceEnvelope: platform.ResourceEnvelope{ID: "run", OrganizationID: "org", Version: 4}, State: "completed", CommittedRows: 2}
	repo := &fakeRepository{run: run, rows: []ImportRow{{ID: "row-1", RowNumber: 2, State: "committed", CanonicalResourceID: "resource-1", ResourceVersionAtCommit: 1, CommittedAt: &committedAt}, {ID: "row-2", RowNumber: 3, State: "committed", CanonicalResourceID: "resource-2", ResourceVersionAtCommit: 1, CommittedAt: &committedAt}}}
	evidence := &fakePlatform{}
	service := Service{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	rollbacker := &fakeRollbacker{changed: map[platform.ID]bool{"row-2": true}}
	result, err := service.RollbackCommitted(context.Background(), "org", "run", rollbacker, "Import uploaded to wrong branch", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "rollback")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "rollback-blocked" || result.RollbackBlockedRows != 1 || result.RollbackManifestHash == "" || repo.rows[0].State != "rolled-back" || repo.rows[1].State != "rollback-blocked" || evidence.audits[0].Outcome != "partial" {
		t.Fatalf("bad rollback result: run=%+v rows=%+v", result, repo.rows)
	}
}
