package participation

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

const definitionsCollection = "chms_service_definitions"
const occurrencesCollection = "chms_occurrences"
const attendanceCollection = "chms_attendance"
const attendanceEventsCollection = "chms_attendance_events"
const headcountsCollection = "chms_headcounts"
const attendanceLocksCollection = "chms_attendance_locks"
const attendancePeriodsCollection = "chms_attendance_period_closes"
const checkinSessionsCollection = "chms_checkin_sessions"
const checkinCommandsCollection = "chms_checkin_command_receipts"
const guardianAuthorizationsCollection = "chms_guardian_authorizations"
const childCheckinsCollection = "chms_child_checkins"
const pickupEventsCollection = "chms_pickup_events"
const safeguardingIncidentsCollection = "chms_safeguarding_incidents"

type Store interface {
	InsertDefinition(context.Context, ServiceDefinition) error
	FindDefinition(context.Context, platform.ID, platform.ID) (*ServiceDefinition, error)
	ListDefinitions(context.Context, platform.ID, platform.ID, bool) ([]ServiceDefinition, error)
	UpdateDefinition(context.Context, platform.ID, platform.ID, int64, DefinitionInput, time.Time, platform.Actor) error
	InsertOccurrence(context.Context, Occurrence) error
	FindOccurrence(context.Context, platform.ID, platform.ID) (*Occurrence, error)
	FindOccurrenceByKey(context.Context, platform.ID, string) (*Occurrence, error)
	ListOccurrences(context.Context, platform.ID, platform.ID, time.Time, time.Time) ([]Occurrence, error)
	UpdateOccurrence(context.Context, platform.ID, platform.ID, int64, OccurrenceInput, time.Time, platform.Actor) error
	CancelOccurrence(context.Context, platform.ID, platform.ID, int64, time.Time, string, platform.Actor) error
	FindAttendance(context.Context, platform.ID, platform.ID, platform.ID) (*AttendanceFact, error)
	ListAttendance(context.Context, platform.ID, platform.ID) ([]AttendanceFact, error)
	InsertAttendance(context.Context, AttendanceFact) error
	UpdateAttendance(context.Context, platform.ID, platform.ID, platform.ID, int64, AttendanceInput, time.Time, platform.Actor) error
	InsertAttendanceEvent(context.Context, AttendanceEvent) error
	ListAttendanceEvents(context.Context, platform.ID, platform.ID, platform.ID, int64) ([]AttendanceEvent, error)
	InsertHeadcount(context.Context, Headcount) error
	FindHeadcount(context.Context, platform.ID, platform.ID) (*Headcount, error)
	ListHeadcounts(context.Context, platform.ID, platform.ID) ([]Headcount, error)
	UpdateHeadcount(context.Context, platform.ID, platform.ID, int64, HeadcountInput, time.Time, platform.Actor) error
	InsertAttendanceLock(context.Context, AttendanceLock) error
	FindAttendanceLock(context.Context, platform.ID, platform.ID) (*AttendanceLock, error)
	InsertAttendancePeriodClose(context.Context, AttendancePeriodClose) error
	FindClosedAttendancePeriod(context.Context, platform.ID, platform.ID, time.Time) (*AttendancePeriodClose, error)
	ListAttendancePeriodCloses(context.Context, platform.ID, platform.ID) ([]AttendancePeriodClose, error)
	InsertCheckinSession(context.Context, CheckinSession) error
	FindCheckinSession(context.Context, platform.ID, platform.ID) (*CheckinSession, error)
	LockCheckinSession(context.Context, platform.ID, platform.ID, int64, time.Time, string, platform.Actor) error
	AdvanceCheckinSession(context.Context, platform.ID, platform.ID, int64, time.Time, platform.Actor) error
	FindCheckinCommandReceipt(context.Context, platform.ID, platform.ID, string) (*CheckinCommandReceipt, error)
	ClaimCheckinCommand(context.Context, CheckinCommandReceipt) (bool, error)
	ReclaimCheckinCommand(context.Context, platform.ID, platform.ID, time.Time) (bool, error)
	CompleteCheckinCommand(context.Context, platform.ID, platform.ID, CheckinCommandResult, time.Time) error
	InsertGuardianAuthorization(context.Context, GuardianAuthorization) error
	FindActiveGuardianAuthorization(context.Context, platform.ID, platform.ID, platform.ID, platform.ID, time.Time) (*GuardianAuthorization, error)
	InsertChildCheckin(context.Context, ChildCheckin) error
	FindChildCheckin(context.Context, platform.ID, platform.ID) (*ChildCheckin, error)
	FindChildCheckinByAttendance(context.Context, platform.ID, platform.ID) (*ChildCheckin, error)
	ReleaseChildCheckin(context.Context, platform.ID, platform.ID, int64, platform.ID, time.Time, platform.Actor) error
	InsertPickupEvent(context.Context, PickupEvent) error
	InsertSafeguardingIncident(context.Context, SafeguardingIncident) error
}
type MongoRepository struct{ database *mongo.Database }

