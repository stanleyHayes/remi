package participation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

type RecurrencePattern struct {
	Frequency  string   `json:"frequency" bson:"frequency"`
	DaysOfWeek []string `json:"daysOfWeek" bson:"daysOfWeek"`
	LocalStart string   `json:"localStart" bson:"localStart"`
	Interval   int      `json:"interval" bson:"interval"`
	StartsOn   string   `json:"startsOn" bson:"startsOn"`
	EndsOn     string   `json:"endsOn,omitempty" bson:"endsOn,omitempty"`
}
type ServiceDefinition struct {
	platform.ResourceEnvelope `bson:",inline"`
	Name                      string             `json:"name" bson:"name"`
	Description               string             `json:"description,omitempty" bson:"description,omitempty"`
	HomeBranchID              platform.ID        `json:"homeBranchId" bson:"homeBranchId"`
	Timezone                  string             `json:"timezone" bson:"timezone"`
	DefaultDurationMinutes    int                `json:"defaultDurationMinutes" bson:"defaultDurationMinutes"`
	DefaultRoomIDs            []platform.ID      `json:"defaultRoomIds" bson:"defaultRoomIds"`
	DefaultCapacity           int                `json:"defaultCapacity" bson:"defaultCapacity"`
	Recurrence                *RecurrencePattern `json:"recurrence,omitempty" bson:"recurrence,omitempty"`
	Status                    string             `json:"status" bson:"status"`
}
type Occurrence struct {
	platform.ResourceEnvelope `bson:",inline"`
	ServiceDefinitionID       platform.ID   `json:"serviceDefinitionId,omitempty" bson:"serviceDefinitionId,omitempty"`
	HomeBranchID              platform.ID   `json:"homeBranchId" bson:"homeBranchId"`
	OccurrenceKey             string        `json:"occurrenceKey" bson:"occurrenceKey"`
	Name                      string        `json:"name" bson:"name"`
	StartsAt                  time.Time     `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time     `json:"endsAt" bson:"endsAt"`
	Timezone                  string        `json:"timezone" bson:"timezone"`
	RoomIDs                   []platform.ID `json:"roomIds" bson:"roomIds"`
	Capacity                  int           `json:"capacity" bson:"capacity"`
	Status                    string        `json:"status" bson:"status"`
	CancelledAt               *time.Time    `json:"cancelledAt,omitempty" bson:"cancelledAt,omitempty"`
	CancellationReason        string        `json:"cancellationReason,omitempty" bson:"cancellationReason,omitempty"`
}
type DefinitionInput struct {
	HomeBranchID           platform.ID        `json:"homeBranchId"`
	Name                   string             `json:"name"`
	Description            string             `json:"description"`
	Timezone               string             `json:"timezone"`
	DefaultDurationMinutes int                `json:"defaultDurationMinutes"`
	DefaultRoomIDs         []platform.ID      `json:"defaultRoomIds"`
	DefaultCapacity        int                `json:"defaultCapacity"`
	Recurrence             *RecurrencePattern `json:"recurrence"`
	Status                 string             `json:"status"`
}
type OccurrenceInput struct {
	HomeBranchID        platform.ID   `json:"homeBranchId"`
	ServiceDefinitionID platform.ID   `json:"serviceDefinitionId"`
	Name                string        `json:"name"`
	StartsAt            time.Time     `json:"startsAt"`
	EndsAt              time.Time     `json:"endsAt"`
	Timezone            string        `json:"timezone"`
	RoomIDs             []platform.ID `json:"roomIds"`
	Capacity            int           `json:"capacity"`
}

func (in *DefinitionInput) NormalizeAndValidate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.Timezone = strings.TrimSpace(in.Timezone)
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	if !in.HomeBranchID.Valid() {
		return errors.New("home branch is required")
	}
	if in.Name == "" || len(in.Name) > 150 {
		return errors.New("service name must be 1 to 150 characters")
	}
	if len(in.Description) > 1000 {
		return errors.New("description must not exceed 1000 characters")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return errors.New("timezone must be a valid IANA timezone")
	}
	if in.DefaultDurationMinutes < 1 || in.DefaultDurationMinutes > 1440 {
		return errors.New("duration must be between 1 and 1440 minutes")
	}
	if in.DefaultCapacity < 0 || in.DefaultCapacity > 100000 {
		return errors.New("capacity must be between 0 and 100000")
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "inactive" {
		return errors.New("status must be active or inactive")
	}
	in.DefaultRoomIDs = normalizeIDs(in.DefaultRoomIDs)
	if in.Recurrence != nil {
		if err := in.Recurrence.normalize(); err != nil {
			return err
		}
	}
	return nil
}
func (r *RecurrencePattern) normalize() error {
	r.Frequency = strings.ToLower(strings.TrimSpace(r.Frequency))
	r.LocalStart = strings.TrimSpace(r.LocalStart)
	if r.Frequency != "weekly" {
		return errors.New("recurrence frequency must be weekly")
	}
	if r.Interval == 0 {
		r.Interval = 1
	}
	if r.Interval < 1 || r.Interval > 52 {
		return errors.New("recurrence interval must be between 1 and 52 weeks")
	}
	if _, err := time.Parse("15:04", r.LocalStart); err != nil {
		return errors.New("recurrence local start must use HH:MM")
	}
	if _, err := time.Parse("2006-01-02", r.StartsOn); err != nil {
		return errors.New("recurrence start must use YYYY-MM-DD")
	}
	if r.EndsOn != "" {
		end, err := time.Parse("2006-01-02", r.EndsOn)
		if err != nil {
			return errors.New("recurrence end must use YYYY-MM-DD")
		}
		start, _ := time.Parse("2006-01-02", r.StartsOn)
		if end.Before(start) {
			return errors.New("recurrence end cannot precede start")
		}
	}
	allowed := map[string]bool{"monday": true, "tuesday": true, "wednesday": true, "thursday": true, "friday": true, "saturday": true, "sunday": true}
	seen := map[string]bool{}
	days := []string{}
	for _, day := range r.DaysOfWeek {
		day = strings.ToLower(strings.TrimSpace(day))
		if !allowed[day] {
			return errors.New("recurrence contains an invalid weekday")
		}
		if !seen[day] {
			seen[day] = true
			days = append(days, day)
		}
	}
	if len(days) == 0 {
		return errors.New("weekly recurrence requires at least one weekday")
	}
	sort.Strings(days)
	r.DaysOfWeek = days
	return nil
}
func (in *OccurrenceInput) NormalizeAndValidate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Timezone = strings.TrimSpace(in.Timezone)
	if !in.HomeBranchID.Valid() {
		return errors.New("home branch is required")
	}
	if in.Name == "" || len(in.Name) > 150 {
		return errors.New("occurrence name must be 1 to 150 characters")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return errors.New("timezone must be a valid IANA timezone")
	}
	in.StartsAt = in.StartsAt.UTC()
	in.EndsAt = in.EndsAt.UTC()
	if in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) {
		return errors.New("occurrence end must be after its start")
	}
	if in.EndsAt.Sub(in.StartsAt) > 24*time.Hour {
		return errors.New("occurrence cannot exceed 24 hours")
	}
	if in.Capacity < 0 || in.Capacity > 100000 {
		return errors.New("capacity must be between 0 and 100000")
	}
	in.RoomIDs = normalizeIDs(in.RoomIDs)
	return nil
}
func OccurrenceKey(organizationID, definitionID platform.ID, startsAt time.Time) string {
	seed := fmt.Sprintf("%s|%s|%s", organizationID, definitionID, startsAt.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(seed))
	return "occ_" + hex.EncodeToString(sum[:12])
}
func ExpandRecurrence(definition ServiceDefinition, from, to time.Time) ([]OccurrenceInput, error) {
	if definition.Recurrence == nil {
		return nil, errors.New("service definition has no recurrence")
	}
	pattern := *definition.Recurrence
	if err := pattern.normalize(); err != nil {
		return nil, err
	}
	if from.IsZero() || to.IsZero() || !to.After(from) || to.Sub(from) > 550*24*time.Hour {
		return nil, errors.New("generation range must be positive and no longer than 550 days")
	}
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return nil, errors.New("service definition timezone is invalid")
	}
	startDate, _ := time.ParseInLocation("2006-01-02", pattern.StartsOn, location)
	rangeStart := from.In(location)
	cursor := time.Date(rangeStart.Year(), rangeStart.Month(), rangeStart.Day(), 0, 0, 0, 0, location)
	if cursor.Before(startDate) {
		cursor = startDate
	}
	var endDate time.Time
	if pattern.EndsOn != "" {
		endDate, _ = time.ParseInLocation("2006-01-02", pattern.EndsOn, location)
	}
	clock, _ := time.Parse("15:04", pattern.LocalStart)
	wanted := map[string]bool{}
	for _, day := range pattern.DaysOfWeek {
		wanted[day] = true
	}
	results := []OccurrenceInput{}
	for day := cursor; day.Before(to.In(location)); day = day.AddDate(0, 0, 1) {
		if !endDate.IsZero() && day.After(endDate) {
			break
		}
		weeks := int(day.Sub(startDate).Hours()/24) / 7
		if weeks < 0 || weeks%pattern.Interval != 0 || !wanted[strings.ToLower(day.Weekday().String())] {
			continue
		}
		localStart := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, location)
		if localStart.Hour() != clock.Hour() || localStart.Minute() != clock.Minute() {
			continue
		}
		startsAt := localStart.UTC()
		if startsAt.Before(from.UTC()) || !startsAt.Before(to.UTC()) {
			continue
		}
		results = append(results, OccurrenceInput{HomeBranchID: definition.HomeBranchID, ServiceDefinitionID: definition.ID, Name: definition.Name, StartsAt: startsAt, EndsAt: startsAt.Add(time.Duration(definition.DefaultDurationMinutes) * time.Minute), Timezone: definition.Timezone, RoomIDs: append([]platform.ID(nil), definition.DefaultRoomIDs...), Capacity: definition.DefaultCapacity})
	}
	return results, nil
}
func normalizeIDs(values []platform.ID) []platform.ID {
	seen := map[platform.ID]bool{}
	out := []platform.ID{}
	for _, value := range values {
		if value.Valid() && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
