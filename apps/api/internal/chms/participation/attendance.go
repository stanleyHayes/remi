package participation

import (
	"errors"
	"sort"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const (
	AttendancePresent = "present"
	AttendanceAbsent  = "absent"
	AttendanceExcused = "excused"
)

// AttendanceFact is the current, unique person/occurrence projection. Every
// mutation also appends an AttendanceEvent so the projection is never the only
// evidence of what happened.
type AttendanceFact struct {
	platform.ResourceEnvelope `bson:",inline"`
	OccurrenceID              platform.ID `json:"occurrenceId" bson:"occurrenceId"`
	PersonID                  platform.ID `json:"personId" bson:"personId"`
	Status                    string      `json:"status" bson:"status"`
	Source                    string      `json:"source" bson:"source"`
	Confidence                int         `json:"confidence" bson:"confidence"`
	CheckedInAt               *time.Time  `json:"checkedInAt,omitempty" bson:"checkedInAt,omitempty"`
	CheckedOutAt              *time.Time  `json:"checkedOutAt,omitempty" bson:"checkedOutAt,omitempty"`
	StationID                 platform.ID `json:"stationId,omitempty" bson:"stationId,omitempty"`
	OperatorID                platform.ID `json:"operatorId,omitempty" bson:"operatorId,omitempty"`
	Guest                     bool        `json:"guest" bson:"guest"`
	SyncCommandID             string      `json:"syncCommandId,omitempty" bson:"syncCommandId,omitempty"`
}

type AttendanceSnapshot struct {
	Status       string      `json:"status" bson:"status"`
	Source       string      `json:"source" bson:"source"`
	Confidence   int         `json:"confidence" bson:"confidence"`
	CheckedInAt  *time.Time  `json:"checkedInAt,omitempty" bson:"checkedInAt,omitempty"`
	CheckedOutAt *time.Time  `json:"checkedOutAt,omitempty" bson:"checkedOutAt,omitempty"`
	StationID    platform.ID `json:"stationId,omitempty" bson:"stationId,omitempty"`
	OperatorID   platform.ID `json:"operatorId,omitempty" bson:"operatorId,omitempty"`
	Guest        bool        `json:"guest" bson:"guest"`
}

type AttendanceEvent struct {
	ID             platform.ID         `json:"id" bson:"_id"`
	OrganizationID platform.ID         `json:"organizationId" bson:"organizationId"`
	BranchID       platform.ID         `json:"branchId" bson:"branchId"`
	AttendanceID   platform.ID         `json:"attendanceId" bson:"attendanceId"`
	OccurrenceID   platform.ID         `json:"occurrenceId" bson:"occurrenceId"`
	PersonID       platform.ID         `json:"personId" bson:"personId"`
	Type           string              `json:"type" bson:"type"`
	Before         *AttendanceSnapshot `json:"before,omitempty" bson:"before,omitempty"`
	After          AttendanceSnapshot  `json:"after" bson:"after"`
	Reason         string              `json:"reason,omitempty" bson:"reason,omitempty"`
	Actor          platform.Actor      `json:"actor" bson:"actor"`
	RequestID      string              `json:"requestId" bson:"requestId"`
	OccurredAt     time.Time           `json:"occurredAt" bson:"occurredAt"`
}

type AttendanceInput struct {
	Status        string      `json:"status"`
	Source        string      `json:"source"`
	Confidence    int         `json:"confidence"`
	CheckedInAt   *time.Time  `json:"checkedInAt"`
	CheckedOutAt  *time.Time  `json:"checkedOutAt"`
	StationID     platform.ID `json:"stationId"`
	Guest         bool        `json:"guest"`
	SyncCommandID string      `json:"syncCommandId"`
	Reason        string      `json:"reason"`
}

type Headcount struct {
	platform.ResourceEnvelope `bson:",inline"`
	OccurrenceID              platform.ID `json:"occurrenceId" bson:"occurrenceId"`
	Category                  string      `json:"category" bson:"category"`
	RoomID                    platform.ID `json:"roomId,omitempty" bson:"roomId,omitempty"`
	Count                     int         `json:"count" bson:"count"`
	Source                    string      `json:"source" bson:"source"`
	Confidence                int         `json:"confidence" bson:"confidence"`
	ObservedAt                time.Time   `json:"observedAt" bson:"observedAt"`
	Reason                    string      `json:"reason,omitempty" bson:"reason,omitempty"`
}

type HeadcountInput struct {
	Category   string      `json:"category"`
	RoomID     platform.ID `json:"roomId"`
	Count      int         `json:"count"`
	Source     string      `json:"source"`
	Confidence int         `json:"confidence"`
	ObservedAt time.Time   `json:"observedAt"`
	Reason     string      `json:"reason"`
}

var attendanceSources = map[string]bool{"operator": true, "roster": true, "kiosk": true, "offline": true, "import": true}

func (in *AttendanceInput) NormalizeAndValidate() error {
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	in.Source = strings.ToLower(strings.TrimSpace(in.Source))
	in.SyncCommandID = strings.TrimSpace(in.SyncCommandID)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Status != AttendancePresent && in.Status != AttendanceAbsent && in.Status != AttendanceExcused {
		return errors.New("status must be present, absent or excused")
	}
	if !attendanceSources[in.Source] {
		return errors.New("source must be operator, roster, kiosk, offline or import")
	}
	if in.Confidence == 0 {
		in.Confidence = 100
	}
	if in.Confidence < 1 || in.Confidence > 100 {
		return errors.New("confidence must be between 1 and 100")
	}
	normalizeOptionalTime(&in.CheckedInAt)
	normalizeOptionalTime(&in.CheckedOutAt)
	if in.CheckedInAt != nil && in.CheckedOutAt != nil && in.CheckedOutAt.Before(*in.CheckedInAt) {
		return errors.New("check-out cannot precede check-in")
	}
	if in.Status != AttendancePresent && (in.CheckedInAt != nil || in.CheckedOutAt != nil) {
		return errors.New("only present attendance can carry check-in or check-out times")
	}
	if len(in.SyncCommandID) > 100 || len(in.Reason) > 500 {
		return errors.New("attendance metadata is too long")
	}
	return nil
}

func (in *HeadcountInput) NormalizeAndValidate() error {
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	in.Source = strings.ToLower(strings.TrimSpace(in.Source))
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Category == "" || len(in.Category) > 80 {
		return errors.New("headcount category must be 1 to 80 characters")
	}
	if !attendanceSources[in.Source] {
		return errors.New("source must be operator, roster, kiosk, offline or import")
	}
	if in.Count < 0 || in.Count > 100000 {
		return errors.New("count must be between 0 and 100000")
	}
	if in.Confidence == 0 {
		in.Confidence = 100
	}
	if in.Confidence < 1 || in.Confidence > 100 {
		return errors.New("confidence must be between 1 and 100")
	}
	if in.ObservedAt.IsZero() {
		return errors.New("observed time is required")
	}
	in.ObservedAt = in.ObservedAt.UTC()
	if len(in.Reason) > 500 {
		return errors.New("reason must not exceed 500 characters")
	}
	return nil
}

func attendanceSnapshot(f AttendanceFact) AttendanceSnapshot {
	return AttendanceSnapshot{Status: f.Status, Source: f.Source, Confidence: f.Confidence, CheckedInAt: cloneTime(f.CheckedInAt), CheckedOutAt: cloneTime(f.CheckedOutAt), StationID: f.StationID, OperatorID: f.OperatorID, Guest: f.Guest}
}

func normalizeOptionalTime(value **time.Time) {
	if *value == nil {
		return
	}
	normalized := (*value).UTC()
	*value = &normalized
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func sortAttendance(values []AttendanceFact) {
	sort.Slice(values, func(i, j int) bool { return values[i].PersonID < values[j].PersonID })
}
