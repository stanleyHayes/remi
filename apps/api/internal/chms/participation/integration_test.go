package participation

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type testPlatform struct {
	mu     sync.Mutex
	audits []platform.AuditEvent
	events []platform.OutboxRecord
}

type testPersonReferences struct{}

type failingPersonReferences struct{}

func (failingPersonReferences) ResolvePersonReference(context.Context, platform.ID, platform.ID) (platform.ID, bool, error) {
	return "", false, errors.New("temporary people dependency failure")
}

func (testPersonReferences) ResolvePersonReference(_ context.Context, organizationID, personID platform.ID) (platform.ID, bool, error) {
	if organizationID == "org-1" && (personID == "person-1" || personID == "child-1" || personID == "guardian-1" || personID == "guardian-2") {
		return "accra", true, nil
	}
	return "", false, nil
}

func (p *testPlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (p *testPlatform) AppendAudit(_ context.Context, event platform.AuditEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.audits = append(p.audits, event)
	return nil
}
func (p *testPlatform) EnqueueEvent(_ context.Context, event platform.OutboxRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

func TestServiceDefinitionAndOccurrenceLifecycleAgainstMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_participation_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := NewMongoRepository(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	evidence := &testPlatform{}
	pickupSecret := sha256.Sum256([]byte("participation-test-pickup-secret"))
	pickupCodes, _ := NewPickupCodeManager(pickupSecret[:])
	now := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	service := Service{Store: repo, Platform: evidence, People: testPersonReferences{}, PickupCodes: pickupCodes, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := participationPrincipal()
	definition, err := service.CreateDefinition(ctx, principal, DefinitionInput{HomeBranchID: "accra", Name: "Sunday Celebration", Timezone: "Africa/Accra", DefaultDurationMinutes: 120, DefaultRoomIDs: []platform.ID{"main-hall"}, DefaultCapacity: 600, Recurrence: &RecurrencePattern{Frequency: "weekly", DaysOfWeek: []string{"sunday"}, LocalStart: "09:00", StartsOn: "2026-08-16"}}, "request-definition")
	if err != nil {
		t.Fatal(err)
	}
	updatedDefinition, err := service.UpdateDefinition(ctx, principal, definition.ID, 1, DefinitionInput{HomeBranchID: "accra", Name: "Sunday Celebration", Timezone: "Africa/Accra", DefaultDurationMinutes: 150, DefaultRoomIDs: []platform.ID{"main-hall"}, DefaultCapacity: 650, Recurrence: definition.Recurrence, Status: "active"}, "request-definition-update")
	if err != nil {
		t.Fatal(err)
	}
	if updatedDefinition.Version != 2 || updatedDefinition.DefaultDurationMinutes != 150 {
		t.Fatalf("bad definition update: %+v", updatedDefinition)
	}
	start := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	occurrence, err := service.CreateOccurrence(ctx, principal, OccurrenceInput{HomeBranchID: "accra", ServiceDefinitionID: definition.ID, Name: "Sunday Celebration", StartsAt: start, EndsAt: start.Add(2 * time.Hour), Timezone: "Africa/Accra", RoomIDs: []platform.ID{"main-hall"}, Capacity: 600}, "request-occurrence")
	if err != nil {
		t.Fatal(err)
	}
	originalKey := occurrence.OccurrenceKey
	changed, err := service.UpdateOccurrence(ctx, principal, occurrence.ID, 1, OccurrenceInput{HomeBranchID: "accra", ServiceDefinitionID: definition.ID, Name: "Sunday Celebration — Evening", StartsAt: start.Add(8 * time.Hour), EndsAt: start.Add(10 * time.Hour), Timezone: "Africa/Accra", RoomIDs: []platform.ID{"main-hall"}, Capacity: 500}, "request-occurrence-update")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version != 2 || changed.OccurrenceKey != originalKey {
		t.Fatalf("occurrence identity changed: %+v", changed)
	}
	attendance, err := service.RecordAttendance(ctx, principal, occurrence.ID, "person-1", 0, AttendanceInput{Status: "present", Source: "roster", Confidence: 90, Guest: true}, "request-attendance")
	if err != nil {
		t.Fatal(err)
	}
	if attendance.Version != 1 || !attendance.Guest || attendance.OperatorID != principal.Actor.ID {
		t.Fatalf("bad attendance: %+v", attendance)
	}
	attendance, err = service.RecordAttendance(ctx, principal, occurrence.ID, "person-1", 1, AttendanceInput{Status: "present", Source: "operator", Confidence: 100}, "request-attendance-update")
	if err != nil || attendance.Version != 2 {
		t.Fatalf("attendance update: value=%+v err=%v", attendance, err)
	}
	now = changed.EndsAt.Add(time.Hour)
	if _, err = service.RecordAttendance(ctx, principal, occurrence.ID, "person-1", 2, AttendanceInput{Status: "excused", Source: "operator", Confidence: 100}, "request-late-without-reason"); err == nil {
		t.Fatal("late attendance correction did not require a reason")
	}
	attendance, err = service.RecordAttendance(ctx, principal, occurrence.ID, "person-1", 2, AttendanceInput{Status: "excused", Source: "operator", Confidence: 100, Reason: "Pastoral correction after roster review"}, "request-late-correction")
	if err != nil || attendance.Version != 3 || attendance.Status != AttendanceExcused {
		t.Fatalf("late correction: value=%+v err=%v", attendance, err)
	}
	events, err := service.ListAttendanceEvents(ctx, principal, occurrence.ID, "person-1")
	if err != nil || len(events) != 3 || events[2].Type != "corrected" || events[2].Before == nil {
		t.Fatalf("attendance events: values=%+v err=%v", events, err)
	}
	headcount, err := service.CreateHeadcount(ctx, principal, occurrence.ID, HeadcountInput{Category: "auditorium", Count: 420, Source: "operator", Confidence: 85, ObservedAt: changed.StartsAt.Add(time.Hour)}, "request-headcount")
	if err != nil {
		t.Fatal(err)
	}
	headcount, err = service.UpdateHeadcount(ctx, principal, occurrence.ID, headcount.ID, 1, HeadcountInput{Category: "auditorium", Count: 427, Source: "operator", Confidence: 95, ObservedAt: changed.StartsAt.Add(time.Hour), Reason: "Reconciled both seating sections"}, "request-headcount-correction")
	if err != nil || headcount.Version != 2 || headcount.Count != 427 {
		t.Fatalf("headcount correction: value=%+v err=%v", headcount, err)
	}
	lock, err := service.LockAttendance(ctx, principal, occurrence.ID, "Service attendance reviewed and approved", "request-lock")
	if err != nil || lock.OccurrenceID != occurrence.ID {
		t.Fatalf("attendance lock: value=%+v err=%v", lock, err)
	}
	if _, err = service.RecordAttendance(ctx, principal, occurrence.ID, "person-1", 3, AttendanceInput{Status: "present", Source: "operator", Confidence: 100, Reason: "ordinary write must not bypass lock"}, "request-locked-write"); err == nil {
		t.Fatal("ordinary attendance write bypassed lock")
	}
	attendance, err = service.CorrectAttendance(ctx, principal, occurrence.ID, "person-1", 3, AttendanceInput{Status: "present", Source: "operator", Confidence: 100, Reason: "Approved correction after reconciliation"}, "request-approved-correction")
	if err != nil || attendance.Version != 4 || attendance.Status != AttendancePresent {
		t.Fatalf("approved correction: value=%+v err=%v", attendance, err)
	}
	period, err := service.CloseAttendancePeriod(ctx, principal, PeriodCloseInput{BranchID: "accra", StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EndsAt: now, Reason: "August attendance period reconciled"}, "request-period-close")
	if err != nil || period.BranchID != "accra" {
		t.Fatalf("period close: value=%+v err=%v", period, err)
	}
	periods, err := service.ListAttendancePeriodCloses(ctx, principal, "accra")
	if err != nil || len(periods) != 1 {
		t.Fatalf("period closes: values=%+v err=%v", periods, err)
	}
	checkinOccurrence, err := service.CreateOccurrence(ctx, principal, OccurrenceInput{HomeBranchID: "accra", ServiceDefinitionID: definition.ID, Name: "Evening Check-in Test", StartsAt: now.Add(time.Hour), EndsAt: now.Add(3 * time.Hour), Timezone: "Africa/Accra", RoomIDs: []platform.ID{"main-hall"}, Capacity: 200}, "request-checkin-occurrence")
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.CreateCheckinSession(ctx, principal, CheckinSessionInput{BranchID: "accra", DeviceLabel: "Main foyer tablet", OccurrenceIDs: []platform.ID{checkinOccurrence.ID}, ExpiresAt: now.Add(6 * time.Hour)}, "request-checkin-session")
	if err != nil {
		t.Fatal(err)
	}
	checkin := CheckinCommand{ClientCommandID: "device-a-command-0001", LocalSequence: 1, Type: "check-in", OccurrenceID: checkinOccurrence.ID, PersonID: "person-1", CapturedAt: now.Add(time.Hour), Guest: true, Confidence: 95}
	service.People = failingPersonReferences{}
	sync, err := service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{checkin}}, "request-checkin-sync")
	if err != nil || len(sync.Results) != 1 || sync.Results[0].Classification != "retry" || sync.Results[0].Code != "dependency_error" {
		t.Fatalf("transient check-in failure: response=%+v err=%v", sync, err)
	}
	checkpoint, err := repo.FindCheckinSession(ctx, principal.OrganizationID, session.ID)
	if err != nil || checkpoint.LastSequence != 0 {
		t.Fatalf("retry advanced station checkpoint: value=%+v err=%v", checkpoint, err)
	}
	service.People = testPersonReferences{}
	sync, err = service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{checkin}}, "request-checkin-recovered")
	if err != nil || len(sync.Results) != 1 || sync.Results[0].Classification != "applied" || sync.Results[0].Version != 1 {
		t.Fatalf("recovered check-in sync: response=%+v err=%v", sync, err)
	}
	replay, err := service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{checkin}}, "request-checkin-replay")
	if err != nil || replay.Results[0].Classification != "duplicate" || replay.Results[0].AttendanceID != sync.Results[0].AttendanceID {
		t.Fatalf("check-in replay: response=%+v err=%v", replay, err)
	}
	checkout := CheckinCommand{ClientCommandID: "device-a-command-0002", LocalSequence: 2, Type: "check-out", OccurrenceID: checkinOccurrence.ID, PersonID: "person-1", ExpectedVersion: 1, CapturedAt: now.Add(2 * time.Hour), Guest: true, Confidence: 100}
	sync, err = service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{checkout}}, "request-checkout-sync")
	if err != nil || sync.Results[0].Classification != "applied" || sync.Results[0].Version != 2 {
		t.Fatalf("check-out sync: response=%+v err=%v", sync, err)
	}
	conflict := CheckinCommand{ClientCommandID: "device-a-command-0003", LocalSequence: 3, Type: "mark", OccurrenceID: checkinOccurrence.ID, PersonID: "person-1", ExpectedVersion: 1, Status: "absent", CapturedAt: now.Add(2 * time.Hour), Confidence: 100}
	sync, err = service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{conflict}}, "request-checkin-conflict")
	if err != nil || sync.Results[0].Classification != "conflict" || sync.Results[0].Code != "version_conflict" {
		t.Fatalf("check-in conflict: response=%+v err=%v", sync, err)
	}
	authorization, err := service.CreateGuardianAuthorization(ctx, principal, GuardianAuthorizationInput{BranchID: "accra", ChildPersonID: "child-1", GuardianPersonID: "guardian-1", Relationship: "parent", ValidFrom: now, Source: "household-review"}, "request-guardian-authorization")
	if err != nil || authorization.Status != "active" {
		t.Fatalf("guardian authorization: value=%+v err=%v", authorization, err)
	}
	childCheckin, label, err := service.CheckinChild(ctx, principal, session.ID, ChildCheckinInput{OccurrenceID: checkinOccurrence.ID, ChildPersonID: "child-1", GuardianPersonID: "guardian-1", CapturedAt: now.Add(time.Hour)}, "request-child-checkin")
	if err != nil || len(label.SecurityCode) != 6 || childCheckin.CodeHash == label.SecurityCode {
		t.Fatalf("child check-in: value=%+v label=%+v err=%v", childCheckin, label, err)
	}
	storedCheckin, err := repo.FindChildCheckin(ctx, principal.OrganizationID, childCheckin.ID)
	if err != nil || storedCheckin.CodeHash == "" || storedCheckin.CodeHash == label.SecurityCode {
		t.Fatalf("plaintext pickup code was retained: value=%+v err=%v", storedCheckin, err)
	}
	if _, err = service.PickupChild(ctx, principal, label.AttendanceID, PickupInput{GuardianPersonID: "guardian-2", SecurityCode: label.SecurityCode}, "request-unauthorized-pickup"); err == nil {
		t.Fatal("unauthorized guardian pickup succeeded")
	}
	if _, err = service.PickupChild(ctx, principal, label.AttendanceID, PickupInput{GuardianPersonID: "guardian-1", SecurityCode: "AAAAAA"}, "request-guessed-code"); err == nil {
		t.Fatal("guessed pickup code succeeded")
	}
	receipt, err := service.PickupChild(ctx, principal, label.AttendanceID, PickupInput{GuardianPersonID: "guardian-1", SecurityCode: label.SecurityCode}, "request-authorized-pickup")
	if err != nil || receipt.ChildCheckinID != childCheckin.ID {
		t.Fatalf("authorized pickup: value=%+v err=%v", receipt, err)
	}
	if _, err = service.PickupChild(ctx, principal, label.AttendanceID, PickupInput{GuardianPersonID: "guardian-1", SecurityCode: label.SecurityCode}, "request-replayed-pickup"); err == nil {
		t.Fatal("replayed pickup succeeded")
	}
	incident, err := service.CreateSafeguardingIncident(ctx, principal, childCheckin.ID, "pickup concern", "Guardian reported a misplaced label before identity verification.", "request-safeguarding-incident")
	if err != nil || incident.Status != "open" {
		t.Fatalf("safeguarding incident: value=%+v err=%v", incident, err)
	}
	lockedSession, err := service.LockCheckinSession(ctx, principal, session.ID, "Foyer station handed back", "request-lock-checkin")
	if err != nil || lockedSession.State != "locked" {
		t.Fatalf("lock check-in session: value=%+v err=%v", lockedSession, err)
	}
	if _, err = service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{{ClientCommandID: "device-a-command-0004", LocalSequence: 4, Type: "mark", OccurrenceID: checkinOccurrence.ID, PersonID: "person-1", ExpectedVersion: 2, Status: "present", CapturedAt: now.Add(2 * time.Hour), Confidence: 100}}}, "request-locked-session"); err == nil {
		t.Fatal("locked check-in session accepted commands")
	}
	cancelled, err := service.CancelOccurrence(ctx, principal, occurrence.ID, 2, "Severe weather advisory", "request-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || cancelled.Version != 3 || cancelled.CancelledAt == nil {
		t.Fatalf("bad cancellation: %+v", cancelled)
	}
	if len(evidence.audits) != 26 || len(evidence.events) != 23 {
		t.Fatalf("missing lifecycle evidence audits=%d events=%d", len(evidence.audits), len(evidence.events))
	}
}
func participationPrincipal() platform.Principal {
	actions := []string{"create", "read", "update", "cancel", "record", "approve", "operate", "pickup"}
	grants := make([]platform.Grant, 0, len(actions))
	for _, action := range actions {
		grants = append(grants, platform.Grant{Action: action, Resource: "*", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldChildSafeguarding}})
	}
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "staff-1"}, OrganizationID: "org-1", Roles: []string{"editor"}, Grants: grants}
}
