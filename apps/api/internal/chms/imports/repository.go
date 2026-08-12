package imports

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const runsCollection = "chms_import_runs"
const rowsCollection = "chms_import_rows"

type Repository interface {
	InsertRunAndRows(context.Context, ImportRun, []ImportRow) error
	FindBySourceHash(context.Context, platform.ID, string, string) (*ImportRun, error)
	FindRun(context.Context, platform.ID, platform.ID) (*ImportRun, error)
	SetMapping(context.Context, platform.ID, platform.ID, int64, MappingInput, time.Time, platform.Actor) error
	SetRunState(context.Context, platform.ID, platform.ID, string, string, time.Time, platform.Actor) error
	ListRowsAfter(context.Context, platform.ID, platform.ID, int, int) ([]ImportRow, error)
	UpdateRowValidation(context.Context, platform.ID, platform.ID, []ImportRow) error
	FinishValidation(context.Context, platform.ID, platform.ID, int, int, time.Time, platform.Actor) error
	ListRowsByStatesAfter(context.Context, platform.ID, platform.ID, []string, int, int) ([]ImportRow, error)
	RecordRowCommitted(context.Context, platform.ID, platform.ID, platform.ID, int, platform.ID, int64, time.Time) error
	ListRowsByStates(context.Context, platform.ID, platform.ID, []string) ([]ImportRow, error)
	CompleteCommit(context.Context, platform.ID, platform.ID, int, string, time.Time, platform.Actor) error
	RecordRowRollback(context.Context, platform.ID, platform.ID, platform.ID, bool, RowError, time.Time) error
	FinishRollback(context.Context, platform.ID, platform.ID, int, string, time.Time, platform.Actor) error
}
type MongoRepository struct{ database *mongo.Database }

func NewMongoRepository(database *mongo.Database) (*MongoRepository, error) {
	if database == nil {
		return nil, errors.New("import database is required")
	}
	return &MongoRepository{database: database}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	defs := []struct {
		name   string
		models []mongo.IndexModel
	}{{runsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "entityType", Value: 1}, {Key: "sourceHash", Value: 1}, {Key: "state", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "createdAt", Value: -1}}}}}, {rowsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "importRunId", Value: 1}, {Key: "rowNumber", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "importRunId", Value: 1}, {Key: "state", Value: 1}, {Key: "rowNumber", Value: 1}}}}}}
	for _, def := range defs {
		if _, err := r.database.Collection(def.name).Indexes().CreateMany(ctx, def.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", def.name, err)
		}
	}
	return nil
}
func (r *MongoRepository) InsertRunAndRows(ctx context.Context, run ImportRun, rows []ImportRow) error {
	if _, err := r.database.Collection(runsCollection).InsertOne(ctx, run); err != nil {
		return fmt.Errorf("insert import run: %w", err)
	}
	documents := make([]any, len(rows))
	for i := range rows {
		documents[i] = rows[i]
	}
	if _, err := r.database.Collection(rowsCollection).InsertMany(ctx, documents); err != nil {
		return fmt.Errorf("insert import rows: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindBySourceHash(ctx context.Context, organizationID platform.ID, entityType, hash string) (*ImportRun, error) {
	var run ImportRun
	err := r.database.Collection(runsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "entityType": entityType, "sourceHash": hash, "state": bson.M{"$nin": bson.A{"voided", "rolled-back"}}}, options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}})).Decode(&run)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find import source: %w", err)
	}
	return &run, nil
}

