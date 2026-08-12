package reporting

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

const savedViewsCollection = "chms_report_saved_views"
const exportRunsCollection = "chms_report_export_runs"
const schedulesCollection = "chms_report_schedules"
const boardPackRunsCollection = "chms_report_board_pack_runs"
const deliveriesCollection = "chms_report_deliveries"

type MongoRepository struct{ database *mongo.Database }

func NewMongoRepository(database *mongo.Database) (*MongoRepository, error) {
	if database == nil {
		return nil, errors.New("reporting database is required")
	}
	return &MongoRepository{database: database}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	definitions := map[string][]mongo.IndexModel{
		savedViewsCollection:    {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "updatedAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)}},
		exportRunsCollection:    {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "createdAt", Value: -1}}}, {Keys: bson.D{{Key: "state", Value: 1}, {Key: "createdAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "artifactHash", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"artifactHash": bson.M{"$type": "string"}})}},
		schedulesCollection:     {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "updatedAt", Value: -1}}}, {Keys: bson.D{{Key: "state", Value: 1}, {Key: "nextRunAt", Value: 1}, {Key: "leaseUntil", Value: 1}}}},
		boardPackRunsCollection: {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "scheduleId", Value: 1}, {Key: "generatedAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "artifactHash", Value: 1}}, Options: options.Index().SetUnique(true)}},
		deliveriesCollection:    {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "recipientUserId", Value: 1}, {Key: "createdAt", Value: -1}}}, {Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 60 * 60)}, {Keys: bson.D{{Key: "runId", Value: 1}, {Key: "recipientUserId", Value: 1}}, Options: options.Index().SetUnique(true)}},
	}
	for collection, models := range definitions {
		if _, err := r.database.Collection(collection).Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}
