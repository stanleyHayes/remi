package consent

import (
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

var supportedPurposes = map[string]bool{
	"church-updates": true, "event-reminders": true, "group-messages": true,
	"serving-reminders": true, "pastoral-care": true, "giving-communications": true,
}
var supportedChannels = map[string]bool{"email": true, "sms": true, "whatsapp": true, "phone": true, "push": true}

type Choice struct {
	Purpose           string `json:"purpose"`
	Channel           string `json:"channel"`
	State             string `json:"state"`
	NoticeVersion     string `json:"noticeVersion"`
	EvidenceReference string `json:"evidenceReference,omitempty"`
	Source            string `json:"source"`
	ExpectedVersion   int64  `json:"expectedVersion"`
}

func (c *Choice) NormalizeAndValidate() error {
	c.Purpose = strings.ToLower(strings.TrimSpace(c.Purpose))
	c.Channel = strings.ToLower(strings.TrimSpace(c.Channel))
	c.State = strings.ToLower(strings.TrimSpace(c.State))
	c.NoticeVersion = strings.TrimSpace(c.NoticeVersion)
	c.EvidenceReference = strings.TrimSpace(c.EvidenceReference)
	c.Source = strings.ToLower(strings.TrimSpace(c.Source))
	if !supportedPurposes[c.Purpose] || !supportedChannels[c.Channel] {
		return errors.New("choose a supported purpose and channel")
	}
	if c.State != "granted" && c.State != "withdrawn" {
		return errors.New("state must be granted or withdrawn")
	}
	if c.NoticeVersion == "" || c.Source == "" {
		return errors.New("notice version and source are required")
	}
	if c.ExpectedVersion < 0 || len(c.EvidenceReference) > 500 {
		return errors.New("expected version and evidence reference are invalid")
	}
	return nil
}

type Event struct {
	ID                platform.ID    `json:"id" bson:"_id"`
	OrganizationID    platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID          platform.ID    `json:"branchId" bson:"branchId"`
	PersonID          platform.ID    `json:"personId" bson:"personId"`
	Purpose           string         `json:"purpose" bson:"purpose"`
	Channel           string         `json:"channel" bson:"channel"`
	State             string         `json:"state" bson:"state"`
	NoticeVersion     string         `json:"noticeVersion" bson:"noticeVersion"`
	EvidenceReference string         `json:"evidenceReference,omitempty" bson:"evidenceReference,omitempty"`
	Source            string         `json:"source" bson:"source"`
	Actor             platform.Actor `json:"actor" bson:"actor"`
	RequestID         string         `json:"requestId" bson:"requestId"`
	OccurredAt        time.Time      `json:"occurredAt" bson:"occurredAt"`
}

type Projection struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	Purpose                   string      `json:"purpose" bson:"purpose"`
	Channel                   string      `json:"channel" bson:"channel"`
	State                     string      `json:"state" bson:"state"`
	NoticeVersion             string      `json:"noticeVersion" bson:"noticeVersion"`
	EvidenceReference         string      `json:"evidenceReference,omitempty" bson:"evidenceReference,omitempty"`
	Source                    string      `json:"source" bson:"source"`
	LastEventID               platform.ID `json:"lastEventId" bson:"lastEventId"`
}

type Suppression struct {
	platform.ResourceEnvelope `bson:",inline"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	Purpose                   string      `json:"purpose,omitempty" bson:"purpose,omitempty"`
	Channel                   string      `json:"channel,omitempty" bson:"channel,omitempty"`
	Reason                    string      `json:"reason" bson:"reason"`
	Source                    string      `json:"source" bson:"source"`
	State                     string      `json:"state" bson:"state"`
	ReleasedAt                *time.Time  `json:"releasedAt,omitempty" bson:"releasedAt,omitempty"`
}

type SuppressionInput struct {
	ID              platform.ID `json:"id"`
	Purpose         string      `json:"purpose"`
	Channel         string      `json:"channel"`
	Reason          string      `json:"reason"`
	Source          string      `json:"source"`
	State           string      `json:"state"`
	ExpectedVersion int64       `json:"expectedVersion"`
}

func (input *SuppressionInput) NormalizeAndValidate() error {
	input.Purpose = strings.ToLower(strings.TrimSpace(input.Purpose))
	input.Channel = strings.ToLower(strings.TrimSpace(input.Channel))
	input.Reason = strings.ToLower(strings.TrimSpace(input.Reason))
	input.Source = strings.ToLower(strings.TrimSpace(input.Source))
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	if input.Purpose != "" && !supportedPurposes[input.Purpose] {
		return errors.New("choose a supported purpose")
	}
	if input.Channel != "" && !supportedChannels[input.Channel] {
		return errors.New("choose a supported channel")
	}
	if input.Reason == "" || input.Source == "" || (input.State != "active" && input.State != "released") || input.ExpectedVersion < 0 {
		return errors.New("reason, source, active or released state, and valid expected version are required")
	}
	if input.State == "released" && !input.ID.Valid() {
		return errors.New("releasing a suppression requires its id")
	}
	return nil
}

type Eligibility struct {
	Eligible        bool          `json:"eligible"`
	Consent         *Projection   `json:"consent,omitempty"`
	BlockingReasons []Suppression `json:"blockingSuppressions"`
	Decision        string        `json:"decision"`
	EvaluatedAt     time.Time     `json:"evaluatedAt"`
}
