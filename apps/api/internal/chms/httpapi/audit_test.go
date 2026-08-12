package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

type auditHTTPStore struct {
	items  []platform.AuditEvent
	audits []platform.AuditEvent
}

func (s *auditHTTPStore) FindAuditEvents(context.Context, platform.ID, platform.AuditQuery, time.Time, platform.ID, int) ([]platform.AuditEvent, error) {
	return append([]platform.AuditEvent(nil), s.items...), nil
}
func (s *auditHTTPStore) AppendAudit(_ context.Context, event platform.AuditEvent) error {
	s.audits = append(s.audits, event)
	return nil
}

func TestAuditRoutesReturnSafeMetadataAndControlledCSV(t *testing.T) {
	now := time.Date(2026, 8, 12, 6, 0, 0, 0, time.UTC)
	store := &auditHTTPStore{items: []platform.AuditEvent{{ID: "audit-http-1", OrganizationID: "org-1", BranchID: "branch-1", Actor: platform.Actor{Type: platform.ActorStaff, ID: "actor-1"}, Action: "care.restricted-note.read", ResourceType: "care-case", ResourceID: "case-1", SubjectIDs: []platform.ID{"person-1"}, ChangedFields: []string{"noteAccess"}, Outcome: "success", Reason: "Approved pastoral review", RequestID: "request-source", OccurredAt: now.Add(-time.Hour)}}}
	principal := platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "auditor-1"}, OrganizationID: "org-1", Roles: []string{"auditor"}, MFAConfirmedAt: &now, Grants: []platform.Grant{{Action: "read", Resource: "audit-event", BranchIDs: []platform.ID{"branch-1"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}, {Action: "export", Resource: "audit-event", BranchIDs: []platform.ID{"branch-1"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}}}
	handler := New(Services{Audit: platform.AuditService{Store: store, Evidence: store, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}, Authorizer: platform.GrantAuthorizer{}}, "org-1")
	handler.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return principal, true }
	server := httptest.NewServer(handler.Routes())
	defer server.Close()

	query := url.Values{"branchId": {"branch-1"}, "startsAt": {now.Add(-24 * time.Hour).Format(time.RFC3339)}, "endsAt": {now.Format(time.RFC3339)}}
	response, err := http.Get(server.URL + "/audit-events?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var page map[string]any
	if json.NewDecoder(response.Body).Decode(&page) != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("audit list status=%d page=%v", response.StatusCode, page)
	}
	raw, _ := json.Marshal(page)
	for _, forbidden := range []string{"encryptedContent", "password", "token", "Private HTTP pastoral note"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("audit response leaked %q: %s", forbidden, raw)
		}
	}

	body, _ := json.Marshal(map[string]any{"query": platform.AuditQuery{BranchID: "branch-1", StartsAt: now.Add(-24 * time.Hour), EndsAt: now, Limit: 100}, "reason": "Quarterly privacy control review"})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/audit-events/export", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	export, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer export.Body.Close()
	if export.StatusCode != http.StatusOK || export.Header.Get("X-Artifact-SHA256") == "" || export.Header.Get("X-Row-Count") != "1" || !strings.Contains(export.Header.Get("Content-Disposition"), "attachment") || export.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("audit export status=%d headers=%v", export.StatusCode, export.Header)
	}
	if len(store.audits) != 2 || store.audits[0].Action != "audit.read" || store.audits[1].Action != "audit.export" {
		t.Fatalf("audit-of-audit evidence=%+v", store.audits)
	}
}
