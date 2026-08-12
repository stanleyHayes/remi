package care

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

const definitionsCollection = "chms_workflow_definitions"
const instancesCollection = "chms_workflow_instances"
const tasksCollection = "chms_tasks"
const eventsCollection = "chms_workflow_events"
const automationsCollection = "chms_workflow_automations"
const assimilationCollection = "chms_assimilation_progress"

type Store interface {
	InsertDefinition(context.Context, Definition) error
	FindDefinition(context.Context, platform.ID, platform.ID) (*Definition, error)
	ListDefinitions(context.Context, platform.ID, platform.ID) ([]Definition, error)
	UpdateDefinition(context.Context, platform.ID, platform.ID, int64, DefinitionInput, time.Time, platform.Actor) error
	PublishDefinition(context.Context, platform.ID, platform.ID, int64, time.Time, platform.Actor) error
	InsertInstance(context.Context, Instance) error
	FindInstance(context.Context, platform.ID, platform.ID) (*Instance, error)
	ListInstances(context.Context, platform.ID, platform.ID, string) ([]Instance, error)
	UpdateInstance(context.Context, platform.ID, platform.ID, int64, string, platform.ID, *time.Time, string, string, string, *time.Time, time.Time, platform.Actor) error
	InsertTask(context.Context, Task) error
	ListTasks(context.Context, platform.ID, platform.ID) ([]Task, error)
	FindTask(context.Context, platform.ID, platform.ID) (*Task, error)
	CompleteTask(context.Context, platform.ID, platform.ID, int64, string, time.Time, platform.Actor) error
	RemindTask(context.Context, platform.ID, platform.ID, time.Time, platform.Actor) error
	ReassignOpenTasks(context.Context, platform.ID, platform.ID, platform.ID, time.Time, platform.Actor) error
	CloseOpenTasks(context.Context, platform.ID, platform.ID, string, time.Time, platform.Actor) error
	InsertEvent(context.Context, Event) error
	ClaimAutomation(context.Context, platform.ID, string, string, platform.ID, time.Time) (bool, error)
	FindAutomation(context.Context, platform.ID, string) (string, platform.ID, bool, error)
	InsertAssimilationProgress(context.Context, AssimilationProgress) error
	FindAssimilationProgress(context.Context, platform.ID, platform.ID, platform.ID) (*AssimilationProgress, error)
	AppendAssimilationVisit(context.Context, platform.ID, platform.ID, int64, AssimilationEvidence, time.Time, platform.Actor) error
	ListAssimilationProgress(context.Context, platform.ID, platform.ID, int64) ([]AssimilationProgress, error)
}
type MongoRepository struct{ database *mongo.Database }

