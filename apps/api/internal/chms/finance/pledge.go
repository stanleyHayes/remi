package finance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

const (
	pledgesCollection         = "chms_finance_pledges"
	pledgeRemindersCollection = "chms_finance_pledge_reminders"
)

type PledgeSchedule struct {
	Frequency         string         `json:"frequency" bson:"frequency"`
	InstallmentAmount platform.Money `json:"installmentAmount" bson:"installmentAmount"`
	StartsAt          time.Time      `json:"startsAt" bson:"startsAt"`
	EndsAt            *time.Time     `json:"endsAt,omitempty" bson:"endsAt,omitempty"`
}

type PledgeReminderPreference struct {
	OptedIn           bool        `json:"optedIn" bson:"optedIn"`
	Channels          []string    `json:"channels" bson:"channels"`
	Cadence           string      `json:"cadence" bson:"cadence"`
	RecipientPersonID platform.ID `json:"recipientPersonId,omitempty" bson:"recipientPersonId,omitempty"`
}

type Pledge struct {
	platform.ResourceEnvelope `bson:",inline"`
	Donor                     DonorAttribution         `json:"donor" bson:"donor"`
	FundID                    platform.ID              `json:"fundId" bson:"fundId"`
	CampaignID                platform.ID              `json:"campaignId,omitempty" bson:"campaignId,omitempty"`
	Target                    platform.Money           `json:"target" bson:"target"`
	Schedule                  PledgeSchedule           `json:"schedule" bson:"schedule"`
	Reminder                  PledgeReminderPreference `json:"reminder" bson:"reminder"`
	State                     string                   `json:"state" bson:"state"`
	Note                      string                   `json:"note,omitempty" bson:"note,omitempty"`
	FulfilledAmountMinor      int64                    `json:"fulfilledAmountMinor" bson:"-"`
	RemainingAmountMinor      int64                    `json:"remainingAmountMinor" bson:"-"`
	EffectiveState            string                   `json:"effectiveState" bson:"-"`
}

type PledgeInput struct {
	BranchID        platform.ID              `json:"branchId"`
	Donor           DonorAttribution         `json:"donor"`
	FundID          platform.ID              `json:"fundId"`
	CampaignID      platform.ID              `json:"campaignId,omitempty"`
	Target          platform.Money           `json:"target"`
	Schedule        PledgeSchedule           `json:"schedule"`
	Reminder        PledgeReminderPreference `json:"reminder"`
	State           string                   `json:"state"`
	Note            string                   `json:"note,omitempty"`
	ExpectedVersion int64                    `json:"expectedVersion"`
}

type PledgeReminderInput struct {
	Channel string `json:"channel"`
}

