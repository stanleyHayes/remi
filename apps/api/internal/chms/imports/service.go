package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

type UnitOfWork interface {
	WithTransaction(context.Context, func(context.Context) error) error
	AppendAudit(context.Context, platform.AuditEvent) error
	EnqueueEvent(context.Context, platform.OutboxRecord) error
}
type Service struct {
	Repository Repository
	Platform   UnitOfWork
	Limits     ParseLimits
	Now        func() time.Time
}
type StageInput struct {
	OrganizationID platform.ID
	BranchID       platform.ID
	EntityType     string
	SourceType     string
	SourceName     string
	Reader         io.Reader
}

type RowValidator func(context.Context, map[string]string) []RowError
type ValidationSchema struct {
	AllowedTargets  map[string]bool
	RequiredTargets []string
	Validate        RowValidator
}
type RowCommitter interface {
	Commit(context.Context, ImportRun, ImportRow) (platform.ID, int64, error)
}
type RowRollbacker interface {
	Rollback(context.Context, ImportRun, ImportRow, string) *RowError
}

func (s Service) StageCSV(ctx context.Context, input StageInput, actor platform.Actor, requestID string) (*ImportRun, error) {
	input.EntityType = strings.ToLower(strings.TrimSpace(input.EntityType))
	input.SourceType = strings.ToLower(strings.TrimSpace(input.SourceType))
	input.SourceName = strings.TrimSpace(input.SourceName)
	if !input.OrganizationID.Valid() || input.EntityType == "" || input.SourceType == "" || input.SourceName == "" || input.Reader == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_import", Message: "Organization, entity, source and CSV file are required."})
	}
	limits := s.Limits
	if limits.MaxBytes == 0 {
		limits = DefaultParseLimits()
	}
	parsed, err := ParseCSV(input.Reader, limits)
	if err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "file", Code: "invalid_csv", Message: err.Error()})
	}
	existing, err := s.Repository.FindBySourceHash(ctx, input.OrganizationID, input.EntityType, parsed.Hash)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &platform.DomainError{Code: "conflict", Message: "This CSV file already has an active import run.", Details: map[string]any{"importRunId": existing.ID}}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	runID := platform.ID(bson.NewObjectID().Hex())
	run := ImportRun{ResourceEnvelope: platform.ResourceEnvelope{ID: runID, OrganizationID: input.OrganizationID, BranchID: input.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: actor, UpdatedAt: now, UpdatedBy: actor}, EntityType: input.EntityType, SourceType: input.SourceType, SourceName: input.SourceName, SourceHash: parsed.Hash, Headers: parsed.Headers, State: "staged", TotalRows: len(parsed.Rows)}
	rows := make([]ImportRow, len(parsed.Rows))
	for i, source := range parsed.Rows {
		rows[i] = ImportRow{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: input.OrganizationID, ImportRunID: runID, RowNumber: i + 2, Source: source, State: "staged"}
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.InsertRunAndRows(tx, run, rows); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: input.OrganizationID, BranchID: input.BranchID, Actor: actor, Action: "governance.import.stage", ResourceType: "import-run", ResourceID: runID, ChangedFields: []string{"source", "rows"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: input.OrganizationID, BranchID: input.BranchID, Type: "governance.import.staged", EventVersion: 1, AggregateType: "import-run", AggregateID: runID, AggregateVersion: 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"importRunId": runID, "entityType": input.EntityType, "rowCount": len(rows), "sourceHash": parsed.Hash}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("stage CSV import: %w", err)
	}
	return &run, nil
}

func (s Service) ConfigureMapping(ctx context.Context, organizationID, runID platform.ID, expectedVersion int64, input MappingInput, schema ValidationSchema, actor platform.Actor, requestID string) (*ImportRun, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	run, err := s.Repository.FindRun(ctx, organizationID, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Import run not found."}
	}
	if err := ValidateMapping(run.Headers, input, schema.AllowedTargets); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "fields", Code: "invalid_mapping", Message: err.Error()})
	}
	mappedTargets := map[string]bool{}
	for _, target := range input.Fields {
		mappedTargets[target] = true
	}
	for _, required := range schema.RequiredTargets {
		if !mappedTargets[required] {
			return nil, platform.ValidationError(platform.FieldError{Path: "fields", Code: "missing_required_mapping", Message: "A required target field is not mapped: " + required})
		}
	}
	now := s.now()
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.SetMapping(tx, organizationID, runID, expectedVersion, input, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Actor: actor, Action: "governance.import.map", ResourceType: "import-run", ResourceID: runID, ChangedFields: []string{"mapping"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Type: "governance.import.mapped", EventVersion: 1, AggregateType: "import-run", AggregateID: runID, AggregateVersion: expectedVersion + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"importRunId": runID, "mappingVersion": input.Version}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("configure import mapping: %w", err)
	}
	run.MappingVersion = input.Version
	run.Mapping = input.Fields
	run.State = "mapped"
	run.Version = expectedVersion + 1
	run.UpdatedAt = now
	run.UpdatedBy = actor
	return run, nil
}

