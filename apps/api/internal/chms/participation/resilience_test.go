package participation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type loadPersonReferences struct{}

func (loadPersonReferences) ResolvePersonReference(_ context.Context, organizationID, personID platform.ID) (platform.ID, bool, error) {
	return "accra", organizationID == "org-1" && personID.Valid(), nil
}

func TestAttendanceConcurrencyAndBoundedLoadAgainstMongo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_participation_resilience_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, err := NewMongoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	occurrence := Occurrence{ResourceEnvelope: platform.ResourceEnvelope{ID: "load-occurrence", OrganizationID: "org-1", BranchID: "accra", SchemaVersion: 1, Version: 1, CreatedAt: now, UpdatedAt: now}, HomeBranchID: "accra", OccurrenceKey: "load-key", Name: "Load Test Gathering", StartsAt: now.Add(time.Hour), EndsAt: now.Add(3 * time.Hour), Timezone: "Africa/Accra", Status: "scheduled"}
	if err = repo.InsertOccurrence(ctx, occurrence); err != nil {
		t.Fatal(err)
	}
	evidence := &testPlatform{}
	service := Service{Store: repo, Platform: evidence, People: loadPersonReferences{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := participationPrincipal()

	var successes atomic.Int64
	var conflicts atomic.Int64
	var group sync.WaitGroup
	for index := 0; index < 32; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, recordErr := service.RecordAttendance(ctx, principal, occurrence.ID, "same-person", 0, AttendanceInput{Status: AttendancePresent, Source: "offline", Confidence: 100, SyncCommandID: fmt.Sprintf("race-%02d", index)}, fmt.Sprintf("race-request-%02d", index))
			if recordErr == nil {
				successes.Add(1)
			} else {
				conflicts.Add(1)
			}
		}(index)
	}
	group.Wait()
	if successes.Load() != 1 || conflicts.Load() != 31 {
		t.Fatalf("same-person race successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
	fact, err := repo.FindAttendance(ctx, "org-1", occurrence.ID, "same-person")
	if err != nil || fact == nil || fact.Version != 1 {
		t.Fatalf("race projection value=%+v err=%v", fact, err)
	}
	events, err := repo.ListAttendanceEvents(ctx, "org-1", occurrence.ID, "same-person", 100)
	if err != nil || len(events) != 1 {
		t.Fatalf("race evidence count=%d err=%v", len(events), err)
	}

	successes.Store(0)
	conflicts.Store(0)
	for index := 0; index < 100; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			personID := platform.ID(fmt.Sprintf("load-person-%03d", index))
			_, recordErr := service.RecordAttendance(ctx, principal, occurrence.ID, personID, 0, AttendanceInput{Status: AttendancePresent, Source: "kiosk", Confidence: 95}, fmt.Sprintf("load-request-%03d", index))
			if recordErr == nil {
				successes.Add(1)
			} else {
				conflicts.Add(1)
			}
		}(index)
	}
	group.Wait()
	if successes.Load() != 100 || conflicts.Load() != 0 {
		t.Fatalf("bounded load successes=%d errors=%d", successes.Load(), conflicts.Load())
	}
	all, err := repo.ListAttendance(ctx, "org-1", occurrence.ID)
	if err != nil || len(all) != 101 {
		t.Fatalf("bounded load projections=%d err=%v", len(all), err)
	}

	command := CheckinCommand{ClientCommandID: "expiry-command-01", LocalSequence: 1, Type: "check-in", OccurrenceID: occurrence.ID, PersonID: "expiry-person", CapturedAt: now.Add(time.Hour), Confidence: 100}
	for _, session := range []CheckinSession{
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "expired-session", OrganizationID: "org-1", BranchID: "accra", Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: principal.Actor}, DeviceLabel: "Expired tablet", OccurrenceIDs: []platform.ID{occurrence.ID}, State: "active", ExpiresAt: now.Add(-time.Minute)},
		{ResourceEnvelope: platform.ResourceEnvelope{ID: "locked-session", OrganizationID: "org-1", BranchID: "accra", Version: 2, CreatedAt: now, UpdatedAt: now, CreatedBy: principal.Actor}, DeviceLabel: "Locked tablet", OccurrenceIDs: []platform.ID{occurrence.ID}, State: "locked", ExpiresAt: now.Add(time.Hour)},
	} {
		if err = repo.InsertCheckinSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		_, syncErr := service.SyncCheckinCommands(ctx, principal, session.ID, CheckinSyncRequest{Commands: []CheckinCommand{command}}, "expiry-lock-proof")
		var domain *platform.DomainError
		if !errors.As(syncErr, &domain) || domain.Code != "checkin_session_locked" {
			t.Fatalf("session %s accepted command: %v", session.ID, syncErr)
		}
	}
	active := CheckinSession{ResourceEnvelope: platform.ResourceEnvelope{ID: "ordered-session", OrganizationID: "org-1", BranchID: "accra", Version: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: principal.Actor}, DeviceLabel: "Ordering tablet", OccurrenceIDs: []platform.ID{occurrence.ID}, State: "active", ExpiresAt: now.Add(time.Hour)}
	if err = repo.InsertCheckinSession(ctx, active); err != nil {
		t.Fatal(err)
	}
	reordered := []CheckinCommand{command, command}
	reordered[0].ClientCommandID, reordered[0].LocalSequence = "ordering-command-02", 2
	reordered[1].ClientCommandID, reordered[1].LocalSequence = "ordering-command-01", 1
	_, err = service.SyncCheckinCommands(ctx, principal, active.ID, CheckinSyncRequest{Commands: reordered}, "ordering-proof")
	if !errors.As(err, new(*platform.DomainError)) {
		t.Fatalf("reordered batch was accepted: %v", err)
	}
	stored, findErr := repo.FindCheckinCommandReceipt(ctx, "org-1", active.ID, "ordering-command-02")
	if findErr != nil || stored != nil {
		t.Fatalf("invalid batch partially persisted: value=%+v err=%v", stored, findErr)
	}
}
