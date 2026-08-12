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

const householdsCollection = "chms_households"
const householdMembershipsCollection = "chms_household_memberships"
const relationshipsCollection = "chms_relationships"

type HouseholdRepository struct{ database *mongo.Database }

type HouseholdStore interface {
	InsertHousehold(context.Context, Household) error
	InsertMembership(context.Context, HouseholdMembership) error
	InsertRelationship(context.Context, Relationship) error
	PersonExists(context.Context, platform.ID, platform.ID) (bool, error)
	HouseholdExists(context.Context, platform.ID, platform.ID) (bool, error)
	EndMembership(context.Context, platform.ID, platform.ID, time.Time, string) error
	FindRelationship(context.Context, platform.ID, platform.ID) (*Relationship, error)
	EndRelationship(context.Context, platform.ID, platform.ID, int64, time.Time, time.Time, platform.Actor) error
	FindHouseholdByID(context.Context, platform.ID, platform.ID) (*Household, error)
	UpdateHousehold(context.Context, platform.ID, platform.ID, int64, UpdateHouseholdInput, time.Time, platform.Actor) error
	FindActiveMembership(context.Context, platform.ID, platform.ID, platform.ID) (*HouseholdMembership, error)
}

func (r *HouseholdRepository) FindProfileByPersonID(ctx context.Context, organizationID, personID platform.ID) (*HouseholdProfile, error) {
	var own HouseholdMembership
	err := r.database.Collection(householdMembershipsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "personId": personID, "endedAt": nil}).Decode(&own)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find household membership: %w", err)
	}
	var household Household
	if err := r.database.Collection(householdsCollection).FindOne(ctx, bson.M{"_id": own.HouseholdID, "organizationId": organizationID, "archivedAt": nil}).Decode(&household); err == mongo.ErrNoDocuments {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("find household: %w", err)
	}
	cursor, err := r.database.Collection(householdMembershipsCollection).Find(ctx, bson.M{"organizationId": organizationID, "householdId": household.ID, "endedAt": nil}, options.Find().SetSort(bson.D{{Key: "startedAt", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list household members: %w", err)
	}
	defer cursor.Close(ctx)
	var memberships []HouseholdMembership
	if err := cursor.All(ctx, &memberships); err != nil {
		return nil, err
	}
	ids := make([]platform.ID, 0, len(memberships))
	for _, membership := range memberships {
		ids = append(ids, membership.PersonID)
	}
	peopleCursor, err := r.database.Collection(collectionName).Find(ctx, bson.M{"organizationId": organizationID, "_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"names": 1, "photoAssetId": 1}))
	if err != nil {
		return nil, fmt.Errorf("load household people: %w", err)
	}
	defer peopleCursor.Close(ctx)
	var personRows []struct {
		ID           platform.ID `bson:"_id"`
		Names        Names       `bson:"names"`
		PhotoAssetID platform.ID `bson:"photoAssetId"`
	}
	if err := peopleCursor.All(ctx, &personRows); err != nil {
		return nil, err
	}
	byID := map[platform.ID]struct {
		Name  string
		Photo platform.ID
	}{}
	for _, row := range personRows {
		name := strings.TrimSpace(strings.Join([]string{row.Names.Given, row.Names.Middle, row.Names.Family}, " "))
		if row.Names.Preferred != "" {
			name = row.Names.Preferred
		}
		byID[row.ID] = struct {
			Name  string
			Photo platform.ID
		}{name, row.PhotoAssetID}
	}
	relCursor, err := r.database.Collection(relationshipsCollection).Find(ctx, bson.M{"organizationId": organizationID, "endedAt": nil, "$or": bson.A{bson.M{"fromPersonId": personID}, bson.M{"toPersonId": personID}}})
	if err != nil {
		return nil, fmt.Errorf("load household relationships: %w", err)
	}
	defer relCursor.Close(ctx)
	var relationships []Relationship
	if err := relCursor.All(ctx, &relationships); err != nil {
		return nil, err
	}
	relationshipByPerson := map[platform.ID]string{}
	for _, rel := range relationships {
		other := rel.ToPersonID
		if other == personID {
			other = rel.FromPersonID
		}
		relationshipByPerson[other] = rel.Type
	}
	profile := &HouseholdProfile{ID: household.ID, Name: household.Name, PrimaryContactID: household.PrimaryContactPersonID, Members: make([]HouseholdProfileMember, 0, len(memberships))}
	if len(household.SharedAddresses) > 0 {
		parts := []string{household.SharedAddresses[0].Line1, household.SharedAddresses[0].Line2, household.SharedAddresses[0].City, household.SharedAddresses[0].Region}
		clean := parts[:0]
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				clean = append(clean, strings.TrimSpace(part))
			}
		}
		profile.SharedAddress = strings.Join(clean, ", ")
	}
	for _, membership := range memberships {
		summary := byID[membership.PersonID]
		profile.Members = append(profile.Members, HouseholdProfileMember{ID: membership.PersonID, Name: summary.Name, Role: membership.Role, Relationship: relationshipByPerson[membership.PersonID], PhotoAssetID: summary.Photo})
	}
	return profile, nil
}

func (r *HouseholdRepository) FindHouseholdByID(ctx context.Context, organizationID, householdID platform.ID) (*Household, error) {
	var household Household
	err := r.database.Collection(householdsCollection).FindOne(ctx, bson.M{"_id": householdID, "organizationId": organizationID}).Decode(&household)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find household: %w", err)
	}
	return &household, nil
}
func (r *HouseholdRepository) UpdateHousehold(ctx context.Context, organizationID, householdID platform.ID, expectedVersion int64, input UpdateHouseholdInput, updatedAt time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(householdsCollection).UpdateOne(ctx, bson.M{"_id": householdID, "organizationId": organizationID, "version": expectedVersion, "archivedAt": nil}, bson.M{"$set": bson.M{"homeBranchId": input.HomeBranchID, "branchId": input.HomeBranchID, "name": input.Name, "sharedContactPoints": input.SharedContactPoints, "sharedAddresses": input.SharedAddresses, "primaryContactPersonId": input.PrimaryContactPersonID, "statementPreference": input.StatementPreference, "updatedAt": updatedAt, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update household: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *HouseholdRepository) ListHouseholds(ctx context.Context, organizationID, branchID platform.ID, limit int64) ([]Household, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	cursor, err := r.database.Collection(householdsCollection).Find(ctx, bson.M{"organizationId": organizationID, "homeBranchId": branchID, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list households: %w", err)
	}
	defer cursor.Close(ctx)
	var households []Household
	if err := cursor.All(ctx, &households); err != nil {
		return nil, err
	}
	if households == nil {
		households = []Household{}
	}
	return households, nil
}
func (r *HouseholdRepository) FindProfileByHouseholdID(ctx context.Context, organizationID, householdID platform.ID) (*HouseholdProfile, error) {
	var membership HouseholdMembership
	err := r.database.Collection(householdMembershipsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "householdId": householdID, "endedAt": nil}).Decode(&membership)
	if err == nil {
		return r.FindProfileByPersonID(ctx, organizationID, membership.PersonID)
	}
	if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("find household member: %w", err)
	}
	household, err := r.FindHouseholdByID(ctx, organizationID, householdID)
	if err != nil || household == nil {
		return nil, err
	}
	return &HouseholdProfile{ID: household.ID, Name: household.Name, PrimaryContactID: household.PrimaryContactPersonID, Members: []HouseholdProfileMember{}}, nil
}
func (r *HouseholdRepository) FindActiveMembership(ctx context.Context, organizationID, householdID, personID platform.ID) (*HouseholdMembership, error) {
	var membership HouseholdMembership
	err := r.database.Collection(householdMembershipsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "householdId": householdID, "personId": personID, "endedAt": nil}).Decode(&membership)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active household membership: %w", err)
	}
	return &membership, nil
}

func NewHouseholdRepository(database *mongo.Database) (*HouseholdRepository, error) {
	if database == nil {
		return nil, errors.New("household database is required")
	}
	return &HouseholdRepository{database: database}, nil
}
func (r *HouseholdRepository) EnsureIndexes(ctx context.Context) error {
	definitions := []struct {
		name   string
		models []mongo.IndexModel
	}{
		{householdsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "homeBranchId", Value: 1}, {Key: "archivedAt", Value: 1}, {Key: "name", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "sharedContactPoints.normalized", Value: 1}}}}},
		{householdMembershipsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "householdId", Value: 1}, {Key: "endedAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "endedAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "householdId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"endedAt": nil})}}},
		{relationshipsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "fromPersonId", Value: 1}, {Key: "endedAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "toPersonId", Value: 1}, {Key: "endedAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "fromPersonId", Value: 1}, {Key: "toPersonId", Value: 1}, {Key: "type", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"endedAt": nil})}}},
	}
	for _, definition := range definitions {
		if _, err := r.database.Collection(definition.name).Indexes().CreateMany(ctx, definition.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", definition.name, err)
		}
	}
	return nil
}
func (r *HouseholdRepository) InsertHousehold(ctx context.Context, household Household) error {
	_, err := r.database.Collection(householdsCollection).InsertOne(ctx, household)
	if err != nil {
		return fmt.Errorf("insert household: %w", err)
	}
	return nil
}
func (r *HouseholdRepository) InsertMembership(ctx context.Context, membership HouseholdMembership) error {
	_, err := r.database.Collection(householdMembershipsCollection).InsertOne(ctx, membership)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This person is already an active member of the household."}
	}
	if err != nil {
		return fmt.Errorf("insert household membership: %w", err)
	}
	return nil
}
func (r *HouseholdRepository) InsertRelationship(ctx context.Context, relationship Relationship) error {
	_, err := r.database.Collection(relationshipsCollection).InsertOne(ctx, relationship)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This active relationship already exists."}
	}
	if err != nil {
		return fmt.Errorf("insert relationship: %w", err)
	}
	return nil
}

