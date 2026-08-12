package people

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

const collectionName = "chms_people"

type Repository interface {
	Insert(context.Context, Person) error
	FindDuplicateIDs(context.Context, platform.ID, []ContactPoint) ([]platform.ID, error)
	FindByID(context.Context, platform.ID, platform.ID) (*Person, error)
	Update(context.Context, platform.ID, platform.ID, int64, UpdateInput, time.Time, platform.Actor) error
	SetArchive(context.Context, platform.ID, platform.ID, int64, *time.Time, *platform.Actor, string, time.Time, platform.Actor) error
	TransitionMembership(context.Context, platform.ID, platform.ID, int64, string, MembershipEvent, time.Time, platform.Actor) error
	FindMembershipEvent(context.Context, platform.ID, platform.ID, platform.ID) (*MembershipEvent, error)
	ReverseMembershipEvent(context.Context, platform.ID, platform.ID, int64, MembershipEvent, platform.ID, time.Time, platform.Actor) error
}
type MongoRepository struct{ collection *mongo.Collection }

func (r *MongoRepository) ResolveVolunteerEligibility(ctx context.Context, organizationID, id platform.ID) (platform.ID, bool, string, string, string, error) {
	value, err := r.FindByID(ctx, organizationID, id)
	if err != nil {
		return "", false, "", "", "", err
	}
	if value == nil {
		return "", false, "", "", "", nil
	}
	birthValue, birthPrecision := "", ""
	if value.DateOfBirth != nil {
		birthValue, birthPrecision = value.DateOfBirth.Value, value.DateOfBirth.Precision
	}
	return value.HomeBranchID, value.ArchivedAt == nil, value.MembershipStage, birthValue, birthPrecision, nil
}

