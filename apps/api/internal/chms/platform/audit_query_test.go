package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

type auditQueryStore struct {
	items []AuditEvent
	query AuditQuery
}

func (s *auditQueryStore) FindAuditEvents(_ context.Context, _ ID, query AuditQuery, _ time.Time, _ ID, limit int) ([]AuditEvent, error) {
	s.query = query
	if limit < len(s.items) {
		return s.items[:limit], nil
	}
	return append([]AuditEvent(nil), s.items...), nil
}

type auditQueryEvidence struct{ items []AuditEvent }

func (e *auditQueryEvidence) AppendAudit(_ context.Context, event AuditEvent) error {
	e.items = append(e.items, event)
	return nil
}

func auditPrincipal(branches []ID, now time.Time) Principal {
	return Principal{Actor: Actor{Type: ActorStaff, ID: "auditor-1"}, OrganizationID: "org-1", Roles: []string{"auditor"}, MFAConfirmedAt: &now, Grants: []Grant{{Action: "read", Resource: "audit-event", BranchIDs: branches, FieldClasses: []FieldClass{FieldOperational}}, {Action: "export", Resource: "audit-event", BranchIDs: branches, FieldClasses: []FieldClass{FieldOperational}}}}
}

func TestAuditServiceRequiresScopeAndBindsCursorToQuery(t *testing.T) {
	now := time.Date(2026, 8, 12, 5, 0, 0, 0, time.UTC)
	store := &auditQueryStore{items: []AuditEvent{{ID: "event-3", OrganizationID: "org-1", BranchID: "branch-1", Action: "people.update", ResourceType: "person", ResourceID: "person-1", RequestID: "req-3", OccurredAt: now.Add(-time.Hour)}, {ID: "event-2", OrganizationID: "org-1", BranchID: "branch-1", Action: "people.update", ResourceType: "person", ResourceID: "person-2", RequestID: "req-2", OccurredAt: now.Add(-2 * time.Hour)}, {ID: "event-1", OrganizationID: "org-1", BranchID: "branch-1", Action: "people.create", ResourceType: "person", ResourceID: "person-3", RequestID: "req-1", OccurredAt: now.Add(-3 * time.Hour)}}}
	evidence := &auditQueryEvidence{}
	service := AuditService{Store: store, Evidence: evidence, Authorizer: GrantAuthorizer{}, Now: func() time.Time { return now }}
	query := AuditQuery{StartsAt: now.Add(-24 * time.Hour), EndsAt: now, Limit: 2}
	if _, err := service.List(context.Background(), auditPrincipal([]ID{"branch-1"}, now), query, "request-unscoped"); err == nil {
		t.Fatal("branch-scoped auditor read without explicit branch")
	}
	query.BranchID = "branch-1"
	page, err := service.List(context.Background(), auditPrincipal([]ID{"branch-1"}, now), query, "request-list")
	if err != nil || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if len(evidence.items) != 1 || evidence.items[0].Action != "audit.read" || strings.Join(evidence.items[0].ChangedFields, ",") != "branchId,range" {
		t.Fatalf("unsafe or missing read evidence: %+v", evidence.items)
	}
	query.Cursor = page.NextCursor
	query.Action = "people.create"
	if _, err = service.List(context.Background(), auditPrincipal([]ID{"branch-1"}, now), query, "request-tamper"); err == nil {
		t.Fatal("cursor was accepted after filter scope changed")
	}
}

func TestAuditExportRequiresRecentMFAAndNeutralizesFormulaText(t *testing.T) {
	now := time.Date(2026, 8, 12, 5, 0, 0, 0, time.UTC)
	store := &auditQueryStore{items: []AuditEvent{{ID: "event-export", OrganizationID: "org-1", BranchID: "branch-1", Actor: Actor{Type: ActorStaff, ID: "actor-1"}, Action: "=cmd", Outcome: "success", ResourceType: "person", ResourceID: "person-1", SubjectIDs: []ID{"subject-1"}, Reason: "+unsafe", RequestID: "request-1", ChangedFields: []string{"name"}, BeforeVersion: 1, AfterVersion: 2, OccurredAt: now.Add(-time.Hour)}}}
	evidence := &auditQueryEvidence{}
	service := AuditService{Store: store, Evidence: evidence, Authorizer: GrantAuthorizer{}, Now: func() time.Time { return now }}
	query := AuditQuery{BranchID: "branch-1", StartsAt: now.Add(-24 * time.Hour), EndsAt: now, Limit: 100}
	stale := now.Add(-11 * time.Minute)
	principal := auditPrincipal([]ID{"branch-1"}, stale)
	if _, err := service.Export(context.Background(), principal, query, "Quarterly control review", "request-denied"); err == nil {
		t.Fatal("audit export accepted stale MFA")
	}
	principal = auditPrincipal([]ID{"branch-1"}, now)
	artifact, err := service.Export(context.Background(), principal, query, "Quarterly control review", "request-export")
	if err != nil {
		t.Fatal(err)
	}
	csvText := string(artifact.Bytes)
	if !strings.Contains(csvText, "'=cmd") || !strings.Contains(csvText, "'+unsafe") {
		t.Fatalf("formula text was not neutralized: %s", csvText)
	}
	sum := sha256.Sum256(artifact.Bytes)
	if artifact.ArtifactHash != hex.EncodeToString(sum[:]) || artifact.RowCount != 1 {
		t.Fatalf("artifact integrity mismatch: %+v", artifact)
	}
	if len(evidence.items) != 1 || evidence.items[0].Action != "audit.export" || strings.Contains(strings.Join(evidence.items[0].ChangedFields, ","), "unsafe") {
		t.Fatalf("unsafe export evidence: %+v", evidence.items)
	}
}
