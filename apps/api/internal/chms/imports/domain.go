package imports

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"remi-api/internal/chms/platform"
)

const CurrentSchemaVersion = 1

var targetFieldPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*(?:\.[a-z][a-zA-Z0-9]*)*$`)

type ImportRun struct {
	platform.ResourceEnvelope `bson:",inline"`
	EntityType                string            `json:"entityType" bson:"entityType"`
	SourceType                string            `json:"sourceType" bson:"sourceType"`
	SourceName                string            `json:"sourceName" bson:"sourceName"`
	SourceHash                string            `json:"sourceHash" bson:"sourceHash"`
	Headers                   []string          `json:"headers" bson:"headers"`
	MappingVersion            int               `json:"mappingVersion" bson:"mappingVersion"`
	Mapping                   map[string]string `json:"mapping" bson:"mapping"`
	State                     string            `json:"state" bson:"state"`
	TotalRows                 int               `json:"totalRows" bson:"totalRows"`
	ValidRows                 int               `json:"validRows" bson:"validRows"`
	InvalidRows               int               `json:"invalidRows" bson:"invalidRows"`
	CommittedRows             int               `json:"committedRows" bson:"committedRows"`
	CommitCursor              int               `json:"commitCursor" bson:"commitCursor"`
	CommittedAt               *time.Time        `json:"committedAt,omitempty" bson:"committedAt,omitempty"`
	RolledBackAt              *time.Time        `json:"rolledBackAt,omitempty" bson:"rolledBackAt,omitempty"`
	ManifestHash              string            `json:"manifestHash,omitempty" bson:"manifestHash,omitempty"`
	RollbackManifestHash      string            `json:"rollbackManifestHash,omitempty" bson:"rollbackManifestHash,omitempty"`
	RollbackBlockedRows       int               `json:"rollbackBlockedRows" bson:"rollbackBlockedRows"`
}

type ImportRow struct {
	ID                      platform.ID       `json:"id" bson:"_id"`
	OrganizationID          platform.ID       `json:"organizationId" bson:"organizationId"`
	ImportRunID             platform.ID       `json:"importRunId" bson:"importRunId"`
	RowNumber               int               `json:"rowNumber" bson:"rowNumber"`
	Source                  map[string]string `json:"source" bson:"source"`
	Mapped                  map[string]string `json:"mapped,omitempty" bson:"mapped,omitempty"`
	State                   string            `json:"state" bson:"state"`
	Errors                  []RowError        `json:"errors" bson:"errors"`
	CanonicalResourceID     platform.ID       `json:"canonicalResourceId,omitempty" bson:"canonicalResourceId,omitempty"`
	ResourceVersionAtCommit int64             `json:"resourceVersionAtCommit,omitempty" bson:"resourceVersionAtCommit,omitempty"`
	CommittedAt             *time.Time        `json:"committedAt,omitempty" bson:"committedAt,omitempty"`
	RolledBackAt            *time.Time        `json:"rolledBackAt,omitempty" bson:"rolledBackAt,omitempty"`
}

type RowError struct {
	Field   string `json:"field" bson:"field"`
	Code    string `json:"code" bson:"code"`
	Message string `json:"message" bson:"message"`
}

type MappingInput struct {
	Version int               `json:"version"`
	Fields  map[string]string `json:"fields"`
}

func ValidateMapping(headers []string, input MappingInput, allowedTargets map[string]bool) error {
	if input.Version < 1 {
		return errors.New("mapping version must be positive")
	}
	headerSet := map[string]bool{}
	for _, header := range headers {
		headerSet[header] = true
	}
	targetSet := map[string]bool{}
	for source, target := range input.Fields {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if !headerSet[source] {
			return errors.New("mapping contains an unknown source header")
		}
		if !targetFieldPattern.MatchString(target) || !allowedTargets[target] {
			return errors.New("mapping contains an unsupported target field")
		}
		if targetSet[target] {
			return errors.New("multiple source columns cannot map to the same target field")
		}
		targetSet[target] = true
	}
	return nil
}

func CanTransition(from, to string) bool {
	return map[string]map[string]bool{"staged": {"mapped": true, "voided": true}, "mapped": {"validating": true, "voided": true}, "validating": {"validated": true, "failed": true}, "validated": {"committing": true, "mapped": true, "voided": true}, "committing": {"committing": true, "completed": true, "failed": true}, "failed": {"validating": true, "committing": true, "voided": true}, "completed": {"rolling-back": true}, "rolling-back": {"rolled-back": true, "rollback-blocked": true, "failed": true}, "rollback-blocked": {"rolling-back": true}}[from][to]
}

type ManifestEntry struct {
	RowNumber       int         `json:"rowNumber"`
	RowID           platform.ID `json:"rowId"`
	ResourceID      platform.ID `json:"resourceId"`
	ResourceVersion int64       `json:"resourceVersion"`
	State           string      `json:"state"`
}
