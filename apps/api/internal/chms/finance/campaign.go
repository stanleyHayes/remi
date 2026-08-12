package finance

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const campaignsCollection = "chms_finance_campaigns"

var campaignSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Campaign struct {
	platform.ResourceEnvelope `bson:",inline"`
	Slug                      string         `json:"slug" bson:"slug"`
	Title                     string         `json:"title" bson:"title"`
	Summary                   string         `json:"summary" bson:"summary"`
	Story                     string         `json:"story" bson:"story"`
	CoverImageURL             string         `json:"coverImageUrl,omitempty" bson:"coverImageUrl,omitempty"`
	FundID                    platform.ID    `json:"fundId" bson:"fundId"`
	Goal                      platform.Money `json:"goal" bson:"goal"`
	StartsAt                  time.Time      `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time      `json:"endsAt" bson:"endsAt"`
	Status                    string         `json:"status" bson:"status"`
	Featured                  bool           `json:"featured" bson:"featured"`
	RaisedAmountMinor         int64          `json:"raisedAmountMinor" bson:"-"`
	GiftCount                 int64          `json:"giftCount" bson:"-"`
}

type CampaignInput struct {
	Slug            string         `json:"slug"`
	Title           string         `json:"title"`
	Summary         string         `json:"summary"`
	Story           string         `json:"story"`
	CoverImageURL   string         `json:"coverImageUrl"`
	FundID          platform.ID    `json:"fundId"`
	Goal            platform.Money `json:"goal"`
	StartsAt        time.Time      `json:"startsAt"`
	EndsAt          time.Time      `json:"endsAt"`
	Status          string         `json:"status"`
	Featured        bool           `json:"featured"`
	ExpectedVersion int64          `json:"expectedVersion"`
}

func (i *CampaignInput) NormalizeAndValidate() error {
	i.Slug = strings.ToLower(strings.TrimSpace(i.Slug))
	i.Title, i.Summary, i.Story = strings.TrimSpace(i.Title), strings.TrimSpace(i.Summary), strings.TrimSpace(i.Story)
	i.CoverImageURL = strings.TrimSpace(i.CoverImageURL)
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
	i.Goal.Currency = strings.ToUpper(strings.TrimSpace(i.Goal.Currency))
	if !campaignSlugPattern.MatchString(i.Slug) || len(i.Slug) > 100 {
		return fmt.Errorf("slug must be a lowercase URL-safe value")
	}
	if len(i.Title) < 3 || len(i.Title) > 140 || len(i.Summary) < 10 || len(i.Summary) > 320 || len(i.Story) < 20 || len(i.Story) > 20_000 {
		return fmt.Errorf("title, summary or story length is invalid")
	}
	if !i.FundID.Valid() || i.Goal.Currency != "GHS" || i.Goal.AmountMinor <= 0 {
		return fmt.Errorf("an active fund and positive GHS goal are required")
	}
	if i.StartsAt.IsZero() || i.EndsAt.IsZero() || !i.EndsAt.After(i.StartsAt) {
		return fmt.Errorf("campaign end must be after its start")
	}
	if !map[string]bool{"draft": true, "published": true, "paused": true, "completed": true}[i.Status] {
		return fmt.Errorf("unsupported campaign status")
	}
	return nil
}

func (s Service) SaveCampaign(ctx context.Context, p platform.Principal, id platform.ID, input CampaignInput, requestID string) (*Campaign, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, invalid(err)
	}
	action := "create"
	if id.Valid() {
		action = "update"
	}
	if !s.allowed(p, action, "") {
		return nil, denied()
	}
	if fund, err := s.Repository.FindActiveFund(ctx, p.OrganizationID, input.FundID, s.now()); err != nil || fund == nil {
		if err != nil {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "fundId", Code: "inactive_fund", Message: "Campaign fund is not active."})
	}
	now := s.now()
	value := Campaign{ResourceEnvelope: envelope(p, "", now), Slug: input.Slug, Title: input.Title, Summary: input.Summary, Story: input.Story, CoverImageURL: input.CoverImageURL, FundID: input.FundID, Goal: input.Goal, StartsAt: input.StartsAt.UTC(), EndsAt: input.EndsAt.UTC(), Status: input.Status, Featured: input.Featured}
	expected := int64(0)
	if id.Valid() {
		if err := platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
			return nil, err
		}
		current, err := s.Repository.FindCampaign(ctx, p.OrganizationID, id)
		if err != nil || current == nil {
			return nil, denied()
		}
		value.ResourceEnvelope, value.ID, expected = current.ResourceEnvelope, id, input.ExpectedVersion
	}
	set := bson.M{"slug": value.Slug, "title": value.Title, "summary": value.Summary, "story": value.Story, "coverImageUrl": value.CoverImageURL, "fundId": value.FundID, "goal": value.Goal, "startsAt": value.StartsAt, "endsAt": value.EndsAt, "status": value.Status, "featured": value.Featured}
	if err := s.transact(ctx, p, "", "finance.campaign."+action, "campaign", campaignsCollection, value.ID, expected, set, value, []string{"slug", "content", "fundId", "goal", "schedule", "status", "featured"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindCampaign(ctx, p.OrganizationID, value.ID)
}

func (s Service) ListCampaigns(ctx context.Context, p platform.Principal) ([]Campaign, error) {
	if !s.allowed(p, "read", "") {
		return nil, denied()
	}
	return s.Repository.listCampaigns(ctx, p.OrganizationID, false, "")
}

func (s Service) ListPublicCampaigns(ctx context.Context, org platform.ID) ([]Campaign, error) {
	return s.Repository.listCampaigns(ctx, org, true, "")
}

func (s Service) GetPublicCampaign(ctx context.Context, org platform.ID, slug string) (*Campaign, error) {
	items, err := s.Repository.listCampaigns(ctx, org, true, strings.ToLower(strings.TrimSpace(slug)))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &platform.DomainError{Code: "not_found", Message: "Fundraising campaign not found."}
	}
	return &items[0], nil
}

func (r *Repository) FindCampaign(ctx context.Context, org, id platform.ID) (*Campaign, error) {
	return findOne[Campaign](ctx, r, campaignsCollection, org, id)
}

func (r *Repository) listCampaigns(ctx context.Context, org platform.ID, public bool, slug string) ([]Campaign, error) {
	filter := bson.M{"organizationId": org, "archivedAt": nil}
	if public {
		filter["status"] = bson.M{"$in": bson.A{"published", "completed"}}
	}
	if slug != "" {
		filter["slug"] = slug
	}
	cursor, err := r.database.Collection(campaignsCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "featured", Value: -1}, {Key: "startsAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var items []Campaign
	if err = cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	for idx := range items {
		var totals struct {
			Raised int64 `bson:"raised"`
			Gifts  int64 `bson:"gifts"`
		}
		pipe := mongo.Pipeline{{{Key: "$match", Value: bson.M{"organizationId": org, "campaignId": items[idx].ID, "state": "posted"}}}, {{Key: "$group", Value: bson.M{"_id": nil, "raised": bson.M{"$sum": "$total.amountMinor"}, "gifts": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$gt": bson.A{"$total.amountMinor", 0}}, 1, 0}}}}}}}
		cur, aggregateErr := r.database.Collection(contributionsCollection).Aggregate(ctx, pipe)
		if aggregateErr != nil {
			return nil, aggregateErr
		}
		if cur.Next(ctx) {
			_ = cur.Decode(&totals)
		}
		_ = cur.Close(ctx)
		items[idx].RaisedAmountMinor, items[idx].GiftCount = totals.Raised, totals.Gifts
	}
	return items, nil
}