func (r *MongoRepository) FindRun(ctx context.Context, organizationID, runID platform.ID) (*ImportRun, error) {
	var run ImportRun
	err := r.database.Collection(runsCollection).FindOne(ctx, bson.M{"_id": runID, "organizationId": organizationID}).Decode(&run)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find import run: %w", err)
	}
	return &run, nil
}
func (r *MongoRepository) SetMapping(ctx context.Context, organizationID, runID platform.ID, expectedVersion int64, mapping MappingInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(runsCollection).UpdateOne(ctx, bson.M{"_id": runID, "organizationId": organizationID, "version": expectedVersion, "state": bson.M{"$in": bson.A{"staged", "mapped", "validated"}}}, bson.M{"$set": bson.M{"mappingVersion": mapping.Version, "mapping": mapping.Fields, "state": "mapped", "validRows": 0, "invalidRows": 0, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("set import mapping: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	_, err = r.database.Collection(rowsCollection).UpdateMany(ctx, bson.M{"organizationId": organizationID, "importRunId": runID}, bson.M{"$set": bson.M{"state": "staged", "mapped": bson.M{}, "errors": bson.A{}}})
	if err != nil {
		return fmt.Errorf("reset mapped import rows: %w", err)
	}
	return nil
}
func (r *MongoRepository) SetRunState(ctx context.Context, organizationID, runID platform.ID, from, to string, now time.Time, actor platform.Actor) error {
	if !CanTransition(from, to) {
		return &platform.DomainError{Code: "invalid_transition", Message: "Import run cannot make that transition."}
	}
	result, err := r.database.Collection(runsCollection).UpdateOne(ctx, bson.M{"_id": runID, "organizationId": organizationID, "state": from}, bson.M{"$set": bson.M{"state": to, "updatedAt": now, "updatedBy": actor}})
	if err != nil {
		return fmt.Errorf("transition import run: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "invalid_transition", Message: "Import run state changed before this action completed."}
	}
	return nil
}
func (r *MongoRepository) ListRowsAfter(ctx context.Context, organizationID, runID platform.ID, afterRow, limit int) ([]ImportRow, error) {
	if limit < 1 || limit > 2000 {
		return nil, errors.New("import row page limit must be 1 to 2000")
	}
	cursor, err := r.database.Collection(rowsCollection).Find(ctx, bson.M{"organizationId": organizationID, "importRunId": runID, "rowNumber": bson.M{"$gt": afterRow}}, options.Find().SetSort(bson.D{{Key: "rowNumber", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list import rows: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []ImportRow
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode import rows: %w", err)
	}
	return rows, nil
}
func (r *MongoRepository) UpdateRowValidation(ctx context.Context, organizationID, runID platform.ID, rows []ImportRow) error {
	if len(rows) == 0 {
		return nil
	}
	models := make([]mongo.WriteModel, len(rows))
	for i, row := range rows {
		models[i] = mongo.NewUpdateOneModel().SetFilter(bson.M{"_id": row.ID, "organizationId": organizationID, "importRunId": runID}).SetUpdate(bson.M{"$set": bson.M{"mapped": row.Mapped, "state": row.State, "errors": row.Errors}})
	}
	if _, err := r.database.Collection(rowsCollection).BulkWrite(ctx, models, options.BulkWrite().SetOrdered(true)); err != nil {
		return fmt.Errorf("update import row validation: %w", err)
	}
	return nil
}
func (r *MongoRepository) FinishValidation(ctx context.Context, organizationID, runID platform.ID, valid, invalid int, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(runsCollection).UpdateOne(ctx, bson.M{"_id": runID, "organizationId": organizationID, "state": "validating"}, bson.M{"$set": bson.M{"state": "validated", "validRows": valid, "invalidRows": invalid, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("finish import validation: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "invalid_transition", Message: "Import validation is no longer active."}
	}
	return nil
}
func (r *MongoRepository) ListRowsByStatesAfter(ctx context.Context, organizationID, runID platform.ID, states []string, afterRow, limit int) ([]ImportRow, error) {
	if limit < 1 || limit > 500 {
		return nil, errors.New("import commit page limit must be 1 to 500")
	}
	cursor, err := r.database.Collection(rowsCollection).Find(ctx, bson.M{"organizationId": organizationID, "importRunId": runID, "state": bson.M{"$in": states}, "rowNumber": bson.M{"$gt": afterRow}}, options.Find().SetSort(bson.D{{Key: "rowNumber", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list import commit rows: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []ImportRow
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode import commit rows: %w", err)
	}
	return rows, nil
}
func (r *MongoRepository) RecordRowCommitted(ctx context.Context, organizationID, runID, rowID platform.ID, rowNumber int, resourceID platform.ID, resourceVersion int64, now time.Time) error {
	result, err := r.database.Collection(rowsCollection).UpdateOne(ctx, bson.M{"_id": rowID, "organizationId": organizationID, "importRunId": runID, "state": "valid"}, bson.M{"$set": bson.M{"state": "committed", "canonicalResourceId": resourceID, "resourceVersionAtCommit": resourceVersion, "committedAt": now}})
	if err != nil {
		return fmt.Errorf("record committed import row: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "conflict", Message: "Import row is no longer committable."}
	}
	runResult, err := r.database.Collection(runsCollection).UpdateOne(ctx, bson.M{"_id": runID, "organizationId": organizationID, "state": "committing"}, bson.M{"$inc": bson.M{"committedRows": 1}, "$max": bson.M{"commitCursor": rowNumber}})
	if err != nil {
		return fmt.Errorf("advance import commit: %w", err)
	}
	if runResult.MatchedCount != 1 {
		return &platform.DomainError{Code: "invalid_transition", Message: "Import run is no longer committing."}
	}
	return nil
}
func (r *MongoRepository) ListRowsByStates(ctx context.Context, organizationID, runID platform.ID, states []string) ([]ImportRow, error) {
	cursor, err := r.database.Collection(rowsCollection).Find(ctx, bson.M{"organizationId": organizationID, "importRunId": runID, "state": bson.M{"$in": states}}, options.Find().SetSort(bson.D{{Key: "rowNumber", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list import manifest rows: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []ImportRow
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode import manifest rows: %w", err)
	}
	return rows, nil
}
func (r *MongoRepository) CompleteCommit(ctx context.Context, organizationID, runID platform.ID, committed int, manifest string, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(runsCollection).UpdateOne(ctx, bson.M{"_id": runID, "organizationId": organizationID, "state": "committing", "committedRows": committed}, bson.M{"$set": bson.M{"state": "completed", "manifestHash": manifest, "committedAt": now, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("complete import commit: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "conflict", Message: "Import committed-row count changed before completion."}
	}
	return nil
}
func (r *MongoRepository) RecordRowRollback(ctx context.Context, organizationID, runID, rowID platform.ID, success bool, rowError RowError, now time.Time) error {
	state := "rolled-back"
	set := bson.M{"state": state, "rolledBackAt": now}
	if !success {
		state = "rollback-blocked"
		set = bson.M{"state": state, "errors": bson.A{rowError}}
	}
	result, err := r.database.Collection(rowsCollection).UpdateOne(ctx, bson.M{"_id": rowID, "organizationId": organizationID, "importRunId": runID, "state": bson.M{"$in": bson.A{"committed", "rollback-blocked"}}}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("record import row rollback: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "conflict", Message: "Import row is no longer rollback eligible."}
	}
	return nil
}
func (r *MongoRepository) FinishRollback(ctx context.Context, organizationID, runID platform.ID, blocked int, manifest string, now time.Time, actor platform.Actor) error {
	state := "rolled-back"
	if blocked > 0 {
		state = "rollback-blocked"
	}
	result, err := r.database.Collection(runsCollection).UpdateOne(ctx, bson.M{"_id": runID, "organizationId": organizationID, "state": "rolling-back"}, bson.M{"$set": bson.M{"state": state, "rollbackBlockedRows": blocked, "rollbackManifestHash": manifest, "rolledBackAt": now, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("finish import rollback: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "invalid_transition", Message: "Import rollback is no longer active."}
	}
	return nil
}
