package reporting

import (
	"testing"
	"time"

	"remi-api/internal/chms/platform"
)

func TestCatalogFiltersDefinitionsByResourceAndFieldPermission(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	service := Service{Authorizer: platform.GrantAuthorizer{}, Now: func() time.Time { return now }}
	viewer := platform.Principal{OrganizationID: "remi", Grants: []platform.Grant{{Action: "read", Resource: "*", BranchIDs: []platform.ID{"accra"}, FieldClasses: []platform.FieldClass{platform.FieldOperational, platform.FieldPersonal}}}}
	catalog := service.Catalog(viewer, "remi", "accra")
	if catalog.GeneratedAt != now || len(catalog.Items) == 0 {
		t.Fatalf("unexpected catalog metadata: %+v", catalog)
	}
	for _, item := range catalog.Items {
		if item.Domain == "finance" || item.Domain == "care" || item.Domain == "retention" {
			t.Fatalf("viewer received restricted definition %q", item.ID)
		}
	}

	finance := platform.Principal{OrganizationID: "remi", Grants: []platform.Grant{{Action: "read", Resource: "finance-ledger", BranchIDs: []platform.ID{"*"}, FieldClasses: []platform.FieldClass{platform.FieldFinancial}}}}
	financeCatalog := service.Catalog(finance, "remi", "accra")
	if len(financeCatalog.Items) != 2 {
		t.Fatalf("finance catalog contains %d metrics, want 2", len(financeCatalog.Items))
	}
	for _, item := range financeCatalog.Items {
		if item.Domain != "finance" {
			t.Fatalf("finance principal received %q", item.ID)
		}
	}
}

func TestCatalogFailsClosedWithoutAuthorizer(t *testing.T) {
	if got := (Service{}).Catalog(platform.Principal{}, "remi", ""); len(got.Items) != 0 {
		t.Fatalf("catalog disclosed %d definitions without an authorizer", len(got.Items))
	}
}
