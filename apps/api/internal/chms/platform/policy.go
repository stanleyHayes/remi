package platform

import (
	"strings"
	"time"
)

type FieldClass string

const (
	FieldOperational       FieldClass = "operational"
	FieldPersonal          FieldClass = "personal"
	FieldSensitiveMinistry FieldClass = "sensitive-ministry"
	FieldChildSafeguarding FieldClass = "child-safeguarding"
	FieldFinancial         FieldClass = "financial"
	FieldSecret            FieldClass = "secret"
)

type Grant struct {
	Action       string
	Resource     string
	BranchIDs    []ID
	MinistryIDs  []ID
	FieldClasses []FieldClass
}

type Principal struct {
	Actor               Actor
	OrganizationID      ID
	Roles               []string
	Grants              []Grant
	AssignedResourceIDs []ID
	MFAConfirmedAt      *time.Time
}

type AccessRequest struct {
	Action            string
	ResourceType      string
	ResourceID        ID
	OrganizationID    ID
	BranchID          ID
	MinistryID        ID
	FieldClasses      []FieldClass
	RequireAssignment bool
	RequireRecentMFA  bool
	Now               time.Time
}

type Decision struct {
	Allowed bool
	Code    string
}

type Authorizer interface {
	Authorize(Principal, AccessRequest) Decision
}

type GrantAuthorizer struct{ RecentMFAWindow time.Duration }

func (a GrantAuthorizer) Authorize(p Principal, r AccessRequest) Decision {
	if !p.OrganizationID.Valid() || p.OrganizationID != r.OrganizationID {
		return Decision{Code: "organization_scope_denied"}
	}
	if r.RequireRecentMFA {
		window := a.RecentMFAWindow
		if window <= 0 {
			window = 10 * time.Minute
		}
		if p.MFAConfirmedAt == nil || r.Now.Sub(*p.MFAConfirmedAt) > window || r.Now.Before(*p.MFAConfirmedAt) {
			return Decision{Code: "recent_mfa_required"}
		}
	}
	for _, grant := range p.Grants {
		if !matches(grant.Action, r.Action) || !matches(grant.Resource, r.ResourceType) {
			continue
		}
		if requiresExplicitScope(grant.BranchIDs) && !r.BranchID.Valid() {
			continue
		}
		if r.BranchID.Valid() && !containsID(grant.BranchIDs, r.BranchID) {
			continue
		}
		if requiresExplicitScope(grant.MinistryIDs) && !r.MinistryID.Valid() {
			continue
		}
		if len(grant.MinistryIDs) > 0 && r.MinistryID.Valid() && !containsID(grant.MinistryIDs, r.MinistryID) {
			continue
		}
		if r.RequireAssignment && (!r.ResourceID.Valid() || !containsID(p.AssignedResourceIDs, r.ResourceID)) {
			continue
		}
		if !containsFields(grant.FieldClasses, r.FieldClasses) {
			continue
		}
		return Decision{Allowed: true, Code: "allowed"}
	}
	return Decision{Code: "grant_scope_denied"}
}

func requiresExplicitScope(values []ID) bool {
	return len(values) > 0 && !containsID(values, "*")
}

func matches(pattern, value string) bool { return pattern == "*" || strings.EqualFold(pattern, value) }
func containsID(values []ID, wanted ID) bool {
	for _, value := range values {
		if value == wanted || value == "*" {
			return true
		}
	}
	return false
}
func containsFields(allowed, requested []FieldClass) bool {
	for _, wanted := range requested {
		if wanted == FieldSecret {
			return false
		}
		found := false
		for _, value := range allowed {
			if value == wanted || value == "*" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
