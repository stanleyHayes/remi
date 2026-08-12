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

const segmentsCollection = "chms_people_segments"

type SavedSegment struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string             `json:"name" bson:"name"`
	Description               string             `json:"description,omitempty" bson:"description,omitempty"`
	Filter                    PersonSearchFilter `json:"filter" bson:"filter"`
	Visibility                string             `json:"visibility" bson:"visibility"`
	OwnerID                   platform.ID        `json:"ownerId" bson:"ownerId"`
}

type SaveSegmentInput struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Filter      PersonSearchFilter `json:"filter"`
	Visibility  string             `json:"visibility"`
}

func (in *SaveSegmentInput) NormalizeAndValidate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.Visibility = strings.ToLower(strings.TrimSpace(in.Visibility))
	if in.Name == "" || len(in.Name) > 100 {
		return errors.New("segment name is required and must not exceed 100 characters")
	}
	if len(in.Description) > 500 {
		return errors.New("segment description must not exceed 500 characters")
	}
	if in.Visibility == "" {
		in.Visibility = "private"
	}
	if in.Visibility != "private" && in.Visibility != "organization" {
		return errors.New("segment visibility must be private or organization")
	}
	return in.Filter.NormalizeAndValidate()
}

type SegmentStore interface {
	InsertSegment(context.Context, SavedSegment) error
	FindSegment(context.Context, platform.ID, platform.ID) (*SavedSegment, error)
	ListSegments(context.Context, platform.ID, platform.ID, platform.ID) ([]SavedSegment, error)
	ReplaceSegment(context.Context, SavedSegment, int64) error
	ArchiveSegment(context.Context, platform.ID, platform.ID, int64, time.Time, platform.Actor) error
}

type SegmentService struct {
	Store      SegmentStore
	Platform   UnitOfWork
	Authorizer platform.Authorizer
	Now        func() time.Time
}

func (s SegmentService) Create(ctx context.Context, principal platform.Principal, input SaveSegmentInput, requestID string) (*SavedSegment, error) {
	if s.Store == nil || s.Platform == nil || s.Authorizer == nil {
		return nil, errors.New("segment service is not configured")
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_segment", Message: err.Error()})
	}
	now := s.now()
	if !s.allowed(principal, "create", input.Filter.BranchID, now) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot save segments for this branch."}
	}
	segment := SavedSegment{
		ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: input.Filter.BranchID, SchemaVersion: CurrentSchemaVersion, Version: 1, CreatedAt: now, CreatedBy: principal.Actor, UpdatedAt: now, UpdatedBy: principal.Actor},
		Name:             input.Name, Description: input.Description, Filter: input.Filter, Visibility: input.Visibility, OwnerID: principal.Actor.ID,
	}
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.InsertSegment(tx, segment); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: segment.BranchID, Actor: principal.Actor, Action: "people.segment.create", ResourceType: "people-segment", ResourceID: segment.ID, ChangedFields: []string{"name", "filter", "visibility"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: segment.BranchID, Type: "people.segment.created", EventVersion: 1, AggregateType: "people-segment", AggregateID: segment.ID, AggregateVersion: 1, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"segmentId": segment.ID}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, fmt.Errorf("create saved segment: %w", err)
	}
	return &segment, nil
}

func (s SegmentService) Update(ctx context.Context, principal platform.Principal, segmentID platform.ID, expectedVersion int64, input SaveSegmentInput, requestID string) (*SavedSegment, error) {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return nil, err
	}
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_segment", Message: err.Error()})
	}
	current, err := s.Store.FindSegment(ctx, principal.OrganizationID, segmentID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ArchivedAt != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Saved segment not found."}
	}
	now := s.now()
	if current.OwnerID != principal.Actor.ID || !s.allowed(principal, "update", current.BranchID, now) || input.Filter.BranchID != current.BranchID {
		return nil, &platform.DomainError{Code: "forbidden", Message: "Only the segment owner can update it within its branch."}
	}
	updated := *current
	updated.Name, updated.Description, updated.Filter, updated.Visibility = input.Name, input.Description, input.Filter, input.Visibility
	updated.Version, updated.UpdatedAt, updated.UpdatedBy = expectedVersion+1, now, principal.Actor
	if err := s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.ReplaceSegment(tx, updated, expectedVersion); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: current.BranchID, Actor: principal.Actor, Action: "people.segment.update", ResourceType: "people-segment", ResourceID: segmentID, ChangedFields: []string{"name", "description", "filter", "visibility"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: current.BranchID, Type: "people.segment.updated", EventVersion: 1, AggregateType: "people-segment", AggregateID: segmentID, AggregateVersion: updated.Version, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"segmentId": segmentID}}, State: "pending", AvailableAt: now})
	}); err != nil {
		return nil, fmt.Errorf("update saved segment: %w", err)
	}
	return &updated, nil
}

