package care

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/testsupport"
)

type assimilationAttendance map[platform.ID]struct {
	Branch platform.ID
	Status string
	At     time.Time
}

func (a assimilationAttendance) ResolveAttendanceEvidence(_ context.Context, _ platform.ID, occurrenceID, _ platform.ID) (platform.ID, string, time.Time, error) {
	value := a[occurrenceID]
	return value.Branch, value.Status, value.At, nil
}
func assimilationDefinitionInput() DefinitionInput {
	return DefinitionInput{BranchID: "accra", Name: "Visitor pathway", Purpose: "visitor-assimilation", InitialStageKey: "first-visit", Stages: []Stage{{Key: "first-visit", Name: "First visit", SLAMinutes: 1440}, {Key: "second-visit", Name: "Second visit", SLAMinutes: 2880}, {Key: "membership-class", Name: "Membership class"}, {Key: "baptism", Name: "Baptism"}, {Key: "completed", Name: "Completed", Terminal: true, OutcomeRequired: true}}, Transitions: []Transition{{From: "first-visit", To: "second-visit", Name: "Returned"}, {From: "second-visit", To: "membership-class", Name: "Start membership class"}, {From: "membership-class", To: "baptism", Name: "Record baptism pathway"}, {From: "membership-class", To: "completed", Name: "Close pathway"}, {From: "baptism", To: "completed", Name: "Close pathway"}}, Tasks: []TaskTemplate{{Key: "welcome-call", StageKey: "first-visit", Title: "Make welcome call", DueMinutes: 720}, {Key: "return-follow-up", StageKey: "second-visit", Title: "Offer a next step", DueMinutes: 1440}}}
}

func workflowPrincipal() platform.Principal {
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: "operator-1"}, OrganizationID: "org-1", Grants: []platform.Grant{{Action: "*", Resource: "workflow", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}}}
}
func carePrincipal(id platform.ID) platform.Principal {
	return platform.Principal{Actor: platform.Actor{Type: platform.ActorStaff, ID: id}, OrganizationID: "org-1", Grants: []platform.Grant{{Action: "*", Resource: "care-case", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldSensitiveMinistry}}, {Action: "*", Resource: "workflow", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational}}}}
}

