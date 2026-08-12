package communications

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CampaignSchemaVersion = 1

var placeholderPattern = regexp.MustCompile(`\{\{([a-z_]+)\}\}`)
var allowedPlaceholders = map[string]bool{"first_name": true, "church_name": true, "unsubscribe_url": true}

type Template struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string     `json:"name" bson:"name"`
	Channel                   string     `json:"channel" bson:"channel"`
	Subject                   string     `json:"subject,omitempty" bson:"subject,omitempty"`
	Body                      string     `json:"body" bson:"body"`
	State                     string     `json:"state" bson:"state"`
	PublishedAt               *time.Time `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
}
type TemplateInput struct {
	Name            string      `json:"name"`
	BranchID        platform.ID `json:"branchId"`
	Channel         string      `json:"channel"`
	Subject         string      `json:"subject"`
	Body            string      `json:"body"`
	ExpectedVersion int64       `json:"expectedVersion"`
}

func (in *TemplateInput) NormalizeAndValidate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Channel = strings.ToLower(strings.TrimSpace(in.Channel))
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(in.Body)
	if in.Name == "" || len(in.Name) > 100 || !in.BranchID.Valid() || !map[string]bool{"email": true, "sms": true, "whatsapp": true}[in.Channel] {
		return errors.New("name, branch and email, sms or whatsapp channel are required")
	}
	if in.Channel == "email" && (in.Subject == "" || len(in.Subject) > 180) {
		return errors.New("email templates require a subject up to 180 characters")
	}
	if in.Channel != "email" && in.Subject != "" {
		return errors.New("only email templates use a subject")
	}
	limit := 10000
	if in.Channel != "email" {
		limit = 1200
	}
	if in.Body == "" || len(in.Body) > limit {
		return errors.New("template body is empty or exceeds the channel limit")
	}
	for _, match := range placeholderPattern.FindAllStringSubmatch(in.Subject+" "+in.Body, -1) {
		if !allowedPlaceholders[match[1]] {
			return errors.New("template contains an unsupported placeholder")
		}
	}
	if in.Channel == "email" && !strings.Contains(in.Body, "{{unsubscribe_url}}") {
		return errors.New("email templates must include {{unsubscribe_url}}")
	}
	return nil
}

type Campaign struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string         `json:"name" bson:"name"`
	AudienceID                platform.ID    `json:"audienceId" bson:"audienceId"`
	AudienceVersion           int64          `json:"audienceVersion" bson:"audienceVersion"`
	TemplateID                platform.ID    `json:"templateId" bson:"templateId"`
	TemplateVersion           int64          `json:"templateVersion" bson:"templateVersion"`
	Purpose                   string         `json:"purpose" bson:"purpose"`
	Channel                   string         `json:"channel" bson:"channel"`
	State                     string         `json:"state" bson:"state"`
	ApprovalRequestedAt       *time.Time     `json:"approvalRequestedAt,omitempty" bson:"approvalRequestedAt,omitempty"`
	ApprovedAt                *time.Time     `json:"approvedAt,omitempty" bson:"approvedAt,omitempty"`
	ApprovedBy                platform.Actor `json:"approvedBy,omitempty" bson:"approvedBy,omitempty"`
	ScheduledAt               *time.Time     `json:"scheduledAt,omitempty" bson:"scheduledAt,omitempty"`
	Timezone                  string         `json:"timezone,omitempty" bson:"timezone,omitempty"`
	StartedAt                 *time.Time     `json:"startedAt,omitempty" bson:"startedAt,omitempty"`
	CompletedAt               *time.Time     `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	RecipientCount            int            `json:"recipientCount" bson:"recipientCount"`
	AcceptedCount             int            `json:"acceptedCount" bson:"acceptedCount"`
	DeliveredCount            int            `json:"deliveredCount" bson:"deliveredCount"`
	FailedCount               int            `json:"failedCount" bson:"failedCount"`
	SuppressedCount           int            `json:"suppressedCount" bson:"suppressedCount"`
}
type CampaignInput struct {
	Name            string      `json:"name"`
	BranchID        platform.ID `json:"branchId"`
	AudienceID      platform.ID `json:"audienceId"`
	TemplateID      platform.ID `json:"templateId"`
	ExpectedVersion int64       `json:"expectedVersion"`
}

func (in *CampaignInput) NormalizeAndValidate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 || !in.BranchID.Valid() || !in.AudienceID.Valid() || !in.TemplateID.Valid() {
		return errors.New("name, branch, audience and template are required")
	}
	return nil
}

type ScheduleInput struct {
	ExpectedVersion int64     `json:"expectedVersion"`
	ScheduledAt     time.Time `json:"scheduledAt"`
	Timezone        string    `json:"timezone"`
}

func (in *ScheduleInput) NormalizeAndValidate(now time.Time) error {
	in.ScheduledAt = in.ScheduledAt.UTC()
	in.Timezone = strings.TrimSpace(in.Timezone)
	if in.ExpectedVersion < 1 || in.ScheduledAt.Before(now.Add(-time.Minute)) || in.Timezone == "" {
		return errors.New("current version, a future schedule and timezone are required")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return errors.New("timezone must be an IANA timezone")
	}
	return nil
}

type Delivery struct {
	ID                platform.ID `json:"id" bson:"_id"`
	OrganizationID    platform.ID `json:"organizationId" bson:"organizationId"`
	BranchID          platform.ID `json:"branchId" bson:"branchId"`
	CampaignID        platform.ID `json:"campaignId" bson:"campaignId"`
	PersonID          platform.ID `json:"personId" bson:"personId"`
	Channel           string      `json:"channel" bson:"channel"`
	DestinationHint   string      `json:"destinationHint" bson:"destinationHint"`
	InboxSubject      string      `json:"inboxSubject,omitempty" bson:"inboxSubject,omitempty"`
	InboxBody         string      `json:"inboxBody,omitempty" bson:"inboxBody,omitempty"`
	Purpose           string      `json:"purpose,omitempty" bson:"purpose,omitempty"`
	State             string      `json:"state" bson:"state"`
	Provider          string      `json:"provider" bson:"provider"`
	ProviderReference string      `json:"providerReference,omitempty" bson:"providerReference,omitempty"`
	AttemptCount      int         `json:"attemptCount" bson:"attemptCount"`
	LastErrorCode     string      `json:"lastErrorCode,omitempty" bson:"lastErrorCode,omitempty"`
	CreatedAt         time.Time   `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt" bson:"updatedAt"`
}
type DeliveryEvent struct {
	ID              platform.ID `json:"id" bson:"_id"`
	OrganizationID  platform.ID `json:"organizationId" bson:"organizationId"`
	CampaignID      platform.ID `json:"campaignId" bson:"campaignId"`
	DeliveryID      platform.ID `json:"deliveryId" bson:"deliveryId"`
	Provider        string      `json:"provider" bson:"provider"`
	ProviderEventID string      `json:"providerEventId" bson:"providerEventId"`
	Type            string      `json:"type" bson:"type"`
	OccurredAt      time.Time   `json:"occurredAt" bson:"occurredAt"`
	RecordedAt      time.Time   `json:"recordedAt" bson:"recordedAt"`
}
type ProviderEventInput struct {
	ProviderEventID string    `json:"providerEventId"`
	Reference       string    `json:"reference"`
	Type            string    `json:"type"`
	OccurredAt      time.Time `json:"occurredAt"`
}

type ProviderReceipt struct {
	Provider   string
	Reference  string
	AcceptedAt time.Time
}
type Sender interface {
	Configured(channel string) bool
	Send(context.Context, string, string, string, string) (ProviderReceipt, error)
}