func (r *MongoRepository) ResolveAttendanceEvidence(ctx context.Context, organizationID, occurrenceID, personID platform.ID) (platform.ID, string, time.Time, error) {
	value, err := r.FindAttendance(ctx, organizationID, occurrenceID, personID)
	if err != nil || value == nil {
		return "", "", time.Time{}, err
	}
	occurredAt := value.CreatedAt
	if value.CheckedInAt != nil {
		occurredAt = *value.CheckedInAt
	}
	return value.BranchID, value.Status, occurredAt, nil
}

func NewMongoRepository(database *mongo.Database) (*MongoRepository, error) {
	if database == nil {
		return nil, errors.New("participation database is required")
	}
	return &MongoRepository{database: database}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	definitions := []struct {
		name   string
		models []mongo.IndexModel
	}{
		{definitionsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "homeBranchId", Value: 1}, {Key: "status", Value: 1}, {Key: "name", Value: 1}}}}},
		{occurrencesCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceKey", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "homeBranchId", Value: 1}, {Key: "startsAt", Value: 1}, {Key: "_id", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "serviceDefinitionId", Value: 1}, {Key: "startsAt", Value: 1}}}}},
		{attendanceCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "updatedAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}, {Key: "status", Value: 1}}}}},
		{attendanceEventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "attendanceId", Value: 1}, {Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}, {Key: "personId", Value: 1}, {Key: "occurredAt", Value: 1}}}}},
		{headcountsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}, {Key: "observedAt", Value: 1}, {Key: "_id", Value: 1}}}}},
		{attendanceLocksCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}}, Options: options.Index().SetUnique(true)}}},
		{attendancePeriodsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "startsAt", Value: 1}, {Key: "endsAt", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "endsAt", Value: -1}}}}},
		{checkinSessionsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "state", Value: 1}, {Key: "expiresAt", Value: 1}}}, {Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(30 * 24 * 60 * 60)}}},
		{checkinCommandsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "sessionId", Value: 1}, {Key: "result.clientCommandId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "sessionId", Value: 1}, {Key: "result.localSequence", Value: 1}}}}},
		{guardianAuthorizationsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "childPersonId", Value: 1}, {Key: "guardianPersonId", Value: 1}, {Key: "status", Value: 1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "guardianPersonId", Value: 1}, {Key: "validUntil", Value: 1}}}}},
		{childCheckinsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "attendanceId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "occurrenceId", Value: 1}, {Key: "childPersonId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "codeExpiresAt", Value: 1}, {Key: "pickedUpAt", Value: 1}}}}},
		{pickupEventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "childCheckinId", Value: 1}, {Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}}}},
		{safeguardingIncidentsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "status", Value: 1}, {Key: "occurredAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "childCheckinId", Value: 1}, {Key: "occurredAt", Value: -1}}}}},
	}
	for _, definition := range definitions {
		if _, err := r.database.Collection(definition.name).Indexes().CreateMany(ctx, definition.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", definition.name, err)
		}
	}
	return nil
}

