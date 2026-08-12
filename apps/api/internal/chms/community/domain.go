package community

import (
	"errors"
	"sort"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

type MeetingPattern struct {
	Frequency       string `json:"frequency" bson:"frequency"`
	Weekday         string `json:"weekday" bson:"weekday"`
	LocalStart      string `json:"localStart" bson:"localStart"`
	DurationMinutes int    `json:"durationMinutes" bson:"durationMinutes"`
	Timezone        string `json:"timezone" bson:"timezone"`
	Location        string `json:"location,omitempty" bson:"location,omitempty"`
}

type Group struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string          `json:"name" bson:"name"`
	Type                      string          `json:"type" bson:"type"`
	Description               string          `json:"description,omitempty" bson:"description,omitempty"`
	HomeBranchID              platform.ID     `json:"homeBranchId" bson:"homeBranchId"`
	MinistryID                platform.ID     `json:"ministryId,omitempty" bson:"ministryId,omitempty"`
	LeaderPersonIDs           []platform.ID   `json:"leaderPersonIds" bson:"leaderPersonIds"`
	Capacity                  int             `json:"capacity" bson:"capacity"`
	ActiveMemberCount         int             `json:"activeMemberCount" bson:"activeMemberCount"`
	MeetingPattern            *MeetingPattern `json:"meetingPattern,omitempty" bson:"meetingPattern,omitempty"`
	Privacy                   string          `json:"privacy" bson:"privacy"`
	Discoverability           string          `json:"discoverability" bson:"discoverability"`
	Status                    string          `json:"status" bson:"status"`
	ClosedAt                  *time.Time      `json:"closedAt,omitempty" bson:"closedAt,omitempty"`
	ClosureReason             string          `json:"closureReason,omitempty" bson:"closureReason,omitempty"`
}

type GroupInput struct {
	HomeBranchID    platform.ID     `json:"homeBranchId"`
	MinistryID      platform.ID     `json:"ministryId,omitempty"`
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	Description     string          `json:"description"`
	LeaderPersonIDs []platform.ID   `json:"leaderPersonIds"`
	Capacity        int             `json:"capacity"`
	MeetingPattern  *MeetingPattern `json:"meetingPattern"`
	Privacy         string          `json:"privacy"`
	Discoverability string          `json:"discoverability"`
	Status          string          `json:"status"`
	Reason          string          `json:"reason"`
}

var groupTypes = map[string]bool{"community": true, "class": true, "ministry": true, "support": true, "next-step": true, "youth": true, "children": true, "other": true}
var weekdays = map[string]bool{"monday": true, "tuesday": true, "wednesday": true, "thursday": true, "friday": true, "saturday": true, "sunday": true}

func (input *GroupInput) NormalizeAndValidate() error {
	input.Name = strings.TrimSpace(input.Name)
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Description = strings.TrimSpace(input.Description)
	input.MinistryID = platform.ID(strings.TrimSpace(string(input.MinistryID)))
	input.Privacy = strings.ToLower(strings.TrimSpace(input.Privacy))
	input.Discoverability = strings.ToLower(strings.TrimSpace(input.Discoverability))
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	input.Reason = strings.TrimSpace(input.Reason)
	if !input.HomeBranchID.Valid() {
		return errors.New("home branch is required")
	}
	if len(input.Name) < 2 || len(input.Name) > 150 {
		return errors.New("name must be 2 to 150 characters")
	}
	if !groupTypes[input.Type] {
		return errors.New("choose a supported group type")
	}
	if len(input.Description) > 2000 {
		return errors.New("description must not exceed 2000 characters")
	}
	if input.Capacity < 0 || input.Capacity > 100000 {
		return errors.New("capacity must be between 0 and 100000")
	}
	input.LeaderPersonIDs = normalizeIDs(input.LeaderPersonIDs)
	if len(input.LeaderPersonIDs) > 10 {
		return errors.New("a group can have at most 10 leaders")
	}
	if input.Privacy == "" {
		input.Privacy = "request"
	}
	if input.Privacy != "open" && input.Privacy != "request" && input.Privacy != "invite-only" {
		return errors.New("privacy must be open, request or invite-only")
	}
	if input.Discoverability == "" {
		input.Discoverability = "staff"
	}
	if input.Discoverability != "staff" && input.Discoverability != "members" && input.Discoverability != "public" {
		return errors.New("discoverability must be staff, members or public")
	}
	if input.Status == "" {
		input.Status = "draft"
	}
	if input.Status != "draft" && input.Status != "active" && input.Status != "paused" && input.Status != "closed" {
		return errors.New("status must be draft, active, paused or closed")
	}
	if input.Discoverability == "public" && input.Privacy == "invite-only" {
		return errors.New("invite-only groups cannot be publicly discoverable")
	}
	if input.Status == "closed" && len(input.Reason) < 3 {
		return errors.New("closing a group requires a reason")
	}
	if input.MeetingPattern != nil {
		if err := input.MeetingPattern.normalize(); err != nil {
			return err
		}
	}
	return nil
}

func (pattern *MeetingPattern) normalize() error {
	pattern.Frequency = strings.ToLower(strings.TrimSpace(pattern.Frequency))
	pattern.Weekday = strings.ToLower(strings.TrimSpace(pattern.Weekday))
	pattern.LocalStart = strings.TrimSpace(pattern.LocalStart)
	pattern.Timezone = strings.TrimSpace(pattern.Timezone)
	pattern.Location = strings.TrimSpace(pattern.Location)
	if pattern.Frequency != "weekly" && pattern.Frequency != "biweekly" && pattern.Frequency != "monthly" {
		return errors.New("meeting frequency must be weekly, biweekly or monthly")
	}
	if !weekdays[pattern.Weekday] {
		return errors.New("meeting weekday is invalid")
	}
	if _, err := time.Parse("15:04", pattern.LocalStart); err != nil {
		return errors.New("meeting start must use HH:MM")
	}
	if _, err := time.LoadLocation(pattern.Timezone); err != nil {
		return errors.New("meeting timezone must be a valid IANA timezone")
	}
	if pattern.DurationMinutes < 15 || pattern.DurationMinutes > 1440 {
		return errors.New("meeting duration must be 15 to 1440 minutes")
	}
	if len(pattern.Location) > 250 {
		return errors.New("meeting location must not exceed 250 characters")
	}
	return nil
}

func validateTransition(from, to, reason string) error {
	if from == to {
		return nil
	}
	allowed := map[string]map[string]bool{"draft": {"active": true, "closed": true}, "active": {"paused": true, "closed": true}, "paused": {"active": true, "closed": true}}
	if !allowed[from][to] {
		return errors.New("group lifecycle transition is not allowed")
	}
	if (to == "paused" || to == "closed") && len(strings.TrimSpace(reason)) < 3 {
		return errors.New("pausing or closing a group requires a reason")
	}
	return nil
}

func normalizeIDs(values []platform.ID) []platform.ID {
	seen := map[platform.ID]bool{}
	result := []platform.ID{}
	for _, id := range values {
		if id.Valid() && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