func NewMongoRepository(database *mongo.Database) (*MongoRepository, error) {
	if database == nil {
		return nil, errors.New("people database is required")
	}
	return &MongoRepository{collection: database.Collection(collectionName)}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personNumber", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "contactPoints.normalized", Value: 1}}},
		{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "homeBranchId", Value: 1}, {Key: "membershipStage", Value: 1}, {Key: "archivedAt", Value: 1}}},
		{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "names.family", Value: 1}, {Key: "names.given", Value: 1}, {Key: "_id", Value: 1}}},
	}
	if _, err := r.collection.Indexes().CreateMany(ctx, models); err != nil {
		return fmt.Errorf("create people indexes: %w", err)
	}
	if _, err := r.collection.Database().Collection("chms_membership_events").Indexes().CreateMany(ctx, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "effectiveAt", Value: -1}, {Key: "createdAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "reversesEventId", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"reversesEventId": bson.M{"$gt": ""}})}}); err != nil {
		return fmt.Errorf("create membership event indexes: %w", err)
	}
	definitions := []struct {
		collection string
		models     []mongo.IndexModel
	}{
		{segmentsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "archivedAt", Value: 1}, {Key: "name", Value: 1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "ownerId", Value: 1}, {Key: "updatedAt", Value: -1}}},
		}},
		{bulkPreviewsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "createdBy.id", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		}},
		{bulkJobsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "state", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "previewId", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
		{bulkResultsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "jobId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "jobId", Value: 1}, {Key: "state", Value: 1}, {Key: "processedAt", Value: 1}, {Key: "_id", Value: 1}}},
		}},
		{mergeAliasesCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "aliasPersonId", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "canonicalPersonId", Value: 1}, {Key: "createdAt", Value: -1}}},
		}},
		{mergeEventsCollection, []mongo.IndexModel{
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "canonicalPersonId", Value: 1}, {Key: "createdAt", Value: -1}}},
			{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "duplicatePersonId", Value: 1}}, Options: options.Index().SetUnique(true)},
		}},
	}
	for _, definition := range definitions {
		if _, err := r.collection.Database().Collection(definition.collection).Indexes().CreateMany(ctx, definition.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", definition.collection, err)
		}
	}
	return nil
}
func (r *MongoRepository) Insert(ctx context.Context, person Person) error {
	if _, err := r.collection.InsertOne(ctx, person); err != nil {
		return fmt.Errorf("insert person: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindDuplicateIDs(ctx context.Context, organizationID platform.ID, contacts []ContactPoint) ([]platform.ID, error) {
	values := []string{}
	for _, point := range contacts {
		if point.Normalized != "" {
			values = append(values, point.Normalized)
		}
	}
	if len(values) == 0 {
		return nil, nil
	}
	cursor, err := r.collection.Find(ctx, bson.M{"organizationId": organizationID, "contactPoints.normalized": bson.M{"$in": values}, "archivedAt": nil}, options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(20))
	if err != nil {
		return nil, fmt.Errorf("find duplicate people: %w", err)
	}
	defer cursor.Close(ctx)
	var rows []struct {
		ID platform.ID `bson:"_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	ids := make([]platform.ID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}
func (r *MongoRepository) FindByID(ctx context.Context, organizationID, personID platform.ID) (*Person, error) {
	var person Person
	err := r.collection.FindOne(ctx, bson.M{"_id": personID, "organizationId": organizationID}).Decode(&person)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find person: %w", err)
	}
	return &person, nil
}

// ResolvePersonReference is the narrow cross-context read port used by
// participation. It deliberately exposes only scope and active state, never a
// person's identity or contact fields.
func (r *MongoRepository) ResolvePersonReference(ctx context.Context, organizationID, personID platform.ID) (platform.ID, bool, error) {
	person, err := r.FindByID(ctx, organizationID, personID)
	if err != nil {
		return "", false, err
	}
	if person == nil || person.ArchivedAt != nil {
		return "", false, nil
	}
	return person.HomeBranchID, true, nil
}

func (r *MongoRepository) Update(ctx context.Context, organizationID, personID platform.ID, expectedVersion int64, input UpdateInput, updatedAt time.Time, actor platform.Actor) error {
	set := bson.M{"homeBranchId": input.HomeBranchID, "branchId": input.HomeBranchID, "names": input.Names, "aliases": input.Aliases, "photoAssetId": input.PhotoAssetID, "dateOfBirth": input.DateOfBirth, "gender": input.Gender, "contactPoints": input.ContactPoints, "addresses": input.Addresses, "membershipStage": input.MembershipStage, "tags": input.Tags, "customFields": input.CustomFields, "communicationPreferences": input.CommunicationPreferences, "updatedAt": updatedAt, "updatedBy": actor}
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": personID, "organizationId": organizationID, "version": expectedVersion, "archivedAt": nil}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update person: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) SetArchive(ctx context.Context, organizationID, personID platform.ID, expectedVersion int64, archivedAt *time.Time, archivedBy *platform.Actor, reason string, updatedAt time.Time, updatedBy platform.Actor) error {
	set := bson.M{"archivedAt": archivedAt, "archivedBy": archivedBy, "archiveReason": reason, "updatedAt": updatedAt, "updatedBy": updatedBy}
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": personID, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("set person archive state: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) TransitionMembership(ctx context.Context, organizationID, personID platform.ID, expectedVersion int64, toStage string, event MembershipEvent, updatedAt time.Time, actor platform.Actor) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": personID, "organizationId": organizationID, "version": expectedVersion, "archivedAt": nil, "membershipStage": event.FromStage}, bson.M{"$set": bson.M{"membershipStage": toStage, "updatedAt": updatedAt, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update membership projection: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	if _, err := r.collection.Database().Collection("chms_membership_events").InsertOne(ctx, event); err != nil {
		return fmt.Errorf("append membership event: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindMembershipEvent(ctx context.Context, organizationID, personID, eventID platform.ID) (*MembershipEvent, error) {
	var event MembershipEvent
	err := r.collection.Database().Collection("chms_membership_events").FindOne(ctx, bson.M{"_id": eventID, "organizationId": organizationID, "personId": personID}).Decode(&event)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find membership event: %w", err)
	}
	return &event, nil
}
func (r *MongoRepository) ListMembershipEvents(ctx context.Context, organizationID, personID platform.ID, limit int64) ([]MembershipEvent, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	cursor, err := r.collection.Database().Collection("chms_membership_events").Find(ctx, bson.M{"organizationId": organizationID, "personId": personID}, options.Find().SetSort(bson.D{{Key: "effectiveAt", Value: -1}, {Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list membership events: %w", err)
	}
	defer cursor.Close(ctx)
	var events []MembershipEvent
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	if events == nil {
		events = []MembershipEvent{}
	}
	return events, nil
}
func (r *MongoRepository) ReverseMembershipEvent(ctx context.Context, organizationID, personID platform.ID, expectedVersion int64, reversal MembershipEvent, reversedEventID platform.ID, updatedAt time.Time, actor platform.Actor) error {
	events := r.collection.Database().Collection("chms_membership_events")
	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": personID, "organizationId": organizationID, "version": expectedVersion, "archivedAt": nil, "membershipStage": reversal.FromStage}, bson.M{"$set": bson.M{"membershipStage": reversal.ToStage, "updatedAt": updatedAt, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("reverse membership projection: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	markResult, err := events.UpdateOne(ctx, bson.M{"_id": reversedEventID, "organizationId": organizationID, "reversedByEventId": bson.M{"$in": bson.A{"", nil}}}, bson.M{"$set": bson.M{"reversedByEventId": reversal.ID}})
	if err != nil {
		return fmt.Errorf("mark membership event reversed: %w", err)
	}
	if markResult.MatchedCount != 1 {
		return &platform.DomainError{Code: "invalid_transition", Message: "Membership transition has already been reversed."}
	}
	if _, err := events.InsertOne(ctx, reversal); err != nil {
		return fmt.Errorf("append membership reversal: %w", err)
	}
	return nil
}
