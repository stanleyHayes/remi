package governance

import (
	"context"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

type memoryStore struct {
	value   DataRequest
	history []Transition
}

func (m *memoryStore) List(context.Context, platform.ID, platform.ID, string, int64) ([]DataRequest, error) {
	return []DataRequest{m.value}, nil
}
func (m *memoryStore) Find(_ context.Context, org, id platform.ID) (*DataRequest, error) {
	if m.value.OrganizationID != org || m.value.ID != id {
		return nil, nil
	}
	v := m.value
	return &v, nil
}
func (m *memoryStore) History(context.Context, platform.ID, platform.ID) ([]Transition, error) {
	return m.history, nil
}
func (m *memoryStore) Transition(_ context.Context, current *DataRequest, event Transition, input TransitionInput) (*DataRequest, error) {
	if input.ExpectedVersion != m.value.Version {
		return nil, platform.VersionConflict(m.value.Version)
	}
	m.value.Status = input.Status
	m.value.Version++
	if input.AssigneeID.Valid() {
		m.value.AssigneeID = input.AssigneeID
	}
	m.history = append(m.history, event)
	v := m.value
	return &v, nil
}

type evidence struct{ events []platform.AuditEvent }

func (e *evidence) AppendAudit(_ context.Context, v platform.AuditEvent) error {
	e.events = append(e.events, v)
	return nil
}
func principal(role string, mfa *time.Time) platform.Principal {
	actions := []string{"read"}
	if role == "data-protection-supervisor" {
		actions = append(actions, "operate")
	}
	grants := []platform.Grant{}
	for _, a := range actions {
		grants = append(grants, platform.Grant{Action: a, Resource: "data-request", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}})
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: platform.ID(role)}, OrganizationID: "org", Roles: []string{role}, Grants: grants, MFAConfirmedAt: mfa}
}

func TestDataRequestLifecycleRequiresScopeVersionAndIndependentMFAApproval(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{value: DataRequest{ID: "request", OrganizationID: "org", BranchID: "branch", PersonID: "person", Status: "received", Version: 1}}
	log := &evidence{}
	service := Service{Store: store, Evidence: log, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	if _, err := service.List(context.Background(), platform.Principal{OrganizationID: "org"}, "branch", ""); err == nil {
		t.Fatal("ungranted read succeeded")
	}
	operator := principal("data-protection-supervisor", &now)
	value, err := service.Move(context.Background(), operator, "request", TransitionInput{ExpectedVersion: 1, Status: "triaged", Priority: "routine", AssigneeID: "fulfiller", Reason: "Scope and identity were reviewed."}, "req-1")
	if err != nil || value.Version != 2 || len(log.events) != 1 {
		t.Fatalf("triage=%+v err=%v audit=%d", value, err, len(log.events))
	}
	store.value.Status = "awaiting-approval"
	store.value.Version = 4
	store.value.AssigneeID = "fulfiller"
	fulfiller := operator
	fulfiller.Actor.ID = "fulfiller"
	if _, err = service.Move(context.Background(), fulfiller, "request", TransitionInput{ExpectedVersion: 4, Status: "completed", Reason: "Approved fulfilment after review.", OutcomeCode: "fulfilled", MemberResponse: "Your request is complete."}, "req-2"); err == nil {
		t.Fatal("fulfiller approved own work")
	}
	stale := now.Add(-11 * time.Minute)
	approver := principal("data-protection-supervisor", &stale)
	if _, err = service.Move(context.Background(), approver, "request", TransitionInput{ExpectedVersion: 4, Status: "completed", Reason: "Approved fulfilment after review.", OutcomeCode: "fulfilled", MemberResponse: "Your request is complete."}, "req-3"); err == nil {
		t.Fatal("stale MFA approved final decision")
	}
	approver = principal("data-protection-supervisor", &now)
	approver.Actor.ID = "approver"
	if _, err = service.Move(context.Background(), approver, "request", TransitionInput{ExpectedVersion: 4, Status: "completed", Reason: "Approved fulfilment after review.", OutcomeCode: "fulfilled", MemberResponse: "Your request is complete."}, "req-4"); err != nil {
		t.Fatal(err)
	}
}
