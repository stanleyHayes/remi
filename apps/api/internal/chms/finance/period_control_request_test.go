package finance

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"remi-api/internal/chms/platform"
)

// The finance configuration screen loads every period's control requests in one
// call so it can badge each period with its pending close or reopen request. An
// omitted periodId must therefore list the whole organization rather than be
// rejected as a missing filter.
func TestListPeriodControlRequestsWithoutPeriodCoversOrganization(t *testing.T) {
	ctx, _, service, database, admin := financeTest(t)
	requestedAt := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	seed := func(id, period, org, state string, at time.Time) {
		_, err := database.Collection(periodControlRequestsCollection).InsertOne(ctx, PeriodControlRequest{
			ID: platform.ID(id), OrganizationID: platform.ID(org), PeriodID: platform.ID(period),
			Action: "close", State: state, Reason: "Month-end close for " + period,
			RequestedBy: "finance-admin", RequestedAt: at, Snapshot: bson.M{},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("request-july", "period-july", "remi", "approved", requestedAt.AddDate(0, -1, 0))
	seed("request-august", "period-august", "remi", "pending", requestedAt)
	seed("request-other-org", "period-august", "other-org", "pending", requestedAt)

	all, err := service.ListPeriodControlRequests(ctx, admin, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("organization-wide list = %+v, want the two remi requests", all)
	}
	if all[0].ID != "request-august" || all[1].ID != "request-july" {
		t.Fatalf("organization-wide list is not newest-first: %+v", all)
	}

	scoped, err := service.ListPeriodControlRequests(ctx, admin, "period-august")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].ID != "request-august" {
		t.Fatalf("period-scoped list = %+v, want only request-august", scoped)
	}
}

func TestListPeriodControlRequestsDeniesUngrantedPrincipal(t *testing.T) {
	ctx, _, service, _, _ := financeTest(t)
	outsider := platform.Principal{
		Actor:          platform.Actor{Type: platform.ActorStaff, ID: "outsider"},
		OrganizationID: "remi",
		Grants: []platform.Grant{{
			Action: "read", Resource: "finance-config", BranchIDs: []platform.ID{"*"},
			FieldClasses: []platform.FieldClass{platform.FieldOperational},
		}},
	}
	if _, err := service.ListPeriodControlRequests(ctx, outsider, ""); err == nil {
		t.Fatal("a principal without the financial field class listed control requests")
	}
}
