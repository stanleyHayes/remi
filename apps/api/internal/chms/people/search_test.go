package people

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

type fakeSearchStore struct {
	items           []PersonSummary
	filter          PersonSearchFilter
	cursor          *personCursor
	includeContacts bool
}

func (s *fakeSearchStore) SearchPeople(_ context.Context, _ platform.ID, filter PersonSearchFilter, cursor *personCursor, limit int, includeContacts bool) ([]PersonSummary, error) {
	s.filter = filter
	s.cursor = cursor
	s.includeContacts = includeContacts
	if len(s.items) > limit {
		return s.items[:limit], nil
	}
	return s.items, nil
}
func searchPrincipal(branch platform.ID) platform.Principal {
	return platform.Principal{OrganizationID: "org", Grants: []platform.Grant{{Action: "search", Resource: "person", BranchIDs: []platform.ID{branch}, FieldClasses: []platform.FieldClass{platform.FieldPersonal}}}}
}
func searchService(t *testing.T, store SearchStore) SearchService {
	t.Helper()
	codec, err := platform.NewCursorCodec(bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return SearchService{Store: store, Authorizer: platform.GrantAuthorizer{}, Cursors: codec, Now: func() time.Time { return time.Date(2026, 8, 11, 17, 0, 0, 0, time.UTC) }}
}

func TestSearchNormalizesFiltersAuthorizesBranchAndPaginates(t *testing.T) {
	store := &fakeSearchStore{items: []PersonSummary{{ID: "p1", Names: Names{Given: "Ama", Family: "Aboagye"}}, {ID: "p2", Names: Names{Given: "Kojo", Family: "Mensah"}}, {ID: "p3", Names: Names{Given: "Efua", Family: "Owusu"}}}}
	service := searchService(t, store)
	page, err := service.Search(context.Background(), searchPrincipal("branch-1"), PersonSearchRequest{Filter: PersonSearchFilter{Query: "  Ama  ", BranchID: "branch-1", MembershipStages: []string{"MEMBER", "member"}, Tags: []string{"Choir", "choir"}}, Limit: 2, IncludeContacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore || page.NextCursor == "" || store.filter.Query != "Ama" || len(store.filter.MembershipStages) != 1 || len(store.filter.Tags) != 1 || !store.includeContacts {
		t.Fatalf("bad search page/filter: page=%+v filter=%+v", page, store.filter)
	}
	next, err := service.Search(context.Background(), searchPrincipal("branch-1"), PersonSearchRequest{Filter: PersonSearchFilter{Query: "Ama", BranchID: "branch-1", MembershipStages: []string{"member"}, Tags: []string{"choir"}}, Cursor: page.NextCursor, Limit: 2, IncludeContacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if store.cursor == nil || store.cursor.ID != "p2" || next.HasMore != true {
		t.Fatalf("cursor not decoded/bound: %+v", store.cursor)
	}
}

func TestSearchRejectsCrossBranchAndFilterCursorReuse(t *testing.T) {
	store := &fakeSearchStore{items: []PersonSummary{{ID: "p1", Names: Names{Given: "Ama"}}, {ID: "p2", Names: Names{Given: "Kojo"}}}}
	service := searchService(t, store)
	if _, err := service.Search(context.Background(), searchPrincipal("branch-1"), PersonSearchRequest{Filter: PersonSearchFilter{BranchID: "branch-2"}}); err == nil {
		t.Fatal("cross-branch search was allowed")
	}
	page, err := service.Search(context.Background(), searchPrincipal("branch-1"), PersonSearchRequest{Filter: PersonSearchFilter{BranchID: "branch-1"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Search(context.Background(), searchPrincipal("branch-1"), PersonSearchRequest{Filter: PersonSearchFilter{BranchID: "branch-1", Tags: []string{"changed"}}, Cursor: page.NextCursor, Limit: 1})
	var domain *platform.DomainError
	if err == nil || errors.As(err, &domain) && domain.Code == "forbidden" {
		t.Fatalf("expected invalid cursor validation, got %v", err)
	}
}

func TestSearchFilterRejectsUnsafeBounds(t *testing.T) {
	filter := PersonSearchFilter{BranchID: "branch", Query: string(bytes.Repeat([]byte{'x'}, 101))}
	if err := filter.NormalizeAndValidate(); err == nil {
		t.Fatal("long query accepted")
	}
	filter = PersonSearchFilter{BranchID: "branch", MembershipStages: []string{"faithful"}}
	if err := filter.NormalizeAndValidate(); err == nil {
		t.Fatal("unknown membership stage accepted")
	}
}
