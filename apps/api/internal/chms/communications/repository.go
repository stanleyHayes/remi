package communications

import (
	"context"
	"errors"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
)

const audiencesCollection = "chms_saved_audiences"
const exportsCollection = "chms_audience_exports"
const templatesCollection = "chms_communication_templates"
const campaignsCollection = "chms_communication_campaigns"
const deliveriesCollection = "chms_communication_deliveries"
const deliveryEventsCollection = "chms_communication_delivery_events"

type Repository struct{ db *mongo.Database }

func NewRepository(db *mongo.Database) (*Repository, error) {
	if db == nil {
		return nil, errors.New("communications database is required")
	}
	return &Repository{db: db}, nil
}
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	for name, models := range map[string][]mongo.IndexModel{
		audiencesCollection:      {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "archivedAt", Value: 1}, {Key: "name", Value: 1}}}},
		exportsCollection:        {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "audienceId", Value: 1}, {Key: "exportedAt", Value: -1}}}},
		templatesCollection:      {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "channel", Value: 1}, {Key: "state", Value: 1}, {Key: "updatedAt", Value: -1}}}},
		campaignsCollection:      {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "branchId", Value: 1}, {Key: "state", Value: 1}, {Key: "scheduledAt", Value: 1}}}},
		deliveriesCollection:     {{Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "campaignId", Value: 1}, {Key: "personId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "campaignId", Value: 1}, {Key: "state", Value: 1}}}},
		deliveryEventsCollection: {{Keys: bson.D{{Key: "provider", Value: 1}, {Key: "providerEventId", Value: 1}}, Options: options.Index().SetUnique(true)}, {Keys: bson.D{{Key: "organizationId", Value: 1}, {Key: "campaignId", Value: 1}, {Key: "occurredAt", Value: -1}}}},
	} {
		if _, err := r.db.Collection(name).Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) InsertTemplate(ctx context.Context, v Template) error {
	_, err := r.db.Collection(templatesCollection).InsertOne(ctx, v)
	return err
}
func (r *Repository) ReplaceTemplate(ctx context.Context, v Template, expected int64) error {
	res, err := r.db.Collection(templatesCollection).ReplaceOne(ctx, bson.M{"_id": v.ID, "organizationId": v.OrganizationID, "version": expected, "archivedAt": nil}, v)
	if err == nil && res.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return err
}
func (r *Repository) FindTemplate(ctx context.Context, org, id platform.ID) (*Template, error) {
	var v Template
	err := r.db.Collection(templatesCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, err
}
func (r *Repository) ListTemplates(ctx context.Context, org, branch platform.ID) ([]Template, error) {
	cur, err := r.db.Collection(templatesCollection).Find(ctx, bson.M{"organizationId": org, "branchId": branch, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Template
	err = cur.All(ctx, &out)
	if out == nil {
		out = []Template{}
	}
	return out, err
}
func (r *Repository) InsertCampaign(ctx context.Context, v Campaign) error {
	_, err := r.db.Collection(campaignsCollection).InsertOne(ctx, v)
	return err
}
func (r *Repository) ReplaceCampaign(ctx context.Context, v Campaign, expected int64) error {
	res, err := r.db.Collection(campaignsCollection).ReplaceOne(ctx, bson.M{"_id": v.ID, "organizationId": v.OrganizationID, "version": expected, "archivedAt": nil}, v)
	if err == nil && res.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return err
}
func (r *Repository) FindCampaign(ctx context.Context, org, id platform.ID) (*Campaign, error) {
	var v Campaign
	err := r.db.Collection(campaignsCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": org, "archivedAt": nil}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, err
}
func (r *Repository) ListCampaigns(ctx context.Context, org, branch platform.ID) ([]Campaign, error) {
	cur, err := r.db.Collection(campaignsCollection).Find(ctx, bson.M{"organizationId": org, "branchId": branch, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Campaign
	err = cur.All(ctx, &out)
	if out == nil {
		out = []Campaign{}
	}
	return out, err
}
func (r *Repository) ListDueCampaigns(ctx context.Context, now time.Time, limit int) ([]Campaign, error) {
	cur, err := r.db.Collection(campaignsCollection).Find(ctx, bson.M{"state": "scheduled", "scheduledAt": bson.M{"$lte": now}}, options.Find().SetSort(bson.D{{Key: "scheduledAt", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Campaign
	err = cur.All(ctx, &out)
	return out, err
}
func (r *Repository) ClaimCampaign(ctx context.Context, org, id platform.ID, expected int64, now time.Time) (bool, error) {
	res, err := r.db.Collection(campaignsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "version": expected, "state": "scheduled", "scheduledAt": bson.M{"$lte": now}}, bson.M{"$set": bson.M{"state": "sending", "startedAt": now, "updatedAt": now}, "$inc": bson.M{"version": 1}})
	return err == nil && res.ModifiedCount == 1, err
}
func (r *Repository) UpsertDelivery(ctx context.Context, v Delivery) (*Delivery, bool, error) {
	_, err := r.db.Collection(deliveriesCollection).InsertOne(ctx, v)
	if err == nil {
		return &v, true, nil
	}
	if !mongo.IsDuplicateKeyError(err) {
		return nil, false, err
	}
	var existing Delivery
	err = r.db.Collection(deliveriesCollection).FindOne(ctx, bson.M{"organizationId": v.OrganizationID, "campaignId": v.CampaignID, "personId": v.PersonID}).Decode(&existing)
	return &existing, false, err
}

// EvaluateMemberDeliveryPolicy applies member-configured quiet hours and the
// daily external-message cap. Canonical consent and suppressions are evaluated
// earlier by Preview; this policy only narrows eligible campaign delivery.
func (r *Repository) EvaluateMemberDeliveryPolicy(ctx context.Context, org, person platform.ID, now time.Time) (bool, string, error) {
	var preference struct {
		QuietEnabled bool   `bson:"quietEnabled"`
		QuietStart   string `bson:"quietStart"`
		QuietEnd     string `bson:"quietEnd"`
		Timezone     string `bson:"timezone"`
		DailyCap     int    `bson:"dailyCap"`
	}
	err := r.db.Collection("chms_member_communication_preferences").FindOne(ctx, bson.M{"organizationId": org, "personId": person}).Decode(&preference)
	if err == mongo.ErrNoDocuments {
		return true, "", nil
	}
	if err != nil {
		return false, "", err
	}
	location, locationErr := time.LoadLocation(preference.Timezone)
	if locationErr != nil {
		return false, "", locationErr
	}
	local := now.In(location)
	if preference.QuietEnabled {
		start, startErr := time.Parse("15:04", preference.QuietStart)
		end, endErr := time.Parse("15:04", preference.QuietEnd)
		if startErr != nil || endErr != nil {
			return false, "", errors.New("invalid member quiet hours")
		}
		minute := local.Hour()*60 + local.Minute()
		startMinute := start.Hour()*60 + start.Minute()
		endMinute := end.Hour()*60 + end.Minute()
		quiet := startMinute < endMinute && minute >= startMinute && minute < endMinute
		if startMinute > endMinute {
			quiet = minute >= startMinute || minute < endMinute
		}
		if quiet {
			return false, "quiet_hours", nil
		}
	}
	if preference.DailyCap > 0 {
		localStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
		count, countErr := r.db.Collection(deliveriesCollection).CountDocuments(ctx, bson.M{"organizationId": org, "personId": person, "state": bson.M{"$in": bson.A{"accepted", "delivered"}}, "updatedAt": bson.M{"$gte": localStart.UTC(), "$lte": now}})
		if countErr != nil {
			return false, "", countErr
		}
		if count >= int64(preference.DailyCap) {
			return false, "daily_cap", nil
		}
	}
	return true, "", nil
}
func (r *Repository) UpdateDelivery(ctx context.Context, id platform.ID, state, provider, reference, errorCode string, now time.Time) error {
	_, err := r.db.Collection(deliveriesCollection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"state": state, "provider": provider, "providerReference": reference, "lastErrorCode": errorCode, "updatedAt": now}, "$inc": bson.M{"attemptCount": 1}})
	return err
}
func (r *Repository) ListDeliveries(ctx context.Context, org, campaign platform.ID) ([]Delivery, error) {
	cur, err := r.db.Collection(deliveriesCollection).Find(ctx, bson.M{"organizationId": org, "campaignId": campaign}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Delivery
	err = cur.All(ctx, &out)
	if out == nil {
		out = []Delivery{}
	}
	return out, err
}
func (r *Repository) FinalizeCampaign(ctx context.Context, org, id platform.ID, recipient, accepted, failed, suppressed int, now time.Time) error {
	state := "dispatched"
	if failed > 0 {
		state = "partially-failed"
	}
	_, err := r.db.Collection(campaignsCollection).UpdateOne(ctx, bson.M{"_id": id, "organizationId": org, "state": "sending"}, bson.M{"$set": bson.M{"state": state, "recipientCount": recipient, "acceptedCount": accepted, "deliveredCount": 0, "failedCount": failed, "suppressedCount": suppressed, "completedAt": now, "updatedAt": now}, "$inc": bson.M{"version": 1}})
	return err
}
func (r *Repository) InsertDeliveryEvent(ctx context.Context, v DeliveryEvent) (bool, error) {
	_, err := r.db.Collection(deliveryEventsCollection).InsertOne(ctx, v)
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return err == nil, err
}
func (r *Repository) FindDeliveryByReference(ctx context.Context, provider, ref string) (*Delivery, error) {
	var v Delivery
	err := r.db.Collection(deliveriesCollection).FindOne(ctx, bson.M{"provider": provider, "providerReference": ref}).Decode(&v)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &v, err
}
func (r *Repository) TransitionDelivery(ctx context.Context, id platform.ID, state string, now time.Time) error {
	_, err := r.db.Collection(deliveriesCollection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"state": state, "updatedAt": now}})
	return err
}
func (r *Repository) RefreshCampaignCounts(ctx context.Context, organizationID, campaignID platform.ID) error {
	counts := map[string]int64{}
	for _, state := range []string{"accepted", "delivered", "failed", "bounced", "complained"} {
		value, err := r.db.Collection(deliveriesCollection).CountDocuments(ctx, bson.M{"organizationId": organizationID, "campaignId": campaignID, "state": state})
		if err != nil {
			return err
		}
		counts[state] = value
	}
	_, err := r.db.Collection(campaignsCollection).UpdateOne(ctx, bson.M{"_id": campaignID, "organizationId": organizationID}, bson.M{"$set": bson.M{"acceptedCount": counts["accepted"] + counts["delivered"] + counts["bounced"] + counts["complained"], "deliveredCount": counts["delivered"], "failedCount": counts["failed"] + counts["bounced"] + counts["complained"], "updatedAt": time.Now().UTC()}})
	return err
}
func (r *Repository) SuppressFromProvider(ctx context.Context, d Delivery, reason string, now time.Time) error {
	filter := bson.M{"organizationId": d.OrganizationID, "personId": d.PersonID, "purpose": bson.M{"$exists": true}, "channel": d.Channel, "source": "provider-opt-out"}
	campaign, err := r.FindCampaign(ctx, d.OrganizationID, d.CampaignID)
	if err != nil {
		return err
	}
	filter["purpose"] = campaign.Purpose
	_, err = r.db.Collection("chms_suppressions").UpdateOne(ctx, filter, bson.M{"$setOnInsert": bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": d.OrganizationID, "branchId": d.BranchID, "personId": d.PersonID, "purpose": campaign.Purpose, "channel": d.Channel, "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": platform.Actor{Type: platform.ActorSystem, ID: "communication-provider"}}, "$set": bson.M{"state": "active", "reason": reason, "source": "provider-opt-out", "updatedAt": now, "updatedBy": platform.Actor{Type: platform.ActorSystem, ID: "communication-provider"}}}, options.UpdateOne().SetUpsert(true))
	return err
}
func (r *Repository) SuppressPerson(ctx context.Context, organizationID, branchID, personID platform.ID, purpose, channel, source string, now time.Time) error {
	filter := bson.M{"organizationId": organizationID, "personId": personID, "purpose": purpose, "channel": channel, "source": source}
	actor := platform.Actor{Type: platform.ActorSystem, ID: "communication-opt-out"}
	_, err := r.db.Collection("chms_suppressions").UpdateOne(ctx, filter, bson.M{"$setOnInsert": bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": organizationID, "branchId": branchID, "personId": personID, "purpose": purpose, "channel": channel, "schemaVersion": 1, "version": 1, "createdAt": now, "createdBy": actor}, "$set": bson.M{"state": "active", "reason": "member-opt-out", "source": source, "releasedAt": nil, "updatedAt": now, "updatedBy": actor}}, options.UpdateOne().SetUpsert(true))
	return err
}
func (r *Repository) InsertAudience(ctx context.Context, value Audience) error {
	_, err := r.db.Collection(audiencesCollection).InsertOne(ctx, value)
	return err
}
func (r *Repository) ReplaceAudience(ctx context.Context, value Audience, expected int64) error {
	result, err := r.db.Collection(audiencesCollection).ReplaceOne(ctx, bson.M{"_id": value.ID, "organizationId": value.OrganizationID, "version": expected, "archivedAt": nil}, value)
	if err == nil && result.MatchedCount != 1 {
		return platform.VersionConflict(expected)
	}
	return err
}
func (r *Repository) FindAudience(ctx context.Context, organizationID, id platform.ID) (*Audience, error) {
	var value Audience
	err := r.db.Collection(audiencesCollection).FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "archivedAt": nil}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) ListAudiences(ctx context.Context, organizationID, branchID platform.ID) ([]Audience, error) {
	cur, err := r.db.Collection(audiencesCollection).Find(ctx, bson.M{"organizationId": organizationID, "branchId": branchID, "archivedAt": nil}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Audience
	err = cur.All(ctx, &out)
	if out == nil {
		out = []Audience{}
	}
	return out, err
}
func (r *Repository) FindSegment(ctx context.Context, organizationID, id platform.ID) (*people.SavedSegment, error) {
	var value people.SavedSegment
	err := r.db.Collection("chms_people_segments").FindOne(ctx, bson.M{"_id": id, "organizationId": organizationID, "archivedAt": nil}).Decode(&value)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &value, err
}
func (r *Repository) ResolveSegment(ctx context.Context, organizationID platform.ID, filter people.PersonSearchFilter) ([]people.Person, error) {
	q := bson.M{"organizationId": organizationID, "homeBranchId": filter.BranchID, "archivedAt": nil}
	if len(filter.MembershipStages) > 0 {
		q["membershipStage"] = bson.M{"$in": filter.MembershipStages}
	}
	if len(filter.Tags) > 0 {
		q["tags"] = bson.M{"$all": filter.Tags}
	}
	if filter.Query != "" {
		pattern := regexp.QuoteMeta(filter.Query)
		q["$or"] = bson.A{bson.M{"names.given": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"names.family": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"names.preferred": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"personNumber": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"contactPoints.normalized": bson.Regex{Pattern: pattern, Options: "i"}}}
	}
	if filter.UpdatedFrom != nil || filter.UpdatedTo != nil {
		dates := bson.M{}
		if filter.UpdatedFrom != nil {
			dates["$gte"] = filter.UpdatedFrom.UTC()
		}
		if filter.UpdatedTo != nil {
			dates["$lt"] = filter.UpdatedTo.UTC()
		}
		q["updatedAt"] = dates
	}
	cur, err := r.db.Collection("chms_people").Find(ctx, q, options.Find().SetSort(bson.D{{Key: "names.family", Value: 1}, {Key: "names.given", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []people.Person
	err = cur.All(ctx, &out)
	return out, err
}
func (r *Repository) ConsentState(ctx context.Context, organizationID, personID platform.ID, purpose, channel string) (string, bool, error) {
	var p struct {
		State string `bson:"state"`
	}
	err := r.db.Collection("chms_consent_projections").FindOne(ctx, bson.M{"organizationId": organizationID, "personId": personID, "purpose": purpose, "channel": channel}).Decode(&p)
	if err == mongo.ErrNoDocuments {
		return "no-grant", false, nil
	}
	if err != nil {
		return "", false, err
	}
	count, err := r.db.Collection("chms_suppressions").CountDocuments(ctx, bson.M{"organizationId": organizationID, "personId": personID, "state": "active", "$and": bson.A{bson.M{"$or": bson.A{bson.M{"purpose": bson.M{"$exists": false}}, bson.M{"purpose": ""}, bson.M{"purpose": purpose}}}, bson.M{"$or": bson.A{bson.M{"channel": bson.M{"$exists": false}}, bson.M{"channel": ""}, bson.M{"channel": channel}}}}})
	if err != nil {
		return "", false, err
	}
	if count > 0 {
		return "suppressed", false, nil
	}
	if p.State != "granted" {
		return "withdrawn", false, nil
	}
	return "eligible", true, nil
}
func (r *Repository) InsertExport(ctx context.Context, value ExportRecord) error {
	_, err := r.db.Collection(exportsCollection).InsertOne(ctx, value)
	return err
}
func (r *Repository) ListExports(ctx context.Context, organizationID, audienceID platform.ID) ([]ExportRecord, error) {
	cur, err := r.db.Collection(exportsCollection).Find(ctx, bson.M{"organizationId": organizationID, "audienceId": audienceID}, options.Find().SetSort(bson.D{{Key: "exportedAt", Value: -1}}).SetLimit(100))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []ExportRecord
	err = cur.All(ctx, &out)
	if out == nil {
		out = []ExportRecord{}
	}
	return out, err
}
