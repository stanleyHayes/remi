package people

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func operationsPrincipal(actorID platform.ID, branches ...platform.ID) platform.Principal {
	return platform.Principal{
		Actor:          platform.Actor{Type: platform.ActorStaff, ID: actorID},
		OrganizationID: "org",
		Grants: []platform.Grant{
			{Action: "*", Resource: "people-segment", BranchIDs: branches, FieldClasses: []platform.FieldClass{platform.FieldPersonal}},
			{Action: "bulk-update", Resource: "person", BranchIDs: branches, FieldClasses: []platform.FieldClass{platform.FieldPersonal}},
			{Action: "*", Resource: "person-merge", BranchIDs: branches, FieldClasses: []platform.FieldClass{platform.FieldPersonal}},
		},
	}
}

type fakeSegmentStore struct{ segments map[platform.ID]SavedSegment }

func (f *fakeSegmentStore) InsertSegment(_ context.Context, segment SavedSegment) error {
	if f.segments == nil {
		f.segments = map[platform.ID]SavedSegment{}
	}
	f.segments[segment.ID] = segment
	return nil
}
func (f *fakeSegmentStore) FindSegment(_ context.Context, _ platform.ID, id platform.ID) (*SavedSegment, error) {
	segment, ok := f.segments[id]
	if !ok {
		return nil, nil
	}
	return &segment, nil
}
func (f *fakeSegmentStore) ListSegments(_ context.Context, _ platform.ID, branchID, actorID platform.ID) ([]SavedSegment, error) {
	items := []SavedSegment{}
	for _, segment := range f.segments {
		if segment.BranchID == branchID && segment.ArchivedAt == nil && (segment.Visibility == "organization" || segment.OwnerID == actorID) {
			items = append(items, segment)
		}
	}
	return items, nil
}
func (f *fakeSegmentStore) ReplaceSegment(_ context.Context, segment SavedSegment, expected int64) error {
	current := f.segments[segment.ID]
	if current.Version != expected {
		return platform.VersionConflict(current.Version)
	}
	f.segments[segment.ID] = segment
	return nil
}
func (f *fakeSegmentStore) ArchiveSegment(_ context.Context, _ platform.ID, id platform.ID, expected int64, now time.Time, actor platform.Actor) error {
	segment := f.segments[id]
	if segment.Version != expected {
		return platform.VersionConflict(segment.Version)
	}
	segment.ArchivedAt, segment.ArchivedBy, segment.Version = &now, &actor, expected+1
	f.segments[id] = segment
	return nil
}

