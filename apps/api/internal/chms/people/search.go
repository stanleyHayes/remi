package people

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
)

type PersonSearchFilter struct {
	Query            string      `json:"query,omitempty"`
	BranchID         platform.ID `json:"branchId"`
	MembershipStages []string    `json:"membershipStages,omitempty"`
	Tags             []string    `json:"tags,omitempty"`
	Archived         bool        `json:"archived"`
	UpdatedFrom      *time.Time  `json:"updatedFrom,omitempty"`
	UpdatedTo        *time.Time  `json:"updatedTo,omitempty"`
}
type PersonSearchRequest struct {
	Filter          PersonSearchFilter
	Cursor          string
	Limit           int
	IncludeContacts bool
}
type PersonSummary struct {
	ID              platform.ID    `json:"id" bson:"_id"`
	Version         int64          `json:"version" bson:"version"`
	PersonNumber    string         `json:"personNumber" bson:"personNumber"`
	Names           Names          `json:"names" bson:"names"`
	PhotoAssetID    platform.ID    `json:"photoAssetId,omitempty" bson:"photoAssetId,omitempty"`
	HomeBranchID    platform.ID    `json:"homeBranchId" bson:"homeBranchId"`
	MembershipStage string         `json:"membershipStage" bson:"membershipStage"`
	Tags            []string       `json:"tags" bson:"tags"`
	ContactPoints   []ContactPoint `json:"contactPoints,omitempty" bson:"contactPoints,omitempty"`
	UpdatedAt       time.Time      `json:"updatedAt" bson:"updatedAt"`
}
type PersonSearchPage struct {
	Items      []PersonSummary `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
	HasMore    bool            `json:"hasMore"`
}
type personCursor struct {
	Family          string      `json:"family"`
	Given           string      `json:"given"`
	ID              platform.ID `json:"id"`
	OrganizationID  platform.ID `json:"organizationId"`
	BranchID        platform.ID `json:"branchId"`
	FilterHash      string      `json:"filterHash"`
	IncludeContacts bool        `json:"includeContacts"`
}

func (f *PersonSearchFilter) NormalizeAndValidate() error {
	f.Query = strings.TrimSpace(f.Query)
	if len(f.Query) > 100 {
		return errors.New("search query must not exceed 100 characters")
	}
	if !f.BranchID.Valid() {
		return errors.New("branch is required")
	}
	stages := map[string]bool{}
	for _, stage := range f.MembershipStages {
		stage = strings.ToLower(strings.TrimSpace(stage))
		if !allowedStage(stage) {
			return errors.New("invalid membership stage filter")
		}
		stages[stage] = true
	}
	f.MembershipStages = f.MembershipStages[:0]
	for stage := range stages {
		f.MembershipStages = append(f.MembershipStages, stage)
	}
	sort.Strings(f.MembershipStages)
	f.Tags = normalizeTags(f.Tags)
	if f.UpdatedFrom != nil && f.UpdatedTo != nil && !f.UpdatedFrom.Before(*f.UpdatedTo) {
		return errors.New("updated date range is invalid")
	}
	return nil
}
func (f PersonSearchFilter) Hash() string {
	encoded, _ := json.Marshal(f)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

type SearchStore interface {
	SearchPeople(context.Context, platform.ID, PersonSearchFilter, *personCursor, int, bool) ([]PersonSummary, error)
}
type SearchService struct {
	Store      SearchStore
	Authorizer platform.Authorizer
	Cursors    *platform.CursorCodec
	Now        func() time.Time
}

func (s SearchService) Search(ctx context.Context, principal platform.Principal, request PersonSearchRequest) (PersonSearchPage, error) {
	if s.Store == nil || s.Authorizer == nil || s.Cursors == nil {
		return PersonSearchPage{}, errors.New("person search service is not configured")
	}
	if err := request.Filter.NormalizeAndValidate(); err != nil {
		return PersonSearchPage{}, platform.ValidationError(platform.FieldError{Path: "filter", Code: "invalid_filter", Message: err.Error()})
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 200 {
		return PersonSearchPage{}, platform.ValidationError(platform.FieldError{Path: "limit", Code: "invalid_limit", Message: "Limit must be between 1 and 200."})
	}
	fields := []platform.FieldClass{platform.FieldPersonal}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	decision := s.Authorizer.Authorize(principal, platform.AccessRequest{Action: "search", ResourceType: "person", OrganizationID: principal.OrganizationID, BranchID: request.Filter.BranchID, FieldClasses: fields, Now: now})
	if !decision.Allowed {
		return PersonSearchPage{}, &platform.DomainError{Code: "forbidden", Message: "You do not have access to people in this branch."}
	}
	var cursor *personCursor
	if request.Cursor != "" {
		var decoded personCursor
		if s.Cursors.Decode(request.Cursor, &decoded) != nil || decoded.OrganizationID != principal.OrganizationID || decoded.BranchID != request.Filter.BranchID || decoded.FilterHash != request.Filter.Hash() || decoded.IncludeContacts != request.IncludeContacts {
			return PersonSearchPage{}, platform.ValidationError(platform.FieldError{Path: "cursor", Code: "invalid_cursor", Message: "The search cursor is invalid for these filters."})
		}
		cursor = &decoded
	}
	items, err := s.Store.SearchPeople(ctx, principal.OrganizationID, request.Filter, cursor, request.Limit+1, request.IncludeContacts)
	if err != nil {
		return PersonSearchPage{}, err
	}
	page := PersonSearchPage{Items: items}
	if len(items) > request.Limit {
		page.HasMore = true
		page.Items = items[:request.Limit]
		last := page.Items[len(page.Items)-1]
		next := personCursor{Family: last.Names.Family, Given: last.Names.Given, ID: last.ID, OrganizationID: principal.OrganizationID, BranchID: request.Filter.BranchID, FilterHash: request.Filter.Hash(), IncludeContacts: request.IncludeContacts}
		page.NextCursor, err = s.Cursors.Encode(next)
		if err != nil {
			return PersonSearchPage{}, err
		}
	}
	return page, nil
}

func (r *MongoRepository) SearchPeople(ctx context.Context, organizationID platform.ID, filter PersonSearchFilter, cursor *personCursor, limit int, includeContacts bool) ([]PersonSummary, error) {
	query := bson.M{"organizationId": organizationID, "homeBranchId": filter.BranchID}
	if filter.Archived {
		query["archivedAt"] = bson.M{"$ne": nil}
	} else {
		query["archivedAt"] = nil
	}
	if len(filter.MembershipStages) > 0 {
		query["membershipStage"] = bson.M{"$in": filter.MembershipStages}
	}
	if len(filter.Tags) > 0 {
		query["tags"] = bson.M{"$all": filter.Tags}
	}
	if filter.UpdatedFrom != nil || filter.UpdatedTo != nil {
		dates := bson.M{}
		if filter.UpdatedFrom != nil {
			dates["$gte"] = filter.UpdatedFrom.UTC()
		}
		if filter.UpdatedTo != nil {
			dates["$lt"] = filter.UpdatedTo.UTC()
		}
		query["updatedAt"] = dates
	}
	if filter.Query != "" {
		pattern := regexp.QuoteMeta(filter.Query)
		query["$or"] = bson.A{bson.M{"names.given": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"names.family": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"names.preferred": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"personNumber": bson.Regex{Pattern: pattern, Options: "i"}}, bson.M{"contactPoints.normalized": bson.Regex{Pattern: pattern, Options: "i"}}}
	}
	if cursor != nil {
		after := bson.A{bson.M{"names.family": bson.M{"$gt": cursor.Family}}, bson.M{"names.family": cursor.Family, "names.given": bson.M{"$gt": cursor.Given}}, bson.M{"names.family": cursor.Family, "names.given": cursor.Given, "_id": bson.M{"$gt": cursor.ID}}}
		if existing, ok := query["$and"].(bson.A); ok {
			query["$and"] = append(existing, bson.M{"$or": after})
		} else {
			query["$and"] = bson.A{bson.M{"$or": after}}
		}
	}
	projection := bson.M{"personNumber": 1, "names": 1, "photoAssetId": 1, "homeBranchId": 1, "membershipStage": 1, "tags": 1, "updatedAt": 1, "version": 1}
	if includeContacts {
		projection["contactPoints"] = 1
	}
	findOptions := options.Find().SetProjection(projection).SetSort(bson.D{{Key: "names.family", Value: 1}, {Key: "names.given", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(int64(limit))
	result, err := r.collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, err
	}
	defer result.Close(ctx)
	var items []PersonSummary
	if err := result.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}
