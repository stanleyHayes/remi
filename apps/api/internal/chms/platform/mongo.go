package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	auditCollection       = "chms_audit_events"
	outboxCollection      = "chms_outbox"
	idempotencyCollection = "chms_idempotency"
	jobsCollection        = "chms_jobs"
)

type MongoPlatformStore struct{ database *mongo.Database }

func NewMongoPlatformStore(database *mongo.Database) (*MongoPlatformStore, error) {
	if database == nil {
		return nil, errors.New("chms platform database is required")
	}
	return &MongoPlatformStore{database: database}, nil
}

func (s *MongoPlatformStore) EnsureIndexes(ctx context.Context) error {
	definitions := []struct {
		collection string
		models     []mongo.IndexModel
	}{
		{auditCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurredAt", Value: -1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "resourceType", Value: 1}, {Key: "resourceId", Value: 1}, {Key: "occurredAt", Value: -1}}},
		}},
		{outboxCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "state", Value: 1}, {Key: "availableAt", Value: 1}, {Key: "recordedAt", Value: 1}}},
		}},
		{idempotencyCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "scope", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{jobsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "state", Value: 1}, {Key: "availableAt", Value: 1}, {Key: "createdAt", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "type", Value: 1}, {Key: "createdAt", Value: -1}}},
		}},
	}
	for _, definition := range definitions {
		if _, err := s.database.Collection(definition.collection).Indexes().CreateMany(ctx, definition.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", definition.collection, err)
		}
	}
	return nil
}

// WithTransaction ensures domain state, safe audit and outbox events commit together.
// The callback must use the supplied context for every Mongo operation.
func (s *MongoPlatformStore) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	session, err := s.database.Client().StartSession()
	if err != nil {
		return fmt.Errorf("start chms transaction: %w", err)
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tx context.Context) (any, error) {
		if err := fn(tx); err != nil {
			return nil, err
		}
		return struct{}{}, nil
	})
	if err != nil {
		// Local/demo Mongo commonly runs as a single standalone node. It cannot
		// start multi-document transactions, but every domain operation still
		// needs to remain usable for local verification. The server rejects the
		// first transactional write before applying it, so retrying the callback
		// without a session is safe. Production deployments should use a replica
		// set and therefore never enter this compatibility path.
		var commandErr mongo.CommandError
		if (errors.As(err, &commandErr) && commandErr.Code == 20) || strings.Contains(strings.ToLower(err.Error()), "transaction numbers are only allowed") {
			if fallbackErr := fn(ctx); fallbackErr != nil {
				return fmt.Errorf("chms standalone transaction fallback: %w", fallbackErr)
			}
			return nil
		}
		return fmt.Errorf("chms transaction: %w", err)
	}
	return nil
}

func (s *MongoPlatformStore) AppendAudit(ctx context.Context, event AuditEvent) error {
	if !event.ID.Valid() || !event.OrganizationID.Valid() || event.Action == "" || event.ResourceType == "" || !event.ResourceID.Valid() || event.RequestID == "" {
		return errors.New("audit event is missing required safe metadata")
	}
	_, err := s.database.Collection(auditCollection).InsertOne(ctx, event)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

func (s *MongoPlatformStore) EnqueueEvent(ctx context.Context, record OutboxRecord) error {
	if !record.ID.Valid() || !record.OrganizationID.Valid() || record.Type == "" || record.EventVersion < 1 || !record.AggregateID.Valid() {
		return errors.New("outbox event is missing required metadata")
	}
	if record.State == "" {
		record.State = "pending"
	}
	if record.AvailableAt.IsZero() {
		record.AvailableAt = record.RecordedAt
	}
	_, err := s.database.Collection(outboxCollection).InsertOne(ctx, record)
	if err != nil {
		return fmt.Errorf("enqueue domain event: %w", err)
	}
	return nil
}

func (s *MongoPlatformStore) Begin(ctx context.Context, scope, key, requestHash string, now time.Time, expiresAt *time.Time) (IdempotencyRecord, bool, error) {
	if scope == "" || requestHash == "" {
		return IdempotencyRecord{}, false, errors.New("idempotency scope and request hash are required")
	}
	if err := ValidateIdempotencyKey(key); err != nil {
		return IdempotencyRecord{}, false, err
	}
	record := IdempotencyRecord{Scope: scope, Key: key, RequestHash: requestHash, State: "processing", CreatedAt: now.UTC(), ExpiresAt: expiresAt}
	_, err := s.database.Collection(idempotencyCollection).InsertOne(ctx, record)
	if err == nil {
		return record, true, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return IdempotencyRecord{}, false, fmt.Errorf("begin idempotent command: %w", err)
	}
	var existing IdempotencyRecord
	if findErr := s.database.Collection(idempotencyCollection).FindOne(ctx, bson.M{"scope": scope, "key": key}).Decode(&existing); findErr != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("read idempotent command: %w", findErr)
	}
	if existing.RequestHash != requestHash {
		return existing, false, &DomainError{Code: "idempotency_key_reused", Message: "This idempotency key was already used for a different request."}
	}
	if existing.State == "processing" && existing.CreatedAt.Before(now.UTC().Add(-10*time.Minute)) {
		result, reclaimErr := s.database.Collection(idempotencyCollection).UpdateOne(ctx, bson.M{"scope": scope, "key": key, "requestHash": requestHash, "state": "processing", "createdAt": existing.CreatedAt}, bson.M{"$set": bson.M{"createdAt": now.UTC(), "expiresAt": expiresAt}})
		if reclaimErr != nil {
			return existing, false, fmt.Errorf("reclaim idempotent command: %w", reclaimErr)
		}
		if result.ModifiedCount == 1 {
			existing.CreatedAt = now.UTC()
			existing.ExpiresAt = expiresAt
			return existing, true, nil
		}
	}
	return existing, false, nil
}