func (r *MongoRepository) FindAttendance(ctx context.Context, organizationID, occurrenceID, personID platform.ID) (*AttendanceFact, error) {
	var value AttendanceFact
	err := r.database.Collection(attendanceCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "occurrenceId": occurrenceID, "personId": personID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find attendance: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ListAttendance(ctx context.Context, organizationID, occurrenceID platform.ID) ([]AttendanceFact, error) {
	cursor, err := r.database.Collection(attendanceCollection).Find(ctx, bson.M{"organizationId": organizationID, "occurrenceId": occurrenceID}, options.Find().SetSort(bson.D{{Key: "personId", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list attendance: %w", err)
	}
	defer cursor.Close(ctx)
	var values []AttendanceFact
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []AttendanceFact{}
	}
	return values, nil
}

func (r *MongoRepository) LoadAttendanceAnalytics(ctx context.Context, organizationID, branchID platform.ID, from, to time.Time) (AnalyticsDataset, error) {
	followupTo := to.Add(90 * 24 * time.Hour)
	occurrences, err := r.ListOccurrences(ctx, organizationID, branchID, from, followupTo)
	if err != nil {
		return AnalyticsDataset{}, err
	}
	occurrenceIDs := make([]platform.ID, 0, len(occurrences))
	for _, occurrence := range occurrences {
		occurrenceIDs = append(occurrenceIDs, occurrence.ID)
	}
	dataset := AnalyticsDataset{Occurrences: occurrences, Attendance: []AttendanceFact{}, Headcounts: []Headcount{}, GuestVisits: map[platform.ID][]time.Time{}}
	if len(occurrenceIDs) == 0 {
		return dataset, nil
	}
	attendanceCursor, err := r.database.Collection(attendanceCollection).Find(ctx, bson.M{"organizationId": organizationID, "occurrenceId": bson.M{"$in": occurrenceIDs}, "status": AttendancePresent})
	if err != nil {
		return AnalyticsDataset{}, fmt.Errorf("load attendance analytics: %w", err)
	}
	defer attendanceCursor.Close(ctx)
	if err := attendanceCursor.All(ctx, &dataset.Attendance); err != nil {
		return AnalyticsDataset{}, fmt.Errorf("decode attendance analytics: %w", err)
	}
	headcountCursor, err := r.database.Collection(headcountsCollection).Find(ctx, bson.M{"organizationId": organizationID, "occurrenceId": bson.M{"$in": occurrenceIDs}})
	if err != nil {
		return AnalyticsDataset{}, fmt.Errorf("load headcount analytics: %w", err)
	}
	defer headcountCursor.Close(ctx)
	if err := headcountCursor.All(ctx, &dataset.Headcounts); err != nil {
		return AnalyticsDataset{}, fmt.Errorf("decode headcount analytics: %w", err)
	}
	guestIDs := []platform.ID{}
	seenGuest := map[platform.ID]bool{}
	for _, fact := range dataset.Attendance {
		if fact.Guest && !seenGuest[fact.PersonID] {
			seenGuest[fact.PersonID] = true
			guestIDs = append(guestIDs, fact.PersonID)
		}
	}
	if len(guestIDs) == 0 {
		return dataset, nil
	}
	guestCursor, err := r.database.Collection(attendanceCollection).Find(ctx, bson.M{"organizationId": organizationID, "personId": bson.M{"$in": guestIDs}, "status": AttendancePresent, "guest": true})
	if err != nil {
		return AnalyticsDataset{}, fmt.Errorf("load guest attendance history: %w", err)
	}
	defer guestCursor.Close(ctx)
	var guestFacts []AttendanceFact
	if err := guestCursor.All(ctx, &guestFacts); err != nil {
		return AnalyticsDataset{}, fmt.Errorf("decode guest attendance history: %w", err)
	}
	guestOccurrenceIDs := []platform.ID{}
	seenOccurrence := map[platform.ID]bool{}
	for _, fact := range guestFacts {
		if !seenOccurrence[fact.OccurrenceID] {
			seenOccurrence[fact.OccurrenceID] = true
			guestOccurrenceIDs = append(guestOccurrenceIDs, fact.OccurrenceID)
		}
	}
	guestOccurrenceCursor, err := r.database.Collection(occurrencesCollection).Find(ctx, bson.M{"_id": bson.M{"$in": guestOccurrenceIDs}, "organizationId": organizationID, "homeBranchId": branchID, "status": bson.M{"$ne": "cancelled"}, "startsAt": bson.M{"$lt": followupTo}})
	if err != nil {
		return AnalyticsDataset{}, fmt.Errorf("load guest occurrence history: %w", err)
	}
	defer guestOccurrenceCursor.Close(ctx)
	var guestOccurrences []Occurrence
	if err := guestOccurrenceCursor.All(ctx, &guestOccurrences); err != nil {
		return AnalyticsDataset{}, fmt.Errorf("decode guest occurrence history: %w", err)
	}
	startsAt := map[platform.ID]time.Time{}
	for _, occurrence := range guestOccurrences {
		startsAt[occurrence.ID] = occurrence.StartsAt
	}
	for _, fact := range guestFacts {
		if value, ok := startsAt[fact.OccurrenceID]; ok {
			dataset.GuestVisits[fact.PersonID] = append(dataset.GuestVisits[fact.PersonID], value)
		}
	}
	return dataset, nil
}

func (r *MongoRepository) InsertAttendance(ctx context.Context, value AttendanceFact) error {
	_, err := r.database.Collection(attendanceCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "version_conflict", Message: "Attendance changed while it was being recorded."}
	}
	if err != nil {
		return fmt.Errorf("insert attendance: %w", err)
	}
	return nil
}

func (r *MongoRepository) InsertAttendanceLock(ctx context.Context, value AttendanceLock) error {
	_, err := r.database.Collection(attendanceLocksCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "Attendance is already locked for this occurrence."}
	}
	if err != nil {
		return fmt.Errorf("insert attendance lock: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindAttendanceLock(ctx context.Context, organizationID, occurrenceID platform.ID) (*AttendanceLock, error) {
	var value AttendanceLock
	err := r.database.Collection(attendanceLocksCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "occurrenceId": occurrenceID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find attendance lock: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) InsertAttendancePeriodClose(ctx context.Context, value AttendancePeriodClose) error {
	_, err := r.database.Collection(attendancePeriodsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This attendance period is already closed."}
	}
	if err != nil {
		return fmt.Errorf("insert attendance period close: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindClosedAttendancePeriod(ctx context.Context, organizationID, branchID platform.ID, occurredAt time.Time) (*AttendancePeriodClose, error) {
	var value AttendancePeriodClose
	err := r.database.Collection(attendancePeriodsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "branchId": branchID, "startsAt": bson.M{"$lte": occurredAt.UTC()}, "endsAt": bson.M{"$gt": occurredAt.UTC()}}, options.FindOne().SetSort(bson.D{{Key: "closedAt", Value: -1}})).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find closed attendance period: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ListAttendancePeriodCloses(ctx context.Context, organizationID, branchID platform.ID) ([]AttendancePeriodClose, error) {
	cursor, err := r.database.Collection(attendancePeriodsCollection).Find(ctx, bson.M{"organizationId": organizationID, "branchId": branchID}, options.Find().SetSort(bson.D{{Key: "endsAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, fmt.Errorf("list attendance period closes: %w", err)
	}
	defer cursor.Close(ctx)
	var values []AttendancePeriodClose
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []AttendancePeriodClose{}
	}
	return values, nil
}

func (r *MongoRepository) InsertCheckinSession(ctx context.Context, value CheckinSession) error {
	_, err := r.database.Collection(checkinSessionsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert check-in session: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindCheckinSession(ctx context.Context, organizationID, id platform.ID) (*CheckinSession, error) {
	var value CheckinSession
	err := r.database.Collection(checkinSessionsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find check-in session: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) LockCheckinSession(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, now time.Time, reason string, actor platform.Actor) error {
	result, err := r.database.Collection(checkinSessionsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion, "state": "active"}, bson.M{"$set": bson.M{"state": "locked", "lockedAt": now, "lockedBy": actor, "lockReason": reason, "lastActivityAt": now, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("lock check-in session: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) AdvanceCheckinSession(ctx context.Context, organizationID, id platform.ID, sequence int64, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(checkinSessionsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "state": "active"}, bson.M{"$max": bson.M{"lastSequence": sequence}, "$set": bson.M{"lastActivityAt": now, "updatedAt": now, "updatedBy": actor}})
	if err != nil {
		return fmt.Errorf("advance check-in session: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "checkin_session_locked", Message: "Check-in session was locked while syncing."}
	}
	return nil
}

func (r *MongoRepository) FindCheckinCommandReceipt(ctx context.Context, organizationID, sessionID platform.ID, clientCommandID string) (*CheckinCommandReceipt, error) {
	var value CheckinCommandReceipt
	err := r.database.Collection(checkinCommandsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "sessionId": sessionID, "result.clientCommandId": clientCommandID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find check-in command receipt: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ClaimCheckinCommand(ctx context.Context, value CheckinCommandReceipt) (bool, error) {
	_, err := r.database.Collection(checkinCommandsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim check-in command: %w", err)
	}
	return true, nil
}

func (r *MongoRepository) ReclaimCheckinCommand(ctx context.Context, organizationID, receiptID platform.ID, now time.Time) (bool, error) {
	result, err := r.database.Collection(checkinCommandsCollection).UpdateOne(ctx, bson.M{"_id": receiptID, "organizationId": organizationID, "result.classification": "retry"}, bson.M{"$set": bson.M{"result.classification": "processing", "result.code": "", "result.message": "", "createdAt": now}, "$unset": bson.M{"completedAt": ""}})
	if err != nil {
		return false, fmt.Errorf("reclaim check-in command: %w", err)
	}
	return result.ModifiedCount == 1, nil
}

func (r *MongoRepository) CompleteCheckinCommand(ctx context.Context, organizationID, id platform.ID, result CheckinCommandResult, now time.Time) error {
	update, err := r.database.Collection(checkinCommandsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "result.classification": "processing"}, bson.M{"$set": bson.M{"result": result, "completedAt": now}})
	if err != nil {
		return fmt.Errorf("complete check-in command: %w", err)
	}
	if update.MatchedCount != 1 {
		return errors.New("check-in command is no longer claimable")
	}
	return nil
}

func (r *MongoRepository) InsertGuardianAuthorization(ctx context.Context, value GuardianAuthorization) error {
	_, err := r.database.Collection(guardianAuthorizationsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert guardian authorization: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindActiveGuardianAuthorization(ctx context.Context, organizationID, branchID, childID, guardianID platform.ID, at time.Time) (*GuardianAuthorization, error) {
	filter := bson.M{"organizationId": organizationID, "branchId": branchID, "childPersonId": childID, "guardianPersonId": guardianID, "status": "active", "validFrom": bson.M{"$lte": at.UTC()}, "$or": bson.A{bson.M{"validUntil": nil}, bson.M{"validUntil": bson.M{"$gt": at.UTC()}}}}
	var value GuardianAuthorization
	err := r.database.Collection(guardianAuthorizationsCollection).FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "validFrom", Value: -1}, {Key: "_id", Value: -1}})).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find guardian authorization: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) InsertChildCheckin(ctx context.Context, value ChildCheckin) error {
	_, err := r.database.Collection(childCheckinsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This child is already checked in for the occurrence."}
	}
	if err != nil {
		return fmt.Errorf("insert child check-in: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindChildCheckin(ctx context.Context, organizationID, id platform.ID) (*ChildCheckin, error) {
	var value ChildCheckin
	err := r.database.Collection(childCheckinsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find child check-in: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) FindChildCheckinByAttendance(ctx context.Context, organizationID, attendanceID platform.ID) (*ChildCheckin, error) {
	var value ChildCheckin
	err := r.database.Collection(childCheckinsCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "attendanceId": attendanceID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find child check-in by attendance: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ReleaseChildCheckin(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, guardianID platform.ID, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(childCheckinsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion, "pickedUpAt": nil}, bson.M{"$set": bson.M{"pickedUpAt": now, "pickedUpByGuardianId": guardianID, "releasedBy": actor, "updatedAt": now, "updatedBy": actor, "codeHash": ""}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("release child check-in: %w", err)
	}
	if result.MatchedCount != 1 {
		return &platform.DomainError{Code: "pickup_denied", Message: "Pickup has already been completed."}
	}
	return nil
}

func (r *MongoRepository) InsertPickupEvent(ctx context.Context, value PickupEvent) error {
	_, err := r.database.Collection(pickupEventsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert pickup event: %w", err)
	}
	return nil
}

func (r *MongoRepository) InsertSafeguardingIncident(ctx context.Context, value SafeguardingIncident) error {
	_, err := r.database.Collection(safeguardingIncidentsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert safeguarding incident: %w", err)
	}
	return nil
}

func (r *MongoRepository) UpdateAttendance(ctx context.Context, organizationID, occurrenceID, personID platform.ID, expectedVersion int64, input AttendanceInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(attendanceCollection).UpdateOne(ctx, bson.M{"organizationId": organizationID, "occurrenceId": occurrenceID, "personId": personID, "version": expectedVersion}, bson.M{"$set": bson.M{"status": input.Status, "source": input.Source, "confidence": input.Confidence, "checkedInAt": input.CheckedInAt, "checkedOutAt": input.CheckedOutAt, "stationId": input.StationID, "operatorId": actor.ID, "guest": input.Guest, "syncCommandId": input.SyncCommandID, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update attendance: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}

func (r *MongoRepository) InsertAttendanceEvent(ctx context.Context, value AttendanceEvent) error {
	_, err := r.database.Collection(attendanceEventsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert attendance event: %w", err)
	}
	return nil
}

func (r *MongoRepository) ListAttendanceEvents(ctx context.Context, organizationID, occurrenceID, personID platform.ID, limit int64) ([]AttendanceEvent, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	cursor, err := r.database.Collection(attendanceEventsCollection).Find(ctx, bson.M{"organizationId": organizationID, "occurrenceId": occurrenceID, "personId": personID}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list attendance events: %w", err)
	}
	defer cursor.Close(ctx)
	var values []AttendanceEvent
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []AttendanceEvent{}
	}
	return values, nil
}

func (r *MongoRepository) InsertHeadcount(ctx context.Context, value Headcount) error {
	_, err := r.database.Collection(headcountsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert headcount: %w", err)
	}
	return nil
}

func (r *MongoRepository) FindHeadcount(ctx context.Context, organizationID, id platform.ID) (*Headcount, error) {
	var value Headcount
	err := r.database.Collection(headcountsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find headcount: %w", err)
	}
	return &value, nil
}

func (r *MongoRepository) ListHeadcounts(ctx context.Context, organizationID, occurrenceID platform.ID) ([]Headcount, error) {
	cursor, err := r.database.Collection(headcountsCollection).Find(ctx, bson.M{"organizationId": organizationID, "occurrenceId": occurrenceID}, options.Find().SetSort(bson.D{{Key: "observedAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list headcounts: %w", err)
	}
	defer cursor.Close(ctx)
	var values []Headcount
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []Headcount{}
	}
	return values, nil
}

func (r *MongoRepository) UpdateHeadcount(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input HeadcountInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(headcountsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion}, bson.M{"$set": bson.M{"category": input.Category, "roomId": input.RoomID, "count": input.Count, "source": input.Source, "confidence": input.Confidence, "observedAt": input.ObservedAt, "reason": input.Reason, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update headcount: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) InsertDefinition(ctx context.Context, value ServiceDefinition) error {
	_, err := r.database.Collection(definitionsCollection).InsertOne(ctx, value)
	if err != nil {
		return fmt.Errorf("insert service definition: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindDefinition(ctx context.Context, organizationID, id platform.ID) (*ServiceDefinition, error) {
	var value ServiceDefinition
	err := r.database.Collection(definitionsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find service definition: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListDefinitions(ctx context.Context, organizationID, branchID platform.ID, includeInactive bool) ([]ServiceDefinition, error) {
	filter := bson.M{"organizationId": organizationID, "homeBranchId": branchID, "archivedAt": nil}
	if !includeInactive {
		filter["status"] = "active"
	}
	cursor, err := r.database.Collection(definitionsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list service definitions: %w", err)
	}
	defer cursor.Close(ctx)
	var values []ServiceDefinition
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []ServiceDefinition{}
	}
	return values, nil
}
func (r *MongoRepository) UpdateDefinition(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input DefinitionInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(definitionsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion, "archivedAt": nil}, bson.M{"$set": bson.M{"homeBranchId": input.HomeBranchID, "branchId": input.HomeBranchID, "name": input.Name, "description": input.Description, "timezone": input.Timezone, "defaultDurationMinutes": input.DefaultDurationMinutes, "defaultRoomIds": input.DefaultRoomIDs, "defaultCapacity": input.DefaultCapacity, "recurrence": input.Recurrence, "status": input.Status, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update service definition: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) InsertOccurrence(ctx context.Context, value Occurrence) error {
	_, err := r.database.Collection(occurrencesCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return &platform.DomainError{Code: "conflict", Message: "This service occurrence already exists."}
	}
	if err != nil {
		return fmt.Errorf("insert occurrence: %w", err)
	}
	return nil
}
func (r *MongoRepository) FindOccurrence(ctx context.Context, organizationID, id platform.ID) (*Occurrence, error) {
	var value Occurrence
	err := r.database.Collection(occurrencesCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find occurrence: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ResolveOccurrenceReference(ctx context.Context, organizationID, id platform.ID) (platform.ID, time.Time, time.Time, string, error) {
	value, err := r.FindOccurrence(ctx, organizationID, id)
	if err != nil {
		return "", time.Time{}, time.Time{}, "", err
	}
	if value == nil {
		return "", time.Time{}, time.Time{}, "", nil
	}
	return value.HomeBranchID, value.StartsAt, value.EndsAt, value.Status, nil
}
func (r *MongoRepository) FindOccurrenceByKey(ctx context.Context, organizationID platform.ID, key string) (*Occurrence, error) {
	var value Occurrence
	err := r.database.Collection(occurrencesCollection).FindOne(ctx, bson.M{"organizationId": organizationID, "occurrenceKey": key}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find occurrence by key: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListOccurrences(ctx context.Context, organizationID, branchID platform.ID, from, to time.Time) ([]Occurrence, error) {
	cursor, err := r.database.Collection(occurrencesCollection).Find(ctx, bson.M{"organizationId": organizationID, "homeBranchId": branchID, "startsAt": bson.M{"$gte": from.UTC(), "$lt": to.UTC()}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list occurrences: %w", err)
	}
	defer cursor.Close(ctx)
	var values []Occurrence
	if err := cursor.All(ctx, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = []Occurrence{}
	}
	return values, nil
}
func (r *MongoRepository) UpdateOccurrence(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, input OccurrenceInput, now time.Time, actor platform.Actor) error {
	result, err := r.database.Collection(occurrencesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion, "status": bson.M{"$ne": "cancelled"}}, bson.M{"$set": bson.M{"homeBranchId": input.HomeBranchID, "branchId": input.HomeBranchID, "name": input.Name, "startsAt": input.StartsAt, "endsAt": input.EndsAt, "timezone": input.Timezone, "roomIds": input.RoomIDs, "capacity": input.Capacity, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("update occurrence: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
func (r *MongoRepository) CancelOccurrence(ctx context.Context, organizationID, id platform.ID, expectedVersion int64, now time.Time, reason string, actor platform.Actor) error {
	result, err := r.database.Collection(occurrencesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "version": expectedVersion, "status": bson.M{"$ne": "cancelled"}}, bson.M{"$set": bson.M{"status": "cancelled", "cancelledAt": now, "cancellationReason": reason, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return fmt.Errorf("cancel occurrence: %w", err)
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expectedVersion)
	}
	return nil
}
