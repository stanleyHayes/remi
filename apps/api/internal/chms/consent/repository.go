package consent

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/chms/platform"
)

const eventsCollection = "chms_consent_events"
const projectionsCollection = "chms_consent_projections"
const suppressionsCollection = "chms_suppressions"

type Repository struct{ db *mongo.Database }

func NewRepository(db *mongo.Database) (*Repository, error) {
	if db == nil {
		return nil, errors.New("consent database is required")
	}
	return &Repository{db: db}, nil
}

func (r *Repository) EnsureIndexes(ctx context.Context) error {
	definitions := []struct {
		name   string
		models []mongo.IndexModel
	}{
		{eventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "occurredAt", Value: -1}}}}},
		{projectionsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "purpose", Value: 1}, {Key: "channel", Value: 1}}, Options: options.Index().SetUnique(true)}}},
		{suppressionsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "state", Value: 1}, {Key: "channel", Value: 1}, {Key: "purpose", Value: 1}}}}},
	}
	for _, definition := range definitions {
		if _, err := r.db.Collection(definition.name).Indexes().CreateMany(ctx, definition.models); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) FindProjection(ctx context.Context, organizationID, personID platform.ID, purpose, channel string) (*Projection, error) {
	var value Projection
	err := r.db.Collection(projectionsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "personId": personID, "purpose": purpose, "channel": channel}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) ListProjections(ctx context.Context, organizationID, personID platform.ID) ([]Projection, error) {
	cursor, err := r.db.Collection(projectionsCollection).Find(ctx, bson.M{"organizationId": organizationID, "personId": personID}, options.Find().SetSort(bson.D{{Key: "purpose", Value: 1}, {Key: "channel", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var values []Projection
	err = cursor.All(ctx, &values)
	if values == nil {
		values = []Projection{}
	}
	return values, err
}
func (r *Repository) InsertEvent(ctx context.Context, value Event) error {
	_, err := r.db.Collection(eventsCollection).InsertOne(ctx, value)
	return err
}
func (r *Repository) InsertProjection(ctx context.Context, value Projection) error {
	_, err := r.db.Collection(projectionsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return platform.VersionConflict(0)
	}
	return err
}
func (r *Repository) UpdateProjection(ctx context.Context, value Projection, expected int64) error {
	result, err := r.db.Collection(projectionsCollection).UpdateOne(ctx, bson.M{"_id": value.ID, "organizationId": value.OrganizationID, "version": expected}, bson.M{"$set": bson.M{"state": value.State, "noticeVersion": value.NoticeVersion, "evidenceReference": value.EvidenceReference, "source": value.Source, "lastEventId": value.LastEventID, "updatedAt": value.UpdatedAt, "updatedBy": value.UpdatedBy}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func (r *Repository) ListActiveSuppressions(ctx context.Context, organizationID, personID platform.ID, purpose, channel string) ([]Suppression, error) {
	filter := bson.M{"organizationId": organizationID, "personId": personID, "state": "active", "$and": bson.A{bson.M{"$or": bson.A{bson.M{"purpose": bson.M{"$exists": false}}, bson.M{"purpose": ""}, bson.M{"purpose": purpose}}}, bson.M{"$or": bson.A{bson.M{"channel": bson.M{"$exists": false}}, bson.M{"channel": ""}, bson.M{"channel": channel}}}}}
	cursor, err := r.db.Collection(suppressionsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var values []Suppression
	err = cursor.All(ctx, &values)
	if values == nil {
		values = []Suppression{}
	}
	return values, err
}
func (r *Repository) ListSuppressions(ctx context.Context, organizationID, personID platform.ID) ([]Suppression, error) {
	cursor, err := r.db.Collection(suppressionsCollection).Find(ctx, bson.M{"organizationId": organizationID, "personId": personID}, options.Find().SetSort(bson.D{{Key: "state", Value: 1}, {Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var values []Suppression
	err = cursor.All(ctx, &values)
	if values == nil {
		values = []Suppression{}
	}
	return values, err
}
func (r *Repository) FindSuppression(ctx context.Context, organizationID, id platform.ID) (*Suppression, error) {
	var value Suppression
	err := r.db.Collection(suppressionsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) InsertSuppression(ctx context.Context, value Suppression) error {
	_, err := r.db.Collection(suppressionsCollection).InsertOne(ctx, value)
	return err
}
func (r *Repository) UpdateSuppression(ctx context.Context, value Suppression, expected int64) error {
	result, err := r.db.Collection(suppressionsCollection).UpdateOne(ctx, bson.M{"_id": value.ID, "organizationId": value.OrganizationID, "version": expected}, bson.M{"$set": bson.M{"state": value.State, "reason": value.Reason, "source": value.Source, "releasedAt": value.ReleasedAt, "updatedAt": value.UpdatedAt, "updatedBy": value.UpdatedBy}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func (r *Repository) UpsertMemberWithdrawal(ctx context.Context, projection Projection, now time.Time, actor platform.Actor) error {
	filter := bson.M{"organizationId": projection.OrganizationID, "personId": projection.PersonID, "purpose": projection.Purpose, "channel": projection.Channel, "source": "member-withdrawal"}
	if projection.State == "withdrawn" {
		_, err := r.db.Collection(suppressionsCollection).UpdateOne(ctx, filter, bson.M{"$setOnInsert": bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": projection.OrganizationID, "branchId": projection.BranchID, "personId": projection.PersonID, "purpose": projection.Purpose, "channel": projection.Channel, "reason": "member-withdrawal", "source": "member-withdrawal", "schemaVersion": CurrentSchemaVersion, "version": 1, "createdAt": now, "createdBy": actor}, "$set": bson.M{"state": "active", "releasedAt": nil, "updatedAt": now, "updatedBy": actor}}, options.UpdateOne().SetUpsert(true))
		return err
	}
	_, err := r.db.Collection(suppressionsCollection).UpdateMany(ctx, filter, bson.M{"$set": bson.M{"state": "released", "releasedAt": now, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	return err
}