func (s Service) ValidateDryRun(ctx context.Context, organizationID, runID platform.ID, schema ValidationSchema, actor platform.Actor, requestID string) (*ImportRun, error) {
	run, err := s.Repository.FindRun(ctx, organizationID, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Import run not found."}
	}
	if run.State != "mapped" && run.State != "failed" && run.State != "validating" {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Import run must be mapped before validation."}
	}
	now := s.now()
	if run.State != "validating" {
		if err := s.Repository.SetRunState(ctx, organizationID, runID, run.State, "validating", now, actor); err != nil {
			return nil, err
		}
	}
	valid, invalid, after := 0, 0, 0
	for {
		rows, err := s.Repository.ListRowsAfter(ctx, organizationID, runID, after, 500)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			rows[i].Mapped = applyMapping(rows[i].Source, run.Mapping)
			rows[i].Errors = requiredErrors(rows[i].Mapped, schema.RequiredTargets)
			if schema.Validate != nil {
				rows[i].Errors = append(rows[i].Errors, schema.Validate(ctx, rows[i].Mapped)...)
			}
			if len(rows[i].Errors) == 0 {
				rows[i].State = "valid"
				valid++
			} else {
				rows[i].State = "invalid"
				invalid++
			}
			after = rows[i].RowNumber
		}
		if err := s.Repository.UpdateRowValidation(ctx, organizationID, runID, rows); err != nil {
			return nil, err
		}
	}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.FinishValidation(tx, organizationID, runID, valid, invalid, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Actor: actor, Action: "governance.import.validate", ResourceType: "import-run", ResourceID: runID, ChangedFields: []string{"validation"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Type: "governance.import.validated", EventVersion: 1, AggregateType: "import-run", AggregateID: runID, AggregateVersion: run.Version + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"importRunId": runID, "validRows": valid, "invalidRows": invalid}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, fmt.Errorf("finish import dry run: %w", err)
	}
	run.State = "validated"
	run.ValidRows = valid
	run.InvalidRows = invalid
	run.Version++
	run.UpdatedAt = now
	run.UpdatedBy = actor
	return run, nil
}

func (s Service) CommitValidated(ctx context.Context, organizationID, runID platform.ID, committer RowCommitter, actor platform.Actor, requestID string) (*ImportRun, error) {
	if committer == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "committer_required", Message: "An entity import committer is required."})
	}
	run, err := s.Repository.FindRun(ctx, organizationID, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Import run not found."}
	}
	now := s.now()
	if run.State == "validated" {
		if err := s.Repository.SetRunState(ctx, organizationID, runID, "validated", "committing", now, actor); err != nil {
			return nil, err
		}
		run.State = "committing"
	} else if run.State != "committing" {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Import run must be validated before commit."}
	}
	cursor := run.CommitCursor
	committed := run.CommittedRows
	for {
		rows, err := s.Repository.ListRowsByStatesAfter(ctx, organizationID, runID, []string{"valid"}, cursor, 100)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
				resourceID, resourceVersion, commitErr := committer.Commit(tx, *run, row)
				if commitErr != nil {
					return commitErr
				}
				return s.Repository.RecordRowCommitted(tx, organizationID, runID, row.ID, row.RowNumber, resourceID, resourceVersion, now)
			})
			if err != nil {
				return nil, fmt.Errorf("commit import row %d: %w", row.RowNumber, err)
			}
			cursor = row.RowNumber
			committed++
		}
	}
	manifestRows, err := s.Repository.ListRowsByStates(ctx, organizationID, runID, []string{"committed"})
	if err != nil {
		return nil, err
	}
	manifest := manifestForRows(manifestRows)
	if committed != len(manifestRows) {
		return nil, &platform.DomainError{Code: "conflict", Message: "Import commit manifest count does not match committed rows."}
	}
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.CompleteCommit(tx, organizationID, runID, committed, manifest, now, actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Actor: actor, Action: "governance.import.commit", ResourceType: "import-run", ResourceID: runID, ChangedFields: []string{"committedRows", "manifestHash"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Type: "governance.import.completed", EventVersion: 1, AggregateType: "import-run", AggregateID: runID, AggregateVersion: run.Version + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"importRunId": runID, "committedRows": committed, "manifestHash": manifest}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("complete import: %w", err)
	}
	run.State = "completed"
	run.CommittedRows = committed
	run.CommitCursor = cursor
	run.ManifestHash = manifest
	run.CommittedAt = &now
	run.Version++
	return run, nil
}