func TestWorkflowLifecycleIsPinnedAuditedAndReplaySafe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_care_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	uow, _ := platform.NewMongoPlatformStore(db)
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service := Service{Store: repo, Platform: uow, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := workflowPrincipal()
	input := validDefinitionInput()
	definition, err := service.CreateDefinition(ctx, principal, input, "definition-create")
	if err != nil || definition.Status != "draft" {
		t.Fatalf("definition=%+v err=%v", definition, err)
	}
	input.Description = "A versioned welcome pathway"
	definition, err = service.UpdateDefinition(ctx, principal, definition.ID, 1, input, "definition-update")
	if err != nil || definition.Version != 2 {
		t.Fatalf("update=%+v err=%v", definition, err)
	}
	definition, err = service.PublishDefinition(ctx, principal, definition.ID, 2, "definition-publish")
	if err != nil || definition.Status != "published" || definition.Version != 3 {
		t.Fatalf("publish=%+v err=%v", definition, err)
	}
	if _, err = service.UpdateDefinition(ctx, principal, definition.ID, 3, input, "published-edit"); err == nil {
		t.Fatal("published definition remained editable")
	}
	revisionInput := validDefinitionInput()
	revisionInput.SupersedesDefinitionID = definition.ID
	revision, revisionErr := service.CreateDefinition(ctx, principal, revisionInput, "definition-revision")
	if revisionErr != nil || revision.DefinitionVersion != 2 || revision.SupersedesDefinitionID != definition.ID {
		t.Fatalf("revision=%+v err=%v", revision, revisionErr)
	}
	instance, err := service.Start(ctx, principal, StartInput{DefinitionID: definition.ID, SubjectType: "person", SubjectID: "person-1", OwnerID: "pastor-1"}, "instance-start")
	if err != nil || instance.StageKey != "new" || instance.DefinitionVersion != 1 || instance.DueAt == nil {
		t.Fatalf("instance=%+v err=%v", instance, err)
	}
	_, tasks, err := service.GetInstance(ctx, principal, instance.ID)
	if err != nil || len(tasks) != 1 || tasks[0].AssigneeID != "pastor-1" {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	transition := TransitionInput{ExpectedVersion: 1, ToStageKey: "contacted", AutomationKey: "attendance-first-visit/person-1"}
	changed, replayed, err := service.Transition(ctx, principal, instance.ID, transition, "transition")
	if err != nil || replayed || changed.Version != 2 || changed.StageKey != "contacted" {
		t.Fatalf("changed=%+v replayed=%v err=%v", changed, replayed, err)
	}
	replayedValue, replayed, err := service.Transition(ctx, principal, instance.ID, transition, "transition-replay")
	if err != nil || !replayed || replayedValue.Version != 2 {
		t.Fatalf("replay=%+v replayed=%v err=%v", replayedValue, replayed, err)
	}
	transition.ToStageKey = "closed"
	if _, _, err = service.Transition(ctx, principal, instance.ID, transition, "changed-replay"); err == nil {
		t.Fatal("changed automation payload reused the same key")
	}
	closed, _, err := service.Transition(ctx, principal, instance.ID, TransitionInput{ExpectedVersion: 2, ToStageKey: "closed"}, "missing-outcome")
	if err == nil || closed != nil {
		t.Fatal("terminal stage accepted without outcome")
	}
	closed, _, err = service.Transition(ctx, principal, instance.ID, TransitionInput{ExpectedVersion: 2, ToStageKey: "closed", OutcomeCode: "connected", OutcomeSummary: "Joined a group"}, "close")
	if err != nil || closed.State != "completed" || closed.CompletedAt == nil {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	if len(tasks) > 0 {
		completed, completeErr := service.CompleteTask(ctx, principal, tasks[0].ID, 1, "Welcome call completed", "task-complete")
		if completeErr != nil || completed.Status != "completed" {
			t.Fatalf("task=%+v err=%v", completed, completeErr)
		}
	}
	if audits, _ := db.Collection("chms_audit_events").CountDocuments(ctx, map[string]any{"organizationId": "org-1"}); audits < 6 {
		t.Fatalf("audits=%d want >=6", audits)
	}
}

func TestAutomationRaceAppliesTransitionExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_care_race_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := NewMongoRepository(db)
	_ = repo.EnsureIndexes(ctx)
	uow, _ := platform.NewMongoPlatformStore(db)
	service := Service{Store: repo, Platform: uow, Authorizer: platform.GrantAuthorizer{}}
	p := workflowPrincipal()
	d, _ := service.CreateDefinition(ctx, p, validDefinitionInput(), "create")
	d, _ = service.PublishDefinition(ctx, p, d.ID, 1, "publish")
	instance, _ := service.Start(ctx, p, StartInput{DefinitionID: d.ID, SubjectType: "person", SubjectID: "person-race", OwnerID: "pastor-1"}, "start")
	var wg sync.WaitGroup
	wg.Add(12)
	var mu sync.Mutex
	successes, replays := 0, 0
	for range 12 {
		go func() {
			defer wg.Done()
			_, replayed, transitionErr := service.Transition(context.Background(), p, instance.ID, TransitionInput{ExpectedVersion: 1, ToStageKey: "contacted", AutomationKey: "race-key"}, "race")
			mu.Lock()
			defer mu.Unlock()
			if transitionErr == nil {
				if replayed {
					replays++
				} else {
					successes++
				}
			}
		}()
	}
	wg.Wait()
	stored, _ := repo.FindInstance(ctx, "org-1", instance.ID)
	if stored.Version != 2 || successes != 1 || successes+replays != 12 {
		t.Fatalf("stored=%+v successes=%d replays=%d", stored, successes, replays)
	}
}

func TestAssimilationUsesVerifiedAttendanceAndNeverChangesMembership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_assimilation_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	uow, _ := platform.NewMongoPlatformStore(db)
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	attendance := assimilationAttendance{"visit-1": {Branch: "accra", Status: "present", At: now}, "visit-2": {Branch: "accra", Status: "present", At: now.Add(7 * 24 * time.Hour)}, "absent": {Branch: "accra", Status: "absent", At: now}}
	service := Service{Store: repo, Platform: uow, Attendance: attendance, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := workflowPrincipal()
	definition, err := service.CreateDefinition(ctx, principal, assimilationDefinitionInput(), "definition")
	if err != nil {
		t.Fatal(err)
	}
	definition, err = service.PublishDefinition(ctx, principal, definition.ID, 1, "publish")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Collection("chms_people").InsertOne(ctx, map[string]any{"_id": "visitor-1", "organizationId": "org-1", "membershipStage": "guest", "version": 1})
	if _, _, err = service.RecordAssimilationVisit(ctx, principal, AssimilationInput{DefinitionID: definition.ID, OccurrenceID: "absent", PersonID: "visitor-1", OwnerID: "pastor-1"}, "absent"); err == nil {
		t.Fatal("absent attendance started assimilation")
	}
	progress, replayed, err := service.RecordAssimilationVisit(ctx, principal, AssimilationInput{DefinitionID: definition.ID, OccurrenceID: "visit-1", PersonID: "visitor-1", OwnerID: "pastor-1"}, "first")
	if err != nil || replayed || progress.VisitCount != 1 {
		t.Fatalf("first progress=%+v replayed=%v err=%v", progress, replayed, err)
	}
	replayedProgress, replayed, err := service.RecordAssimilationVisit(ctx, principal, AssimilationInput{DefinitionID: definition.ID, OccurrenceID: "visit-1", PersonID: "visitor-1", OwnerID: "pastor-1"}, "first-replay")
	if err != nil || !replayed || replayedProgress.Version != 1 {
		t.Fatalf("replay=%+v replayed=%v err=%v", replayedProgress, replayed, err)
	}
	progress, replayed, err = service.RecordAssimilationVisit(ctx, principal, AssimilationInput{DefinitionID: definition.ID, OccurrenceID: "visit-2", PersonID: "visitor-1", OwnerID: "pastor-1"}, "second")
	if err != nil || replayed || progress.VisitCount != 2 {
		t.Fatalf("second=%+v replayed=%v err=%v", progress, replayed, err)
	}
	instance, tasks, err := service.GetInstance(ctx, principal, progress.WorkflowInstanceID)
	if err != nil || instance.StageKey != "second-visit" || len(tasks) != 2 {
		t.Fatalf("instance=%+v tasks=%+v err=%v", instance, tasks, err)
	}
	var person map[string]any
	if err = db.Collection("chms_people").FindOne(ctx, map[string]any{"_id": "visitor-1"}).Decode(&person); err != nil {
		t.Fatal(err)
	}
	if person["membershipStage"] != "guest" {
		t.Fatalf("assimilation changed membership stage: %v", person)
	}
	var wait sync.WaitGroup
	wait.Add(10)
	for range 10 {
		go func() {
			defer wait.Done()
			_, _, _ = service.RecordAssimilationVisit(context.Background(), principal, AssimilationInput{DefinitionID: definition.ID, OccurrenceID: "visit-1", PersonID: "visitor-race", OwnerID: "pastor-1"}, "race")
		}()
	}
	wait.Wait()
	progressCount, _ := db.Collection(assimilationCollection).CountDocuments(ctx, map[string]any{"personId": "visitor-race"})
	instanceCount, _ := db.Collection(instancesCollection).CountDocuments(ctx, map[string]any{"subjectId": "visitor-race"})
	if progressCount != 1 || instanceCount != 1 {
		t.Fatalf("race created progress=%d instances=%d", progressCount, instanceCount)
	}
}

func TestRestrictedCareCaseRevokesAccessAndNeverLeaksNoteContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_care_case_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repo.EnsureCaseIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	uow, _ := platform.NewMongoPlatformStore(db)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cipher, _ := platform.NewEnvelopeCipher("test-care-v1", key)
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service := Service{Store: repo, Cases: repo, Platform: uow, Cipher: cipher, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	owner := carePrincipal("pastor-1")
	other := carePrincipal("pastor-2")
	definition, err := service.CreateDefinition(ctx, owner, DefinitionInput{BranchID: "accra", Name: "Pastoral care plan", Purpose: "pastoral-care", InitialStageKey: "open", Stages: []Stage{{Key: "open", Name: "Open", SLAMinutes: 1440}, {Key: "closed", Name: "Closed", Terminal: true, OutcomeRequired: true}}, Transitions: []Transition{{From: "open", To: "closed", Name: "Close care"}}, Tasks: []TaskTemplate{{Key: "initial-contact", StageKey: "open", Title: "Make initial contact", DueMinutes: 120}}}, "care-definition")
	if err != nil {
		t.Fatal(err)
	}
	definition, err = service.PublishDefinition(ctx, owner, definition.ID, 1, "care-definition-publish")
	if err != nil {
		t.Fatal(err)
	}
	input := CareCaseInput{BranchID: "accra", MinistryID: "pastoral-care", PersonID: "person-care", Category: "pastoral-care", Urgency: "priority", Consent: ConsentEvidence{State: "granted", Source: "member-request", NoticeVersion: "care-2026-01", CapturedAt: now}, AssignedUserIDs: []platform.ID{"pastor-1"}, WorkflowDefinitionID: definition.ID, DueAt: timePointer(now.Add(24 * time.Hour))}
	created, err := service.CreateCareCase(ctx, owner, input, "case-create")
	if err != nil || created.Version != 1 {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	workflow, tasks, err := service.GetInstance(ctx, owner, created.WorkflowInstanceID)
	if err != nil || workflow.OwnerID != "pastor-1" || len(tasks) != 1 || tasks[0].AssigneeID != "pastor-1" {
		t.Fatalf("workflow=%+v tasks=%+v err=%v", workflow, tasks, err)
	}
	if _, _, err = service.GetCareCase(ctx, other, created.ID); err == nil {
		t.Fatal("unassigned care user read the case")
	}
	secret := "A private pastoral conversation that must never enter lists or audits."
	note, err := service.AddRestrictedNote(ctx, owner, created.ID, secret, "pastoral-restricted", "assigned-care-team", "note-create")
	if err != nil || note.Content != secret {
		t.Fatalf("note=%+v err=%v", note, err)
	}
	var stored map[string]any
	if err = db.Collection(restrictedNotesCollection).FindOne(ctx, map[string]any{"_id": note.ID}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	raw, _ := bson.MarshalExtJSON(stored, false, false)
	if strings.Contains(string(raw), secret) {
		t.Fatal("restricted note plaintext was stored")
	}
	cases, err := service.ListCareCases(ctx, owner, "accra", false)
	if err != nil || len(cases) != 1 {
		t.Fatalf("cases=%+v err=%v", cases, err)
	}
	listedRaw, _ := bson.MarshalExtJSON(cases, false, false)
	if strings.Contains(string(listedRaw), secret) {
		t.Fatal("restricted note leaked into case list")
	}
	views, err := service.ListRestrictedNotes(ctx, owner, created.ID, "note-read")
	if err != nil || len(views) != 1 || views[0].Content != secret {
		t.Fatalf("views=%+v err=%v", views, err)
	}
	contact, err := service.AddContactEvent(ctx, owner, created.ID, "phone", "Pastoral follow-up", "reached", "Agreed a follow-up meeting.", "contact")
	if err != nil || contact.Outcome != "reached" {
		t.Fatalf("contact=%+v err=%v", contact, err)
	}
	reassigned, err := service.AssignCareCase(ctx, owner, created.ID, CareAssignmentInput{ExpectedVersion: 1, AssignedUserIDs: []platform.ID{"pastor-2"}, Reason: "Transferred to the assigned branch pastor"}, "assign")
	if err != nil || reassigned.Version != 2 {
		t.Fatalf("reassigned=%+v err=%v", reassigned, err)
	}
	if _, _, err = service.GetCareCase(ctx, owner, created.ID); err == nil {
		t.Fatal("former assignee retained case access")
	}
	if _, _, err = service.GetCareCase(ctx, other, created.ID); err != nil {
		t.Fatalf("new assignee could not read: %v", err)
	}
	workflow, tasks, err = service.GetInstance(ctx, other, created.WorkflowInstanceID)
	if err != nil || workflow.OwnerID != "pastor-2" || len(tasks) != 1 || tasks[0].AssigneeID != "pastor-2" {
		t.Fatalf("reassigned workflow=%+v tasks=%+v err=%v", workflow, tasks, err)
	}
	if _, err = service.ListRestrictedNotes(ctx, owner, created.ID, "old-read"); err == nil {
		t.Fatal("former assignee retained note access")
	}
	if _, err = service.CloseCareCase(ctx, other, created.ID, CareCloseInput{ExpectedVersion: 1, Outcome: "care-completed", Reason: "Stale version should fail"}, "stale"); err == nil {
		t.Fatal("stale closure succeeded")
	}
	closed, err := service.CloseCareCase(ctx, other, created.ID, CareCloseInput{ExpectedVersion: 2, Outcome: "care-completed", Reason: "Care plan completed with consent"}, "close")
	if err != nil || closed.State != "closed" {
		t.Fatalf("closed=%+v err=%v", closed, err)
	}
	workflow, tasks, err = service.GetInstance(ctx, other, created.WorkflowInstanceID)
	if err != nil || workflow.State != "completed" || workflow.OutcomeCode != "care-completed" || len(tasks) != 1 || tasks[0].Status != "cancelled" {
		t.Fatalf("closed workflow=%+v tasks=%+v err=%v", workflow, tasks, err)
	}
	auditsCursor, _ := db.Collection("chms_audit_events").Find(ctx, map[string]any{})
	defer auditsCursor.Close(ctx)
	var audits []map[string]any
	_ = auditsCursor.All(ctx, &audits)
	auditRaw, _ := bson.MarshalExtJSON(audits, false, false)
	if strings.Contains(string(auditRaw), secret) {
		t.Fatal("restricted note leaked into audit")
	}
	readCount, _ := db.Collection("chms_audit_events").CountDocuments(ctx, map[string]any{"action": "care.restricted-note.read", "resourceId": created.ID})
	if readCount != 1 {
		t.Fatalf("note read audits=%d want 1", readCount)
	}
}
func timePointer(value time.Time) *time.Time { return &value }
