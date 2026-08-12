package engagement

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

const rulesCollection = "chms_engagement_rules"
const signalsCollection = "chms_engagement_signals"
const reviewEventsCollection = "chms_engagement_review_events"
const safetyReviewsCollection = "chms_retention_safety_reviews"

type MongoRepository struct{ db *mongo.Database }

func NewMongoRepository(db *mongo.Database) (*MongoRepository, error) {
	if db == nil {
		return nil, errors.New("engagement database is required")
	}
	return &MongoRepository{db: db}, nil
}
func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	for _, item := range []struct {
		name   string
		models []mongo.IndexModel
	}{{rulesCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "status", Value: 1}, {Key: "kind", Value: 1}}}}}, {signalsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "sourceKey", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "state", Value: 1}, {Key: "snoozedUntil", Value: 1}, {Key: "createdAt", Value: -1}}}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "personId", Value: 1}, {Key: "expiresAt", Value: 1}}}}}, {reviewEventsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "signalId", Value: 1}, {Key: "occurredAt", Value: 1}}}}}, {safetyReviewsCollection, []mongo.IndexModel{{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "metricVersion", Value: 1}, {Key: "policyFingerprint", Value: 1}, {Key: "role", Value: 1}, {Key: "reviewedAt", Value: -1}}}}}} {
		if _, err := r.db.Collection(item.name).Indexes().CreateMany(ctx, item.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", item.name, err)
		}
	}
	return nil
}

func (r *MongoRepository) InsertSafetyReview(ctx context.Context, value SafetyReview) error {
	_, err := r.db.Collection(safetyReviewsCollection).InsertOne(ctx, value)
	return err
}

