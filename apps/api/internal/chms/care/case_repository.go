package care

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const careCasesCollection = "chms_care_cases"
const restrictedNotesCollection = "chms_restricted_notes"
const contactEventsCollection = "chms_contact_events"

func (r *MongoRepository) EnsureCaseIndexes(ctx context.Context) error {
	definitions := []struct {
		name   string
		models []mongo.IndexModel
	}{{careCasesCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "ministryId", Value: 1}, {Key: "state", Value: 1}, {Key: "dueAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "assignedUserIds", Value: 1}, {Key: "state", Value: 1}, {Key: "updatedAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "updatedAt", Value: -1}}}}}, {restrictedNotesCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "caseId", Value: 1}, {Key: "createdAt", Value: 1}}}}}, {contactEventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "caseId", Value: 1}, {Key: "occurredAt", Value: -1}}}}}}
	for _, definition := range definitions {
		if _, err := r.database.Collection(definition.name).Indexes().CreateMany(ctx, definition.models); err != nil {
			return err
		}
	}
	return nil
}
func (r *MongoRepository) InsertCase(c context.Context, v CareCase) error {
	_, e := r.database.Collection(careCasesCollection).InsertOne(c, v)
	return e
}
func (r *MongoRepository) FindCase(c context.Context, o, id platform.ID) (*CareCase, error) {
	var v CareCase
	e := r.database.Collection(careCasesCollection).FindOne(c, bson.M{"_id": id, "organizationId": o}).Decode(&v)
	if e == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, e
}
func (r *MongoRepository) ListCases(c context.Context, o, b platform.ID, assignees []platform.ID, includeClosed bool) ([]CareCase, error) {
	f := bson.M{"organizationId": o, "branchId": b, "assignedUserIds": bson.M{"$in": assignees}}
	if !includeClosed {
		f["state"] = bson.M{"$ne": "closed"}
	}
	cur, e := r.database.Collection(careCasesCollection).Find(c, f, options.Find().SetSort(bson.D{{Key: "urgency", Value: -1}, {Key: "dueAt", Value: 1}, {Key: "updatedAt", Value: -1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(c)
	var v []CareCase
	e = cur.All(c, &v)
	if v == nil {
		v = []CareCase{}
	}
	return v, e
}
func (r *MongoRepository) UpdateAssignment(c context.Context, o, id platform.ID, ver int64, team platform.ID, users []platform.ID, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(careCasesCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "version": ver, "state": bson.M{"$ne": "closed"}}, bson.M{"$set": bson.M{"assignedTeamId": team, "assignedUserIds": users, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return platform.VersionConflict(ver)
	}
	return nil
}
func (r *MongoRepository) CloseCase(c context.Context, o, id platform.ID, ver int64, outcome string, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(careCasesCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "version": ver, "state": bson.M{"$ne": "closed"}}, bson.M{"$set": bson.M{"state": "closed", "closureOutcome": outcome, "closedAt": now, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return platform.VersionConflict(ver)
	}
	return nil
}
func (r *MongoRepository) InsertRestrictedNote(c context.Context, v RestrictedNote) error {
	_, e := r.database.Collection(restrictedNotesCollection).InsertOne(c, v)
	return e
}
func (r *MongoRepository) ListRestrictedNotes(c context.Context, o, caseID platform.ID) ([]RestrictedNote, error) {
	cur, e := r.database.Collection(restrictedNotesCollection).Find(c, bson.M{"organizationId": o, "caseId": caseID}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(c)
	var v []RestrictedNote
	e = cur.All(c, &v)
	if v == nil {
		v = []RestrictedNote{}
	}
	return v, e
}
func (r *MongoRepository) InsertContactEvent(c context.Context, v ContactEvent) error {
	_, e := r.database.Collection(contactEventsCollection).InsertOne(c, v)
	return e
}
func (r *MongoRepository) ListContactEvents(c context.Context, o, caseID platform.ID) ([]ContactEvent, error) {
	cur, e := r.database.Collection(contactEventsCollection).Find(c, bson.M{"organizationId": o, "caseId": caseID}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: -1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(c)
	var v []ContactEvent
	e = cur.All(c, &v)
	if v == nil {
		v = []ContactEvent{}
	}
	return v, e
}
