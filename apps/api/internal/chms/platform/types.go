// Package platform contains cross-cutting ChMS primitives. Domain packages may
// depend on these types; platform must never depend on a feature context.
package platform

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ID string

func (id ID) Valid() bool { return strings.TrimSpace(string(id)) != "" }

type Money struct {
	AmountMinor int64  `json:"amountMinor" bson:"amountMinor"`
	Currency    string `json:"currency" bson:"currency"`
}

func NewMoney(amountMinor int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return Money{}, errors.New("currency must be a three-letter ISO code")
	}
	return Money{AmountMinor: amountMinor, Currency: currency}, nil
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency || m.Currency == "" {
		return Money{}, errors.New("money currency mismatch")
	}
	result := m.AmountMinor + other.AmountMinor
	if (other.AmountMinor > 0 && result < m.AmountMinor) || (other.AmountMinor < 0 && result > m.AmountMinor) {
		return Money{}, errors.New("money amount overflow")
	}
	return Money{AmountMinor: result, Currency: m.Currency}, nil
}

type ActorType string

const (
	ActorStaff  ActorType = "staff"
	ActorMember ActorType = "member"
	ActorSystem ActorType = "system"
	ActorImport ActorType = "import"
)

type Actor struct {
	Type ActorType `json:"type" bson:"type"`
	ID   ID        `json:"id" bson:"id"`
}

type ResourceEnvelope struct {
	ID             ID         `json:"id" bson:"_id"`
	OrganizationID ID         `json:"organizationId" bson:"organizationId"`
	BranchID       ID         `json:"branchId,omitempty" bson:"branchId,omitempty"`
	SchemaVersion  int        `json:"-" bson:"schemaVersion"`
	Version        int64      `json:"version" bson:"version"`
	CreatedAt      time.Time  `json:"createdAt" bson:"createdAt"`
	CreatedBy      Actor      `json:"createdBy" bson:"createdBy"`
	UpdatedAt      time.Time  `json:"updatedAt" bson:"updatedAt"`
	UpdatedBy      Actor      `json:"updatedBy" bson:"updatedBy"`
	ArchivedAt     *time.Time `json:"archivedAt,omitempty" bson:"archivedAt,omitempty"`
	ArchivedBy     *Actor     `json:"archivedBy,omitempty" bson:"archivedBy,omitempty"`
	ArchiveReason  string     `json:"-" bson:"archiveReason,omitempty"`
}

type DomainError struct {
	Code    string
	Message string
	Fields  []FieldError
	Details map[string]any
}

func (e *DomainError) Error() string { return e.Message }

type FieldError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ValidationError(fields ...FieldError) *DomainError {
	return &DomainError{Code: "validation_failed", Message: "Check the highlighted fields.", Fields: fields}
}

func VersionConflict(current int64) *DomainError {
	return &DomainError{Code: "version_conflict", Message: "The record changed since it was loaded.", Details: map[string]any{"currentVersion": current}}
}

func RequireExpectedVersion(expected int64) error {
	if expected < 1 {
		return &DomainError{Code: "precondition_required", Message: "An expected record version is required."}
	}
	return nil
}

func ValidateReason(reason string) error {
	length := len(strings.TrimSpace(reason))
	if length < 3 || length > 500 {
		return fmt.Errorf("reason must be between 3 and 500 characters")
	}
	return nil
}