func (r *MongoRepository) ListSafetyReviews(ctx context.Context, org, branch platform.ID) ([]SafetyReview, error) {
	cursor, err := r.db.Collection(safetyReviewsCollection).Find(ctx, bson.M{"organizationId": org, "branchId": branch}, options.Find().SetSort(bson.D{{Key: "reviewedAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []SafetyReview{}
	err = cursor.All(ctx, &values)
	return values, err
}
func (r *MongoRepository) InsertRule(ctx context.Context, value Rule) error {
	_, err := r.db.Collection(rulesCollection).InsertOne(ctx, value)
	return err
}
func (r *MongoRepository) FindRule(ctx context.Context, org, id platform.ID) (*Rule, error) {
	var value Rule
	err := r.db.Collection(rulesCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find engagement rule: %w", err)
	}
	return &value, nil
}
func (r *MongoRepository) ListRules(ctx context.Context, org, branch platform.ID) ([]Rule, error) {
	cursor, err := r.db.Collection(rulesCollection).Find(ctx, bson.M{"organizationId": org, "branchId": branch}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "version", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []Rule{}
	err = cursor.All(ctx, &values)
	return values, err
}
func (r *MongoRepository) UpdateRule(ctx context.Context, org, id platform.ID, expected int64, input RuleInput, now time.Time, actor platform.Actor) error {
	result, err := r.db.Collection(rulesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "version": expected, "status": "draft"}, bson.M{"$set": bson.M{"branchId": input.BranchID, "name": input.Name, "kind": input.Kind, "description": input.Description, "timezone": input.Timezone, "windowDays": input.WindowDays, "lookbackDays": input.LookbackDays, "expiresAfterDays": input.ExpiresAfterDays, "approval": input.Approval, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func (r *MongoRepository) PublishRule(ctx context.Context, org, id platform.ID, expected int64, approval ApprovalEvidence, now time.Time, actor platform.Actor) error {
	result, err := r.db.Collection(rulesCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "version": expected, "status": "draft"}, bson.M{"$set": bson.M{"status": "published", "approval": approval, "publishedAt": now, "updatedAt": now, "updatedBy": actor}, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return nil
}
func (r *MongoRepository) InsertSignal(ctx context.Context, value Signal) (bool, error) {
	_, err := r.db.Collection(signalsCollection).InsertOne(ctx, value)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert engagement signal: %w", err)
	}
	return true, nil
}
func (r *MongoRepository) FindSignal(ctx context.Context, org, id platform.ID) (*Signal, error) {
	var value Signal
	err := r.db.Collection(signalsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}
func (r *MongoRepository) ListSignals(ctx context.Context, org, branch platform.ID, state string, now time.Time, limit int) ([]Signal, error) {
	filter := bson.M{"organizationId": org, "branchId": branch}
	if state == "active" {
		filter["$or"] = bson.A{bson.M{"state": bson.M{"$in": bson.A{"open", "in-review"}}}, bson.M{"state": "snoozed", "snoozedUntil": bson.M{"$lte": now}}}
		filter["expiresAt"] = bson.M{"$gt": now}
	} else if state != "" {
		filter["state"] = state
	}
	if state == "open" {
		filter["expiresAt"] = bson.M{"$gt": now}
	}
	cursor, err := r.db.Collection(signalsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []Signal{}
	err = cursor.All(ctx, &values)
	return values, err
}

func (r *MongoRepository) ApplyReview(ctx context.Context, org, id platform.ID, expected int64, event ReviewEvent, now time.Time, actor platform.Actor) error {
	set := bson.M{"state": event.ToState, "updatedAt": now, "updatedBy": actor}
	switch event.Type {
	case "assigned":
		set["assigneeId"] = event.AssigneeID
		set["snoozedUntil"] = nil
	case "snoozed":
		set["snoozedUntil"] = event.SnoozedUntil
	case "suppressed":
		set["resolutionOutcome"] = event.Outcome
		set["falsePositive"] = event.FalsePositive
		set["resolvedAt"] = now
	case "contact-recorded":
		// Contact evidence advances the version but does not infer resolution.
	case "resolved":
		set["resolutionOutcome"] = event.Outcome
		set["falsePositive"] = event.FalsePositive
		set["resolvedAt"] = now
	}
	result, err := r.db.Collection(signalsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "version": expected, "state": bson.M{"$nin": bson.A{"resolved", "suppressed"}}}, bson.M{"$set": set, "$inc": bson.M{"version": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return nil
}

func (r *MongoRepository) InsertReviewEvent(ctx context.Context, value ReviewEvent) error {
	_, err := r.db.Collection(reviewEventsCollection).InsertOne(ctx, value)
	return err
}

func (r *MongoRepository) ListReviewEvents(ctx context.Context, org, signalID platform.ID) ([]ReviewEvent, error) {
	cursor, err := r.db.Collection(reviewEventsCollection).Find(ctx, bson.M{"organizationId": org, "signalId": signalID}, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []ReviewEvent{}
	err = cursor.All(ctx, &values)
	return values, err
}

func (r *MongoRepository) FindPersonSummary(ctx context.Context, org, id platform.ID) (*PersonSummary, error) {
	keys := bson.A{id}
	if parsed, parseErr := bson.ObjectIDFromHex(string(id)); parseErr == nil {
		keys = append(keys, parsed)
	}
	var raw struct {
		PersonNumber string     `bson:"personNumber"`
		ArchivedAt   *time.Time `bson:"archivedAt"`
		Names        struct {
			Given     string `bson:"given"`
			Family    string `bson:"family"`
			Preferred string `bson:"preferred"`
		} `bson:"names"`
	}
	err := r.db.Collection("chms_people").FindOne(ctx, bson.M{"_id": bson.M{"$in": keys}, "organizationId": org}, options.FindOne().SetProjection(bson.M{"_id": 0, "personNumber": 1, "names": 1, "archivedAt": 1})).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(raw.Names.Preferred + " " + raw.Names.Family)
	if raw.Names.Preferred == "" {
		name = strings.TrimSpace(raw.Names.Given + " " + raw.Names.Family)
	}
	return &PersonSummary{ID: id, DisplayName: name, PersonNumber: raw.PersonNumber, Archived: raw.ArchivedAt != nil}, nil
}

func (r *MongoRepository) ListCareStaff(ctx context.Context) ([]StaffOption, error) {
	filter := bson.M{"role": bson.M{"$in": bson.A{"super-admin"}}, "$or": bson.A{bson.M{"invitationStatus": "accepted"}, bson.M{"passwordHash": bson.M{"$exists": true, "$ne": ""}}}}
	cursor, err := r.db.Collection("users").Find(ctx, filter, options.Find().SetProjection(bson.M{"name": 1, "email": 1, "role": 1}).SetSort(bson.D{{Key: "name", Value: 1}, {Key: "email", Value: 1}}).SetLimit(200))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	values := []StaffOption{}
	for cursor.Next(ctx) {
		var raw bson.M
		if err = cursor.Decode(&raw); err != nil {
			return nil, err
		}
		id := platform.ID("")
		switch value := raw["_id"].(type) {
		case bson.ObjectID:
			id = platform.ID(value.Hex())
		case string:
			id = platform.ID(value)
		}
		if id.Valid() {
			values = append(values, StaffOption{ID: id, Name: fmt.Sprint(raw["name"]), Email: fmt.Sprint(raw["email"]), Role: fmt.Sprint(raw["role"])})
		}
	}
	return values, cursor.Err()
}

func (r *MongoRepository) CareStaffExists(ctx context.Context, id platform.ID) (bool, error) {
	var key any = id
	if parsed, err := bson.ObjectIDFromHex(string(id)); err == nil {
		key = parsed
	}
	count, err := r.db.Collection("users").CountDocuments(ctx, bson.M{"_id": key, "role": "super-admin", "$or": bson.A{bson.M{"invitationStatus": "accepted"}, bson.M{"passwordHash": bson.M{"$exists": true, "$ne": ""}}}})
	return count == 1, err
}

func (r *MongoRepository) LoadDataset(ctx context.Context, org, branch platform.ID, from, to time.Time) (Dataset, error) {
	_ = from // Full visit history is required to prove a first visit; evaluation applies the configured lookback.
	data := Dataset{Visits: []VisitFact{}, Connections: []ConnectionFact{}, ActivePeople: map[platform.ID]bool{}, Caveats: []string{}}
	type occurrence struct {
		ID       platform.ID `bson:"_id"`
		StartsAt time.Time   `bson:"startsAt"`
		EndsAt   time.Time   `bson:"endsAt"`
		Status   string      `bson:"status"`
	}
	cur, err := r.db.Collection("chms_occurrences").Find(ctx, bson.M{"organizationId": org, "homeBranchId": branch, "status": bson.M{"$ne": "cancelled"}, "endsAt": bson.M{"$lte": to}}, options.Find().SetSort(bson.D{{Key: "startsAt", Value: 1}}))
	if err != nil {
		return data, err
	}
	var occurrences []occurrence
	if err = cur.All(ctx, &occurrences); err != nil {
		return data, err
	}
	cur.Close(ctx)
	occurrenceByID := map[platform.ID]occurrence{}
	ids := []platform.ID{}
	for _, o := range occurrences {
		occurrenceByID[o.ID] = o
		ids = append(ids, o.ID)
	}
	if len(ids) == 0 {
		data.Caveats = append(data.Caveats, "No completed non-cancelled occurrences are available; candidate generation is unavailable rather than zero.")
		return data, nil
	}
	// Separate decoding struct tags avoid relying on projection field order.
	type attendanceDoc struct {
		ID           platform.ID `bson:"_id"`
		OccurrenceID platform.ID `bson:"occurrenceId"`
		PersonID     platform.ID `bson:"personId"`
		Guest        bool        `bson:"guest"`
		UpdatedAt    time.Time   `bson:"updatedAt"`
	}
	aCur, err := r.db.Collection("chms_attendance").Find(ctx, bson.M{"organizationId": org, "occurrenceId": bson.M{"$in": ids}, "status": "present"})
	if err != nil {
		return data, err
	}
	var marks []attendanceDoc
	if err = aCur.All(ctx, &marks); err != nil {
		return data, err
	}
	aCur.Close(ctx)
	personIDs := []platform.ID{}
	seen := map[platform.ID]bool{}
	for _, mark := range marks {
		if !seen[mark.PersonID] {
			seen[mark.PersonID] = true
			personIDs = append(personIDs, mark.PersonID)
		}
	}
	if len(personIDs) > 0 {
		pCur, findErr := r.db.Collection("chms_people").Find(ctx, bson.M{"organizationId": org, "_id": bson.M{"$in": personIDs}, "archivedAt": nil}, options.Find().SetProjection(bson.M{"_id": 1}))
		if findErr != nil {
			return data, findErr
		}
		var people []struct {
			ID platform.ID `bson:"_id"`
		}
		if findErr = pCur.All(ctx, &people); findErr != nil {
			return data, findErr
		}
		pCur.Close(ctx)
		for _, person := range people {
			data.ActivePeople[person.ID] = true
		}
	}
	for _, mark := range marks {
		o := occurrenceByID[mark.OccurrenceID]
		data.Visits = append(data.Visits, VisitFact{AttendanceID: mark.ID, OccurrenceID: mark.OccurrenceID, PersonID: mark.PersonID, StartsAt: o.StartsAt, RecordedAt: mark.UpdatedAt, Guest: mark.Guest})
		if mark.UpdatedAt.After(timeValue(data.LatestRecordedAt)) {
			v := mark.UpdatedAt
			data.LatestRecordedAt = &v
		}
	}
	if len(personIDs) > 0 {
		gCur, findErr := r.db.Collection("chms_group_memberships").Find(ctx, bson.M{"organizationId": org, "personId": bson.M{"$in": personIDs}, "status": "active", "joinedAt": bson.M{"$ne": nil}})
		if findErr != nil {
			return data, findErr
		}
		type groupDoc struct {
			ID       platform.ID `bson:"_id"`
			PersonID platform.ID `bson:"personId"`
			JoinedAt *time.Time  `bson:"joinedAt"`
		}
		var groupDocs []groupDoc
		if findErr = gCur.All(ctx, &groupDocs); findErr != nil {
			return data, findErr
		}
		gCur.Close(ctx)
		for _, g := range groupDocs {
			if g.JoinedAt != nil {
				data.Connections = append(data.Connections, ConnectionFact{ID: g.ID, PersonID: g.PersonID, OccurredAt: *g.JoinedAt, Kind: "group"})
			}
		}
		sCur, findErr := r.db.Collection("chms_assignments").Find(ctx, bson.M{"organizationId": org, "personId": bson.M{"$in": personIDs}, "status": bson.M{"$in": []string{"accepted", "completed"}}})
		if findErr != nil {
			return data, findErr
		}
		type assignmentDoc struct {
			ID       platform.ID `bson:"_id"`
			PersonID platform.ID `bson:"personId"`
			StartsAt time.Time   `bson:"startsAt"`
		}
		var assignments []assignmentDoc
		if findErr = sCur.All(ctx, &assignments); findErr != nil {
			return data, findErr
		}
		sCur.Close(ctx)
		for _, a := range assignments {
			data.Connections = append(data.Connections, ConnectionFact{ID: a.ID, PersonID: a.PersonID, OccurredAt: a.StartsAt, Kind: "serving"})
		}
	}
	if len(data.Visits) == 0 {
		data.Caveats = append(data.Caveats, "Completed occurrences have no qualifying named attendance; absence cannot be inferred.")
	}
	return data, nil
}
func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
