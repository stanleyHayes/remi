package communications

import (
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

var purposes = map[string]bool{"church-updates": true, "event-reminders": true, "group-messages": true, "serving-reminders": true, "pastoral-care": true, "giving-communications": true}
var channels = map[string]bool{"email": true, "sms": true, "whatsapp": true, "phone": true, "push": true}

type Audience struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string        `json:"name" bson:"name"`
	Description               string        `json:"description,omitempty" bson:"description,omitempty"`
	SegmentIDs                []platform.ID `json:"segmentIds" bson:"segmentIds"`
	ExcludedPersonIDs         []platform.ID `json:"excludedPersonIds,omitempty" bson:"excludedPersonIds,omitempty"`
	Purpose                   string        `json:"purpose" bson:"purpose"`
	Channel                   string        `json:"channel" bson:"channel"`
	OwnerID                   platform.ID   `json:"ownerId" bson:"ownerId"`
}

type SaveInput struct {
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	BranchID          platform.ID   `json:"branchId"`
	SegmentIDs        []platform.ID `json:"segmentIds"`
	ExcludedPersonIDs []platform.ID `json:"excludedPersonIds"`
	Purpose           string        `json:"purpose"`
	Channel           string        `json:"channel"`
	ExpectedVersion   int64         `json:"expectedVersion"`
}

func (in *SaveInput) NormalizeAndValidate() error {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	in.Purpose, in.Channel = strings.ToLower(strings.TrimSpace(in.Purpose)), strings.ToLower(strings.TrimSpace(in.Channel))
	if in.Name == "" || len(in.Name) > 100 || len(in.Description) > 500 || !in.BranchID.Valid() {
		return errors.New("name, valid branch, and bounded description are required")
	}
	if !purposes[in.Purpose] || !channels[in.Channel] {
		return errors.New("choose a supported purpose and channel")
	}
	in.SegmentIDs = uniqueIDs(in.SegmentIDs)
	in.ExcludedPersonIDs = uniqueIDs(in.ExcludedPersonIDs)
	if len(in.SegmentIDs) == 0 || len(in.SegmentIDs) > 20 || len(in.ExcludedPersonIDs) > 500 || in.ExpectedVersion < 0 {
		return errors.New("choose 1 to 20 segments, no more than 500 exclusions, and a valid expected version")
	}
	return nil
}

func uniqueIDs(values []platform.ID) []platform.ID {
	seen, out := map[platform.ID]bool{}, make([]platform.ID, 0, len(values))
	for _, value := range values {
		if value.Valid() && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

type Recipient struct {
	PersonID     platform.ID `json:"personId" bson:"personId"`
	DisplayName  string      `json:"displayName" bson:"displayName"`
	Destination  string      `json:"-" bson:"destination"`
	Masked       string      `json:"maskedDestination" bson:"-"`
	SegmentCount int         `json:"segmentCount" bson:"segmentCount"`
}

type Preview struct {
	AudienceID       platform.ID    `json:"audienceId"`
	AudienceVersion  int64          `json:"audienceVersion"`
	Purpose          string         `json:"purpose"`
	Channel          string         `json:"channel"`
	CandidateCount   int            `json:"candidateCount"`
	RecipientCount   int            `json:"recipientCount"`
	ExclusionCounts  map[string]int `json:"exclusionCounts"`
	Recipients       []Recipient    `json:"recipients"`
	Truncated        bool           `json:"truncated"`
	ConsentEvaluated time.Time      `json:"consentEvaluatedAt"`
}

type ExportRecord struct {
	ID              platform.ID    `json:"id" bson:"_id"`
	OrganizationID  platform.ID    `json:"organizationId" bson:"organizationId"`
	BranchID        platform.ID    `json:"branchId" bson:"branchId"`
	AudienceID      platform.ID    `json:"audienceId" bson:"audienceId"`
	AudienceVersion int64          `json:"audienceVersion" bson:"audienceVersion"`
	RecipientCount  int            `json:"recipientCount" bson:"recipientCount"`
	Purpose         string         `json:"purpose" bson:"purpose"`
	Channel         string         `json:"channel" bson:"channel"`
	Reason          string         `json:"reason" bson:"reason"`
	Actor           platform.Actor `json:"actor" bson:"actor"`
	RequestID       string         `json:"requestId" bson:"requestId"`
	ExportedAt      time.Time      `json:"exportedAt" bson:"exportedAt"`
}