func (r *MongoRepository) InsertSavedView(ctx context.Context, v SavedView) error {
	_, err := r.database.Collection(savedViewsCollection).InsertOne(ctx, v)
	return err
}
func (r *MongoRepository) UpdateSavedView(ctx context.Context, org, owner, id platform.ID, version int64, input SavedViewInput, now time.Time) error {
	result, err := r.database.Collection(savedViewsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "ownerId": owner, "version": version}, bson.M{"$set": bson.M{"name": input.Name, "query": input.Query, "updatedAt": now}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(version)
	}
	return nil
}
func (r *MongoRepository) FindSavedView(ctx context.Context, org, owner, id platform.ID) (*SavedView, error) {
	var value SavedView
	err := r.database.Collection(savedViewsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "ownerId": owner}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) ListSavedViews(ctx context.Context, org, owner platform.ID) ([]SavedView, error) {
	cursor, err := r.database.Collection(savedViewsCollection).Find(ctx, bson.M{"organizationId": org, "ownerId": owner}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []SavedView{}
	return values, cursor.All(ctx, &values)
}
func (r *MongoRepository) InsertExport(ctx context.Context, v ExportRun) error {
	_, err := r.database.Collection(exportRunsCollection).InsertOne(ctx, v)
	return err
}
func (r *MongoRepository) FindExport(ctx context.Context, org, owner, id platform.ID) (*ExportRun, error) {
	var value ExportRun
	err := r.database.Collection(exportRunsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "ownerId": owner}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) ListExports(ctx context.Context, org, owner platform.ID) ([]ExportRun, error) {
	cursor, err := r.database.Collection(exportRunsCollection).Find(ctx, bson.M{"organizationId": org, "ownerId": owner}, options.Find().SetProjection(bson.M{"artifact": 0}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []ExportRun{}
	return values, cursor.All(ctx, &values)
}
func (r *MongoRepository) ClaimPendingExport(ctx context.Context, now time.Time) (*ExportRun, error) {
	var value ExportRun
	err := r.database.Collection(exportRunsCollection).FindOneAndUpdate(ctx, bson.M{"state": "pending"}, bson.M{"$set": bson.M{"state": "processing", "startedAt": now}}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "createdAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) CompleteExport(ctx context.Context, id platform.ID, artifact []byte, hash, name string, rowCount int, now time.Time) error {
	_, err := r.database.Collection(exportRunsCollection).UpdateOne(ctx, bson.M{"_id": id, "state": "processing"}, bson.M{"$set": bson.M{"state": "completed", "artifact": artifact, "artifactHash": hash, "fileName": name, "contentType": "text/csv; charset=utf-8", "rowCount": rowCount, "completedAt": now}})
	return err
}
func (r *MongoRepository) FailExport(ctx context.Context, id platform.ID, code string, now time.Time) error {
	_, err := r.database.Collection(exportRunsCollection).UpdateOne(ctx, bson.M{"_id": id, "state": "processing"}, bson.M{"$set": bson.M{"state": "failed", "errorCode": code, "completedAt": now}})
	return err
}

func (r *MongoRepository) FindStaffRecipients(ctx context.Context, _ platform.ID, ids []platform.ID) ([]ApprovedRecipient, error) {
	storageIDs := make([]any, 0, len(ids))
	for _, id := range ids {
		if objectID, err := bson.ObjectIDFromHex(string(id)); err == nil {
			storageIDs = append(storageIDs, objectID)
		} else {
			storageIDs = append(storageIDs, id)
		}
	}
	cursor, err := r.database.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": storageIDs}, "invitationStatus": bson.M{"$in": bson.A{"accepted", "active"}}, "email": bson.M{"$type": "string"}}, options.Find().SetProjection(bson.M{"email": 1, "name": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	type staffRow struct {
		ID    any    `bson:"_id"`
		Email string `bson:"email"`
		Name  string `bson:"name"`
	}
	rows := []staffRow{}
	if err = cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	values := make([]ApprovedRecipient, 0, len(rows))
	for _, item := range rows {
		id := platform.ID(fmt.Sprint(item.ID))
		if objectID, ok := item.ID.(bson.ObjectID); ok {
			id = platform.ID(objectID.Hex())
		}
		values = append(values, ApprovedRecipient{UserID: id, Email: item.Email, Name: item.Name})
	}
	return values, nil
}
func (r *MongoRepository) FindStaffRole(ctx context.Context, id platform.ID) (string, []string, []string, []string, bool, error) {
	storageID := any(id)
	if objectID, err := bson.ObjectIDFromHex(string(id)); err == nil {
		storageID = objectID
	}
	var row struct {
		Role                string   `bson:"role"`
		InvitationStatus    string   `bson:"invitationStatus"`
		BranchIDs           []string `bson:"branchIds"`
		MinistryIDs         []string `bson:"ministryIds"`
		AssignedResourceIDs []string `bson:"assignedResourceIds"`
	}
	err := r.database.Collection("users").FindOne(ctx, bson.M{"_id": storageID}).Decode(&row)
	if err == mongo.ErrNoDocuments {
		return "", nil, nil, nil, false, nil
	}
	if err != nil {
		return "", nil, nil, nil, false, err
	}
	if row.Role == "super-admin" {
		row.BranchIDs, row.MinistryIDs = []string{"*"}, []string{"*"}
	}
	return row.Role, row.BranchIDs, row.MinistryIDs, row.AssignedResourceIDs, row.InvitationStatus == "accepted" || row.InvitationStatus == "active", nil
}
func (r *MongoRepository) InsertSchedule(ctx context.Context, v Schedule) error {
	_, err := r.database.Collection(schedulesCollection).InsertOne(ctx, v)
	return err
}
func (r *MongoRepository) ListSchedules(ctx context.Context, org, owner platform.ID) ([]Schedule, error) {
	cursor, err := r.database.Collection(schedulesCollection).Find(ctx, bson.M{"organizationId": org, "ownerId": owner}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []Schedule{}
	return values, cursor.All(ctx, &values)
}
func (r *MongoRepository) ClaimDueSchedule(ctx context.Context, now time.Time) (*Schedule, error) {
	var value Schedule
	err := r.database.Collection(schedulesCollection).FindOneAndUpdate(ctx, bson.M{"state": "active", "nextRunAt": bson.M{"$lte": now}, "$or": bson.A{bson.M{"leaseUntil": bson.M{"$exists": false}}, bson.M{"leaseUntil": bson.M{"$lte": now}}}}, bson.M{"$set": bson.M{"leaseUntil": now.Add(10 * time.Minute), "updatedAt": now}}, options.FindOneAndUpdate().SetSort(bson.D{{Key: "nextRunAt", Value: 1}}).SetReturnDocument(options.After)).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) CompleteSchedule(ctx context.Context, id platform.ID, last, next time.Time) error {
	_, err := r.database.Collection(schedulesCollection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"lastRunAt": last, "nextRunAt": next, "updatedAt": last, "lastErrorCode": ""}, "$unset": bson.M{"leaseUntil": ""}, "$inc": bson.M{"version": 1}})
	return err
}
func (r *MongoRepository) PauseSchedule(ctx context.Context, id platform.ID, code string, now time.Time) error {
	_, err := r.database.Collection(schedulesCollection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"state": "paused", "lastErrorCode": code, "updatedAt": now}, "$unset": bson.M{"leaseUntil": ""}, "$inc": bson.M{"version": 1}})
	return err
}
func (r *MongoRepository) InsertBoardPackRun(ctx context.Context, v BoardPackRun) error {
	_, err := r.database.Collection(boardPackRunsCollection).InsertOne(ctx, v)
	return err
}
func (r *MongoRepository) InsertDelivery(ctx context.Context, v Delivery) error {
	_, err := r.database.Collection(deliveriesCollection).InsertOne(ctx, v)
	return err
}
func (r *MongoRepository) MarkDeliverySent(ctx context.Context, id platform.ID, now time.Time) error {
	_, err := r.database.Collection(deliveriesCollection).UpdateOne(ctx, bson.M{"_id": id, "state": "pending"}, bson.M{"$set": bson.M{"state": "delivered", "deliveredAt": now}})
	return err
}
func (r *MongoRepository) MarkDeliveryFailed(ctx context.Context, id platform.ID, now time.Time) error {
	_, err := r.database.Collection(deliveriesCollection).UpdateOne(ctx, bson.M{"_id": id, "state": "pending"}, bson.M{"$set": bson.M{"state": "failed", "deliveredAt": now}})
	return err
}
func (r *MongoRepository) FindDelivery(ctx context.Context, org, recipient, id platform.ID) (*Delivery, error) {
	var value Delivery
	err := r.database.Collection(deliveriesCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "recipientUserId": recipient}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) FindBoardPackRun(ctx context.Context, org, id platform.ID) (*BoardPackRun, error) {
	var value BoardPackRun
	err := r.database.Collection(boardPackRunsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) MarkDeliveryOpened(ctx context.Context, id platform.ID, now time.Time) error {
	_, err := r.database.Collection(deliveriesCollection).UpdateOne(ctx, bson.M{"_id": id, "openedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"openedAt": now, "state": "opened"}})
	return err
}
func (r *MongoRepository) MarkDeliveryExpired(ctx context.Context, id platform.ID, now time.Time) error {
	_, err := r.database.Collection(deliveriesCollection).UpdateOne(ctx, bson.M{"_id": id, "state": bson.M{"$ne": "expired"}}, bson.M{"$set": bson.M{"state": "expired", "expiredAt": now}})
	return err
}

func (s Service) ListExports(ctx context.Context, p platform.Principal, organizationID platform.ID) ([]ExportRun, error) {
	if s.Repository == nil {
		return nil, errors.New("report repository is unavailable")
	}
	return s.Repository.ListExports(ctx, organizationID, p.Actor.ID)
}
func (s Service) GetExport(ctx context.Context, p platform.Principal, organizationID, id platform.ID) (*ExportRun, error) {
	if s.Repository == nil {
		return nil, errors.New("report repository is unavailable")
	}
	run, err := s.Repository.FindExport(ctx, organizationID, p.Actor.ID, id)
	if err != nil || run == nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Report export not found."}
	}
	if err := s.validateQuery(p, organizationID, run.Result.Query); err != nil {
		return nil, &platform.DomainError{Code: "not_found", Message: "Report export not found."}
	}
	return run, nil
}