func (r *HouseholdRepository) PersonExists(ctx context.Context, organizationID, personID platform.ID) (bool, error) {
	count, err := r.database.Collection(collectionName).CountDocuments(ctx, bson.M{"_id": personID, "organizationId": organizationID, "archivedAt": nil}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("check person: %w", err)
	}
	return count == 1, nil
}
func (r *HouseholdRepository) HouseholdExists(ctx context.Context, organizationID, householdID platform.ID) (bool, error) {
	count, err := r.database.Collection(householdsCollection).CountDocuments(ctx, bson.M{"_id": householdID, "organizationId": organizationID, "archivedAt": nil}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("check household: %w", err)
	}
	return count == 1, nil
}
func (r *HouseholdRepository) EndMembership(ctx context.Context, organizationID, membershipID platform.ID, endedAt time.Time, reason string) error {
	result, err := r.database.Collection(householdMembershipsCollection).UpdateOne(ctx, bson.M{"_id": membershipID, "organizationId": organizationID, "endedAt": nil}, bson.M{"$set": bson.M{"endedAt": endedAt, "endReason": reason}})
	if err != nil {
		return fmt.Errorf("end household membership: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "not_found", Message: "Active household membership not found."}
	}
	return nil
}
func (r *HouseholdRepository) FindRelationship(ctx context.Context, organizationID, relationshipID platform.ID) (*Relationship, error) {
	var relationship Relationship
	err := r.database.Collection(relationshipsCollection).FindOne(ctx, bson.M{"_id": relationshipID, "organizationId": organizationID}).Decode(&relationship)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find relationship: %w", err)
	}
	return &relationship, nil
}
func (r *HouseholdRepository) EndRelationship(ctx context.Context, organizationID, relationshipID platform.ID, expectedVersion int64, endedAt, updatedAt time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(relationshipsCollection).UpdateOne(ctx, bson.M{"_id": relationshipID, "organizationId": organizationID, "version": expectedVersion, "endedAt": nil}, bson.M{"$set": bson.M{"endedAt": endedAt, "updatedAt": updatedAt, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("end relationship: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
