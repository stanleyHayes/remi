package participation

import (
	"errors"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

type AttendanceLock struct {
	platform.ResourceEnvelope `bson:",inline"`
	OccurrenceID              platform.ID    `json:"occurrenceId" bson:"occurrenceId"`
	Reason                    string         `json:"reason" bson:"reason"`
	LockedAt                  time.Time      `json:"lockedAt" bson:"lockedAt"`
	LockedBy                  platform.Actor `json:"lockedBy" bson:"lockedBy"`
}

type AttendancePeriodClose struct {
	platform.ResourceEnvelope `bson:",inline"`
	StartsAt                  time.Time      `json:"startsAt" bson:"startsAt"`
	EndsAt                    time.Time      `json:"endsAt" bson:"endsAt"`
	Reason                    string         `json:"reason" bson:"reason"`
	ClosedAt                  time.Time      `json:"closedAt" bson:"closedAt"`
	ClosedBy                  platform.Actor `json:"closedBy" bson:"closedBy"`
}

type PeriodCloseInput struct {
	BranchID platform.ID `json:"branchId"`
	StartsAt time.Time   `json:"startsAt"`
	EndsAt   time.Time   `json:"endsAt"`
	Reason   string      `json:"reason"`
}

type AttendanceControlStatus struct {
	Locked bool                   `json:"locked"`
	Lock   *AttendanceLock        `json:"lock,omitempty"`
	Period *AttendancePeriodClose `json:"period,omitempty"`
}

func (in *PeriodCloseInput) NormalizeAndValidate(now time.Time) error {
	in.Reason = strings.TrimSpace(in.Reason)
	in.StartsAt = in.StartsAt.UTC()
	in.EndsAt = in.EndsAt.UTC()
	if !in.BranchID.Valid() {
		return errors.New("branch is required")
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() || !in.EndsAt.After(in.StartsAt) {
		return errors.New("period end must be after its start")
	}
	if in.EndsAt.Sub(in.StartsAt) > 366*24*time.Hour {
		return errors.New("period cannot exceed 366 days")
	}
	if in.EndsAt.After(now.Add(time.Minute)) {
		return errors.New("a future attendance period cannot be closed")
	}
	return platform.ValidateReason(in.Reason)
}
