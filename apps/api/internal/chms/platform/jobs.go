package platform

import (
	"context"
	"errors"
	"time"
)

type Job struct {
	ID             ID         `bson:"_id" json:"id"`
	OrganizationID ID         `bson:"organizationId" json:"organizationId"`
	Type           string     `bson:"type" json:"type"`
	State          string     `bson:"state" json:"state"`
	Payload        any        `bson:"payload" json:"-"`
	Attempts       int        `bson:"attempts" json:"attempts"`
	MaxAttempts    int        `bson:"maxAttempts" json:"maxAttempts"`
	AvailableAt    time.Time  `bson:"availableAt" json:"availableAt"`
	ClaimedAt      *time.Time `bson:"claimedAt,omitempty" json:"claimedAt,omitempty"`
	ClaimedBy      string     `bson:"claimedBy,omitempty" json:"claimedBy,omitempty"`
	CompletedAt    *time.Time `bson:"completedAt,omitempty" json:"completedAt,omitempty"`
	LastErrorCode  string     `bson:"lastErrorCode,omitempty" json:"lastErrorCode,omitempty"`
	CreatedAt      time.Time  `bson:"createdAt" json:"createdAt"`
}

type JobStore interface {
	Claim(context.Context, string, time.Time, time.Duration) (*Job, error)
	Complete(context.Context, ID, string, time.Time) error
	Retry(context.Context, ID, string, string, time.Time) error
	DeadLetter(context.Context, ID, string, string, time.Time) error
}

type JobHandler func(context.Context, Job) error

type JobRunner struct {
	Store        JobStore
	WorkerID     string
	Lease        time.Duration
	RetryBackoff func(attempt int) time.Duration
	Handlers     map[string]JobHandler
	Now          func() time.Time
}

func (r JobRunner) RunOne(ctx context.Context) (bool, error) {
	if r.Store == nil || r.WorkerID == "" {
		return false, errors.New("job runner is not configured")
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	lease := r.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	job, err := r.Store.Claim(ctx, r.WorkerID, now, lease)
	if err != nil || job == nil {
		return false, err
	}
	handler := r.Handlers[job.Type]
	if handler == nil {
		return true, r.Store.DeadLetter(ctx, job.ID, r.WorkerID, "handler_not_registered", now)
	}
	if err := handler(ctx, *job); err != nil {
		if job.Attempts >= job.MaxAttempts {
			return true, r.Store.DeadLetter(ctx, job.ID, r.WorkerID, "attempts_exhausted", now)
		}
		backoff := time.Duration(job.Attempts) * time.Second
		if r.RetryBackoff != nil {
			backoff = r.RetryBackoff(job.Attempts)
		}
		return true, r.Store.Retry(ctx, job.ID, r.WorkerID, "handler_failed", now.Add(backoff))
	}
	return true, r.Store.Complete(ctx, job.ID, r.WorkerID, now)
}
