package people

import (
	"context"
	"errors"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func validInput() CreateInput {
	return CreateInput{OrganizationID: "org-1", HomeBranchID: "branch-1", Names: Names{Given: " Ama ", Family: " Mensah "}, ContactPoints: []ContactPoint{{Type: "mobile", Value: "024 123 4567", Primary: true}, {Type: "email", Value: "AMA@example.com", Primary: true}}, MembershipStage: "guest", Tags: []string{"Guest", "guest", " New "}, Source: Source{Type: "staff-entry", NoticeVersion: "intake-v1"}}
}

func TestCreateInputNormalizesGhanaContactTagsAndPartialDate(t *testing.T) {
	input := validInput()
	input.DateOfBirth = &PartialDate{Value: "1992-04", Precision: "month"}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if input.ContactPoints[0].Normalized != "+233241234567" || input.ContactPoints[1].Normalized != "ama@example.com" {
		t.Fatalf("contacts not normalized: %+v", input.ContactPoints)
	}
	if len(input.Tags) != 2 || input.Tags[0] != "guest" || input.Tags[1] != "new" {
		t.Fatalf("tags not normalized: %v", input.Tags)
	}
}
func TestCreateInputRejectsBadStatesAndAmbiguousPrimary(t *testing.T) {
	tests := map[string]func(*CreateInput){"bad stage": func(in *CreateInput) { in.MembershipStage = "faithful" }, "bad date": func(in *CreateInput) { in.DateOfBirth = &PartialDate{Value: "1992-99", Precision: "month"} }, "duplicate primary": func(in *CreateInput) {
		in.ContactPoints = append(in.ContactPoints, ContactPoint{Type: "email", Value: "other@example.com", Primary: true})
	}}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validInput()
			mutate(&input)
			if err := input.NormalizeAndValidate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHouseholdsAreFlexibleAndDoNotRequireAHead(t *testing.T) {
	input := CreateHouseholdInput{OrganizationID: "org", HomeBranchID: "branch", Name: "  Mensah and Friends  ", SharedContactPoints: []ContactPoint{{Type: "mobile", Value: "020 000 0000", Primary: true}}, SharedAddresses: []Address{{Line1: "Accra", Country: "gh"}}}
	if err := input.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if input.Name != "Mensah and Friends" || input.StatementPreference != "individual" || input.SharedContactPoints[0].Normalized != "+233200000000" {
		t.Fatalf("unexpected household normalization: %+v", input)
	}
	if err := ValidateHouseholdMembership("house", "person", "adult"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHouseholdMembership("house", "person", "friend"); err != nil {
		t.Fatal("custom practical role should be supported", err)
	}
}
func TestRelationshipRejectsSelfAndInvalidVisibility(t *testing.T) {
	if err := ValidateRelationship("person", "person", "guardian", "staff"); err == nil {
		t.Fatal("self relationship accepted")
	}
	if err := ValidateRelationship("one", "two", "guardian", "public"); err == nil {
		t.Fatal("public relationship visibility accepted")
	}
	if err := ValidateRelationship("one", "two", "guardian", "restricted"); err != nil {
		t.Fatal(err)
	}
}

type fakeRepository struct {
	duplicates       []platform.ID
	inserted         *Person
	insertErr        error
	person           *Person
	archiveCalls     int
	membershipEvents []MembershipEvent
	membershipEvent  *MembershipEvent
}

func (r *fakeRepository) Insert(_ context.Context, p Person) error {
	r.inserted = &p
	return r.insertErr
}
func (r *fakeRepository) FindDuplicateIDs(context.Context, platform.ID, []ContactPoint) ([]platform.ID, error) {
	return r.duplicates, nil
}
func (r *fakeRepository) FindByID(context.Context, platform.ID, platform.ID) (*Person, error) {
	return r.person, nil
}
func (r *fakeRepository) Update(_ context.Context, _ platform.ID, _ platform.ID, expectedVersion int64, input UpdateInput, updatedAt time.Time, actor platform.Actor) error {
	if r.person == nil {
		return errors.New("missing person")
	}
	r.person.Version = expectedVersion + 1
	r.person.HomeBranchID = input.HomeBranchID
	r.person.BranchID = input.HomeBranchID
	r.person.Names = input.Names
	r.person.ContactPoints = input.ContactPoints
	r.person.UpdatedAt = updatedAt
	r.person.UpdatedBy = actor
	return nil
}
func (r *fakeRepository) SetArchive(context.Context, platform.ID, platform.ID, int64, *time.Time, *platform.Actor, string, time.Time, platform.Actor) error {
	r.archiveCalls++
	return nil
}
func (r *fakeRepository) TransitionMembership(_ context.Context, _ platform.ID, _ platform.ID, _ int64, _ string, event MembershipEvent, _ time.Time, _ platform.Actor) error {
	r.membershipEvents = append(r.membershipEvents, event)
	return nil
}
func (r *fakeRepository) FindMembershipEvent(context.Context, platform.ID, platform.ID, platform.ID) (*MembershipEvent, error) {
	return r.membershipEvent, nil
}
func (r *fakeRepository) ReverseMembershipEvent(_ context.Context, _ platform.ID, _ platform.ID, _ int64, event MembershipEvent, _ platform.ID, _ time.Time, _ platform.Actor) error {
	r.membershipEvents = append(r.membershipEvents, event)
	return nil
}

type fakePlatform struct {
	audits   []platform.AuditEvent
	events   []platform.OutboxRecord
	rollback bool
}

func (p *fakePlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (p *fakePlatform) AppendAudit(_ context.Context, e platform.AuditEvent) error {
	p.audits = append(p.audits, e)
	return nil
}
func (p *fakePlatform) EnqueueEvent(_ context.Context, e platform.OutboxRecord) error {
	p.events = append(p.events, e)
	return nil
}

func TestServiceCreateWritesPersonAuditAndOutbox(t *testing.T) {
	repo := &fakeRepository{}
	store := &fakePlatform{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	service := Service{Repository: repo, Platform: store, Now: func() time.Time { return now }}
	person, duplicates, err := service.Create(context.Background(), validInput(), platform.Actor{Type: platform.ActorStaff, ID: "staff-1"}, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicates) != 0 || person == nil || repo.inserted == nil {
		t.Fatalf("missing create result: person=%v duplicates=%v", person, duplicates)
	}
	if person.Version != 1 || person.SchemaVersion != CurrentSchemaVersion || person.CreatedAt != now {
		t.Fatalf("bad envelope: %+v", person.ResourceEnvelope)
	}
	if len(store.audits) != 1 || len(store.events) != 1 || store.events[0].Type != "people.person.created" {
		t.Fatalf("missing evidence: audits=%d events=%d", len(store.audits), len(store.events))
	}
}

func TestServiceUpdatePreservesSourceAndWritesAuditOutbox(t *testing.T) {
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	originalSource := Source{Type: "csv-import", Reference: "import-1", NoticeVersion: "v1"}
	repo := &fakeRepository{person: &Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", BranchID: "branch-1", Version: 3}, HomeBranchID: "branch-1", Names: Names{Given: "Ama"}, MembershipStage: "guest", Source: originalSource}}
	store := &fakePlatform{}
	service := Service{Repository: repo, Platform: store, Now: func() time.Time { return now }}
	input := UpdateInput{ExpectedVersion: 3, HomeBranchID: "branch-2", Names: Names{Given: "Ama", Family: "Mensah"}, MembershipStage: "member", Source: Source{Type: "staff-entry"}}
	updated, err := service.Update(context.Background(), "org-1", "person-1", input, platform.Actor{Type: platform.ActorStaff, ID: "staff-1"}, "request-update")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 4 || updated.HomeBranchID != "branch-2" {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if updated.Source != originalSource {
		t.Fatalf("source provenance changed: %+v", updated.Source)
	}
	if len(store.audits) != 1 || store.audits[0].Action != "people.person.update" || len(store.events) != 1 || store.events[0].Type != "people.person.updated" {
		t.Fatalf("missing evidence: audits=%+v events=%+v", store.audits, store.events)
	}
}
func TestServiceCreateStopsForDuplicateCandidate(t *testing.T) {
	repo := &fakeRepository{duplicates: []platform.ID{"existing"}}
	store := &fakePlatform{}
	service := Service{Repository: repo, Platform: store}
	person, duplicates, err := service.Create(context.Background(), validInput(), platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request")
	var domain *platform.DomainError
	if person != nil || len(duplicates) != 1 || !errors.As(err, &domain) || domain.Code != "duplicate_candidate" {
		t.Fatalf("unexpected duplicate result: person=%v duplicates=%v err=%v", person, duplicates, err)
	}
	if repo.inserted != nil || len(store.audits) > 0 {
		t.Fatal("duplicate candidate was written")
	}
}

func TestServiceArchiveAndRestoreAreVersionedAuditedTransitions(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	person := &Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person-1", OrganizationID: "org-1", Version: 2}, HomeBranchID: "branch-1"}
	repo := &fakeRepository{person: person}
	store := &fakePlatform{}
	service := Service{Repository: repo, Platform: store, Now: func() time.Time { return now }}
	archived, err := service.SetArchived(context.Background(), "org-1", "person-1", 2, true, "Duplicate imported profile", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request-2")
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil || archived.Version != 3 || repo.archiveCalls != 1 || store.audits[0].Action != "people.person.archive" || store.events[0].Type != "people.person.archived" {
		t.Fatalf("archive evidence incomplete: person=%+v audits=%+v events=%+v", archived, store.audits, store.events)
	}
	repo.person = archived
	restored, err := service.SetArchived(context.Background(), "org-1", "person-1", 3, false, "Archive decision corrected", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request-3")
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil || restored.Version != 4 || store.audits[1].Action != "people.person.restore" {
		t.Fatalf("restore evidence incomplete: %+v", restored)
	}
}
func TestServiceArchiveRejectsMissingReasonAndRepeatedState(t *testing.T) {
	repo := &fakeRepository{person: &Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "person", OrganizationID: "org", Version: 1}, HomeBranchID: "branch"}}
	service := Service{Repository: repo, Platform: &fakePlatform{}}
	if _, err := service.SetArchived(context.Background(), "org", "person", 1, true, "", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request"); err == nil {
		t.Fatal("expected reason validation")
	}
	archivedAt := time.Now()
	repo.person.ArchivedAt = &archivedAt
	if _, err := service.SetArchived(context.Background(), "org", "person", 1, true, "Already archived", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request"); err == nil {
		t.Fatal("expected invalid repeated transition")
	}
}

func TestMembershipTransitionAndExplicitReversalPreserveHistory(t *testing.T) {
	now := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	person := &Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "p", OrganizationID: "org", Version: 1}, HomeBranchID: "branch", MembershipStage: "guest"}
	repo := &fakeRepository{person: person}
	evidence := &fakePlatform{}
	service := Service{Repository: repo, Platform: evidence, Now: func() time.Time { return now }}
	event, err := service.TransitionMembership(context.Background(), "org", "p", 1, MembershipTransitionInput{ToStage: "returning-guest", ReasonCode: "return-visit"}, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "transition")
	if err != nil {
		t.Fatal(err)
	}
	if event.FromStage != "guest" || event.ToStage != "returning-guest" || len(repo.membershipEvents) != 1 || evidence.events[0].Type != "people.membership.changed" {
		t.Fatalf("bad transition: %+v", event)
	}
	person.MembershipStage = "returning-guest"
	person.Version = 2
	repo.membershipEvent = event
	reversal, err := service.ReverseMembershipTransition(context.Background(), "org", "p", event.ID, 2, "correction", platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "reverse")
	if err != nil {
		t.Fatal(err)
	}
	if reversal.FromStage != "returning-guest" || reversal.ToStage != "guest" || reversal.ReversesEventID != event.ID || len(repo.membershipEvents) != 2 || evidence.events[1].Type != "people.membership.reversed" {
		t.Fatalf("bad reversal: %+v", reversal)
	}
}
func TestMembershipTransitionRejectsInvalidAndDeceasedProgression(t *testing.T) {
	now := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	for name, stages := range map[string][2]string{"same": {"guest", "guest"}, "deceased progression": {"deceased", "member"}, "unknown": {"member", "faithful"}} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepository{person: &Person{ResourceEnvelope: platform.ResourceEnvelope{ID: "p", OrganizationID: "org", Version: 1}, MembershipStage: stages[0]}}
			service := Service{Repository: repo, Platform: &fakePlatform{}, Now: func() time.Time { return now }}
			if _, err := service.TransitionMembership(context.Background(), "org", "p", 1, MembershipTransitionInput{ToStage: stages[1], ReasonCode: "test-reason"}, platform.Actor{Type: platform.ActorStaff, ID: "staff"}, "request"); err == nil {
				t.Fatal("expected invalid transition")
			}
		})
	}
}
