package platform

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type AuditQuery struct {
	BranchID     ID        `json:"branchId,omitempty"`
	ActorID      ID        `json:"actorId,omitempty"`
	Action       string    `json:"action,omitempty"`
	ResourceType string    `json:"resourceType,omitempty"`
	ResourceID   ID        `json:"resourceId,omitempty"`
	SubjectID    ID        `json:"subjectId,omitempty"`
	RequestID    string    `json:"requestId,omitempty"`
	Outcome      string    `json:"outcome,omitempty"`
	StartsAt     time.Time `json:"startsAt"`
	EndsAt       time.Time `json:"endsAt"`
	Limit        int       `json:"limit,omitempty"`
	Cursor       string    `json:"cursor,omitempty"`
}

type AuditPage struct {
	Items      []AuditEvent `json:"items"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

type AuditExport struct {
	Bytes        []byte
	FileName     string
	ArtifactHash string
	RowCount     int
}

type AuditReader interface {
	FindAuditEvents(context.Context, ID, AuditQuery, time.Time, ID, int) ([]AuditEvent, error)
}

type AuditEvidence interface {
	AppendAudit(context.Context, AuditEvent) error
}

type AuditService struct {
	Store      AuditReader
	Evidence   AuditEvidence
	Authorizer Authorizer
	Now        func() time.Time
}

type auditCursor struct {
	Fingerprint string    `json:"f"`
	OccurredAt  time.Time `json:"t"`
	ID          ID        `json:"i"`
}

func (s AuditService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeAuditQuery(query AuditQuery, export bool) (AuditQuery, error) {
	query.Action = strings.ToLower(strings.TrimSpace(query.Action))
	query.ResourceType = strings.ToLower(strings.TrimSpace(query.ResourceType))
	query.RequestID = strings.TrimSpace(query.RequestID)
	query.Outcome = strings.ToLower(strings.TrimSpace(query.Outcome))
	query.StartsAt, query.EndsAt = query.StartsAt.UTC(), query.EndsAt.UTC()
	if query.StartsAt.IsZero() || query.EndsAt.IsZero() || !query.StartsAt.Before(query.EndsAt) || query.EndsAt.Sub(query.StartsAt) > 366*24*time.Hour {
		return query, ValidationError(FieldError{Path: "range", Code: "invalid", Message: "Choose a valid audit range of no more than 366 days."})
	}
	for path, value := range map[string]string{"action": query.Action, "resourceType": query.ResourceType, "requestId": query.RequestID, "outcome": query.Outcome} {
		if len(value) > 120 || strings.ContainsAny(value, "*$[]{}\\") {
			return query, ValidationError(FieldError{Path: path, Code: "invalid", Message: "Use an exact audit filter."})
		}
	}
	max := 100
	if export {
		max = 5000
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > max {
		query.Limit = max
	}
	return query, nil
}

func auditFingerprint(query AuditQuery) string {
	query.Cursor, query.Limit = "", 0
	raw, _ := json.Marshal(query)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func encodeAuditCursor(query AuditQuery, event AuditEvent) string {
	raw, _ := json.Marshal(auditCursor{Fingerprint: auditFingerprint(query), OccurredAt: event.OccurredAt, ID: event.ID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeAuditCursor(query AuditQuery) (time.Time, ID, error) {
	if query.Cursor == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
	if err != nil {
		return time.Time{}, "", ValidationError(FieldError{Path: "cursor", Code: "invalid", Message: "The audit cursor is invalid."})
	}
	var cursor auditCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Fingerprint != auditFingerprint(query) || cursor.OccurredAt.IsZero() || !cursor.ID.Valid() {
		return time.Time{}, "", ValidationError(FieldError{Path: "cursor", Code: "scope_mismatch", Message: "The audit cursor does not belong to this query."})
	}
	return cursor.OccurredAt.UTC(), cursor.ID, nil
}

func (s AuditService) authorize(principal Principal, action string, query AuditQuery, recentMFA bool) error {
	if s.Authorizer == nil {
		return &DomainError{Code: "forbidden", Message: "Audit access is unavailable."}
	}
	decision := s.Authorizer.Authorize(principal, AccessRequest{Action: action, ResourceType: "audit-event", OrganizationID: principal.OrganizationID, BranchID: query.BranchID, FieldClasses: []FieldClass{FieldOperational}, RequireRecentMFA: recentMFA, Now: s.now()})
	if !decision.Allowed {
		message := "You cannot review this audit scope."
		if decision.Code == "recent_mfa_required" {
			message = "Recent MFA confirmation is required to export audit history."
		}
		return &DomainError{Code: "forbidden", Message: message}
	}
	return nil
}

func (s AuditService) List(ctx context.Context, principal Principal, input AuditQuery, requestID string) (AuditPage, error) {
	query, err := normalizeAuditQuery(input, false)
	if err != nil {
		return AuditPage{}, err
	}
	if err = s.authorize(principal, "read", query, false); err != nil {
		return AuditPage{}, err
	}
	before, beforeID, err := decodeAuditCursor(query)
	if err != nil {
		return AuditPage{}, err
	}
	items, err := s.Store.FindAuditEvents(ctx, principal.OrganizationID, query, before, beforeID, query.Limit+1)
	if err != nil {
		return AuditPage{}, err
	}
	page := AuditPage{Items: items}
	if len(items) > query.Limit {
		page.Items = items[:query.Limit]
		page.NextCursor = encodeAuditCursor(query, page.Items[len(page.Items)-1])
	}
	if s.Evidence != nil {
		_ = s.Evidence.AppendAudit(ctx, AuditEvent{ID: ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: query.BranchID, Actor: principal.Actor, Action: "audit.read", ResourceType: "audit-event", ResourceID: ID(bson.NewObjectID().Hex()), ChangedFields: auditFilterNames(query), Outcome: "success", RequestID: requestID, OccurredAt: s.now()})
	}
	return page, nil
}

func (s AuditService) Export(ctx context.Context, principal Principal, input AuditQuery, reason, requestID string) (AuditExport, error) {
	query, err := normalizeAuditQuery(input, true)
	if err != nil {
		return AuditExport{}, err
	}
	reason = strings.TrimSpace(reason)
	if len(reason) < 10 || len(reason) > 300 {
		return AuditExport{}, ValidationError(FieldError{Path: "reason", Code: "invalid", Message: "Provide a 10–300 character handling reason."})
	}
	if err = s.authorize(principal, "export", query, true); err != nil {
		return AuditExport{}, err
	}
	query.Cursor = ""
	items, err := s.Store.FindAuditEvents(ctx, principal.OrganizationID, query, time.Time{}, "", query.Limit)
	if err != nil {
		return AuditExport{}, err
	}
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	_ = writer.Write([]string{"event_id", "occurred_at", "branch_id", "actor_type", "actor_id", "action", "outcome", "resource_type", "resource_id", "subject_ids", "reason", "request_id", "changed_fields", "before_version", "after_version"})
	for _, event := range items {
		row := []string{string(event.ID), event.OccurredAt.UTC().Format(time.RFC3339Nano), string(event.BranchID), string(event.Actor.Type), string(event.Actor.ID), event.Action, event.Outcome, event.ResourceType, string(event.ResourceID), joinIDs(event.SubjectIDs), event.Reason, event.RequestID, strings.Join(event.ChangedFields, "|"), strconv.FormatInt(event.BeforeVersion, 10), strconv.FormatInt(event.AfterVersion, 10)}
		for index := range row {
			row[index] = safeCSV(row[index])
		}
		_ = writer.Write(row)
	}
	writer.Flush()
	if writer.Error() != nil {
		return AuditExport{}, writer.Error()
	}
	bytes := []byte(builder.String())
	sum := sha256.Sum256(bytes)
	hash := hex.EncodeToString(sum[:])
	artifactID := ID(bson.NewObjectID().Hex())
	if s.Evidence != nil {
		err = s.Evidence.AppendAudit(ctx, AuditEvent{ID: ID(bson.NewObjectID().Hex()), OrganizationID: principal.OrganizationID, BranchID: query.BranchID, Actor: principal.Actor, Action: "audit.export", ResourceType: "audit-export", ResourceID: artifactID, ChangedFields: append(auditFilterNames(query), "rowCount", "artifactHash"), Outcome: "success", Reason: reason, RequestID: requestID, OccurredAt: s.now()})
		if err != nil {
			return AuditExport{}, err
		}
	}
	return AuditExport{Bytes: bytes, FileName: "remi-audit-" + s.now().Format("20060102-150405") + ".csv", ArtifactHash: hash, RowCount: len(items)}, nil
}

func auditFilterNames(query AuditQuery) []string {
	values := map[string]bool{"range": true}
	if query.BranchID.Valid() {
		values["branchId"] = true
	}
	if query.ActorID.Valid() {
		values["actorId"] = true
	}
	if query.Action != "" {
		values["action"] = true
	}
	if query.ResourceType != "" {
		values["resourceType"] = true
	}
	if query.ResourceID.Valid() {
		values["resourceId"] = true
	}
	if query.SubjectID.Valid() {
		values["subjectId"] = true
	}
	if query.RequestID != "" {
		values["requestId"] = true
	}
	if query.Outcome != "" {
		values["outcome"] = true
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func safeCSV(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func joinIDs(values []ID) string {
	result := make([]string, len(values))
	for index := range values {
		result[index] = string(values[index])
	}
	return strings.Join(result, "|")
}

func (s *MongoPlatformStore) FindAuditEvents(ctx context.Context, organizationID ID, query AuditQuery, before time.Time, beforeID ID, limit int) ([]AuditEvent, error) {
	if s == nil || s.database == nil {
		return nil, errors.New("audit store is unavailable")
	}
	filter := bson.M{"organizationId": organizationID, "occurredAt": bson.M{"$gte": query.StartsAt, "$lt": query.EndsAt}}
	if query.BranchID.Valid() {
		filter["branchId"] = query.BranchID
	}
	if query.ActorID.Valid() {
		filter["actor.id"] = query.ActorID
	}
	if query.Action != "" {
		filter["action"] = query.Action
	}
	if query.ResourceType != "" {
		filter["resourceType"] = query.ResourceType
	}
	if query.ResourceID.Valid() {
		filter["resourceId"] = query.ResourceID
	}
	if query.SubjectID.Valid() {
		filter["subjectIds"] = query.SubjectID
	}
	if query.RequestID != "" {
		filter["requestId"] = query.RequestID
	}
	if query.Outcome != "" {
		filter["outcome"] = query.Outcome
	}
	if !before.IsZero() {
		filter["$or"] = bson.A{bson.M{"occurredAt": bson.M{"$lt": before}}, bson.M{"occurredAt": before, "_id": bson.M{"$lt": beforeID}}}
	}
	cursor, err := s.database.Collection(auditCollection).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "occurredAt", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer cursor.Close(ctx)
	var items []AuditEvent
	if err = cursor.All(ctx, &items); err != nil {
		return nil, fmt.Errorf("decode audit events: %w", err)
	}
	return items, nil
}