func (s SegmentService) Archive(ctx context.Context, principal platform.Principal, segmentID platform.ID, expectedVersion int64, requestID string) error {
	if err := platform.RequireExpectedVersion(expectedVersion); err != nil {
		return err
	}
	current, err := s.Store.FindSegment(ctx, principal.OrganizationID, segmentID)
	if err != nil {
		return err
	}
	if current == nil || current.ArchivedAt != nil {
		return &platform.DomainError{Code: "not_found", Message: "Saved segment not found."}
	}
	now := s.now()
	if current.OwnerID != principal.Actor.ID || !s.allowed(principal, "archive", current.BranchID, now) {
		return &platform.DomainError{Code: "forbidden", Message: "Only the segment owner can archive it."}
	}
	return s.Platform.WithTransaction(ctx, func(tx context.Context) error {
		if err := s.Store.ArchiveSegment(tx, principal.OrganizationID, segmentID, expectedVersion, now, principal.Actor); err != nil {
			return err
		}
		if err := s.Platform.AppendAudit(tx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: current.BranchID, Actor: principal.Actor, Action: "people.segment.archive", ResourceType: "people-segment", ResourceID: segmentID, ChangedFields: []string{"archivedAt"}, Outcome: "success", RequestID: requestID, OccurredAt: now}); err != nil {
			return err
		}
		return s.Platform.EnqueueEvent(tx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: current.BranchID, Type: "people.segment.archived", EventVersion: 1, AggregateType: "people-segment", AggregateID: segmentID, AggregateVersion: expectedVersion + 1, Actor: principal.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: map[string]any{"segmentId": segmentID}}, State: "pending", AvailableAt: now})
	})
}

func (s SegmentService) List(ctx context.Context, principal platform.Principal, branchID platform.ID) ([]SavedSegment, error) {
	if !s.allowed(principal, "read", branchID, s.now()) {
		return nil, &platform.DomainError{Code: "forbidden", Message: "You cannot read segments for this branch."}
	}
	return s.Store.ListSegments(ctx, principal.OrganizationID, branchID, principal.Actor.ID)
}

func (s SegmentService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s SegmentService) allowed(principal platform.Principal, action string, branchID platform.ID, now time.Time) bool {
	decision := s.Authorizer.Authorize(principal, platform.AccessRequest{Action: action, ResourceType: "people-segment", OrganizationID: principal.OrganizationID, BranchID: branchID, FieldClasses: []platform.FieldClass{platform.FieldPersonal}, Now: now})
	return decision.Allowed
}

func (r *MongoRepository) InsertSegment(ctx context.Context, segment SavedSegment) error {
	_, err := r.collection.Database().Collection(segmentsCollection).InsertOne(ctx, segment)
	return err
}

func (r *MongoRepository) FindSegment(ctx context.Context, organizationID, segmentID platform.ID) (*SavedSegment, error) {
	var segment SavedSegment
	err := r.collection.Database().Collection(segmentsCollection).FindOne(ctx, bson.M{"_id": segmentID, "organizationId": organizationID}).Decode(&segment)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &segment, err
}

func (r *MongoRepository) ListSegments(ctx context.Context, organizationID, branchID, actorID platform.ID) ([]SavedSegment, error) {
	cursor, err := r.collection.Database().Collection(segmentsCollection).Find(ctx, bson.M{"organizationId": organizationID, "branchId": branchID, "archivedAt": nil, "$or": bson.A{bson.M{"visibility": "organization"}, bson.M{"ownerId": actorID}}}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var segments []SavedSegment
	if err := cursor.All(ctx, &segments); err != nil {
		return nil, err
	}
	return segments, nil
}

func (r *MongoRepository) ReplaceSegment(ctx context.Context, segment SavedSegment, expectedVersion int64) error {
	result, err := r.collection.Database().Collection(segmentsCollection).ReplaceOne(ctx, bson.M{"_id": segment.ID, "organizationId": segment.OrganizationID, "version": expectedVersion, "archivedAt": nil}, segment)
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) ArchiveSegment(ctx context.Context, organizationID, segmentID platform.ID, expectedVersion int64, now time.Time, actor platform.Actor) error {
	result, err := r.collection.Database().Collection(segmentsCollection).UpdateOne(ctx, bson.M{"_id": segmentID, "organizationId": organizationID, "version": expectedVersion, "archivedAt": nil}, bson.M{"$set": bson.M{"archivedAt": now, "archivedBy": actor, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
