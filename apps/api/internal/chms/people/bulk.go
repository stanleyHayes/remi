package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const (
	bulkPreviewsCollection = "chms_people_bulk_previews"
	bulkJobsCollection     = "chms_people_bulk_jobs"
	bulkResultsCollection  = "chms_people_bulk_results"
	maxBulkTargets         = 5000
	defaultBulkChunk       = 100
)

type BulkAction struct {
	Type           string      `json:"type" bson:"type"`
	Tags           []string    `json:"tags,omitempty" bson:"tags,omitempty"`
	TargetBranchID platform.ID `json:"targetBranchId,omitempty" bson:"targetBranchId,omitempty"`
	Reason         string      `json:"reason" bson:"reason"`
}

func (a *BulkAction) NormalizeAndValidate() error {
	a.Type = strings.ToLower(strings.TrimSpace(a.Type))
	a.Tags = normalizeTags(a.Tags)
	a.Reason = strings.TrimSpace(a.Reason)
	if err := platform.ValidateReason(a.Reason); err != nil {
		return err
	}
	switch a.Type {
	case "add-tags", "remove-tags":
		if len(a.Tags) == 0 || len(a.Tags) > 20 {
			return errors.New("tag actions require between 1 and 20 tags")
		}
	case "change-branch":
		if !a.TargetBranchID.Valid() {
			return errors.New("change-branch requires a target branch")
		}
	case "archive", "restore":
	default:
		return errors.New("unsupported bulk action")
	}
	return nil
}

type BulkTarget struct {
	PersonID        platform.ID `json:"personId" bson:"personId"`
	ExpectedVersion int64       `json:"expectedVersion" bson:"expectedVersion"`
	BranchID        platform.ID `json:"branchId" bson:"branchId"`
}

type BulkPreview struct {
	ID             platform.ID        `json:"id" bson:"_id"`
	OrganizationID platform.ID        `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID        `json:"branchId" bson:"branchId"`
	Filter         PersonSearchFilter `json:"filter" bson:"filter"`
	Action         BulkAction         `json:"action" bson:"action"`
	Targets        []BulkTarget       `json:"-" bson:"targets"`
	TargetCount    int                `json:"targetCount" bson:"targetCount"`
	SampleIDs      []platform.ID      `json:"sampleIds" bson:"sampleIds"`
	CreatedBy      platform.Actor     `json:"createdBy" bson:"createdBy"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt"`
	ExpiresAt      time.Time          `json:"expiresAt" bson:"expiresAt"`
	ConsumedAt     *time.Time         `json:"consumedAt,omitempty" bson:"consumedAt,omitempty"`
}

