package reporting

import (
	"time"

	"remi-api/internal/chms/platform"
)

type Service struct {
	Authorizer   platform.Authorizer
	Repository   BuilderRepository
	Runner       ReportRunner
	Evidence     EvidenceStore
	Principals   PrincipalResolver
	Sender       DeliverySender
	AdminAppURL  string
	Now          func() time.Time
	QueryTimeout time.Duration
}

func (s Service) Catalog(principal platform.Principal, organizationID, branchID platform.ID) Catalog {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	items := make([]Definition, 0, len(definitions))
	if s.Authorizer == nil {
		return Catalog{Version: "reporting-dictionary-v1", GeneratedAt: now, Items: items}
	}
	for _, item := range Definitions() {
		decision := s.Authorizer.Authorize(principal, platform.AccessRequest{
			Action:         "read",
			ResourceType:   item.RequiredResource,
			OrganizationID: organizationID,
			BranchID:       branchID,
			FieldClasses:   []platform.FieldClass{item.FieldClass},
			Now:            now,
		})
		if decision.Allowed {
			items = append(items, item)
		}
	}
	return Catalog{Version: "reporting-dictionary-v1", GeneratedAt: now, Items: items}
}
