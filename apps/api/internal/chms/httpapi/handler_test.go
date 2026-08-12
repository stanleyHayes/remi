package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/chms/reporting"
)

type policyStore struct{ person *people.Person }

func (s policyStore) FindByID(context.Context, platform.ID, platform.ID) (*people.Person, error) {
	return s.person, nil
}
func TestGetPersonConcealsCrossBranchRecord(t *testing.T) {
	person := &people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "branch-2", Version: 1}, HomeBranchID: "branch-2", Names: people.Names{Given: "Ama"}}
	authorizer := platform.GrantAuthorizer{}
	h := New(Services{Repository: policyStore{person: person}, Authorizer: authorizer}, "org-1")
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return scopedPrincipal("branch-1"), true }
	req := httptest.NewRequest(http.MethodGet, "/people/person-1", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetPersonReturnsOnlyRoleAuthorizedSections(t *testing.T) {
	person := &people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "branch-1", Version: 2}, HomeBranchID: "branch-1", Names: people.Names{Given: "Ama"}}
	h := New(Services{Repository: policyStore{person: person}, Authorizer: platform.GrantAuthorizer{}}, "org-1")
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return scopedPrincipal("branch-1"), true }
	req := httptest.NewRequest(http.MethodGet, "/people/person-1", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Permissions struct {
			Sections []string `json:"sections"`
			Actions  []string `json:"actions"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, section := range body.Permissions.Sections {
		if section == "care" || section == "giving" {
			t.Fatalf("viewer received restricted section %q", section)
		}
	}
	if len(body.Permissions.Actions) != 0 {
		t.Fatalf("viewer received mutation actions: %v", body.Permissions.Actions)
	}
	if got := rec.Header().Get("ETag"); got != "\"2\"" {
		t.Fatalf("ETag=%q", got)
	}
}

func TestGetPersonRejectsOrganizationMismatch(t *testing.T) {
	person := &people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "branch-1", Version: 1}, HomeBranchID: "branch-1"}
	h := New(Services{Repository: policyStore{person: person}, Authorizer: platform.GrantAuthorizer{}}, "org-1")
	principal := scopedPrincipal("branch-1")
	principal.OrganizationID = "org-other"
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return principal, true }
	req := httptest.NewRequest(http.MethodGet, "/people/person-1", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestUpdatePersonRejectsUnauthorizedDestinationBranch(t *testing.T) {
	person := &people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "branch-1", Version: 2}, HomeBranchID: "branch-1", Names: people.Names{Given: "Ama"}}
	h := New(Services{Repository: policyStore{person: person}, Authorizer: platform.GrantAuthorizer{}}, "org-1")
	principal := scopedPrincipal("branch-1")
	principal.Grants = append(principal.Grants, platform.Grant{Action: "update", Resource: "person", BranchIDs: []platform.ID{"branch-1"}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}})
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return principal, true }
	body := []byte(`{"expectedVersion":2,"homeBranchId":"branch-2","names":{"given":"Ama"},"membershipStage":"guest","contactPoints":[],"addresses":[],"tags":[],"customFields":{},"communicationPreferences":{},"source":{"type":"staff-entry"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/people/person-1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want concealed 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestViewerCannotArchivePerson(t *testing.T) {
	person := &people.Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "branch-1", Version: 2}, HomeBranchID: "branch-1"}
	h := New(Services{Repository: policyStore{person: person}, Authorizer: platform.GrantAuthorizer{}}, "org-1")
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return scopedPrincipal("branch-1"), true }
	req := httptest.NewRequest(http.MethodPost, "/people/person-1/archive", bytes.NewReader([]byte(`{"expectedVersion":2,"reason":"requested by member"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want concealed 404", rec.Code)
	}
}

func TestReportingCatalogIsPrivateAndPermissionScoped(t *testing.T) {
	authorizer := platform.GrantAuthorizer{}
	h := New(Services{Authorizer: authorizer, Reporting: reporting.Service{Authorizer: authorizer}}, "org-1")
	principal := scopedPrincipal("branch-1")
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return principal, true }
	req := httptest.NewRequest(http.MethodGet, "/reporting/metrics?branchId=branch-1", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
	var body reporting.Catalog
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != "people.active" {
		t.Fatalf("viewer catalog=%v", body.Items)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("requiredResource")) || bytes.Contains(rec.Body.Bytes(), []byte("fieldClass")) {
		t.Fatal("internal authorization metadata leaked")
	}
}

func TestOperationalDashboardRolesRemainDomainScoped(t *testing.T) {
	authorizer := platform.GrantAuthorizer{}
	tests := []struct {
		role         string
		allowed      string
		allowedField platform.FieldClass
		denied       string
		deniedField  platform.FieldClass
	}{
		{role: "pastor", allowed: "care-case", allowedField: platform.FieldSensitiveMinistry, denied: "finance-ledger", deniedField: platform.FieldFinancial},
		{role: "membership-admin", allowed: "person", allowedField: platform.FieldPersonal, denied: "care-case", deniedField: platform.FieldSensitiveMinistry},
		{role: "group-admin", allowed: "group", allowedField: platform.FieldOperational, denied: "person", deniedField: platform.FieldPersonal},
		{role: "volunteer-coordinator", allowed: "volunteer", allowedField: platform.FieldOperational, denied: "finance-ledger", deniedField: platform.FieldFinancial},
	}
	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			grants, ok := operationalRoleGrants(test.role)
			if !ok {
				t.Fatal("role was not recognized")
			}
			principal := platform.Principal{OrganizationID: "org-1", Grants: grants}
			allowed := authorizer.Authorize(principal, platform.AccessRequest{Action: "read", ResourceType: test.allowed, OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []platform.FieldClass{test.allowedField}})
			if !allowed.Allowed {
				t.Fatalf("expected %s access", test.allowed)
			}
			denied := authorizer.Authorize(principal, platform.AccessRequest{Action: "read", ResourceType: test.denied, OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []platform.FieldClass{test.deniedField}})
			if denied.Allowed {
				t.Fatalf("unexpected %s access", test.denied)
			}
		})
	}
	if _, ok := operationalRoleGrants("invented-role"); ok {
		t.Fatal("unknown role was accepted")
	}
}

func TestScheduledReportPrincipalRebuildRemainsRoleScoped(t *testing.T) {
	authorizer := platform.GrantAuthorizer{}
	principal, ok := StaffPrincipalForRole("pastor", "pastor-1", "org-1")
	if !ok {
		t.Fatal("pastor role was not rebuilt")
	}
	if decision := authorizer.Authorize(principal, platform.AccessRequest{Action: "read", ResourceType: "care-case", OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}}); !decision.Allowed {
		t.Fatal("pastor lost required current-role access")
	}
	if decision := authorizer.Authorize(principal, platform.AccessRequest{Action: "read", ResourceType: "finance-ledger", OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []platform.FieldClass{platform.FieldFinancial}}); decision.Allowed {
		t.Fatal("pastor inherited finance access")
	}
	if _, ok = StaffPrincipalForRole("invented-role", "user-1", "org-1"); ok {
		t.Fatal("unknown background role was accepted")
	}
}

func TestScopedStaffPrincipalCannotCrossBranchMinistryOrAssignment(t *testing.T) {
	authorizer := platform.GrantAuthorizer{}
	principal, ok := StaffPrincipalForRoleScoped("pastor", "pastor-1", "org-1", []string{"branch-1"}, []string{"pastoral-care"}, []string{"case-1"})
	if !ok {
		t.Fatal("scoped pastor was not rebuilt")
	}
	base := platform.AccessRequest{Action: "read", ResourceType: "care-case", ResourceID: "case-1", OrganizationID: "org-1", BranchID: "branch-1", MinistryID: "pastoral-care", FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}, RequireAssignment: true}
	if decision := authorizer.Authorize(principal, base); !decision.Allowed {
		t.Fatalf("scoped case denied: %s", decision.Code)
	}
	denied := map[string]platform.AccessRequest{"branch": base, "ministry": base, "assignment": base, "field": base}
	request := denied["branch"]
	request.BranchID = "branch-2"
	denied["branch"] = request
	request = denied["ministry"]
	request.MinistryID = "youth"
	denied["ministry"] = request
	request = denied["assignment"]
	request.ResourceID = "case-2"
	denied["assignment"] = request
	request = denied["field"]
	request.FieldClasses = []platform.FieldClass{platform.FieldFinancial}
	denied["field"] = request
	for name, candidate := range denied {
		if authorizer.Authorize(principal, candidate).Allowed {
			t.Fatalf("%s scope crossed", name)
		}
	}
	unscoped, ok := StaffPrincipalForRoleScoped("pastor", "pastor-2", "org-1", nil, nil, nil)
	if !ok {
		t.Fatal("known role rejected")
	}
	request = base
	request.RequireAssignment = false
	if authorizer.Authorize(unscoped, request).Allowed {
		t.Fatal("unassigned operational role received wildcard access")
	}
}

func TestReportBuilderRejectsRawPersonDimensionBeforeExecution(t *testing.T) {
	authorizer := platform.GrantAuthorizer{}
	h := New(Services{Authorizer: authorizer, Reporting: reporting.Service{Authorizer: authorizer}}, "org-1")
	h.resolvePrincipal = func(*http.Request) (platform.Principal, bool) { return scopedPrincipal("branch-1"), true }
	body := []byte(`{"branchId":"branch-1","startsAt":"2026-01-01T00:00:00Z","endsAt":"2026-02-01T00:00:00Z","timezone":"Africa/Accra","metricIds":["attendance.confirmed_people"],"dimensions":["person"]}`)
	req := httptest.NewRequest(http.MethodPost, "/reporting/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Only the approved metric and domain dimensions")) {
		t.Fatalf("unexpected error: %s", rec.Body.String())
	}
}

func scopedPrincipal(branch platform.ID) platform.Principal {
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "user-1"}, OrganizationID: "org-1", Roles: []string{"viewer"}, Grants: []platform.Grant{{Action: "read", Resource: "person", BranchIDs: []platform.ID{branch}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}}}, MFAConfirmedAt: timePointer(time.Now().UTC())}
}
func timePointer(value time.Time) *time.Time { return &value }