func TestSavedSegmentsAreNormalizedScopedOwnedAndVersioned(t *testing.T) {
	store := &fakeSegmentStore{}
	platformStore := &fakePlatform{}
	now := time.Date(2026, 8, 11, 18, 0, 0, 0, time.UTC)
	service := SegmentService{Store: store, Platform: platformStore, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	owner := operationsPrincipal("owner", "branch-1")
	segment, err := service.Create(context.Background(), owner, SaveSegmentInput{Name: "  Choir Members  ", Filter: PersonSearchFilter{BranchID: "branch-1", MembershipStages: []string{"MEMBER"}, Tags: []string{"Choir", "choir"}}, Visibility: "organization"}, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if segment.Name != "Choir Members" || len(segment.Filter.Tags) != 1 || segment.Filter.MembershipStages[0] != "member" || len(platformStore.audits) != 1 || len(platformStore.events) != 1 {
		t.Fatalf("bad saved segment: %+v", segment)
	}
	segment, err = service.Update(context.Background(), owner, segment.ID, 1, SaveSegmentInput{Name: "Choir team", Filter: PersonSearchFilter{BranchID: "branch-1"}, Visibility: "private"}, "req-2")
	if err != nil || segment.Version != 2 {
		t.Fatalf("update failed: segment=%+v err=%v", segment, err)
	}
	if _, err := service.Update(context.Background(), operationsPrincipal("other", "branch-1"), segment.ID, 2, SaveSegmentInput{Name: "Hijack", Filter: PersonSearchFilter{BranchID: "branch-1"}}, "req-3"); err == nil {
		t.Fatal("non-owner updated private segment")
	}
	if _, err := service.Create(context.Background(), owner, SaveSegmentInput{Name: "Wrong branch", Filter: PersonSearchFilter{BranchID: "branch-2"}}, "req-4"); err == nil {
		t.Fatal("cross-branch segment was created")
	}
}

type fakeBulkStore struct {
	targets      []BulkTarget
	previews     map[platform.ID]BulkPreview
	jobs         map[platform.ID]BulkJob
	results      []BulkResult
	versions     map[platform.ID]int64
	undoConflict map[platform.ID]bool
	undoStates   map[platform.ID]string
}

func (f *fakeBulkStore) FindBulkTargets(context.Context, platform.ID, PersonSearchFilter, int) ([]BulkTarget, error) {
	return append([]BulkTarget(nil), f.targets...), nil
}
func (f *fakeBulkStore) InsertBulkPreview(_ context.Context, preview BulkPreview) error {
	if f.previews == nil {
		f.previews = map[platform.ID]BulkPreview{}
	}
	f.previews[preview.ID] = preview
	return nil
}
func (f *fakeBulkStore) FindBulkPreview(_ context.Context, _ platform.ID, id platform.ID) (*BulkPreview, error) {
	preview, ok := f.previews[id]
	if !ok {
		return nil, nil
	}
	return &preview, nil
}
func (f *fakeBulkStore) ConsumePreviewAndInsertJob(_ context.Context, preview BulkPreview, job BulkJob, now time.Time) error {
	preview.ConsumedAt = &now
	f.previews[preview.ID] = preview
	if f.jobs == nil {
		f.jobs = map[platform.ID]BulkJob{}
	}
	f.jobs[job.ID] = job
	return nil
}
func (f *fakeBulkStore) FindBulkJob(_ context.Context, _ platform.ID, id platform.ID) (*BulkJob, error) {
	job, ok := f.jobs[id]
	if !ok {
		return nil, nil
	}
	return &job, nil
}
func (f *fakeBulkStore) ApplyBulkAction(_ context.Context, _ platform.ID, target BulkTarget, _ BulkAction, _ time.Time, _ platform.Actor) (BulkSnapshot, int64, error) {
	if f.versions[target.PersonID] != target.ExpectedVersion {
		return BulkSnapshot{}, 0, platform.VersionConflict(f.versions[target.PersonID])
	}
	f.versions[target.PersonID]++
	return BulkSnapshot{Tags: []string{"before"}, HomeBranchID: target.BranchID}, target.ExpectedVersion + 1, nil
}
func (f *fakeBulkStore) InsertBulkResult(_ context.Context, result BulkResult) error {
	f.results = append(f.results, result)
	return nil
}
func (f *fakeBulkStore) AdvanceBulkJob(_ context.Context, _ platform.ID, id platform.ID, expectedCursor, cursor, succeeded, conflicted, failed int, complete bool, now time.Time) error {
	job := f.jobs[id]
	if job.Cursor != expectedCursor {
		return errors.New("progress conflict")
	}
	job.Cursor, job.Succeeded, job.Conflicted, job.Failed = cursor, job.Succeeded+succeeded, job.Conflicted+conflicted, job.Failed+failed
	job.State = "running"
	if complete {
		job.State = "completed"
		job.CompletedAt = &now
	}
	f.jobs[id] = job
	return nil
}
func (f *fakeBulkStore) ListSuccessfulBulkResults(_ context.Context, _ platform.ID, id platform.ID, limit int) ([]BulkResult, error) {
	rows := []BulkResult{}
	for _, result := range f.results {
		if result.JobID == id && result.State == "succeeded" && f.undoStates[result.PersonID] == "" {
			rows = append(rows, result)
		}
	}
	end := min(limit, len(rows))
	return rows[:end], nil
}
func (f *fakeBulkStore) RestoreBulkSnapshot(_ context.Context, _ platform.ID, result BulkResult, _ time.Time, _ platform.Actor) error {
	if f.undoConflict[result.PersonID] {
		return platform.VersionConflict(result.AfterVersion + 1)
	}
	if f.versions[result.PersonID] != result.AfterVersion {
		return platform.VersionConflict(f.versions[result.PersonID])
	}
	f.versions[result.PersonID]++
	return nil
}
func (f *fakeBulkStore) MarkBulkResultUndo(_ context.Context, _ platform.ID, _ platform.ID, personID platform.ID, state string) error {
	if f.undoStates == nil {
		f.undoStates = map[platform.ID]string{}
	}
	if f.undoStates[personID] != "" {
		return errors.New("already handled")
	}
	f.undoStates[personID] = state
	return nil
}
func (f *fakeBulkStore) BeginBulkUndo(_ context.Context, _ platform.ID, id platform.ID, _ time.Time) error {
	job := f.jobs[id]
	if job.State != "completed" {
		return errors.New("not complete")
	}
	job.State = "undoing"
	f.jobs[id] = job
	return nil
}
func (f *fakeBulkStore) AdvanceBulkUndo(_ context.Context, _ platform.ID, id platform.ID, expectedCursor, cursor, succeeded, conflicted int, complete bool, _ time.Time) error {
	job := f.jobs[id]
	if job.UndoCursor != expectedCursor {
		return errors.New("undo progress conflict")
	}
	job.UndoCursor, job.UndoSucceeded, job.UndoConflicted = cursor, job.UndoSucceeded+succeeded, job.UndoConflicted+conflicted
	if complete {
		job.State = "undone"
	}
	f.jobs[id] = job
	return nil
}

func TestBulkActionFreezesPreviewResumesAndUndoSkipsChangedPeople(t *testing.T) {
	now := time.Date(2026, 8, 11, 18, 30, 0, 0, time.UTC)
	store := &fakeBulkStore{targets: []BulkTarget{{PersonID: "p1", ExpectedVersion: 1, BranchID: "branch-1"}, {PersonID: "p2", ExpectedVersion: 1, BranchID: "branch-1"}, {PersonID: "p3", ExpectedVersion: 1, BranchID: "branch-1"}}, versions: map[platform.ID]int64{"p1": 1, "p2": 2, "p3": 1}, undoConflict: map[platform.ID]bool{"p3": true}}
	service := BulkService{Store: store, Platform: &fakePlatform{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := operationsPrincipal("operator", "branch-1")
	preview, err := service.Preview(context.Background(), principal, PersonSearchFilter{BranchID: "branch-1"}, BulkAction{Type: "add-tags", Tags: []string{"Choir"}, Reason: "Roster correction"}, "req-preview")
	if err != nil || preview.TargetCount != 3 || len(preview.Targets) != 3 {
		t.Fatalf("bad preview: preview=%+v err=%v", preview, err)
	}
	store.targets = nil // Search drift after preview must not change the frozen job targets.
	job, err := service.Start(context.Background(), principal, preview.ID, "req-start")
	if err != nil || len(job.Targets) != 3 {
		t.Fatalf("bad start: job=%+v err=%v", job, err)
	}
	job, err = service.RunChunk(context.Background(), principal, job.ID, 2)
	if err != nil || job.Cursor != 2 || job.Succeeded != 1 || job.Conflicted != 1 || job.State != "running" {
		t.Fatalf("bad first chunk: job=%+v err=%v", job, err)
	}
	job, err = service.RunChunk(context.Background(), principal, job.ID, 2)
	if err != nil || job.State != "completed" || job.Succeeded != 2 || job.Conflicted != 1 {
		t.Fatalf("bad resumed chunk: job=%+v err=%v", job, err)
	}
	job, err = service.UndoChunk(context.Background(), principal, job.ID, 10)
	if err != nil || job.State != "undone" || job.UndoSucceeded != 1 || job.UndoConflicted != 1 {
		t.Fatalf("bad undo: job=%+v err=%v", job, err)
	}
}

func TestBulkActionRequiresDestinationBranchPermissionAndFreshPreview(t *testing.T) {
	now := time.Date(2026, 8, 11, 19, 0, 0, 0, time.UTC)
	store := &fakeBulkStore{versions: map[platform.ID]int64{}}
	service := BulkService{Store: store, Platform: &fakePlatform{}, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	principal := operationsPrincipal("operator", "branch-1")
	if _, err := service.Preview(context.Background(), principal, PersonSearchFilter{BranchID: "branch-1"}, BulkAction{Type: "change-branch", TargetBranchID: "branch-2", Reason: "Member transferred"}, "req"); err == nil {
		t.Fatal("destination branch permission was not enforced")
	}
	preview := BulkPreview{ID: "expired", OrganizationID: "org", BranchID: "branch-1", Action: BulkAction{Type: "add-tags", Tags: []string{"choir"}, Reason: "Roster correction"}, CreatedBy: principal.Actor, ExpiresAt: now.Add(-time.Second)}
	store.previews = map[platform.ID]BulkPreview{preview.ID: preview}
	if _, err := service.Start(context.Background(), principal, preview.ID, "req"); err == nil {
		t.Fatal("expired preview was accepted")
	}
}

type fakeMergeStore struct {
	people    map[platform.ID]Person
	matches   []Person
	canonical *Person
	duplicate *Person
	alias     *PersonMergeAlias
	event     *PersonMergeEvent
}

func (f *fakeMergeStore) FindByID(_ context.Context, _ platform.ID, id platform.ID) (*Person, error) {
	person, ok := f.people[id]
	if !ok {
		return nil, nil
	}
	copy := person
	return &copy, nil
}
func (f *fakeMergeStore) FindDuplicatePeople(context.Context, platform.ID, platform.ID, []ContactPoint, Source) ([]Person, error) {
	return append([]Person(nil), f.matches...), nil
}
func (f *fakeMergeStore) CommitPersonMerge(_ context.Context, canonical, duplicate Person, canonicalExpected, duplicateExpected int64, alias PersonMergeAlias, event PersonMergeEvent, _ time.Time, _ platform.Actor) error {
	if f.people[canonical.ID].Version != canonicalExpected {
		return platform.VersionConflict(f.people[canonical.ID].Version)
	}
	if f.people[duplicate.ID].Version != duplicateExpected {
		return platform.VersionConflict(f.people[duplicate.ID].Version)
	}
	f.canonical, f.duplicate, f.alias, f.event = &canonical, &duplicate, &alias, &event
	return nil
}

func mergePerson(id, branch, email string, version int64) Person {
	return Person{ResourceEnvelope: platform.ResourceEnvelope{ID: platform.ID(id), OrganizationID: "org", BranchID: platform.ID(branch), Version: version}, PersonNumber: "P-" + id, Names: Names{Given: id, Family: "Mensah"}, ContactPoints: []ContactPoint{{Type: "email", Value: email, Normalized: email, Primary: true}}, Addresses: []Address{{Line1: id + " Street", Country: "GH", Primary: true}}, HomeBranchID: platform.ID(branch), MembershipStage: "member", Tags: []string{id}, CustomFields: map[string]string{id: "yes"}, Source: Source{Type: "legacy", Reference: "EXT-42"}}
}

func TestDuplicateCandidatesAreExplainableMaskedAndScopeFiltered(t *testing.T) {
	left := mergePerson("p1", "branch-1", "same@example.com", 2)
	right := mergePerson("p2", "branch-2", "same@example.com", 3)
	store := &fakeMergeStore{people: map[platform.ID]Person{"p1": left}, matches: []Person{right}}
	service := MergeService{Store: store, Authorizer: platform.GrantAuthorizer{}}
	if candidates, err := service.Candidates(context.Background(), operationsPrincipal("reviewer", "branch-1"), "p1"); err != nil || len(candidates) != 0 {
		t.Fatalf("cross-branch candidate leaked: %+v err=%v", candidates, err)
	}
	candidates, err := service.Candidates(context.Background(), operationsPrincipal("reviewer", "branch-1", "branch-2"), "p1")
	if err != nil || len(candidates) != 1 || candidates[0].Score != 100 || len(candidates[0].Signals) != 2 {
		t.Fatalf("bad candidates: %+v err=%v", candidates, err)
	}
	for _, signal := range candidates[0].Signals {
		if strings.Contains(signal.Hint, "same@example.com") || !strings.HasPrefix(signal.Hint, "••••") {
			t.Fatalf("signal was not masked: %+v", signal)
		}
	}
}

func TestReviewedMergePreservesCanonicalLifecycleAndCreatesImmutableAlias(t *testing.T) {
	canonical := mergePerson("p1", "branch-1", "same@example.com", 2)
	canonical.Names.Given = "Ama"
	duplicate := mergePerson("p2", "branch-1", "same@example.com", 4)
	duplicate.Names.Given = "Akua"
	duplicate.MembershipStage = "inactive"
	store := &fakeMergeStore{people: map[platform.ID]Person{"p1": canonical, "p2": duplicate}}
	platformStore := &fakePlatform{}
	service := MergeService{Store: store, Platform: platformStore, Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC) }}
	event, err := service.Merge(context.Background(), operationsPrincipal("reviewer", "branch-1"), MergeInput{CanonicalPersonID: "p1", DuplicatePersonID: "p2", CanonicalExpectedVersion: 2, DuplicateExpectedVersion: 4, Resolution: MergeResolution{Names: "duplicate", ContactPoints: "combine", Tags: "combine", Addresses: "combine", CustomFields: "combine"}, Reason: "Confirmed duplicate records"}, "req-merge")
	if err != nil {
		t.Fatal(err)
	}
	if event.CanonicalVersionAfter != 3 || store.canonical == nil || store.canonical.Names.Given != "Akua" || store.canonical.MembershipStage != "member" {
		t.Fatalf("bad canonical merge result: event=%+v person=%+v", event, store.canonical)
	}
	if store.alias == nil || store.alias.AliasPersonID != "p2" || store.alias.CanonicalPersonID != "p1" || store.alias.MergeEventID != event.ID {
		t.Fatalf("missing immutable alias: %+v", store.alias)
	}
	if len(platformStore.audits) != 1 || len(platformStore.events) != 1 {
		t.Fatalf("merge evidence missing: audits=%d events=%d", len(platformStore.audits), len(platformStore.events))
	}
}

func TestMergeRequiresApprovedSignalAndAccessToBothBranches(t *testing.T) {
	canonical := mergePerson("p1", "branch-1", "one@example.com", 1)
	duplicate := mergePerson("p2", "branch-2", "two@example.com", 1)
	duplicate.Source.Reference = "OTHER"
	store := &fakeMergeStore{people: map[platform.ID]Person{"p1": canonical, "p2": duplicate}}
	service := MergeService{Store: store, Platform: &fakePlatform{}, Authorizer: platform.GrantAuthorizer{}}
	input := MergeInput{CanonicalPersonID: "p1", DuplicatePersonID: "p2", CanonicalExpectedVersion: 1, DuplicateExpectedVersion: 1, Reason: "Reviewed possible duplicate"}
	if _, err := service.Merge(context.Background(), operationsPrincipal("reviewer", "branch-1"), input, "req"); err == nil {
		t.Fatal("cross-branch merge was allowed")
	}
	if _, err := service.Merge(context.Background(), operationsPrincipal("reviewer", "branch-1", "branch-2"), input, "req"); err == nil {
		t.Fatal("merge without duplicate signal was allowed")
	}
}
