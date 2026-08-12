package platform

import "time"

// AuditEvent deliberately stores field names, never sensitive before/after values.
type AuditEvent struct {
	ID             ID        `bson:"_id" json:"id"`
	OrganizationID ID        `bson:"organizationId" json:"organizationId"`
	BranchID       ID        `bson:"branchId,omitempty" json:"branchId,omitempty"`
	Actor          Actor     `bson:"actor" json:"actor"`
	Action         string    `bson:"action" json:"action"`
	ResourceType   string    `bson:"resourceType" json:"resourceType"`
	ResourceID     ID        `bson:"resourceId" json:"resourceId"`
	SubjectIDs     []ID      `bson:"subjectIds,omitempty" json:"subjectIds,omitempty"`
	ChangedFields  []string  `bson:"changedFields,omitempty" json:"changedFields,omitempty"`
	BeforeVersion  int64     `bson:"beforeVersion,omitempty" json:"beforeVersion,omitempty"`
	AfterVersion   int64     `bson:"afterVersion,omitempty" json:"afterVersion,omitempty"`
	Outcome        string    `bson:"outcome" json:"outcome"`
	Reason         string    `bson:"reason,omitempty" json:"reason,omitempty"`
	RequestID      string    `bson:"requestId" json:"requestId"`
	OccurredAt     time.Time `bson:"occurredAt" json:"occurredAt"`
}

type DomainEvent struct {
	ID               ID        `bson:"_id" json:"eventId"`
	OrganizationID   ID        `bson:"organizationId" json:"organizationId"`
	BranchID         ID        `bson:"branchId,omitempty" json:"branchId,omitempty"`
	Type             string    `bson:"type" json:"eventType"`
	EventVersion     int       `bson:"eventVersion" json:"eventVersion"`
	AggregateType    string    `bson:"aggregateType" json:"aggregateType"`
	AggregateID      ID        `bson:"aggregateId" json:"aggregateId"`
	AggregateVersion int64     `bson:"aggregateVersion" json:"aggregateVersion"`
	Actor            Actor     `bson:"actor" json:"actor"`
	RequestID        string    `bson:"requestId" json:"requestId"`
	OccurredAt       time.Time `bson:"occurredAt" json:"occurredAt"`
	RecordedAt       time.Time `bson:"recordedAt" json:"recordedAt"`
	Payload          any       `bson:"payload" json:"payload"`
}

type OutboxRecord struct {
	DomainEvent   `bson:",inline"`
	State         string     `bson:"state" json:"state"`
	Attempts      int        `bson:"attempts" json:"attempts"`
	AvailableAt   time.Time  `bson:"availableAt" json:"availableAt"`
	ClaimedAt     *time.Time `bson:"claimedAt,omitempty" json:"claimedAt,omitempty"`
	DeliveredAt   *time.Time `bson:"deliveredAt,omitempty" json:"deliveredAt,omitempty"`
	LastErrorCode string     `bson:"lastErrorCode,omitempty" json:"lastErrorCode,omitempty"`
}