func NewMongoRepository(db *mongo.Database) (*MongoRepository, error) {
	if db == nil {
		return nil, errors.New("care database is required")
	}
	return &MongoRepository{db}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	defs := []struct {
		name   string
		models []mongo.IndexModel
	}{
		{definitionsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "status", Value: 1}, {Key: "name", Value: 1}}}}},
		{instancesCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "state", Value: 1}, {Key: "dueAt", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "subjectType", Value: 1}, {Key: "subjectId", Value: 1}, {Key: "createdAt", Value: -1}}}}},
		{tasksCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "instanceId", Value: 1}, {Key: "templateKey", Value: 1}, {Key: "stageKey", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "assigneeId", Value: 1}, {Key: "status", Value: 1}, {Key: "dueAt", Value: 1}}}}},
		{eventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "instanceId", Value: 1}, {Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}}}},
		{automationsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)}}},
		{assimilationCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "definitionId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "lastVisitAt", Value: -1}}}}},
	}
	for _, d := range defs {
		if _, err := r.database.Collection(d.name).Indexes().CreateMany(ctx, d.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", d.name, err)
		}
	}
	return nil
}
func (r *MongoRepository) InsertDefinition(c context.Context, v Definition) error {
	_, e := r.database.Collection(definitionsCollection).InsertOne(c, v)
	return e
}
func (r *MongoRepository) FindDefinition(c context.Context, o, id platform.ID) (*Definition, error) {
	var v Definition
	e := r.database.Collection(definitionsCollection).FindOne(c, bson.M{"_id": id, "organizationId": o}).Decode(&v)
	if e == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, e
}
func (r *MongoRepository) ListDefinitions(c context.Context, o, b platform.ID) ([]Definition, error) {
	cur, e := r.database.Collection(definitionsCollection).Find(c, bson.M{"organizationId": o, "branchId": b}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(c)
	var v []Definition
	e = cur.All(c, &v)
	if v == nil {
		v = []Definition{}
	}
	return v, e
}
func (r *MongoRepository) UpdateDefinition(c context.Context, o, id platform.ID, ver int64, in DefinitionInput, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(definitionsCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "version": ver, "status": "draft"}, bson.M{"$set": bson.M{"branchId": in.BranchID, "name": in.Name, "description": in.Description, "purpose": in.Purpose, "initialStageKey": in.InitialStageKey, "stages": in.Stages, "transitions": in.Transitions, "tasks": in.Tasks, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return platform.VersionConflict(ver)
	}
	return nil
}
func (r *MongoRepository) PublishDefinition(c context.Context, o, id platform.ID, ver int64, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(definitionsCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "version": ver, "status": "draft"}, bson.M{"$set": bson.M{"status": "published", "publishedAt": now, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return platform.VersionConflict(ver)
	}
	return nil
}
func (r *MongoRepository) InsertInstance(c context.Context, v Instance) error {
	_, e := r.database.Collection(instancesCollection).InsertOne(c, v)
	return e
}
func (r *MongoRepository) FindInstance(c context.Context, o, id platform.ID) (*Instance, error) {
	var v Instance
	e := r.database.Collection(instancesCollection).FindOne(c, bson.M{"_id": id, "organizationId": o}).Decode(&v)
	if e == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, e
}
func (r *MongoRepository) ListInstances(c context.Context, o, b platform.ID, state string) ([]Instance, error) {
	f := bson.M{"organizationId": o, "branchId": b}
	if state != "" {
		f["state"] = state
	}
	cur, e := r.database.Collection(instancesCollection).Find(c, f, options.Find().SetSort(bson.D{{Key: "dueAt", Value: 1}, {Key: "createdAt", Value: 1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(c)
	var v []Instance
	e = cur.All(c, &v)
	if v == nil {
		v = []Instance{}
	}
	return v, e
}
func (r *MongoRepository) UpdateInstance(c context.Context, o, id platform.ID, ver int64, stage string, owner platform.ID, due *time.Time, state, outcome, summary string, completed *time.Time, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(instancesCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "version": ver, "state": "active"}, bson.M{"$set": bson.M{"stageKey": stage, "ownerId": owner, "dueAt": due, "state": state, "outcomeCode": outcome, "outcomeSummary": summary, "completedAt": completed, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return platform.VersionConflict(ver)
	}
	return nil
}
func (r *MongoRepository) InsertTask(c context.Context, v Task) error {
	_, e := r.database.Collection(tasksCollection).InsertOne(c, v)
	if mongo.IsDuplicateKeyError(e) {
		return nil
	}
	return e
}
func (r *MongoRepository) ListTasks(c context.Context, o, i platform.ID) ([]Task, error) {
	cur, e := r.database.Collection(tasksCollection).Find(c, bson.M{"organizationId": o, "instanceId": i}, options.Find().SetSort(bson.D{{Key: "dueAt", Value: 1}}))
	if e != nil {
		return nil, e
	}
	defer cur.Close(c)
	var v []Task
	e = cur.All(c, &v)
	if v == nil {
		v = []Task{}
	}
	return v, e
}
func (r *MongoRepository) FindTask(c context.Context, o, id platform.ID) (*Task, error) {
	var v Task
	e := r.database.Collection(tasksCollection).FindOne(c, bson.M{"_id": id, "organizationId": o}).Decode(&v)
	if e == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, e
}
func (r *MongoRepository) CompleteTask(c context.Context, o, id platform.ID, ver int64, out string, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(tasksCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "version": ver, "status": bson.M{"$in": bson.A{"open", "in-progress"}}}, bson.M{"$set": bson.M{"status": "completed", "outcome": out, "completedAt": now, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return platform.VersionConflict(ver)
	}
	return nil
}
func (r *MongoRepository) RemindTask(c context.Context, o, id platform.ID, now time.Time, a platform.Actor) error {
	res, e := r.database.Collection(tasksCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": o, "status": bson.M{"$ne": "completed"}}, bson.M{"$set": bson.M{"lastReminderAt": now, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"reminderCount": 1, "version": 1}})
	if e != nil {
		return e
	}
	if res.MatchedCount != 1 {
		return &platform.DomainError{Code: "conflict", Message: "Completed tasks cannot be reminded."}
	}
	return nil
}
func (r *MongoRepository) ReassignOpenTasks(c context.Context, o, instanceID, assigneeID platform.ID, now time.Time, a platform.Actor) error {
	_, err := r.database.Collection(tasksCollection).UpdateMany(c, bson.M{"organizationId": o, "instanceId": instanceID, "status": bson.M{"$in": bson.A{"open", "in-progress"}}}, bson.M{"$set": bson.M{"assigneeId": assigneeID, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	return err
}
func (r *MongoRepository) CloseOpenTasks(c context.Context, o, instanceID platform.ID, outcome string, now time.Time, a platform.Actor) error {
	_, err := r.database.Collection(tasksCollection).UpdateMany(c, bson.M{"organizationId": o, "instanceId": instanceID, "status": bson.M{"$in": bson.A{"open", "in-progress"}}}, bson.M{"$set": bson.M{"status": "cancelled", "outcome": outcome, "completedAt": now, "updatedAt": now, "updatedBy": a}, "$inc": bson.M{"version": 1}})
	return err
}
func (r *MongoRepository) InsertEvent(c context.Context, v Event) error {
	_, e := r.database.Collection(eventsCollection).InsertOne(c, v)
	return e
}
func (r *MongoRepository) ClaimAutomation(c context.Context, o platform.ID, key, payloadHash string, i platform.ID, now time.Time) (bool, error) {
	result, err := r.database.Collection(automationsCollection).UpdateOne(c, bson.M{"organizationId": o, "key": key}, bson.M{"$setOnInsert": bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": o, "key": key, "payloadHash": payloadHash, "instanceId": i, "createdAt": now}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return false, err
	}
	if result.UpsertedCount == 1 {
		return true, nil
	}
	var existing bson.M
	if err = r.database.Collection(automationsCollection).FindOne(c, bson.M{"organizationId": o, "key": key}).Decode(&existing); err != nil {
		return false, err
	}
	if existing["payloadHash"] != payloadHash || existing["instanceId"] != i {
		return false, &platform.DomainError{Code: "idempotency_conflict", Message: "That automation key was already used for different workflow content."}
	}
	return false, nil
}
func (r *MongoRepository) FindAutomation(c context.Context, o platform.ID, key string) (string, platform.ID, bool, error) {
	var value struct {
		PayloadHash string      `bson:"payloadHash"`
		InstanceID  platform.ID `bson:"instanceId"`
	}
	err := r.database.Collection(automationsCollection).FindOne(c, bson.M{"organizationId": o, "key": key}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return "", "", false, nil
	}
	return value.PayloadHash, value.InstanceID, err == nil, err
}

func (r *MongoRepository) InsertAssimilationProgress(c context.Context, value AssimilationProgress) error {
	_, err := r.database.Collection(assimilationCollection).InsertOne(c, value)
	return err
}
func (r *MongoRepository) FindAssimilationProgress(c context.Context, organizationID, personID, definitionID platform.ID) (*AssimilationProgress, error) {
	var value AssimilationProgress
	err := r.database.Collection(assimilationCollection).FindOne(c, bson.M{"organizationId": organizationID, "personId": personID, "definitionId": definitionID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *MongoRepository) AppendAssimilationVisit(c context.Context, organizationID, id platform.ID, expectedVersion int64, evidence AssimilationEvidence, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(assimilationCollection).UpdateOne(c, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion, "evidence": bson.M{"$not": bson.M{"$elemMatch": bson.M{"type": evidence.Type, "sourceId": evidence.SourceID}}}}, bson.M{"$push": bson.M{"evidence": evidence}, "$set": bson.M{"lastVisitAt": evidence.OccurredAt, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"visitCount": 1, "version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) ListAssimilationProgress(c context.Context, organizationID, branchID platform.ID, limit int64) ([]AssimilationProgress, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	cursor, err := r.database.Collection(assimilationCollection).Find(c, bson.M{"organizationId": organizationID, "branchId": branchID}, options.Find().SetSort(bson.D{{Key: "lastVisitAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(c)
	var values []AssimilationProgress
	err = cursor.All(c, &values)
	if values == nil {
		values = []AssimilationProgress{}
	}
	return values, err
}