func (s Service) RollbackCommitted(ctx context.Context, organizationID, runID platform.ID, rollbacker RowRollbacker, reason string, actor platform.Actor, requestID string) (*ImportRun, error) {
	if rollbacker == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "rollbacker_required", Message: "An entity rollback handler is required."})
	}
	if err := platform.ValidateReason(reason); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "reason", Code: "invalid_reason", Message: err.Error()})
	}
	run, err := s.Repository.FindRun(ctx, organizationID, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Import run not found."}
	}
	now := s.now()
	if run.State == "completed" || run.State == "rollback-blocked" {
		if err := s.Repository.SetRunState(ctx, organizationID, runID, run.State, "rolling-back", now, actor); err != nil {
			return nil, err
		}
		run.State = "rolling-back"
	} else if run.State != "rolling-back" {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Only a completed import can be rolled back."}
	}
	rows, err := s.Repository.ListRowsByStates(ctx, organizationID, runID, []string{"committed", "rollback-blocked", "rolled-back"})
	if err != nil {
		return nil, err
	}
	blocked := 0
	for _, row := range rows {
		if row.State == "rolled-back" {
			continue
		}
		var rowError *RowError
		err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
			rowError = rollbacker.Rollback(tx, *run, row, reason)
			success := rowError == nil
			value := RowError{}
			if rowError != nil {
				value = *rowError
			}
			return s.Repository.RecordRowRollback(tx, organizationID, runID, row.ID, success, value, now)
		})
		if err != nil {
			return nil, fmt.Errorf("record rollback for import row %d: %w", row.RowNumber, err)
		}
		if rowError != nil {
			blocked++
		}
	}
	manifestRows, err := s.Repository.ListRowsByStates(ctx, organizationID, runID, []string{"rolled-back", "rollback-blocked"})
	if err != nil {
		return nil, err
	}
	manifest := manifestForRows(manifestRows)
	err = s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Repository.FinishRollback(tx, organizationID, runID, blocked, manifest, now, actor); err != nil {
			return err
		}
		outcome := "success"
		if blocked > 0 {
			outcome = "partial"
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Actor: actor, Action: "governance.import.rollback", ResourceType: "import-run", ResourceID: runID, ChangedFields: []string{"rollbackManifestHash", "rollbackBlockedRows"}, Outcome: outcome, Reason: reason, RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: organizationID, BranchID: run.BranchID, Type: "governance.import.rollback.finished", EventVersion: 1, AggregateType: "import-run", AggregateID: runID, AggregateVersion: run.Version + 1, Actor: actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"importRunId": runID, "blockedRows": blocked, "manifestHash": manifest}}, State: "pending", AvailableAt: now})
	})
	if err != nil {
		return nil, fmt.Errorf("finish import rollback: %w", err)
	}
	run.State = "rolled-back"
	if blocked > 0 {
		run.State = "rollback-blocked"
	}
	run.RollbackBlockedRows = blocked
	run.RollbackManifestHash = manifest
	run.RolledBackAt = &now
	run.Version++
	return run, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func applyMapping(source, mapping map[string]string) map[string]string {
	mapped := make(map[string]string, len(mapping))
	for sourceField, target := range mapping {
		mapped[target] = strings.TrimSpace(source[sourceField])
	}
	return mapped
}
func requiredErrors(mapped map[string]string, required []string) []RowError {
	errors := []RowError{}
	for _, field := range required {
		if strings.TrimSpace(mapped[field]) == "" {
			errors = append(errors, RowError{Field: field, Code: "required", Message: "A value is required."})
		}
	}
	return errors
}

func manifestForRows(rows []ImportRow) string {
	entries := make([]ManifestEntry, len(rows))
	for i, row := range rows {
		entries[i] = ManifestEntry{RowNumber: row.RowNumber, RowID: row.ID, ResourceID: row.CanonicalResourceID, ResourceVersion: row.ResourceVersionAtCommit, State: row.State}
	}
	encoded, _ := json.Marshal(entries)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