type BulkJob struct {
	ID             platform.ID    `json:"id" bson:"_id"`
	OrganizationID platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID    `json:"branchId" bson:"branchId"`
	PreviewID      platform.ID    `json:"previewId" bson:"previewId"`
	Action         BulkAction     `json:"action" bson:"action"`
	Targets        []BulkTarget   `json:"-" bson:"targets"`
	State          string         `json:"state" bson:"state"`
	Cursor         int            `json:"cursor" bson:"cursor"`
	Succeeded      int            `json:"succeeded" bson:"succeeded"`
	Conflicted     int            `json:"conflicted" bson:"conflicted"`
	Failed         int            `json:"failed" bson:"failed"`
	CreatedBy      platform.Actor `json:"createdBy" bson:"createdBy"`
	CreatedAt      time.Time      `json:"createdAt" bson:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt" bson:"updatedAt"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	UndoExpiresAt  time.Time      `json:"undoExpiresAt" bson:"undoExpiresAt"`
	UndoCursor     int            `json:"undoCursor" bson:"undoCursor"`
	UndoSucceeded  int            `json:"undoSucceeded" bson:"undoSucceeded"`
	UndoConflicted int            `json:"undoConflicted" bson:"undoConflicted"`
	RequestID      string         `json:"requestId" bson:"requestId"`
}

type BulkSnapshot struct {
	Tags          []string        `bson:"tags"`
	HomeBranchID  platform.ID     `bson:"homeBranchId"`
	ArchivedAt    *time.Time      `bson:"archivedAt,omitempty"`
	ArchivedBy    *platform.Actor `bson:"archivedBy,omitempty"`
	ArchiveReason string          `bson:"archiveReason,omitempty"`
}

type BulkResult struct {
	ID             platform.ID  `bson:"_id"`
	OrganizationID platform.ID  `bson:"organizationId"`
	JobID          platform.ID  `bson:"jobId"`
	PersonID       platform.ID  `bson:"personId"`
	State          string       `bson:"state"`
	Before         BulkSnapshot `bson:"before"`
	AfterVersion   int64        `bson:"afterVersion"`
	ErrorCode      string       `bson:"errorCode,omitempty"`
	ProcessedAt    time.Time    `bson:"processedAt"`
}

type BulkStore interface {
	FindBulkTargets(context.Context, platform.ID, PersonSearchFilter, int) ([]BulkTarget, error)
	InsertBulkPreview(context.Context, BulkPreview) error
	FindBulkPreview(context.Context, platform.ID, platform.ID) (*BulkPreview, error)
	ConsumePreviewAndInsertJob(context.Context, BulkPreview, BulkJob, time.Time) error
	FindBulkJob(context.Context, platform.ID, platform.ID) (*BulkJob, error)
	ApplyBulkAction(context.Context, platform.ID, BulkTarget, BulkAction, time.Time, platform.Actor) (BulkSnapshot, int64, error)
	InsertBulkResult(context.Context, BulkResult) error
	AdvanceBulkJob(context.Context, platform.ID, platform.ID, int, int, int, int, int, bool, time.Time) error
	ListSuccessfulBulkResults(context.Context, platform.ID, platform.ID, int) ([]BulkResult, error)
	RestoreBulkSnapshot(context.Context, platform.ID, BulkResult, time.Time, platform.Actor) error
	MarkBulkResultUndo(context.Context, platform.ID, platform.ID, platform.ID, string) error
	BeginBulkUndo(context.Context, platform.ID, platform.ID, time.Time) error
	AdvanceBulkUndo(context.Context, platform.ID, platform.ID, int, int, int, int, bool, time.Time) error
}

type BulkService struct {
	Store      BulkStore
	Platform   UnitOfWork
	Authorizer platform.Authorizer
	Now        func() time.Time
}

func (s BulkService) Preview(ctx context.Context, principal platform.Principal, filter PersonSearchFilter, action BulkAction, requestID string) (*BulkPreview, error) {
	if s.Store == nil || s.Platform == nil || s.Authorizer == nil {
		return nil, errors.New("bulk service is not configured")
	}
	if err := filter.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "filter", Code: "invalid_filter", Message: err.Error()})
	}
	if err := action.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "action", Code: "invalid_action", Message: err.Error()})
	}
	now := s.now()
	if !s.allowed(principal, filter.BranchID, action, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot perform this bulk action for the selected branch."}
	}
	targets, err := s.Store.FindBulkTargets(ctx, principal.OrganizationID, filter, maxBulkTargets+1)
	if err != nil {
		return nil, err
	}
	if len(targets) > maxBulkTargets {
		return nil, &platform.DomainError{Code: "bulk_limit_exceeded", Message: "Narrow the segment to 5,000 people or fewer."}
	}
	sample := make([]platform.ID, 0, min(20, len(targets)))
	for i := 0; i < len(targets) && i < 20; i++ {
		sample = append(sample, targets[i].PersonID)
	}
	preview := BulkPreview{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: filter.BranchID, Filter: filter, Action: action, Targets: targets, TargetCount: len(targets), SampleIDs: sample, CreatedBy: principal.Actor, CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertBulkPreview(tx, preview); err != nil {
			return err
		}
		return s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: filter.BranchID, Actor: principal.Actor, Action: "people.bulk.preview", ResourceType: "people-bulk-preview", ResourceID: preview.ID, SubjectIDs: sample, ChangedFields: bulkChangedFields(action), Outcome: "success", Reason: action.Reason, RequestID: requestID, OccurredAt: now})
	}); err != nil {
		return nil, fmt.Errorf("create bulk preview: %w", err)
	}
	return &preview, nil
}

func (s BulkService) Start(ctx context.Context, principal platform.Principal, previewID platform.ID, requestID string) (*BulkJob, error) {
	preview, err := s.Store.FindBulkPreview(ctx, principal.OrganizationID, previewID)
	if err != nil {
		return nil, err
	}
	if preview == nil || preview.ConsumedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Bulk preview is unavailable."}
	}
	now := s.now()
	if !now.Before(preview.ExpiresAt) {
		return nil, &platform.DomainError{Code: "preview_expired", Message: "Review the current matches and create a new preview."}
	}
	if preview.CreatedBy.ID != principal.Actor.ID || !s.allowed(principal, preview.BranchID, preview.Action, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Only the previewing operator can start this bulk action."}
	}
	job := BulkJob{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: preview.BranchID, PreviewID: preview.ID, Action: preview.Action, Targets: preview.Targets, State: "queued", CreatedBy: principal.Actor, CreatedAt: now, UpdatedAt: now, UndoExpiresAt: now.Add(24 * time.Hour), RequestID: requestID}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.ConsumePreviewAndInsertJob(tx, *preview, job, now); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: preview.BranchID, Actor: principal.Actor, Action: "people.bulk.start", ResourceType: "people-bulk-job", ResourceID: job.ID, SubjectIDs: preview.SampleIDs, ChangedFields: bulkChangedFields(preview.Action), Outcome: "success", Reason: preview.Action.Reason, RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: preview.BranchID, Type: "people.bulk.started", EventVersion: 1, AggregateType: "people-bulk-job", AggregateID: job.ID, AggregateVersion: 1, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"jobId": job.ID, "action": job.Action.Type, "targetCount": len(job.Targets)}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, fmt.Errorf("start bulk job: %w", err)
	}
	return &job, nil
}

func (s BulkService) RunChunk(ctx context.Context, principal platform.Principal, jobID platform.ID, chunkSize int) (*BulkJob, error) {
	job, err := s.Store.FindBulkJob(ctx, principal.OrganizationID, jobID)
	if err != nil || job == nil {
		if err != nil {
			return nil, err
		}
		return nil, &platform.DomainError{Code: "not_found", Message: "Bulk job not found."}
	}
	if job.State != "queued" && job.State != "running" {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Bulk job is not runnable."}
	}
	now := s.now()
	if !s.allowed(principal, job.BranchID, job.Action, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot run this bulk job."}
	}
	if chunkSize <= 0 || chunkSize > 250 {
		chunkSize = defaultBulkChunk
	}
	end := min(job.Cursor+chunkSize, len(job.Targets))
	succeeded, conflicted, failed := 0, 0, 0
	complete := end == len(job.Targets)
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		for index := job.Cursor; index < end; index++ {
			target := job.Targets[index]
			before, afterVersion, applyErr := s.Store.ApplyBulkAction(tx, principal.OrganizationID, target, job.Action, now, principal.Actor)
			result := BulkResult{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, JobID: job.ID, PersonID: target.PersonID, Before: before, AfterVersion: afterVersion, ProcessedAt: now}
			if applyErr == nil {
				result.State = "succeeded"
				succeeded++
			} else if isVersionConflict(applyErr) {
				result.State = "conflicted"
				result.ErrorCode = "version_conflict"
				conflicted++
			} else {
				result.State = "failed"
				result.ErrorCode = "apply_failed"
				failed++
			}
			if err := s.Store.InsertBulkResult(tx, result); err != nil {
				return err
			}
		}
		if err := s.Store.AdvanceBulkJob(tx, principal.OrganizationID, job.ID, job.Cursor, end, succeeded, conflicted, failed, complete, now); err != nil {
			return err
		}
		if !complete {
			return nil
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: job.BranchID, Actor: principal.Actor, Action: "people.bulk.complete", ResourceType: "people-bulk-job", ResourceID: job.ID, SubjectIDs: bulkSubjectIDs(job.Targets), ChangedFields: bulkChangedFields(job.Action), Outcome: "success", Reason: job.Action.Reason, RequestID: job.RequestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: job.BranchID, Type: "people.bulk.completed", EventVersion: 1, AggregateType: "people-bulk-job", AggregateID: job.ID, AggregateVersion: 2, Actor: principal.Actor, RequestID: job.RequestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"jobId": job.ID, "succeeded": job.Succeeded + succeeded, "conflicted": job.Conflicted + conflicted, "failed": job.Failed + failed}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, err
	}
	job.Cursor, job.Succeeded, job.Conflicted, job.Failed = end, job.Succeeded+succeeded, job.Conflicted+conflicted, job.Failed+failed
	job.State, job.UpdatedAt = "running", now
	if complete {
		job.State = "completed"
		job.CompletedAt = &now
	}
	return job, nil
}

func (s BulkService) UndoChunk(ctx context.Context, principal platform.Principal, jobID platform.ID, chunkSize int) (*BulkJob, error) {
	job, err := s.Store.FindBulkJob(ctx, principal.OrganizationID, jobID)
	if err != nil || job == nil {
		if err != nil {
			return nil, err
		}
		return nil, &platform.DomainError{Code: "not_found", Message: "Bulk job not found."}
	}
	now := s.now()
	if now.After(job.UndoExpiresAt) {
		return nil, &platform.DomainError{Code: "undo_expired", Message: "The safe undo window has closed."}
	}
	if !s.allowed(principal, job.BranchID, job.Action, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot undo this bulk job."}
	}
	if job.State == "completed" {
		if err := s.Store.BeginBulkUndo(ctx, principal.OrganizationID, job.ID, now); err != nil {
			return nil, err
		}
		job.State = "undoing"
	} else if job.State != "undoing" {
		return nil, &platform.DomainError{Code: "invalid_transition", Message: "Only a completed bulk job can be undone."}
	}
	if chunkSize <= 0 || chunkSize > 250 {
		chunkSize = defaultBulkChunk
	}
	var results []BulkResult
	undoSucceeded, undoConflicted := 0, 0
	newCursor, complete := job.UndoCursor, false
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		var listErr error
		results, listErr = s.Store.ListSuccessfulBulkResults(tx, principal.OrganizationID, job.ID, chunkSize)
		if listErr != nil {
			return listErr
		}
		for _, result := range results {
			outcome := "undone"
			restoreErr := s.Store.RestoreBulkSnapshot(tx, principal.OrganizationID, result, now, principal.Actor)
			if restoreErr != nil {
				if !isVersionConflict(restoreErr) {
					return restoreErr
				}
				outcome = "undo-conflicted"
			}
			if err := s.Store.MarkBulkResultUndo(tx, principal.OrganizationID, job.ID, result.PersonID, outcome); err != nil {
				return err
			}
			if outcome == "undone" {
				undoSucceeded++
			} else {
				undoConflicted++
			}
		}
		newCursor = job.UndoCursor + len(results)
		complete = len(results) < chunkSize || newCursor >= job.Succeeded
		if err := s.Store.AdvanceBulkUndo(tx, principal.OrganizationID, job.ID, job.UndoCursor, newCursor, undoSucceeded, undoConflicted, complete, now); err != nil {
			return err
		}
		if !complete {
			return nil
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: job.BranchID, Actor: principal.Actor, Action: "people.bulk.undo", ResourceType: "people-bulk-job", ResourceID: job.ID, SubjectIDs: bulkSubjectIDs(job.Targets), ChangedFields: bulkChangedFields(job.Action), Outcome: "success", Reason: job.Action.Reason, RequestID: job.RequestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: job.BranchID, Type: "people.bulk.undone", EventVersion: 1, AggregateType: "people-bulk-job", AggregateID: job.ID, AggregateVersion: 3, Actor: principal.Actor, RequestID: job.RequestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"jobId": job.ID, "undoSucceeded": job.UndoSucceeded + undoSucceeded, "undoConflicted": job.UndoConflicted + undoConflicted}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, err
	}
	job.UndoCursor, job.UndoSucceeded, job.UndoConflicted = newCursor, job.UndoSucceeded+undoSucceeded, job.UndoConflicted+undoConflicted
	if complete {
		job.State = "undone"
	}
	return job, nil
}

func (s BulkService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s BulkService) allowed(principal platform.Principal, branchID platform.ID, action BulkAction, now time.Time) bool {
	if !s.Authorizer.Authorize(principal, platform.AccessRequest{Action: "bulk-update", ResourceType: "person", OrganizationID: principal.OrganizationID, BranchID: branchID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: now}).Allowed {
		return false
	}
	if action.Type == "change-branch" && !s.Authorizer.Authorize(principal, platform.AccessRequest{Action: "bulk-update", ResourceType: "person", OrganizationID: principal.OrganizationID, BranchID: action.TargetBranchID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: now}).Allowed {
		return false
	}
	return true
}

func bulkChangedFields(action BulkAction) []string {
	switch action.Type {
	case "add-tags", "remove-tags":
		return []string{"tags"}
	case "change-branch":
		return []string{"homeBranchId"}
	default:
		return []string{"archivedAt", "archiveReason"}
	}
}

func bulkSubjectIDs(targets []BulkTarget) []platform.ID {
	ids := make([]platform.ID, 0, min(20, len(targets)))
	for i := 0; i < len(targets) && i < 20; i++ {
		ids = append(ids, targets[i].PersonID)
	}
	return ids
}

func isVersionConflict(err error) bool {
	var domain *platform.DomainError
	return errors.As(err, &domain) && domain.Code == "version_conflict"
}

func (r *MongoRepository) FindBulkTargets(ctx context.Context, organizationID platform.ID, filter PersonSearchFilter, limit int) ([]BulkTarget, error) {
	query := bulkFilterQuery(organizationID, filter)
	cursor, err := r.collection.Find(ctx, query, options.Find().SetProjection(bson.M{"_id": 1, "version": 1, "homeBranchId": 1}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []struct {
		ID       platform.ID `bson:"_id"`
		Version  int64       `bson:"version"`
		BranchID platform.ID `bson:"homeBranchId"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	targets := make([]BulkTarget, len(rows))
	for i, row := range rows {
		targets[i] = BulkTarget{PersonID: row.ID, ExpectedVersion: row.Version, BranchID: row.BranchID}
	}
	return targets, nil
}