type PledgeReminderDecision struct {
	ID             platform.ID `json:"id" bson:"_id"`
	OrganizationID platform.ID `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID `json:"branchId" bson:"branchId"`
	PledgeID       platform.ID `json:"pledgeId" bson:"pledgeId"`
	PersonID       platform.ID `json:"personId" bson:"personId"`
	Channel        string      `json:"channel" bson:"channel"`
	Decision       string      `json:"decision" bson:"decision"`
	Reason         string      `json:"reason" bson:"reason"`
	RequestID      string      `json:"requestId" bson:"requestId"`
	CreatedAt      time.Time   `json:"createdAt" bson:"createdAt"`
}

type ReminderEligibility func(context.Context, platform.Principal, platform.ID, platform.ID, string, string) (bool, string, error)

func (i *PledgeInput) NormalizeAndValidate() error {
	i.State = strings.ToLower(strings.TrimSpace(i.State))
	i.Note = strings.TrimSpace(i.Note)
	i.Target.Currency = strings.ToUpper(strings.TrimSpace(i.Target.Currency))
	i.Schedule.Frequency = strings.ToLower(strings.TrimSpace(i.Schedule.Frequency))
	i.Schedule.InstallmentAmount.Currency = strings.ToUpper(strings.TrimSpace(i.Schedule.InstallmentAmount.Currency))
	i.Reminder.Cadence = strings.ToLower(strings.TrimSpace(i.Reminder.Cadence))
	if !i.BranchID.Valid() || !i.FundID.Valid() || i.Target.Currency != "GHS" || i.Target.AmountMinor <= 0 {
		return fmt.Errorf("branch, active fund and a positive GHS target are required")
	}
	if err := i.Donor.NormalizeAndValidate(); err != nil || (i.Donor.Type != "person" && i.Donor.Type != "household") {
		return fmt.Errorf("pledges require a person or household donor")
	}
	if !map[string]bool{"one-time": true, "weekly": true, "monthly": true, "quarterly": true}[i.Schedule.Frequency] {
		return fmt.Errorf("choose a supported pledge frequency")
	}
	if i.Schedule.StartsAt.IsZero() || (i.Schedule.EndsAt != nil && !i.Schedule.EndsAt.After(i.Schedule.StartsAt)) {
		return fmt.Errorf("choose a valid pledge schedule")
	}
	if i.Schedule.InstallmentAmount.Currency != "GHS" || i.Schedule.InstallmentAmount.AmountMinor <= 0 || i.Schedule.InstallmentAmount.AmountMinor > i.Target.AmountMinor {
		return fmt.Errorf("installment amount must be positive GHS and not exceed the target")
	}
	if !map[string]bool{"active": true, "paused": true, "cancelled": true}[i.State] {
		return fmt.Errorf("unsupported pledge state")
	}
	seen := map[string]bool{}
	for index, channel := range i.Reminder.Channels {
		channel = strings.ToLower(strings.TrimSpace(channel))
		if !map[string]bool{"email": true, "sms": true}[channel] || seen[channel] {
			return fmt.Errorf("reminder channels must be unique email or sms values")
		}
		seen[channel], i.Reminder.Channels[index] = true, channel
	}
	if i.Reminder.OptedIn {
		if len(i.Reminder.Channels) == 0 || !map[string]bool{"monthly": true, "quarterly": true}[i.Reminder.Cadence] || !i.Reminder.RecipientPersonID.Valid() {
			return fmt.Errorf("reminder opt-in requires channels, cadence and a recipient person")
		}
	} else {
		i.Reminder.Channels, i.Reminder.Cadence, i.Reminder.RecipientPersonID = nil, "", ""
	}
	if len(i.Note) > 500 {
		return fmt.Errorf("pledge note exceeds 500 characters")
	}
	return nil
}

func (s Service) SavePledge(ctx context.Context, p platform.Principal, id platform.ID, input PledgeInput, requestID string) (*Pledge, error) {
	if err := input.NormalizeAndValidate(); err != nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "$", Code: "invalid_pledge", Message: err.Error()})
	}
	if p.Actor.Type == platform.ActorMember {
		if id.Valid() {
			current, err := s.Repository.FindPledge(ctx, p.OrganizationID, id)
			if err != nil || current == nil || !s.ownsDonor(ctx, p, current.Donor) {
				return nil, pledgeDenied()
			}
			input.Donor, input.BranchID, input.FundID, input.CampaignID = current.Donor, current.BranchID, current.FundID, current.CampaignID
		}
		if !s.ownsDonor(ctx, p, input.Donor) {
			return nil, pledgeDenied()
		}
	} else if !s.allowed(p, map[bool]string{true: "update", false: "create"}[id.Valid()], input.BranchID) {
		return nil, pledgeDenied()
	}
	if exists, err := s.Repository.DonorExists(ctx, p.OrganizationID, input.Donor); err != nil || !exists {
		if err != nil {
			return nil, err
		}
		return nil, platform.ValidationError(platform.FieldError{Path: "donor", Code: "not_found", Message: "Donor record does not exist."})
	}
	if input.Reminder.OptedIn {
		if input.Donor.Type == "person" && input.Reminder.RecipientPersonID != input.Donor.PersonID {
			return nil, platform.ValidationError(platform.FieldError{Path: "reminder.recipientPersonId", Code: "donor_mismatch", Message: "Reminder recipient must match the person making the pledge."})
		}
		if input.Donor.Type == "household" && !s.Repository.PersonInHousehold(ctx, p.OrganizationID, input.Donor.HouseholdID, input.Reminder.RecipientPersonID) {
			return nil, platform.ValidationError(platform.FieldError{Path: "reminder.recipientPersonId", Code: "household_mismatch", Message: "Reminder recipient must be an active household member."})
		}
	}
	fund, err := s.Repository.FindActiveFund(ctx, p.OrganizationID, input.FundID, s.now())
	if err != nil {
		return nil, err
	}
	if fund == nil {
		return nil, platform.ValidationError(platform.FieldError{Path: "fundId", Code: "inactive_fund", Message: "Pledge fund is not active."})
	}
	if input.CampaignID.Valid() {
		campaign, e := s.Repository.FindCampaign(ctx, p.OrganizationID, input.CampaignID)
		if e != nil {
			return nil, e
		}
		if campaign == nil || campaign.FundID != input.FundID {
			return nil, platform.ValidationError(platform.FieldError{Path: "campaignId", Code: "fund_mismatch", Message: "Campaign and pledge fund must match."})
		}
	}
	now := s.now()
	value := Pledge{ResourceEnvelope: envelope(p, input.BranchID, now), Donor: input.Donor, FundID: input.FundID, CampaignID: input.CampaignID, Target: input.Target, Schedule: input.Schedule, Reminder: input.Reminder, State: input.State, Note: input.Note}
	expected, action := int64(0), "create"
	if id.Valid() {
		if err = platform.RequireExpectedVersion(input.ExpectedVersion); err != nil {
			return nil, err
		}
		current, e := s.Repository.FindPledge(ctx, p.OrganizationID, id)
		if e != nil || current == nil {
			return nil, pledgeDenied()
		}
		if current.Version != input.ExpectedVersion {
			return nil, platform.VersionConflict(current.Version)
		}
		if current.State == "cancelled" {
			return nil, platform.ValidationError(platform.FieldError{Path: "state", Code: "terminal_state", Message: "A cancelled pledge cannot be reopened."})
		}
		if current.FulfilledAmountMinor > input.Target.AmountMinor {
			return nil, platform.ValidationError(platform.FieldError{Path: "target", Code: "below_fulfilled", Message: "Target cannot be lower than fulfilled contributions."})
		}
		value.ResourceEnvelope, value.ID, expected, action = current.ResourceEnvelope, id, current.Version, "update"
	}
	set := bson.M{"branchId": value.BranchID, "donor": value.Donor, "fundId": value.FundID, "campaignId": value.CampaignID, "target": value.Target, "schedule": value.Schedule, "reminder": value.Reminder, "state": value.State, "note": value.Note}
	if err = s.transact(ctx, p, value.BranchID, "finance.pledge."+action, "pledge", pledgesCollection, value.ID, expected, set, value, []string{"donor", "fundId", "campaignId", "target", "schedule", "reminder", "state"}, requestID); err != nil {
		return nil, err
	}
	return s.Repository.FindPledge(ctx, p.OrganizationID, value.ID)
}

func (s Service) ownsDonor(ctx context.Context, p platform.Principal, donor DonorAttribution) bool {
	if donor.Type == "person" {
		return donor.PersonID == p.Actor.ID
	}
	return donor.Type == "household" && s.Repository.PersonInHousehold(ctx, p.OrganizationID, donor.HouseholdID, p.Actor.ID)
}
func pledgeDenied() error {
	return &platform.DomainError{Code: "not_found", Message: "Pledge not found."}
}

func (s Service) ListPledges(ctx context.Context, p platform.Principal) ([]Pledge, error) {
	if p.Actor.Type != platform.ActorMember && !s.allowed(p, "read", "") {
		return nil, pledgeDenied()
	}
	member := platform.ID("")
	if p.Actor.Type == platform.ActorMember {
		member = p.Actor.ID
	}
	return s.Repository.ListPledges(ctx, p.OrganizationID, member)
}

func (s Service) GetPledge(ctx context.Context, p platform.Principal, id platform.ID) (*Pledge, error) {
	value, err := s.Repository.FindPledge(ctx, p.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if value == nil || (p.Actor.Type == platform.ActorMember && !s.ownsDonor(ctx, p, value.Donor)) || (p.Actor.Type != platform.ActorMember && !s.allowed(p, "read", value.BranchID)) {
		return nil, pledgeDenied()
	}
	return value, nil
}

func (s Service) SendPledgeReminder(ctx context.Context, p platform.Principal, id platform.ID, input PledgeReminderInput, requestID string) (*PledgeReminderDecision, error) {
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	pledge, err := s.Repository.FindPledge(ctx, p.OrganizationID, id)
	if err != nil || pledge == nil || p.Actor.Type == platform.ActorMember || !s.allowed(p, "update", pledge.BranchID) {
		return nil, pledgeDenied()
	}
	if !pledge.Reminder.OptedIn || !contains(pledge.Reminder.Channels, input.Channel) {
		return nil, platform.ValidationError(platform.FieldError{Path: "channel", Code: "not_opted_in", Message: "The donor has not opted in to this reminder channel."})
	}
	now := s.now()
	decision := PledgeReminderDecision{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: pledge.BranchID, PledgeID: pledge.ID, PersonID: pledge.Reminder.RecipientPersonID, Channel: input.Channel, Decision: "suppressed", Reason: "no-current-consent", RequestID: requestID, CreatedAt: now}
	if recent, e := s.Repository.HasRecentPledgeReminder(ctx, p.OrganizationID, pledge.ID, input.Channel, now.Add(-30*24*time.Hour)); e != nil {
		return nil, e
	} else if recent {
		decision.Reason = "frequency-cap"
	} else if s.ReminderEligibility != nil {
		eligible, reason, e := s.ReminderEligibility(ctx, p, pledge.BranchID, pledge.Reminder.RecipientPersonID, "giving-communications", input.Channel)
		if e != nil {
			return nil, e
		}
		decision.Reason = reason
		if eligible {
			decision.Decision, decision.Reason = "queued", "eligible"
		}
	}
	if err = s.Repository.InsertPledgeReminder(ctx, decision); err != nil {
		return nil, err
	}
	if err = s.audit(ctx, p, pledge.BranchID, "finance.pledge.reminder."+decision.Decision, "pledge", pledge.ID, []string{"channel", "consentDecision"}, requestID, now); err != nil {
		return nil, err
	}
	if decision.Decision == "queued" {
		err = s.Platform.EnqueueEvent(ctx, platform.OutboxRecord{DomainEvent: platform.DomainEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: p.OrganizationID, BranchID: pledge.BranchID, Type: "finance.pledge.reminder.queued", EventVersion: 1, AggregateType: "pledge", AggregateID: pledge.ID, AggregateVersion: pledge.Version, Actor: p.Actor, RequestID: requestID, OccurredAt: now, RecordedAt: now, Payload: bson.M{"personId": decision.PersonID, "channel": decision.Channel, "purpose": "giving-communications"}}, State: "pending", AvailableAt: now})
	}
	return &decision, err
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (r *Repository) PersonInHousehold(ctx context.Context, org, household, person platform.ID) bool {
	count, err := r.database.Collection("chms_household_memberships").CountDocuments(ctx, bson.M{"organizationId": org, "householdId": household, "personId": person, "endedAt": nil})
	return err == nil && count == 1
}
func (r *Repository) FindPledge(ctx context.Context, org, id platform.ID) (*Pledge, error) {
	value, err := findOne[Pledge](ctx, r, pledgesCollection, org, id)
	if err != nil || value == nil {
		return value, err
	}
	return value, r.decoratePledge(ctx, value)
}
func (r *Repository) ListPledges(ctx context.Context, org, member platform.ID) ([]Pledge, error) {
	filter := bson.M{"organizationId": org, "archivedAt": nil}
	if member.Valid() {
		households := []platform.ID{}
		if err := r.database.Collection("chms_household_memberships").Distinct(ctx, "householdId", bson.M{"organizationId": org, "personId": member, "endedAt": nil}).Decode(&households); err != nil {
			return nil, err
		}
		filter["$or"] = bson.A{bson.M{"donor.personId": member}, bson.M{"donor.householdId": bson.M{"$in": households}}}
	}
	cursor, err := r.database.Collection(pledgesCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	items := []Pledge{}
	if err = cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	for index := range items {
		if err = r.decoratePledge(ctx, &items[index]); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (r *Repository) decoratePledge(ctx context.Context, pledge *Pledge) error {
	pipe := mongo.Pipeline{{{Key: "$match", Value: bson.M{"organizationId": pledge.OrganizationID, "pledgeId": pledge.ID, "state": "posted"}}}, {{Key: "$group", Value: bson.M{"_id": nil, "amount": bson.M{"$sum": "$total.amountMinor"}}}}}
	cursor, err := r.database.Collection(contributionsCollection).Aggregate(ctx, pipe)
	if err != nil {
		return err
	}
	var total struct {
		Amount int64 `bson:"amount"`
	}
	if cursor.Next(ctx) {
		_ = cursor.Decode(&total)
	}
	_ = cursor.Close(ctx)
	pledge.FulfilledAmountMinor = total.Amount
	pledge.RemainingAmountMinor = pledge.Target.AmountMinor - total.Amount
	if pledge.RemainingAmountMinor < 0 {
		pledge.RemainingAmountMinor = 0
	}
	pledge.EffectiveState = pledge.State
	if pledge.State == "active" && pledge.RemainingAmountMinor == 0 {
		pledge.EffectiveState = "fulfilled"
	}
	if pledge.State == "active" && pledge.Schedule.EndsAt != nil && time.Now().UTC().After(*pledge.Schedule.EndsAt) && pledge.RemainingAmountMinor > 0 {
		pledge.EffectiveState = "expired"
	}
	return nil
}
func (r *Repository) InsertPledgeReminder(ctx context.Context, value PledgeReminderDecision) error {
	_, err := r.database.Collection(pledgeRemindersCollection).InsertOne(ctx, value)
	return err
}
func (r *Repository) HasRecentPledgeReminder(ctx context.Context, org, pledge platform.ID, channel string, since time.Time) (bool, error) {
	count, err := r.database.Collection(pledgeRemindersCollection).CountDocuments(ctx, bson.M{"organizationId": org, "pledgeId": pledge, "channel": channel, "decision": "queued", "createdAt": bson.M{"$gte": since}})
	return count > 0, err
}