func (s *MongoPlatformStore) Complete(ctx context.Context, scope, key, requestHash string, status int, body []byte, resourceID ID) error {
	result, err := s.database.Collection(idempotencyCollection).UpdateOne(ctx, bson.M{"scope": scope, "key": key, "requestHash": requestHash, "state": "processing"}, bson.M{"$set": bson.M{"state": "completed", "responseStatus": status, "responseBody": body, "resourceId": resourceID}})
	if err != nil {
		return fmt.Errorf("complete idempotent command: %w", err)
	}
	if result.MatchedCount != 1 {
		return errors.New("idempotent command is not in a completable state")
	}
	return nil
}

func (s *MongoPlatformStore) Release(ctx context.Context, scope, key, requestHash string) error {
	_, err := s.database.Collection(idempotencyCollection).DeleteOne(ctx, bson.M{"scope": scope, "key": key, "requestHash": requestHash, "state": "processing"})
	if err != nil {
		return fmt.Errorf("release idempotent command: %w", err)
	}
	return nil
}

func (s *MongoPlatformStore) Claim(ctx context.Context, workerID string, now time.Time, lease time.Duration) (*Job, error) {
	filter := bson.M{"availableAt": bson.M{"$lte": now}, "$or": bson.A{bson.M{"state": "queued"}, bson.M{"state": "running", "claimedAt": bson.M{"$lte": now.Add(-lease)}}}}
	update := bson.M{"$set": bson.M{"state": "running", "claimedAt": now, "claimedBy": workerID}, "$inc": bson.M{"attempts": 1}}
	opts := options.FindOneAndUpdate().SetSort(bson.D{{Key: "availableAt", Value: 1}, {Key: "createdAt", Value: 1}}).SetReturnDocument(options.After)
	var job Job
	err := s.database.Collection(jobsCollection).FindOneAndUpdate(ctx, filter, update, opts).Decode(&job)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return &job, nil
}

func (s *MongoPlatformStore) CompleteJob(ctx context.Context, id ID, workerID string, now time.Time) error {
	return s.transitionClaimedJob(ctx, id, workerID, bson.M{"state": "completed", "completedAt": now, "claimedAt": nil, "claimedBy": ""})
}
func (s *MongoPlatformStore) Retry(ctx context.Context, id ID, workerID, code string, availableAt time.Time) error {
	return s.transitionClaimedJob(ctx, id, workerID, bson.M{"state": "queued", "availableAt": availableAt, "lastErrorCode": code, "claimedAt": nil, "claimedBy": ""})
}
func (s *MongoPlatformStore) DeadLetter(ctx context.Context, id ID, workerID, code string, now time.Time) error {
	return s.transitionClaimedJob(ctx, id, workerID, bson.M{"state": "dead-letter", "completedAt": now, "lastErrorCode": code, "claimedAt": nil, "claimedBy": ""})
}

func (s *MongoPlatformStore) transitionClaimedJob(ctx context.Context, id ID, workerID string, set bson.M) error {
	result, err := s.database.Collection(jobsCollection).UpdateOne(ctx, bson.M{"_id": id, "state": "running", "claimedBy": workerID}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("transition job: %w", err)
	}
	if result.MatchedCount != 1 {
		return errors.New("job lease is no longer owned by worker")
	}
	return nil
}

// MongoJobStore adapts method naming shared by the platform store to JobStore.
type MongoJobStore struct{ Store *MongoPlatformStore }

func (s MongoJobStore) Claim(ctx context.Context, w string, n time.Time, l time.Duration) (*Job, error) {
	return s.Store.Claim(ctx, w, n, l)
}
func (s MongoJobStore) Complete(ctx context.Context, id ID, w string, n time.Time) error {
	return s.Store.CompleteJob(ctx, id, w, n)
}
func (s MongoJobStore) Retry(ctx context.Context, id ID, w, c string, n time.Time) error {
	return s.Store.Retry(ctx, id, w, c, n)
}
func (s MongoJobStore) DeadLetter(ctx context.Context, id ID, w, c string, n time.Time) error {
	return s.Store.DeadLetter(ctx, id, w, c, n)
}