func bulkFilterQuery(organizationID platform.ID, filter PersonSearchFilter) bson.M {
	query := bson.M{"organizationId": organizationID, "homeBranchId": filter.BranchID}
	if filter.Archived {
		query["archivedAt"] = bson.M{"$ne": nil}
	} else {
		query["archivedAt"] = nil
	}
	if len(filter.MembershipStages) > 0 {
		query["membershipStage"] = bson.M{"$in": filter.MembershipStages}
	}
	if len(filter.Tags) > 0 {
		query["tags"] = bson.M{"$all": filter.Tags}
	}
	if filter.UpdatedFrom != nil || filter.UpdatedTo != nil {
		dates := bson.M{}
		if filter.UpdatedFrom != nil {
			dates["$gte"] = filter.UpdatedFrom.UTC()
		}
		if filter.UpdatedTo != nil {
			dates["$lt"] = filter.UpdatedTo.UTC()
		}
		query["updatedAt"] = dates
	}
	if filter.Query != "" {
		pattern := regexpEscape(filter.Query)
		query["$or"] = bson.A{bson.M{"names.given": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"names.family": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"names.preferred": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"personNumber": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"contactPoints.normalized": bson.Regex{Pattern: pattern, Options: "i"}}}
	}
	return query
}

func regexpEscape(value string) string {
	return strings.NewReplacer("\\", "\\\\", ".", "\\.", "+", "\\+", "*", "\\*", "?", "\\?", "(", "\\(", ")", "\\)", "[", "\\[", "]", "\\]", "{", "\\{", "}", "\\}", "^", "\\^", "$", "\\$", "|", "\\|").Replace(value)
}

