package people

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

var reasonCodePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type MembershipEvent struct {
	ID                platform.ID    `json:"id" bson:"_id"`
	OrganizationID    platform.ID    `json:"organizationId" bson:"organizationId"`
	PersonID          platform.ID    `json:"personId" bson:"personId"`
	FromStage         string         `json:"fromStage" bson:"fromStage"`
	ToStage           string         `json:"toStage" bson:"toStage"`
	EffectiveAt       time.Time      `json:"effectiveAt" bson:"effectiveAt"`
	ReasonCode        string         `json:"reasonCode" bson:"reasonCode"`
	NoteReference     platform.ID    `json:"noteReference,omitempty" bson:"noteReference,omitempty"`
	ReversesEventID   platform.ID    `json:"reversesEventId,omitempty" bson:"reversesEventId,omitempty"`
	ReversedByEventID platform.ID    `json:"reversedByEventId,omitempty" bson:"reversedByEventId,omitempty"`
	CreatedAt         time.Time      `json:"createdAt" bson:"createdAt"`
	CreatedBy         platform.Actor `json:"createdBy" bson:"createdBy"`
	RequestID         string         `json:"requestId" bson:"requestId"`
}

type MembershipTransitionInput struct {
	ToStage       string      `json:"toStage"`
	EffectiveAt   time.Time   `json:"effectiveAt"`
	ReasonCode    string      `json:"reasonCode"`
	NoteReference platform.ID `json:"noteReference"`
}

func (in *MembershipTransitionInput) NormalizeAndValidate(from string, now time.Time) error {
	in.ToStage = strings.ToLower(strings.TrimSpace(in.ToStage))
	in.ReasonCode = strings.ToLower(strings.TrimSpace(in.ReasonCode))
	if !allowedStage(from) || !allowedStage(in.ToStage) {
		return errors.New("invalid membership stage")
	}
	if from == in.ToStage {
		return errors.New("membership stage must change")
	}
	if !validMembershipTransition(from, in.ToStage) {
		return errors.New("membership transition is not allowed")
	}
	if !reasonCodePattern.MatchString(in.ReasonCode) || len(in.ReasonCode) > 64 {
		return errors.New("reason code must be a lowercase slug up to 64 characters")
	}
	if in.EffectiveAt.IsZero() {
		in.EffectiveAt = now
	}
	in.EffectiveAt = in.EffectiveAt.UTC()
	if in.EffectiveAt.After(now.Add(5 * time.Minute)) {
		return errors.New("membership transition cannot be scheduled in the future")
	}
	return nil
}

func validMembershipTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"guest":            {"returning-guest": true, "regular-attendee": true, "member": true, "inactive": true, "transferred": true, "deceased": true},
		"returning-guest":  {"regular-attendee": true, "member": true, "inactive": true, "transferred": true, "deceased": true},
		"regular-attendee": {"returning-guest": true, "member": true, "inactive": true, "transferred": true, "deceased": true},
		"member":           {"regular-attendee": true, "inactive": true, "transferred": true, "deceased": true},
		"inactive":         {"returning-guest": true, "regular-attendee": true, "member": true, "transferred": true, "deceased": true},
		"transferred":      {"returning-guest": true, "regular-attendee": true, "member": true, "inactive": true, "deceased": true},
		"deceased":         {},
	}
	return allowed[from][to]
}