func (r *MongoRepository) InsertBulkPreview(ctx context.Context, preview BulkPreview) error {
	_, err := r.collection.Database().Collection(bulkPreviewsCollection).InsertOne(ctx, preview)
	return err
}
func (r *MongoRepository) FindBulkPreview(ctx context.Context, organizationID, previewID platform.ID) (*BulkPreview, error) {
	var preview BulkPreview
	err := r.collection.Database().Collection(bulkPreviewsCollection).FindOne(ctx, bson.M{"_id": previewID, "organizationId": organizationID}).Decode(&preview)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &preview, err
}

func (r *MongoRepository) ConsumePreviewAndInsertJob(ctx context.Context, preview BulkPreview, job BulkJob, now time.Time) error {
	result, err := r.collection.Database().Collection(bulkPreviewsCollection).UpdateOne(ctx, bson.M{"_id": preview.ID, "organizationId": preview.OrganizationID, "consumedAt": nil, "expiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"consumedAt": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "preview_unavailable", Message: "Bulk preview was already used or expired."}
	}
	_, err = r.collection.Database().Collection(bulkJobsCollection).InsertOne(ctx, job)
	return err
}
func (r *MongoRepository) FindBulkJob(ctx context.Context, organizationID, jobID platform.ID) (*BulkJob, error) {
	var job BulkJob
	err := r.collection.Database().Collection(bulkJobsCollection).FindOne(ctx, bson.M{"_id": jobID, "organizationId": organizationID}).Decode(&job)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &job, err
}

func (r *MongoRepository) ApplyBulkAction(ctx context.Context, organizationID platform.ID, target BulkTarget, action BulkAction, now time.Time, actor platform.Actor) (BulkSnapshot, int64, error) {
	var person Person
	if err := r.collection.FindOne(ctx, bson.M{"_id": target.PersonID, "organizationId": organizationID}).Decode(&person); err != nil {
		return BulkSnapshot{}, 0, err
	}
	if person.Version != target.ExpectedVersion {
		return BulkSnapshot{}, 0, platform.VersionConflict(person.Version)
	}
	before := BulkSnapshot{Tags: append([]string(nil), person.Tags...), HomeBranchID: person.HomeBranchID, ArchivedAt: person.ArchivedAt, ArchivedBy: person.ArchivedBy, ArchiveReason: person.ArchiveReason}
	set := bson.M{"updatedAt": now, "updatedBy": actor}
	switch action.Type {
	case "add-tags":
		set["tags"] = normalizeTags(append(person.Tags, action.Tags...))
	case "remove-tags":
		remove := map[string]bool{}
		for _, tag := range action.Tags {
			remove[tag] = true
		}
		tags := []string{}
		for _, tag := range person.Tags {
			if !remove[tag] {
				tags = append(tags, tag)
			}
		}
		set["tags"] = tags
	case "change-branch":
		set["homeBranchId"], set["branchId"] = action.TargetBranchID, action.TargetBranchID
	case "archive":
		if person.ArchivedAt != nil {
			return BulkSnapshot{}, 0, &platform.DomainError{Code: "invalid_transition", Message: "Person is already archived."}
		}
		set["archivedAt"], set["archivedBy"], set["archiveReason"] = now, actor, action.Reason
	case "restore":
		if person.ArchivedAt == nil {
			return BulkSnapshot{}, 0, &platform.DomainError{Code: "invalid_transition", Message: "Person is not archived."}
		}
		set["archivedAt"], set["archivedBy"], set["archiveReason"] = nil, nil, action.Reason
	}
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": target.PersonID, "organizationId": organizationID, "version": target.ExpectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return BulkSnapshot{}, 0, err
	}
	if result.MatchedCount != 1 {
		return BulkSnapshot{}, 0, platform.VersionConflict(target.ExpectedVersion)
	}
	return before, target.ExpectedVersion + 1, nil
}

func (r *MongoRepository) InsertBulkResult(ctx context.Context, result BulkResult) error {
	_, err := r.collection.Database().Collection(bulkResultsCollection).InsertOne(ctx, result)
	return err
}
func (r *MongoRepository) AdvanceBulkJob(ctx context.Context, organizationID, jobID platform.ID, expectedCursor, cursor, succeeded, conflicted, failed int, complete bool, now time.Time) error {
	state := "running"
	set := bson.M{"state": state, "cursor": cursor, "updatedAt": now}
	if complete {
		set["state"], set["completedAt"] = "completed", now
	}
	result, err := r.collection.Database().Collection(bulkJobsCollection).UpdateOne(ctx, bson.M{"_id": jobID, "organizationId": organizationID, "cursor": expectedCursor, "state": bson.M{"$in": bson.A{"queued", "running"}}}, bson.M{"$set": set, "$inc": bson.M{"succeeded": succeeded, "conflicted": conflicted, "failed": failed}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return errors.New("bulk job progress conflict")
	}
	return nil
}
func (r *MongoRepository) ListSuccessfulBulkResults(ctx context.Context, organizationID, jobID platform.ID, limit int) ([]BulkResult, error) {
	cursor, err := r.collection.Database().Collection(bulkResultsCollection).Find(ctx, bson.M{"organizationId": organizationID, "jobId": jobID, "state": "succeeded", "undoState": bson.M{"$in": bson.A{"", nil}}}, options.Find().SetSort(bson.D{{Key: "processedAt", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []BulkResult
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *MongoRepository) MarkBulkResultUndo(ctx context.Context, organizationID, jobID, personID platform.ID, state string) error {
	result, err := r.collection.Database().Collection(bulkResultsCollection).UpdateOne(ctx, bson.M{"organizationId": organizationID, "jobId": jobID, "personId": personID, "state": "succeeded", "undoState": bson.M{"$in": bson.A{"", nil}}}, bson.M{"$set": bson.M{"undoState": state}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return errors.New("bulk result is no longer undoable")
	}
	return nil
}
func (r *MongoRepository) RestoreBulkSnapshot(ctx context.Context, organizationID platform.ID, result BulkResult, now time.Time, actor platform.Actor) error {
	set := bson.M{"tags": result.Before.Tags, "homeBranchId": result.Before.HomeBranchID, "branchId": result.Before.HomeBranchID, "archivedAt": result.Before.ArchivedAt, "archivedBy": result.Before.ArchivedBy, "archiveReason": result.Before.ArchiveReason, "updatedAt": now, "updatedBy": actor}
	updated, err := r.collection.UpdateOne(ctx, bson.M{"_id": result.PersonID, "organizationId": organizationID, "version": result.AfterVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if updated.MatchedCount != 1 {
		return platform.VersionConflict(result.AfterVersion)
	}
	return nil
}
func (r *MongoRepository) BeginBulkUndo(ctx context.Context, organizationID, jobID platform.ID, now time.Time) error {
	result, err := r.collection.Database().Collection(bulkJobsCollection).UpdateOne(ctx, bson.M{"_id": jobID, "organizationId": organizationID, "state": "completed", "undoExpiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"state": "undoing", "updatedAt": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "invalid_transition", Message: "Bulk job cannot enter undo."}
	}
	return nil
}
func (r *MongoRepository) AdvanceBulkUndo(ctx context.Context, organizationID, jobID platform.ID, expectedCursor, cursor, succeeded, conflicted int, complete bool, now time.Time) error {
	set := bson.M{"undoCursor": cursor, "updatedAt": now}
	if complete {
		set["state"] = "undone"
	}
	result, err := r.collection.Database().Collection(bulkJobsCollection).UpdateOne(ctx, bson.M{"_id": jobID, "organizationId": organizationID, "state": "undoing", "undoCursor": expectedCursor}, bson.M{"$set": set, "$inc": bson.M{"undoSucceeded": succeeded, "undoConflicted": conflicted}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return errors.New("bulk undo progress conflict")
	}
	return nil
}
