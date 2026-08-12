package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/care"
	"remi-api/internal/chms/community"
	"remi-api/internal/chms/engagement"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/participation"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/middleware"
	"remi-api/internal/services"
	"remi-api/internal/testsupport"
)

type integrationPlatform struct{}

func (integrationPlatform) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (integrationPlatform) AppendAudit(context.Context, platform.AuditEvent) error    { return nil }
func (integrationPlatform) EnqueueEvent(context.Context, platform.OutboxRecord) error { return nil }

func TestFinanceConfigurationHTTPIsFinanciallyScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_finance_http_test")
	t.Cleanup(func() {
		drop, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = db.Drop(drop)
		_ = client.Disconnect(drop)
	})
	repository, _ := finance.NewRepository(db)
	if err = repository.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	authorizer := platform.GrantAuthorizer{}
	store, _ := platform.NewMongoPlatformStore(db)
	if err = store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	h := New(Services{Finance: finance.Service{Repository: repository, Platform: store, Authorizer: authorizer}, Authorizer: authorizer}, "org-test")
	jwt := services.NewJWTService("finance-http-secret-with-at-least-32-bytes")
	superAdmin, _ := jwt.GenerateMFAAuthenticated("finance-1", "finance@example.com", "Finance", "super-admin", time.Now().UTC())
	nonMFAAdmin, _ := jwt.Generate("finance-2", "finance2@example.com", "Finance", "super-admin")
	editor, _ := jwt.Generate("editor-1", "editor@example.com", "Editor", "editor")
	router := chi.NewRouter()
	router.With(middleware.Auth(jwt)).Mount("/api/chms/v1", h.Routes())
	server := httptest.NewServer(router)
	defer server.Close()
	body := map[string]any{"code": "TITHE", "name": "Tithes", "restrictionType": "unrestricted", "activeFrom": "2026-01-01T00:00:00Z"}
	status, _ := integrationJSON(t, server.URL+"/api/chms/v1/finance/funds", http.MethodPost, editor, body)
	if status != http.StatusNotFound {
		t.Fatalf("editor finance create status=%d", status)
	}
	status, created := integrationJSON(t, server.URL+"/api/chms/v1/finance/funds", http.MethodPost, superAdmin, body)
	if status != http.StatusCreated || created["version"] != float64(1) {
		t.Fatalf("create status=%d body=%v", status, created)
	}
	id := created["id"].(string)
	body["expectedVersion"], body["name"] = 1, "Tithes and Offerings"
	status, updated := integrationJSON(t, server.URL+"/api/chms/v1/finance/funds/"+id, http.MethodPatch, superAdmin, body)
	if status != http.StatusOK || updated["version"] != float64(2) {
		t.Fatalf("update status=%d body=%v", status, updated)
	}
	status, list := integrationJSON(t, server.URL+"/api/chms/v1/finance/funds", http.MethodGet, superAdmin, nil)
	if status != http.StatusOK || len(list["items"].([]any)) != 1 {
		t.Fatalf("list status=%d body=%v", status, list)
	}
	_, _ = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "person-finance", "organizationId": "org-test", "homeBranchId": "branch-1", "archivedAt": nil})
	status, campus := integrationJSON(t, server.URL+"/api/chms/v1/finance/campuses", http.MethodPost, superAdmin, map[string]any{"branchId": "branch-1", "name": "Accra Campus", "timezone": "Africa/Accra", "currency": "GHS"})
	if status != http.StatusCreated {
		t.Fatalf("campus status=%d body=%v", status, campus)
	}
	status, method := integrationJSON(t, server.URL+"/api/chms/v1/finance/payment-methods", http.MethodPost, superAdmin, map[string]any{"code": "CARD", "name": "Card", "kind": "card", "provider": "Paystack", "active": true})
	if status != http.StatusCreated {
		t.Fatalf("method status=%d body=%v", status, method)
	}
	status, period := integrationJSON(t, server.URL+"/api/chms/v1/finance/periods", http.MethodPost, superAdmin, map[string]any{"code": "FY2026", "name": "2026 fiscal year", "startsAt": "2026-01-01T00:00:00Z", "endsAt": "2027-01-01T00:00:00Z"})
	if status != http.StatusCreated {
		t.Fatalf("period status=%d body=%v", status, period)
	}
	status, sequence := integrationJSON(t, server.URL+"/api/chms/v1/finance/receipt-sequences", http.MethodPost, superAdmin, map[string]any{"code": "MAIN2026", "prefix": "REMI", "fiscalYear": 2026, "padding": 6, "startingNumber": 1})
	if status != http.StatusCreated {
		t.Fatalf("sequence status=%d body=%v", status, sequence)
	}
	contribution := map[string]any{"receivedAt": "2026-08-10T09:00:00Z", "branchId": "branch-1", "donor": map[string]any{"type": "person", "personId": "person-finance"}, "source": "online", "paymentMethodId": method["id"], "providerReference": "http-paystack-1", "total": map[string]any{"amountMinor": 10000, "currency": "GHS"}, "splits": []any{map[string]any{"fundId": id, "amount": map[string]any{"amountMinor": 10000, "currency": "GHS"}}}, "postingAction": "post"}
	status, _ = integrationJSONWithKey(t, server.URL+"/api/chms/v1/finance/contributions", http.MethodPost, nonMFAAdmin, "finance-http-post-1", contribution)
	if status != http.StatusNotFound {
		t.Fatalf("non-MFA finance post status=%d", status)
	}
	status, posted := integrationJSONWithKey(t, server.URL+"/api/chms/v1/finance/contributions", http.MethodPost, superAdmin, "finance-http-post-2", contribution)
	if status != http.StatusCreated || posted["receiptNumber"] != "REMI-2026-000001" {
		t.Fatalf("post status=%d body=%v", status, posted)
	}
	status, detail := integrationJSON(t, server.URL+"/api/chms/v1/finance/contributions/"+posted["id"].(string), http.MethodGet, superAdmin, nil)
	if status != http.StatusOK || detail["total"].(map[string]any)["amountMinor"] != float64(10000) {
		t.Fatalf("detail status=%d body=%v", status, detail)
	}
	for _, user := range []bson.M{{"_id": "http-counter-1", "name": "Counter One", "email": "counter1@example.com", "role": "finance-counter", "invitationStatus": "accepted"}, {"_id": "http-counter-2", "name": "Counter Two", "email": "counter2@example.com", "role": "finance-counter", "invitationStatus": "accepted"}, {"_id": "http-approver", "name": "Finance Approver", "email": "approver@example.com", "role": "finance-approver", "invitationStatus": "accepted"}} {
		_, _ = db.Collection("users").InsertOne(ctx, user)
	}
	financeAdmin, _ := jwt.Generate("http-finance-admin", "admin-finance@example.com", "Finance Admin", "finance-admin")
	counterOne, _ := jwt.Generate("http-counter-1", "counter1@example.com", "Counter One", "finance-counter")
	counterTwo, _ := jwt.Generate("http-counter-2", "counter2@example.com", "Counter Two", "finance-counter")
	approver, _ := jwt.GenerateMFAAuthenticated("http-approver", "approver@example.com", "Approver", "finance-approver", time.Now().UTC())
	status, cash := integrationJSON(t, server.URL+"/api/chms/v1/finance/payment-methods", http.MethodPost, superAdmin, map[string]any{"code": "CASH", "name": "Cash", "kind": "cash", "active": true})
	if status != http.StatusCreated {
		t.Fatalf("cash method status=%d body=%v", status, cash)
	}
	batchBody := map[string]any{"branchId": "branch-1", "receivedAt": "2026-08-11T09:00:00Z", "counterIds": []string{"http-counter-1", "http-counter-2"}, "dualControlRequired": true, "expectedPaymentMethodIds": []any{cash["id"]}}
	status, batch := integrationJSON(t, server.URL+"/api/chms/v1/finance/batches", http.MethodPost, financeAdmin, batchBody)
	if status != http.StatusCreated {
		t.Fatalf("batch create status=%d body=%v", status, batch)
	}
	batchID := batch["id"].(string)
	status, batch = integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID+"/transitions", http.MethodPost, counterOne, map[string]any{"action": "start-counting", "expectedVersion": 1})
	if status != http.StatusOK || batch["state"] != "counting" {
		t.Fatalf("batch start status=%d body=%v", status, batch)
	}
	entryBody := map[string]any{"donor": map[string]any{"type": "person", "personId": "person-finance"}, "source": "cash", "paymentMethodId": cash["id"], "total": map[string]any{"amountMinor": 10000, "currency": "GHS"}, "splits": []any{map[string]any{"fundId": id, "amount": map[string]any{"amountMinor": 10000, "currency": "GHS"}}}}
	status, entry := integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID+"/entries", http.MethodPost, counterOne, entryBody)
	if status != http.StatusCreated || entry["version"] != float64(1) {
		t.Fatalf("entry status=%d body=%v", status, entry)
	}
	confirmation := map[string]any{"expectedVersion": 3, "tenderTotals": []any{map[string]any{"paymentMethodId": cash["id"], "amount": map[string]any{"amountMinor": 10000, "currency": "GHS"}}}, "denominations": []any{map[string]any{"paymentMethodId": cash["id"], "valueMinor": 5000, "count": 2}}, "attachments": []any{map[string]any{"publicId": "remi/finance/batches/http-sheet", "resourceType": "image", "label": "Count sheet"}}}
	status, batch = integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID+"/confirmations", http.MethodPost, counterOne, confirmation)
	if status != http.StatusOK || batch["confirmationCount"] != float64(1) {
		t.Fatalf("confirm one status=%d body=%v", status, batch)
	}
	confirmation["expectedVersion"] = 4
	status, batch = integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID+"/confirmations", http.MethodPost, counterTwo, confirmation)
	if status != http.StatusOK || batch["state"] != "counted" {
		t.Fatalf("confirm two status=%d body=%v", status, batch)
	}
	status, batch = integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID+"/transitions", http.MethodPost, approver, map[string]any{"action": "approve", "expectedVersion": 5})
	if status != http.StatusOK || batch["state"] != "approved" {
		t.Fatalf("approve status=%d body=%v", status, batch)
	}
	status, batch = integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID+"/transitions", http.MethodPost, approver, map[string]any{"action": "post", "expectedVersion": 6})
	if status != http.StatusOK || batch["state"] != "posted" {
		t.Fatalf("post batch status=%d body=%v", status, batch)
	}
	status, batchDetail := integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID, http.MethodGet, counterOne, nil)
	if status != http.StatusOK || len(batchDetail["entries"].([]any)) != 1 {
		t.Fatalf("batch detail status=%d body=%v", status, batchDetail)
	}
	unassigned, _ := jwt.Generate("http-counter-other", "other@example.com", "Other", "finance-counter")
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/finance/batches/"+batchID, http.MethodGet, unassigned, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unassigned counter detail status=%d", status)
	}
	status, owners := integrationJSON(t, server.URL+"/api/chms/v1/finance/reconciliation-owners", http.MethodGet, superAdmin, nil)
	if status != http.StatusOK || len(owners["items"].([]any)) != 3 {
		t.Fatalf("reconciliation owners status=%d body=%v", status, owners)
	}
	settlementBody := map[string]any{"branchId": "branch-1", "sourceType": "paystack", "sourceName": "Paystack Ghana", "fileName": "http-settlement.csv", "fileHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "privateAssetId": "remi/finance/settlements/http-private", "periodId": period["id"], "reference": "HTTP-SETTLEMENT-1", "settledAt": "2026-08-10T12:00:00Z", "currency": "GHS", "grossAmountMinor": 10000, "feeAmountMinor": 100, "otherDeductionMinor": 0, "netAmountMinor": 9900, "rows": []any{map[string]any{"sourceRowId": "http-row-1", "reference": "http-paystack-1", "occurredAt": "2026-08-10T09:00:00Z", "amountMinor": 10000, "feeAmountMinor": 100, "description": "Verified Paystack transaction"}}}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/finance/settlements", http.MethodPost, editor, settlementBody)
	if status != http.StatusNotFound {
		t.Fatalf("editor settlement import status=%d", status)
	}
	status, settlement := integrationJSON(t, server.URL+"/api/chms/v1/finance/settlements", http.MethodPost, superAdmin, settlementBody)
	if status != http.StatusCreated || settlement["state"] != "imported" {
		t.Fatalf("settlement status=%d body=%v", status, settlement)
	}
	settlementID := settlement["id"].(string)
	status, reconciliation := integrationJSON(t, server.URL+"/api/chms/v1/finance/settlements/"+settlementID+"/items", http.MethodGet, superAdmin, nil)
	if status != http.StatusOK || len(reconciliation["items"].([]any)) != 1 {
		t.Fatalf("reconciliation items status=%d body=%v", status, reconciliation)
	}
	item := reconciliation["items"].([]any)[0].(map[string]any)
	if item["confidence"] != "exact" || item["suggestedTargetId"] != posted["id"] {
		t.Fatalf("suggestion=%v", item)
	}
	status, resolved := integrationJSON(t, server.URL+"/api/chms/v1/finance/reconciliation-items/"+item["id"].(string)+"/resolve", http.MethodPost, superAdmin, map[string]any{"action": "match", "targetType": "contribution", "targetId": posted["id"]})
	if status != http.StatusOK || resolved["resolution"] != "match" {
		t.Fatalf("resolve status=%d body=%v", status, resolved)
	}
	status, settlement = integrationJSON(t, server.URL+"/api/chms/v1/finance/settlements/"+settlementID, http.MethodGet, superAdmin, nil)
	if status != http.StatusOK || settlement["state"] != "reconciled" || settlement["unexplainedVarianceMinor"] != float64(0) {
		t.Fatalf("settlement detail status=%d body=%v", status, settlement)
	}
	reportURL := server.URL + "/api/chms/v1/finance/reports/summary?branchId=branch-1&periodId=" + period["id"].(string)
	status, report := integrationJSON(t, reportURL, http.MethodGet, superAdmin, nil)
	if status != http.StatusOK || report["totals"].(map[string]any)["netContributionMinor"] != float64(20000) || report["meta"].(map[string]any)["sourceWatermark"] == "" {
		t.Fatalf("finance report status=%d body=%v", status, report)
	}
	exportBody := map[string]any{"kind": "accounting-csv", "branchId": "branch-1", "periodId": period["id"]}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/finance/export-runs", http.MethodPost, nonMFAAdmin, exportBody)
	if status != http.StatusNotFound {
		t.Fatalf("non-MFA finance export status=%d", status)
	}
	status, exportRun := integrationJSON(t, server.URL+"/api/chms/v1/finance/export-runs", http.MethodPost, superAdmin, exportBody)
	if status != http.StatusCreated || exportRun["status"] != "completed" || exportRun["artifactHash"] == "" || exportRun["rowCount"] != float64(2) {
		t.Fatalf("finance export status=%d body=%v", status, exportRun)
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/chms/v1/finance/export-runs/"+exportRun["id"].(string)+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+superAdmin)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	artifact, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/csv") || len(artifact) < 100 || response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("finance export download status=%d type=%s cache=%s bytes=%d", response.StatusCode, response.Header.Get("Content-Type"), response.Header.Get("Cache-Control"), len(artifact))
	}
}

func TestPeopleAndHouseholdHTTPMutationLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_chms_http_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	peopleRepo, _ := people.NewMongoRepository(db)
	households, _ := people.NewHouseholdRepository(db)
	participationRepo, _ := participation.NewMongoRepository(db)
	communityRepo, _ := community.NewMongoRepository(db)
	engagementRepo, _ := engagement.NewMongoRepository(db)
	if err := peopleRepo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := households.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := participationRepo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := communityRepo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := engagementRepo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte("integration-cursor-key"))
	cursors, _ := platform.NewCursorCodec(key[:])
	authorizer := platform.GrantAuthorizer{}
	uow := integrationPlatform{}
	pickupSecret := sha256.Sum256([]byte("http-integration-pickup-secret"))
	pickupCodes, _ := participation.NewPickupCodeManager(pickupSecret[:])
	h := New(Services{Search: people.SearchService{Store: peopleRepo, Authorizer: authorizer, Cursors: cursors}, People: people.Service{Repository: peopleRepo, Platform: uow}, HouseholdService: people.HouseholdService{Repository: households, Platform: uow}, Repository: peopleRepo, Households: households, Journey: peopleRepo, Participation: participation.Service{Store: participationRepo, Platform: uow, People: peopleRepo, PickupCodes: pickupCodes, Authorizer: authorizer}, Community: community.Service{Store: communityRepo, Platform: uow, People: peopleRepo, Authorizer: authorizer}, Engagement: engagement.Service{Store: engagementRepo, Platform: uow, Authorizer: authorizer, Flags: platform.NewFeatureFlags(1, map[string]bool{"retention-individual-signals": false}), Consent: func(_ context.Context, _ platform.Principal, _, _ platform.ID, _, _ string) (engagement.ConsentDecision, error) {
		return engagement.ConsentDecision{Eligible: true, Decision: "eligible", EvaluatedAt: time.Now().UTC()}, nil
	}}, Authorizer: authorizer}, "org-test")
	jwt := services.NewJWTService("integration-http-secret-with-at-least-32-bytes")
	token, _ := jwt.Generate("admin-1", "admin@example.com", "Admin", "super-admin")
	viewer, _ := jwt.Generate("viewer-1", "viewer@example.com", "Viewer", "viewer")
	router := chi.NewRouter()
	router.With(middleware.Auth(jwt)).Mount("/api/chms/v1", h.Routes())
	server := httptest.NewServer(router)
	defer server.Close()
	create := map[string]any{"homeBranchId": "branch-1", "names": map[string]any{"given": "Ama", "family": "Mensah"}, "contactPoints": []any{map[string]any{"type": "mobile", "value": "0241234567", "primary": true}}, "addresses": []any{}, "membershipStage": "guest", "tags": []string{"follow-up"}, "customFields": map[string]string{}, "communicationPreferences": map[string]bool{}, "source": map[string]string{"type": "staff-entry"}}
	status, created := integrationJSON(t, server.URL+"/api/chms/v1/people", http.MethodPost, token, create)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d body=%v", status, created)
	}
	personID := created["id"].(string)
	status, list := integrationJSON(t, server.URL+"/api/chms/v1/people?branchId=branch-1", http.MethodGet, token, nil)
	if status != http.StatusOK || len(list["items"].([]any)) != 1 {
		t.Fatalf("list status=%d body=%v", status, list)
	}
	update := create
	update["expectedVersion"] = float64(1)
	update["homeBranchId"] = "branch-2"
	status, updated := integrationJSON(t, server.URL+"/api/chms/v1/people/"+personID, http.MethodPatch, token, update)
	if status != http.StatusOK || updated["version"].(float64) != 2 {
		t.Fatalf("update status=%d body=%v", status, updated)
	}
	householdBody := map[string]any{"homeBranchId": "branch-2", "name": "Mensah Household", "members": []any{map[string]any{"personId": personID, "role": "adult"}}, "primaryContactPersonId": personID, "statementPreference": "household", "sharedContactPoints": []any{}, "sharedAddresses": []any{}}
	status, household := integrationJSON(t, server.URL+"/api/chms/v1/households", http.MethodPost, token, householdBody)
	if status != http.StatusCreated {
		t.Fatalf("household status=%d body=%v", status, household)
	}
	status, profile := integrationJSON(t, server.URL+"/api/chms/v1/people/"+personID, http.MethodGet, token, nil)
	if status != http.StatusOK || profile["household"] == nil {
		t.Fatalf("profile status=%d body=%v", status, profile)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/people/"+personID+"/archive", http.MethodPost, viewer, map[string]any{"expectedVersion": 2, "reason": "viewer should not archive"})
	if status != http.StatusNotFound {
		t.Fatalf("viewer archive status=%d", status)
	}
	status, archived := integrationJSON(t, server.URL+"/api/chms/v1/people/"+personID+"/archive", http.MethodPost, token, map[string]any{"expectedVersion": 2, "reason": "member requested archival"})
	if status != http.StatusOK || archived["version"].(float64) != 3 {
		t.Fatalf("archive status=%d body=%v", status, archived)
	}
	definitionBody := map[string]any{"homeBranchId": "branch-2", "name": "Sunday Celebration", "timezone": "Africa/Accra", "defaultDurationMinutes": 120, "defaultRoomIds": []string{"main-hall"}, "defaultCapacity": 500, "recurrence": map[string]any{"frequency": "weekly", "daysOfWeek": []string{"sunday"}, "localStart": "09:00", "interval": 1, "startsOn": "2026-08-16"}}
	status, definition := integrationJSON(t, server.URL+"/api/chms/v1/service-definitions", http.MethodPost, token, definitionBody)
	if status != http.StatusCreated {
		t.Fatalf("definition status=%d body=%v", status, definition)
	}
	definitionID := definition["id"].(string)
	generationBody := map[string]any{"from": "2026-09-01T00:00:00Z", "to": "2026-10-01T00:00:00Z"}
	status, generated := integrationJSON(t, server.URL+"/api/chms/v1/service-definitions/"+definitionID+"/occurrences:generate", http.MethodPost, token, generationBody)
	if status != http.StatusOK || len(generated["items"].([]any)) != 4 {
		t.Fatalf("generation status=%d body=%v", status, generated)
	}
	firstGeneratedID := generated["items"].([]any)[0].(map[string]any)["id"]
	status, generatedAgain := integrationJSON(t, server.URL+"/api/chms/v1/service-definitions/"+definitionID+"/occurrences:generate", http.MethodPost, token, generationBody)
	if status != http.StatusOK || generatedAgain["items"].([]any)[0].(map[string]any)["id"] != firstGeneratedID {
		t.Fatalf("recurrence generation was not idempotent: %v", generatedAgain)
	}
	occurrenceBody := map[string]any{"homeBranchId": "branch-2", "serviceDefinitionId": definitionID, "name": "Sunday Celebration", "startsAt": "2026-08-16T09:00:00Z", "endsAt": "2026-08-16T11:00:00Z", "timezone": "Africa/Accra", "roomIds": []string{"main-hall"}, "capacity": 500}
	status, occurrence := integrationJSON(t, server.URL+"/api/chms/v1/occurrences", http.MethodPost, token, occurrenceBody)
	if status != http.StatusCreated {
		t.Fatalf("occurrence status=%d body=%v", status, occurrence)
	}
	occurrenceID := occurrence["id"].(string)
	occurrenceKey := occurrence["occurrenceKey"].(string)
	occurrenceBody["expectedVersion"] = 1
	occurrenceBody["name"] = "Sunday Celebration — Updated"
	occurrenceBody["startsAt"] = "2026-08-16T10:00:00Z"
	occurrenceBody["endsAt"] = "2026-08-16T12:00:00Z"
	status, changedOccurrence := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID, http.MethodPatch, token, occurrenceBody)
	if status != http.StatusOK || changedOccurrence["occurrenceKey"] != occurrenceKey {
		t.Fatalf("occurrence update status=%d body=%v", status, changedOccurrence)
	}
	guestCreate := map[string]any{"homeBranchId": "branch-2", "names": map[string]any{"given": "Kojo", "family": "Guest"}, "contactPoints": []any{}, "addresses": []any{}, "membershipStage": "guest", "tags": []string{"first-time"}, "customFields": map[string]string{}, "communicationPreferences": map[string]bool{}, "source": map[string]string{"type": "attendance"}}
	status, guest := integrationJSON(t, server.URL+"/api/chms/v1/people", http.MethodPost, token, guestCreate)
	if status != http.StatusCreated {
		t.Fatalf("guest create status=%d body=%v", status, guest)
	}
	guestID := guest["id"].(string)
	groupBody := map[string]any{"homeBranchId": "branch-2", "name": "Young Adults", "type": "community", "description": "Community and discipleship", "leaderPersonIds": []string{guestID}, "capacity": 30, "privacy": "request", "discoverability": "members", "status": "draft", "meetingPattern": map[string]any{"frequency": "weekly", "weekday": "friday", "localStart": "18:30", "durationMinutes": 90, "timezone": "Africa/Accra", "location": "Fellowship hall"}}
	status, group := integrationJSON(t, server.URL+"/api/chms/v1/groups", http.MethodPost, token, groupBody)
	if status != http.StatusCreated || group["version"].(float64) != 1 {
		t.Fatalf("group create status=%d body=%v", status, group)
	}
	groupID := group["id"].(string)
	status, groupList := integrationJSON(t, server.URL+"/api/chms/v1/groups?branchId=branch-2", http.MethodGet, viewer, nil)
	if status != http.StatusOK || len(groupList["items"].([]any)) != 1 {
		t.Fatalf("group list status=%d body=%v", status, groupList)
	}
	groupBody["expectedVersion"] = 1
	groupBody["status"] = "active"
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID, http.MethodPatch, viewer, groupBody)
	if status != http.StatusNotFound {
		t.Fatalf("viewer group update status=%d", status)
	}
	status, group = integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID, http.MethodPatch, token, groupBody)
	if status != http.StatusOK || group["status"] != "active" || group["version"].(float64) != 2 {
		t.Fatalf("group activate status=%d body=%v", status, group)
	}
	membershipBody := map[string]any{"personId": guestID, "mode": "add", "role": "member", "source": "staff"}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/members", http.MethodPost, viewer, membershipBody)
	if status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("viewer membership add status=%d", status)
	}
	status, membership := integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/members", http.MethodPost, token, membershipBody)
	if status != http.StatusCreated || membership["status"] != "active" {
		t.Fatalf("membership add status=%d body=%v", status, membership)
	}
	meetingBody := map[string]any{"topic": "Community dinner", "startsAt": "2026-08-18T18:30:00Z", "endsAt": "2026-08-18T20:00:00Z", "timezone": "Africa/Accra", "location": "Fellowship hall"}
	status, meeting := integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/meetings", http.MethodPost, token, meetingBody)
	if status != http.StatusCreated {
		t.Fatalf("group meeting status=%d body=%v", status, meeting)
	}
	meetingID := meeting["id"].(string)
	status, groupAttendance := integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/meetings/"+meetingID+"/attendance/"+guestID, http.MethodPut, token, map[string]any{"status": "present", "confidence": 100})
	if status != http.StatusCreated || groupAttendance["version"].(float64) != 1 {
		t.Fatalf("group attendance status=%d body=%v", status, groupAttendance)
	}
	status, roster := integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/members", http.MethodGet, viewer, nil)
	if status != http.StatusOK || len(roster["items"].([]any)) != 1 {
		t.Fatalf("group roster status=%d body=%v", status, roster)
	}
	status, membershipHistory := integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/members/"+guestID+"/history", http.MethodGet, viewer, nil)
	if status != http.StatusOK || len(membershipHistory["items"].([]any)) != 1 {
		t.Fatalf("group membership history status=%d body=%v", status, membershipHistory)
	}
	historyItem := membershipHistory["items"].([]any)[0].(map[string]any)
	if historyItem["toStatus"] != "active" || historyItem["leaderNote"] != nil {
		t.Fatalf("group membership history exposed wrong fields: %v", historyItem)
	}
	teamBody := map[string]any{"name": "Hospitality", "description": "Welcome and hosting", "homeBranchId": "branch-2", "leaderPersonIds": []string{guestID}, "status": "active"}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/teams", http.MethodPost, viewer, teamBody)
	if status != http.StatusForbidden {
		t.Fatalf("viewer volunteer team create status=%d", status)
	}
	status, team := integrationJSON(t, server.URL+"/api/chms/v1/teams", http.MethodPost, token, teamBody)
	if status != http.StatusCreated || team["version"].(float64) != 1 {
		t.Fatalf("volunteer team status=%d body=%v", status, team)
	}
	teamID := team["id"].(string)
	positionBody := map[string]any{"name": "Welcome host", "description": "Welcome families", "status": "active", "eligibility": map[string]any{"minimumAgeYears": 18, "requiredSkills": []string{"hospitality"}, "backgroundCheckRequired": false, "safeguardingTrainingRequired": false}}
	status, volunteerPosition := integrationJSON(t, server.URL+"/api/chms/v1/teams/"+teamID+"/positions", http.MethodPost, token, positionBody)
	if status != http.StatusCreated {
		t.Fatalf("volunteer position status=%d body=%v", status, volunteerPosition)
	}
	positionID := volunteerPosition["id"].(string)
	positionBody["expectedVersion"] = 1
	positionBody["name"] = "Welcome coordinator"
	status, volunteerPosition = integrationJSON(t, server.URL+"/api/chms/v1/teams/"+teamID+"/positions/"+positionID, http.MethodPatch, token, positionBody)
	if status != http.StatusOK || volunteerPosition["version"].(float64) != 2 {
		t.Fatalf("volunteer position update status=%d body=%v", status, volunteerPosition)
	}
	memberToken, _ := jwt.GenerateMember("member-account-1", guestID, "member-session-1", "", "kojo@example.com", "Kojo Guest")
	teamAssignment := community.VolunteerAssignment{ResourceEnvelope: platform.ResourceEnvelope{ID: "leader-http-assignment", OrganizationID: "org-test", BranchID: "branch-2", Version: 1, CreatedAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)}, PlanID: "leader-http-plan", OccurrenceID: platform.ID(occurrenceID), TeamID: platform.ID(teamID), PositionID: platform.ID(positionID), PersonID: platform.ID(guestID), Slot: 1, StartsAt: time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC), Status: "invited"}
	if err = communityRepo.InsertAssignment(ctx, teamAssignment); err != nil {
		t.Fatal(err)
	}
	status, leaderWorkspace := integrationJSON(t, server.URL+"/api/chms/v1/leader-workspace?from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, memberToken, nil)
	if status != http.StatusOK || len(leaderWorkspace["groups"].([]any)) != 1 || len(leaderWorkspace["teams"].([]any)) != 1 {
		t.Fatalf("leader workspace status=%d body=%v", status, leaderWorkspace)
	}
	status, leaderDetail := integrationJSON(t, server.URL+"/api/chms/v1/leader-workspace/groups/"+groupID+"?from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, memberToken, nil)
	if status != http.StatusOK || len(leaderDetail["roster"].([]any)) != 1 || leaderDetail["roster"].([]any)[0].(map[string]any)["name"] != "Kojo Guest" {
		t.Fatalf("leader detail status=%d body=%v", status, leaderDetail)
	}
	status, teamDetail := integrationJSON(t, server.URL+"/api/chms/v1/leader-workspace/teams/"+teamID+"?from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, memberToken, nil)
	if status != http.StatusOK || len(teamDetail["assignments"].([]any)) != 1 || teamDetail["assignments"].([]any)[0].(map[string]any)["name"] != "Kojo Guest" || teamDetail["assignments"].([]any)[0].(map[string]any)["contactPoints"] != nil {
		t.Fatalf("leader team detail status=%d body=%v", status, teamDetail)
	}
	status, communityDashboard := integrationJSON(t, server.URL+"/api/chms/v1/community-analytics?branchId=branch-2&from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, token, nil)
	if status != http.StatusOK || communityDashboard["metricVersion"] != "community-ops-v1" || communityDashboard["privacyThreshold"].(float64) != 5 || len(communityDashboard["groups"].([]any)) != 1 || len(communityDashboard["volunteerTeams"].([]any)) != 1 {
		t.Fatalf("community analytics status=%d body=%v", status, communityDashboard)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/community-analytics?branchId=branch-2&from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, memberToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member branch analytics status=%d", status)
	}
	ruleBody := map[string]any{"branchId": "branch-2", "name": "First visit return evidence", "kind": "first-visit-no-return", "description": "Observed first visits with no later qualifying local date.", "timezone": "Africa/Accra", "windowDays": 30, "lookbackDays": 180, "expiresAfterDays": 14, "approval": map[string]any{"reviewCadenceDays": 30}}
	status, engagementRule := integrationJSON(t, server.URL+"/api/chms/v1/engagement/rules", http.MethodPost, token, ruleBody)
	if status != http.StatusCreated || engagementRule["status"] != "draft" || engagementRule["metricVersion"] != "retention-v1" {
		t.Fatalf("engagement rule status=%d body=%v", status, engagementRule)
	}
	status, engagementRules := integrationJSON(t, server.URL+"/api/chms/v1/engagement/rules?branchId=branch-2", http.MethodGet, token, nil)
	if status != http.StatusOK || engagementRules["activationEnabled"] != false || len(engagementRules["items"].([]any)) != 1 {
		t.Fatalf("engagement rules status=%d body=%v", status, engagementRules)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/engagement/rules?branchId=branch-2", http.MethodGet, memberToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member engagement rules status=%d", status)
	}
	ruleID := engagementRule["id"].(string)
	approval := map[string]any{"productOwnerId": "owner-1", "pastoralApproverId": "pastor-1", "privacyApproverId": "privacy-1", "approvedAt": "2026-08-10T12:00:00Z", "reviewCadenceDays": 30}
	status, disabledPublish := integrationJSON(t, server.URL+"/api/chms/v1/engagement/rules/"+ruleID+"/publish", http.MethodPost, token, map[string]any{"expectedVersion": 1, "approval": approval})
	if status != http.StatusServiceUnavailable || disabledPublish["error"].(map[string]any)["code"] != "feature_disabled" {
		t.Fatalf("disabled engagement publish status=%d body=%v", status, disabledPublish)
	}
	approvedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	publishedRule := engagement.Rule{ResourceEnvelope: platform.ResourceEnvelope{ID: "retention-http-published", OrganizationID: "org-test", BranchID: "branch-2", SchemaVersion: 1, Version: 1, CreatedAt: approvedAt, UpdatedAt: approvedAt}, Name: "Approved aggregate reporting policy", Kind: "first-visit-no-return", Timezone: "Africa/Accra", WindowDays: 30, LookbackDays: 180, ExpiresAfterDays: 14, Status: "published", MetricVersion: "retention-v1", Approval: engagement.ApprovalEvidence{ProductOwnerID: "owner-1", PastoralApproverID: "pastor-1", PrivacyApproverID: "privacy-1", ApprovedAt: &approvedAt, ReviewCadenceDays: 30}}
	if err = engagementRepo.InsertRule(ctx, publishedRule); err != nil {
		t.Fatal(err)
	}
	status, cohorts := integrationJSON(t, server.URL+"/api/chms/v1/engagement/cohorts?branchId=branch-2&from=2026-01-01T00%3A00%3A00Z&to=2026-08-11T00%3A00%3A00Z", http.MethodGet, token, nil)
	if status != http.StatusOK || cohorts["metricVersion"] != "retention-v1" || cohorts["configuration"].(map[string]any)["privacyThreshold"] != float64(5) {
		t.Fatalf("retention cohorts status=%d body=%v", status, cohorts)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/engagement/cohorts?branchId=branch-2&from=2026-01-01T00%3A00%3A00Z&to=2026-08-11T00%3A00%3A00Z", http.MethodGet, memberToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member retention cohorts status=%d", status)
	}
	status, readiness := integrationJSON(t, server.URL+"/api/chms/v1/engagement/safety-readiness?branchId=branch-2", http.MethodGet, token, nil)
	if status != http.StatusOK || readiness["releaseState"] != "awaiting-human-review" || len(readiness["policies"].([]any)) != 1 {
		t.Fatalf("retention safety readiness status=%d body=%v", status, readiness)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/engagement/safety-readiness?branchId=branch-2", http.MethodGet, memberToken, nil)
	if status != http.StatusForbidden {
		t.Fatalf("member retention safety status=%d", status)
	}
	productReviewer, _ := jwt.Generate("owner-1", "owner@example.com", "Product Owner", "super-admin")
	checklist := map[string]any{"reasonUnderstood": true, "caveatsVisible": true, "noDiagnosis": true, "noAutomaticAction": true, "consentBoundaryClear": true, "sparseDataProtected": true, "noFinancialInference": true, "languageIsPastoral": true}
	status, reviewed := integrationJSON(t, server.URL+"/api/chms/v1/engagement/safety-reviews", http.MethodPost, productReviewer, map[string]any{"branchId": "branch-2", "role": "product", "decision": "approved", "findings": "Reviewed the exact policy version and confirmed the complete safety checklist.", "checklist": checklist})
	if status != http.StatusCreated || reviewed["roleStatus"].(map[string]any)["product"] != "approved" || reviewed["releaseState"] != "awaiting-human-review" {
		t.Fatalf("product safety review status=%d body=%v", status, reviewed)
	}
	if _, err = db.Collection("users").InsertOne(ctx, bson.M{"_id": "admin-1", "name": "Admin Reviewer", "email": "admin@example.com", "role": "super-admin", "invitationStatus": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Collection("chms_people").InsertOne(ctx, bson.M{"_id": "review-person-http", "organizationId": "org-test", "homeBranchId": "branch-1", "personNumber": "P-REVIEW", "names": bson.M{"given": "Esi", "family": "Owusu"}, "archivedAt": nil}); err != nil {
		t.Fatal(err)
	}
	reviewNow := time.Now().UTC()
	createdSignal, err := engagementRepo.InsertSignal(ctx, engagement.Signal{ResourceEnvelope: platform.ResourceEnvelope{ID: "signal-http", OrganizationID: "org-test", BranchID: "branch-1", SchemaVersion: 1, Version: 1, CreatedAt: reviewNow, CreatedBy: platform.Actor{Type: platform.ActorSystem, ID: "retention-engine"}, UpdatedAt: reviewNow, UpdatedBy: platform.Actor{Type: platform.ActorSystem, ID: "retention-engine"}}, RuleID: "rule-http", RuleVersion: 1, RuleName: "Attendance review", RuleKind: "attendance-gap", PersonID: "review-person-http", ObservedFrom: reviewNow.AddDate(0, 0, -30), ObservedThrough: reviewNow, Evidence: []engagement.Evidence{{SourceType: "attendance", SourceID: "attendance-http", OccurredAt: reviewNow.AddDate(0, 0, -30), Fact: "last qualifying attendance"}}, Caveats: []string{"Human review required."}, State: "open", SourceKey: "http-review-source", ExpiresAt: reviewNow.AddDate(0, 0, 14)})
	if err != nil || !createdSignal {
		t.Fatalf("seed engagement signal created=%v err=%v", createdSignal, err)
	}
	if personSummary, summaryErr := engagementRepo.FindPersonSummary(ctx, "org-test", "review-person-http"); summaryErr != nil || personSummary == nil {
		var stored bson.M
		_ = db.Collection("chms_people").FindOne(ctx, bson.M{"_id": "review-person-http"}).Decode(&stored)
		t.Fatalf("person summary unavailable stored=%v err=%v", stored, summaryErr)
	}
	status, reviewDetail := integrationJSON(t, server.URL+"/api/chms/v1/engagement/signals/signal-http/review", http.MethodGet, token, nil)
	if status != http.StatusOK || reviewDetail["person"].(map[string]any)["displayName"] != "Esi Owusu" || len(reviewDetail["assignees"].([]any)) != 1 {
		t.Fatalf("review detail status=%d body=%v", status, reviewDetail)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/engagement/signals/signal-http/review", http.MethodGet, memberToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("member review detail status=%d", status)
	}
	status, assignedSignal := integrationJSON(t, server.URL+"/api/chms/v1/engagement/signals/signal-http/assign", http.MethodPost, token, map[string]any{"expectedVersion": 1, "assigneeId": "admin-1", "reason": "Accept this observation for human review."})
	if status != http.StatusOK || assignedSignal["state"] != "in-review" || assignedSignal["version"] != float64(2) {
		t.Fatalf("assign signal status=%d body=%v", status, assignedSignal)
	}
	status, contactedSignal := integrationJSON(t, server.URL+"/api/chms/v1/engagement/signals/signal-http/contact-events", http.MethodPost, token, map[string]any{"expectedVersion": 2, "channel": "phone", "outcome": "connected", "summary": "Consent rechecked immediately before recording contact."})
	if status != http.StatusOK || contactedSignal["version"] != float64(3) {
		t.Fatalf("contact signal status=%d body=%v", status, contactedSignal)
	}
	status, resolvedSignal := integrationJSON(t, server.URL+"/api/chms/v1/engagement/signals/signal-http/resolve", http.MethodPost, token, map[string]any{"expectedVersion": 3, "outcome": "reconnected", "reason": "Review completed after member conversation.", "falsePositive": false})
	if status != http.StatusOK || resolvedSignal["state"] != "resolved" || resolvedSignal["resolutionOutcome"] != "reconnected" {
		t.Fatalf("resolve signal status=%d body=%v", status, resolvedSignal)
	}
	status, reminded := integrationJSON(t, server.URL+"/api/chms/v1/assignments/"+string(teamAssignment.ID)+"/reminders", http.MethodPost, memberToken, map[string]any{"expectedVersion": 1})
	if status != http.StatusOK || reminded["version"].(float64) != 2 || reminded["reminder"].(map[string]any)["sentCount"].(float64) != 1 {
		t.Fatalf("leader reminder status=%d body=%v", status, reminded)
	}
	status, handoff := integrationJSON(t, server.URL+"/api/chms/v1/groups/"+groupID+"/communication-handoffs", http.MethodPost, memberToken, map[string]any{"purpose": "group-operations", "channel": "email", "message": "Community night begins at 6:30 PM."})
	if status != http.StatusAccepted || handoff["state"] != "pending-consent-review" || len(handoff["audiencePersonIds"].([]any)) != 1 || handoff["destination"] != nil {
		t.Fatalf("leader handoff status=%d body=%v", status, handoff)
	}
	unlinkedMember, _ := jwt.GenerateMember("member-account-2", personID, "member-session-2", "", "ama@example.com", "Ama Mensah")
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/leader-workspace/groups/"+groupID+"?from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, unlinkedMember, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unowned leader detail status=%d", status)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/leader-workspace/teams/"+teamID+"?from=2026-08-11T00%3A00%3A00Z&to=2026-09-11T00%3A00%3A00Z", http.MethodGet, unlinkedMember, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unowned leader team detail status=%d", status)
	}
	profileBody := map[string]any{"skills": []string{"hospitality"}, "preferredTeamIds": []string{teamID}, "preferredPositionIds": []string{positionID}, "status": "active", "eligibility": map[string]any{"backgroundCheckStatus": "not-required"}}
	status, volunteerProfile := integrationJSON(t, server.URL+"/api/chms/v1/people/"+guestID+"/volunteer-profile", http.MethodPut, token, profileBody)
	if status != http.StatusOK || volunteerProfile["version"].(float64) != 1 {
		t.Fatalf("volunteer profile status=%d body=%v", status, volunteerProfile)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/people/"+guestID+"/volunteer-profile", http.MethodGet, viewer, nil)
	if status != http.StatusNotFound {
		t.Fatalf("viewer restricted volunteer profile status=%d", status)
	}
	availabilityBody := map[string]any{"startsAt": "2026-08-20T08:00:00Z", "endsAt": "2026-08-20T18:00:00Z", "state": "preferred", "source": "member"}
	status, availability := integrationJSON(t, server.URL+"/api/chms/v1/people/"+guestID+"/availability", http.MethodPost, token, availabilityBody)
	if status != http.StatusCreated || availability["state"] != "preferred" {
		t.Fatalf("volunteer availability status=%d body=%v", status, availability)
	}
	status, availabilityList := integrationJSON(t, server.URL+"/api/chms/v1/people/"+guestID+"/availability?from=2026-08-19T00%3A00%3A00Z&to=2026-08-22T00%3A00%3A00Z", http.MethodGet, viewer, nil)
	if status != http.StatusOK || len(availabilityList["items"].([]any)) != 1 {
		t.Fatalf("volunteer availability list status=%d body=%v", status, availabilityList)
	}
	attendanceBody := map[string]any{"status": "present", "source": "roster", "confidence": 90, "guest": true}
	status, attendance := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/attendance/"+guestID, http.MethodPut, token, attendanceBody)
	if status != http.StatusCreated || attendance["version"].(float64) != 1 || attendance["guest"] != true {
		t.Fatalf("attendance create status=%d body=%v", status, attendance)
	}
	attendanceBody["expectedVersion"] = 1
	attendanceBody["source"] = "operator"
	attendanceBody["confidence"] = 100
	status, attendance = integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/attendance/"+guestID, http.MethodPut, token, attendanceBody)
	if status != http.StatusOK || attendance["version"].(float64) != 2 {
		t.Fatalf("attendance update status=%d body=%v", status, attendance)
	}
	status, attendanceList := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/attendance", http.MethodGet, token, nil)
	if status != http.StatusOK || len(attendanceList["items"].([]any)) != 1 {
		t.Fatalf("attendance list status=%d body=%v", status, attendanceList)
	}
	status, attendanceEvents := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/attendance/"+guestID+"/events", http.MethodGet, token, nil)
	if status != http.StatusOK || len(attendanceEvents["items"].([]any)) != 2 {
		t.Fatalf("attendance events status=%d body=%v", status, attendanceEvents)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/attendance/missing-person", http.MethodPut, token, attendanceBody)
	if status != http.StatusNotFound {
		t.Fatalf("unknown person attendance status=%d", status)
	}
	headcountBody := map[string]any{"category": "main auditorium", "count": 475, "source": "operator", "confidence": 95, "observedAt": "2026-08-16T11:00:00Z"}
	status, headcount := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/headcounts", http.MethodPost, token, headcountBody)
	if status != http.StatusCreated || headcount["count"].(float64) != 475 {
		t.Fatalf("headcount create status=%d body=%v", status, headcount)
	}
	headcountID := headcount["id"].(string)
	headcountBody["expectedVersion"] = 1
	headcountBody["count"] = 481
	headcountBody["reason"] = "Reconciled overflow seating"
	status, headcount = integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/headcounts/"+headcountID, http.MethodPatch, token, headcountBody)
	if status != http.StatusOK || headcount["version"].(float64) != 2 || headcount["count"].(float64) != 481 {
		t.Fatalf("headcount correction status=%d body=%v", status, headcount)
	}
	status, headcountList := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/headcounts", http.MethodGet, token, nil)
	if status != http.StatusOK || len(headcountList["items"].([]any)) != 1 {
		t.Fatalf("headcount list status=%d body=%v", status, headcountList)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/attendance/"+guestID, http.MethodPut, viewer, attendanceBody)
	if status != http.StatusNotFound {
		t.Fatalf("viewer attendance write status=%d", status)
	}
	status, cancelledOccurrence := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+occurrenceID+"/cancel", http.MethodPost, token, map[string]any{"expectedVersion": 2, "reason": "Severe weather advisory"})
	if status != http.StatusOK || cancelledOccurrence["status"] != "cancelled" {
		t.Fatalf("occurrence cancel status=%d body=%v", status, cancelledOccurrence)
	}
	pastOccurrenceBody := map[string]any{"homeBranchId": "branch-2", "serviceDefinitionId": definitionID, "name": "Completed Service", "startsAt": "2026-08-09T09:00:00Z", "endsAt": "2026-08-09T11:00:00Z", "timezone": "Africa/Accra", "roomIds": []string{"main-hall"}, "capacity": 500}
	status, pastOccurrence := integrationJSON(t, server.URL+"/api/chms/v1/occurrences", http.MethodPost, token, pastOccurrenceBody)
	if status != http.StatusCreated {
		t.Fatalf("past occurrence status=%d body=%v", status, pastOccurrence)
	}
	pastOccurrenceID := pastOccurrence["id"].(string)
	lateAttendance := map[string]any{"status": "present", "source": "operator", "confidence": 100, "guest": true, "reason": "Roster entered after service close"}
	status, pastAttendance := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+pastOccurrenceID+"/attendance/"+guestID, http.MethodPut, token, lateAttendance)
	if status != http.StatusCreated {
		t.Fatalf("past attendance status=%d body=%v", status, pastAttendance)
	}
	analyticsURL := server.URL + "/api/chms/v1/attendance-analytics?branchId=branch-2&from=2026-08-01T00:00:00Z&to=2026-08-11T23:59:59Z"
	status, analytics := integrationJSON(t, analyticsURL, http.MethodGet, viewer, nil)
	if status != http.StatusOK || analytics["summary"].(map[string]any)["uniqueNamedPeople"].(float64) != 1 || analytics["quality"].(map[string]any)["namedCoveragePercent"].(float64) != 100 {
		t.Fatalf("attendance analytics status=%d body=%v", status, analytics)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/attendance-analytics?branchId=branch-2&from=invalid&to=2026-08-11T23:59:59Z", http.MethodGet, token, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid analytics range status=%d", status)
	}
	status, lock := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+pastOccurrenceID+"/lock-attendance", http.MethodPost, token, map[string]any{"reason": "Attendance reviewed by service lead"})
	if status != http.StatusCreated || lock["occurrenceId"] != pastOccurrenceID {
		t.Fatalf("attendance lock status=%d body=%v", status, lock)
	}
	lateAttendance["expectedVersion"] = 1
	lateAttendance["status"] = "excused"
	lateAttendance["reason"] = "Ordinary write should be rejected"
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+pastOccurrenceID+"/attendance/"+guestID, http.MethodPut, token, lateAttendance)
	if status != http.StatusConflict {
		t.Fatalf("locked ordinary write status=%d", status)
	}
	correction := map[string]any{"personId": guestID, "expectedVersion": 1, "status": "excused", "source": "operator", "confidence": 100, "guest": true, "reason": "Approved correction from signed roster"}
	status, corrected := integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+pastOccurrenceID+"/attendance-corrections", http.MethodPost, token, correction)
	if status != http.StatusOK || corrected["version"].(float64) != 2 || corrected["status"] != "excused" {
		t.Fatalf("approved correction status=%d body=%v", status, corrected)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/occurrences/"+pastOccurrenceID+"/attendance-corrections", http.MethodPost, viewer, correction)
	if status != http.StatusNotFound {
		t.Fatalf("viewer approved correction status=%d", status)
	}
	periodClose := map[string]any{"branchId": "branch-2", "startsAt": "2026-08-01T00:00:00Z", "endsAt": "2026-08-10T00:00:00Z", "reason": "Completed service period reconciled"}
	status, closedPeriod := integrationJSON(t, server.URL+"/api/chms/v1/attendance-periods/close", http.MethodPost, token, periodClose)
	if status != http.StatusCreated || closedPeriod["branchId"] != "branch-2" {
		t.Fatalf("period close status=%d body=%v", status, closedPeriod)
	}
	status, periodList := integrationJSON(t, server.URL+"/api/chms/v1/attendance-periods?branchId=branch-2", http.MethodGet, token, nil)
	if status != http.StatusOK || len(periodList["items"].([]any)) != 1 {
		t.Fatalf("period list status=%d body=%v", status, periodList)
	}
	stationStart := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	stationOccurrenceBody := map[string]any{"homeBranchId": "branch-2", "serviceDefinitionId": definitionID, "name": "Station Test Service", "startsAt": stationStart.Format(time.RFC3339), "endsAt": stationStart.Add(2 * time.Hour).Format(time.RFC3339), "timezone": "Africa/Accra", "roomIds": []string{"foyer"}, "capacity": 300}
	status, stationOccurrence := integrationJSON(t, server.URL+"/api/chms/v1/occurrences", http.MethodPost, token, stationOccurrenceBody)
	if status != http.StatusCreated {
		t.Fatalf("station occurrence status=%d body=%v", status, stationOccurrence)
	}
	stationOccurrenceID := stationOccurrence["id"].(string)
	sessionBody := map[string]any{"branchId": "branch-2", "deviceLabel": "Foyer tablet", "occurrenceIds": []string{stationOccurrenceID}, "expiresAt": time.Now().UTC().Add(8 * time.Hour).Format(time.RFC3339)}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions", http.MethodPost, viewer, sessionBody)
	if status != http.StatusForbidden {
		t.Fatalf("viewer session create status=%d", status)
	}
	status, session := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions", http.MethodPost, token, sessionBody)
	if status != http.StatusCreated || session["state"] != "active" {
		t.Fatalf("session create status=%d body=%v", status, session)
	}
	sessionID := session["id"].(string)
	status, householdLookup := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/households?q=Kojo", http.MethodGet, token, nil)
	if status != http.StatusOK || len(householdLookup["items"].([]any)) == 0 {
		t.Fatalf("check-in household lookup status=%d body=%v", status, householdLookup)
	}
	quickGuest := map[string]any{"occurrenceId": stationOccurrenceID, "names": map[string]any{"given": "Efua", "family": "Visitor"}, "contactPoints": []any{map[string]any{"type": "mobile", "value": "0249998877", "primary": true}}, "capturedAt": stationStart.Format(time.RFC3339)}
	status, quickGuestResult := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/guests", http.MethodPost, token, quickGuest)
	if status != http.StatusCreated || quickGuestResult["person"] == nil || quickGuestResult["attendance"] == nil {
		t.Fatalf("quick guest status=%d body=%v", status, quickGuestResult)
	}
	status, duplicateGuest := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/guests", http.MethodPost, token, quickGuest)
	if status != http.StatusConflict || duplicateGuest["error"] == nil {
		t.Fatalf("duplicate quick guest status=%d body=%v", status, duplicateGuest)
	}
	command := map[string]any{"clientCommandId": "http-device-command-0001", "localSequence": 1, "type": "check-in", "occurrenceId": stationOccurrenceID, "personId": guestID, "expectedVersion": 0, "capturedAt": stationStart.Format(time.RFC3339), "guest": true, "confidence": 100}
	status, syncResult := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/commands:sync", http.MethodPost, token, map[string]any{"commands": []any{command}})
	results := syncResult["results"].([]any)
	if status != http.StatusOK || results[0].(map[string]any)["classification"] != "applied" {
		t.Fatalf("command sync status=%d body=%v", status, syncResult)
	}
	status, replayResult := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/commands:sync", http.MethodPost, token, map[string]any{"commands": []any{command}})
	if status != http.StatusOK || replayResult["results"].([]any)[0].(map[string]any)["classification"] != "duplicate" {
		t.Fatalf("command replay status=%d body=%v", status, replayResult)
	}
	reused := map[string]any{}
	for key, value := range command {
		reused[key] = value
	}
	reused["type"] = "mark"
	reused["status"] = "absent"
	status, reusedResult := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/commands:sync", http.MethodPost, token, map[string]any{"commands": []any{reused}})
	if status != http.StatusOK || reusedResult["results"].([]any)[0].(map[string]any)["code"] != "idempotency_reused" {
		t.Fatalf("command ID reuse status=%d body=%v", status, reusedResult)
	}
	childCreate := map[string]any{"homeBranchId": "branch-2", "names": map[string]any{"given": "Nana", "family": "Child"}, "contactPoints": []any{}, "addresses": []any{}, "membershipStage": "guest", "tags": []string{"children"}, "customFields": map[string]string{}, "communicationPreferences": map[string]bool{}, "source": map[string]string{"type": "staff-entry"}}
	status, child := integrationJSON(t, server.URL+"/api/chms/v1/people", http.MethodPost, token, childCreate)
	if status != http.StatusCreated {
		t.Fatalf("child create status=%d body=%v", status, child)
	}
	childID := child["id"].(string)
	guardianCreate := map[string]any{"homeBranchId": "branch-2", "names": map[string]any{"given": "Akosua", "family": "Guardian"}, "contactPoints": []any{}, "addresses": []any{}, "membershipStage": "guest", "tags": []string{}, "customFields": map[string]string{}, "communicationPreferences": map[string]bool{}, "source": map[string]string{"type": "staff-entry"}}
	status, guardian := integrationJSON(t, server.URL+"/api/chms/v1/people", http.MethodPost, token, guardianCreate)
	if status != http.StatusCreated {
		t.Fatalf("guardian create status=%d body=%v", status, guardian)
	}
	guardianID := guardian["id"].(string)
	authorizationBody := map[string]any{"branchId": "branch-2", "childPersonId": childID, "guardianPersonId": guardianID, "relationship": "parent", "source": "household-review"}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/guardian-authorizations", http.MethodPost, viewer, authorizationBody)
	if status != http.StatusForbidden {
		t.Fatalf("viewer guardian authorization status=%d", status)
	}
	status, authorization := integrationJSON(t, server.URL+"/api/chms/v1/guardian-authorizations", http.MethodPost, token, authorizationBody)
	if status != http.StatusCreated || authorization["status"] != "active" {
		t.Fatalf("guardian authorization status=%d body=%v", status, authorization)
	}
	childCheckinBody := map[string]any{"occurrenceId": stationOccurrenceID, "childPersonId": childID, "guardianPersonId": guardianID, "capturedAt": stationStart.Format(time.RFC3339)}
	status, childCheckinResponse := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/children", http.MethodPost, token, childCheckinBody)
	if status != http.StatusCreated {
		t.Fatalf("child check-in status=%d body=%v", status, childCheckinResponse)
	}
	childCheckin := childCheckinResponse["checkin"].(map[string]any)
	label := childCheckinResponse["label"].(map[string]any)
	securityCode := label["securityCode"].(string)
	childAttendanceID := label["attendanceId"].(string)
	if len(securityCode) != 6 || childCheckin["codeHash"] != nil {
		t.Fatalf("unsafe child label response: %v", childCheckinResponse)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/checkin/"+childAttendanceID+"/pickup", http.MethodPost, token, map[string]any{"guardianPersonId": guestID, "securityCode": securityCode})
	if status != http.StatusForbidden {
		t.Fatalf("unauthorized guardian pickup status=%d", status)
	}
	status, deniedPickup := integrationJSON(t, server.URL+"/api/chms/v1/checkin/"+childAttendanceID+"/pickup", http.MethodPost, token, map[string]any{"guardianPersonId": guardianID, "securityCode": "AAAAAA"})
	if status != http.StatusForbidden || deniedPickup["securityCode"] != nil {
		t.Fatalf("guessed pickup status=%d body=%v", status, deniedPickup)
	}
	status, incident := integrationJSON(t, server.URL+"/api/chms/v1/child-checkins/"+childCheckin["id"].(string)+"/incidents", http.MethodPost, token, map[string]any{"category": "identity concern", "summary": "Guardian presented an unreadable label and identity was rechecked."})
	if status != http.StatusCreated || incident["status"] != "open" {
		t.Fatalf("incident status=%d body=%v", status, incident)
	}
	status, pickupReceipt := integrationJSON(t, server.URL+"/api/chms/v1/checkin/"+childAttendanceID+"/pickup", http.MethodPost, token, map[string]any{"guardianPersonId": guardianID, "securityCode": securityCode})
	if status != http.StatusOK || pickupReceipt["securityCode"] != nil || pickupReceipt["attendanceId"] != childAttendanceID {
		t.Fatalf("pickup status=%d body=%v", status, pickupReceipt)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/checkin/"+childAttendanceID+"/pickup", http.MethodPost, token, map[string]any{"guardianPersonId": guardianID, "securityCode": securityCode})
	if status != http.StatusForbidden {
		t.Fatalf("replayed pickup status=%d", status)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/child-checkins", http.MethodGet, token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("child roster-like route unexpectedly exposed status=%d", status)
	}
	status, lockedStation := integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/lock", http.MethodPost, token, map[string]any{"reason": "Station shift completed"})
	if status != http.StatusOK || lockedStation["state"] != "locked" {
		t.Fatalf("session lock status=%d body=%v", status, lockedStation)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/households?q=Kojo", http.MethodGet, token, nil)
	if status != http.StatusConflict {
		t.Fatalf("locked station household lookup status=%d", status)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/guests", http.MethodPost, token, quickGuest)
	if status != http.StatusConflict {
		t.Fatalf("locked station guest intake status=%d", status)
	}
	command["clientCommandId"] = "http-device-command-0002"
	command["localSequence"] = 2
	command["expectedVersion"] = 1
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/checkin/sessions/"+sessionID+"/commands:sync", http.MethodPost, token, map[string]any{"commands": []any{command}})
	if status != http.StatusConflict {
		t.Fatalf("locked station sync status=%d", status)
	}
}

func TestWorkflowHTTPAuthorizationAndLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	db := client.Database("remi_workflow_http_test")
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_ = db.Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})
	repo, _ := care.NewMongoRepository(db)
	if err = repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repo.EnsureCaseIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	attendanceRepo, _ := participation.NewMongoRepository(db)
	if err = attendanceRepo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	authorizer := platform.GrantAuthorizer{}
	careKey := make([]byte, 32)
	for index := range careKey {
		careKey[index] = byte(index + 1)
	}
	careCipher, _ := platform.NewEnvelopeCipher("http-care-v1", careKey)
	h := New(Services{Care: care.Service{Store: repo, Cases: repo, Platform: integrationPlatform{}, Attendance: attendanceRepo, Cipher: careCipher, Authorizer: authorizer}, Authorizer: authorizer}, "org-test")
	jwt := services.NewJWTService("integration-http-secret-with-at-least-32-bytes")
	admin, _ := jwt.Generate("admin-1", "admin@example.com", "Admin", "super-admin")
	viewer, _ := jwt.Generate("viewer-1", "viewer@example.com", "Viewer", "viewer")
	router := chi.NewRouter()
	router.With(middleware.Auth(jwt)).Mount("/api/chms/v1", h.Routes())
	server := httptest.NewServer(router)
	defer server.Close()
	definitionBody := map[string]any{"branchId": "branch-1", "name": "First visit follow-up", "purpose": "visitor-assimilation", "initialStageKey": "new", "stages": []any{map[string]any{"key": "new", "name": "New", "slaMinutes": 1440}, map[string]any{"key": "contacted", "name": "Contacted", "slaMinutes": 2880}, map[string]any{"key": "closed", "name": "Closed", "terminal": true, "outcomeRequired": true}}, "transitions": []any{map[string]any{"from": "new", "to": "contacted", "name": "Record contact"}, map[string]any{"from": "contacted", "to": "closed", "name": "Close"}}, "tasks": []any{map[string]any{"key": "welcome-call", "stageKey": "new", "title": "Make welcome call", "dueMinutes": 720}}}
	status, _ := integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions", http.MethodPost, viewer, definitionBody)
	if status != http.StatusForbidden {
		t.Fatalf("viewer workflow create status=%d", status)
	}
	status, definition := integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions", http.MethodPost, admin, definitionBody)
	if status != http.StatusCreated {
		t.Fatalf("definition status=%d body=%v", status, definition)
	}
	definitionID := definition["id"].(string)
	status, published := integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions/"+definitionID+"/publish", http.MethodPost, admin, map[string]any{"expectedVersion": 1})
	if status != http.StatusOK || published["status"] != "published" {
		t.Fatalf("publish status=%d body=%v", status, published)
	}
	status, instance := integrationJSON(t, server.URL+"/api/chms/v1/workflow-instances", http.MethodPost, admin, map[string]any{"definitionId": definitionID, "subjectType": "person", "subjectId": "person-1", "ownerId": "pastor-1"})
	if status != http.StatusCreated {
		t.Fatalf("instance status=%d body=%v", status, instance)
	}
	instanceID := instance["id"].(string)
	status, detail := integrationJSON(t, server.URL+"/api/chms/v1/workflow-instances/"+instanceID, http.MethodGet, admin, nil)
	if status != http.StatusOK || len(detail["tasks"].([]any)) != 1 {
		t.Fatalf("detail status=%d body=%v", status, detail)
	}
	status, transitioned := integrationJSON(t, server.URL+"/api/chms/v1/workflow-instances/"+instanceID+"/transitions", http.MethodPost, admin, map[string]any{"expectedVersion": 1, "toStageKey": "contacted", "automationKey": "http-first-visit/person-1"})
	if status != http.StatusOK || transitioned["replayed"] != false {
		t.Fatalf("transition status=%d body=%v", status, transitioned)
	}
	status, replayed := integrationJSON(t, server.URL+"/api/chms/v1/workflow-instances/"+instanceID+"/transitions", http.MethodPost, admin, map[string]any{"expectedVersion": 1, "toStageKey": "contacted", "automationKey": "http-first-visit/person-1"})
	if status != http.StatusOK || replayed["replayed"] != true {
		t.Fatalf("replay status=%d body=%v", status, replayed)
	}
	assimilationBody := map[string]any{"branchId": "branch-1", "name": "Visitor pathway", "purpose": "visitor-assimilation", "initialStageKey": "first-visit", "stages": []any{map[string]any{"key": "first-visit", "name": "First visit", "slaMinutes": 1440}, map[string]any{"key": "second-visit", "name": "Second visit", "slaMinutes": 2880}, map[string]any{"key": "completed", "name": "Completed", "terminal": true, "outcomeRequired": true}}, "transitions": []any{map[string]any{"from": "first-visit", "to": "second-visit", "name": "Returned"}, map[string]any{"from": "second-visit", "to": "completed", "name": "Close"}}, "tasks": []any{map[string]any{"key": "welcome-call", "stageKey": "first-visit", "title": "Make welcome call", "dueMinutes": 720}}}
	status, assimilationDefinition := integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions", http.MethodPost, admin, assimilationBody)
	if status != http.StatusCreated {
		t.Fatalf("assimilation definition status=%d body=%v", status, assimilationDefinition)
	}
	assimilationID := assimilationDefinition["id"].(string)
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions/"+assimilationID+"/publish", http.MethodPost, admin, map[string]any{"expectedVersion": 1})
	if status != http.StatusOK {
		t.Fatalf("assimilation publish status=%d", status)
	}
	_, _ = db.Collection("chms_attendance").InsertOne(ctx, map[string]any{"_id": "attendance-http", "organizationId": "org-test", "branchId": "branch-1", "version": 1, "occurrenceId": "occurrence-http", "personId": "visitor-http", "status": "present", "source": "operator", "confidence": 100, "guest": true, "createdAt": time.Now().UTC(), "updatedAt": time.Now().UTC()})
	trigger := map[string]any{"definitionId": assimilationID, "occurrenceId": "occurrence-http", "personId": "visitor-http", "ownerId": "pastor-http"}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/assimilation/attendance-triggers", http.MethodPost, viewer, trigger)
	if status != http.StatusNotFound {
		t.Fatalf("viewer assimilation trigger status=%d", status)
	}
	status, triggered := integrationJSON(t, server.URL+"/api/chms/v1/assimilation/attendance-triggers", http.MethodPost, admin, trigger)
	if status != http.StatusOK || triggered["replayed"] != false {
		t.Fatalf("assimilation trigger status=%d body=%v", status, triggered)
	}
	careDefinitionBody := map[string]any{"branchId": "branch-1", "name": "Pastoral care plan", "purpose": "pastoral-care", "initialStageKey": "open", "stages": []any{map[string]any{"key": "open", "name": "Open", "slaMinutes": 1440}, map[string]any{"key": "closed", "name": "Closed", "terminal": true, "outcomeRequired": true}}, "transitions": []any{map[string]any{"from": "open", "to": "closed", "name": "Close care"}}, "tasks": []any{map[string]any{"key": "initial-contact", "stageKey": "open", "title": "Make initial contact", "dueMinutes": 120}}}
	status, careDefinition := integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions", http.MethodPost, admin, careDefinitionBody)
	if status != http.StatusCreated {
		t.Fatalf("care definition status=%d body=%v", status, careDefinition)
	}
	careDefinitionID := careDefinition["id"].(string)
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/workflow-definitions/"+careDefinitionID+"/publish", http.MethodPost, admin, map[string]any{"expectedVersion": 1})
	if status != http.StatusOK {
		t.Fatalf("care definition publish status=%d", status)
	}
	careBody := map[string]any{"branchId": "branch-1", "ministryId": "pastoral-care", "personId": "visitor-http", "category": "pastoral-care", "urgency": "priority", "consent": map[string]any{"state": "granted", "source": "member-request", "noticeVersion": "care-2026-01", "capturedAt": time.Now().UTC().Format(time.RFC3339)}, "assignedUserIds": []string{"admin-1"}, "workflowDefinitionId": careDefinitionID}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/care-cases", http.MethodPost, viewer, careBody)
	if status != http.StatusForbidden {
		t.Fatalf("viewer care create status=%d", status)
	}
	status, careCase := integrationJSON(t, server.URL+"/api/chms/v1/care-cases", http.MethodPost, admin, careBody)
	if status != http.StatusCreated {
		t.Fatalf("care create status=%d body=%v", status, careCase)
	}
	caseID := careCase["id"].(string)
	workflowInstanceID := careCase["workflowInstanceId"].(string)
	status, careWorkflow := integrationJSON(t, server.URL+"/api/chms/v1/workflow-instances/"+workflowInstanceID, http.MethodGet, admin, nil)
	if status != http.StatusOK || len(careWorkflow["tasks"].([]any)) != 1 {
		t.Fatalf("care workflow status=%d body=%v", status, careWorkflow)
	}
	privateContent := "Private HTTP pastoral note"
	status, note := integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/notes", http.MethodPost, admin, map[string]any{"content": privateContent, "classification": "pastoral-restricted", "authorizedGroup": "assigned-care-team"})
	if status != http.StatusCreated || note["content"] != privateContent {
		t.Fatalf("note create status=%d body=%v", status, note)
	}
	scopedPastor, _ := jwt.GenerateScoped("admin-1", "pastor@example.com", "Assigned Pastor", "pastor", []string{"branch-1"}, []string{"pastoral-care"}, []string{caseID}, 0)
	wrongMinistryPastor, _ := jwt.GenerateScoped("admin-1", "pastor@example.com", "Assigned Pastor", "pastor", []string{"branch-1"}, []string{"youth"}, []string{caseID}, 0)
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID, http.MethodGet, wrongMinistryPastor, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-ministry assigned pastor detail status=%d", status)
	}
	status, scopedNotes := integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/notes", http.MethodGet, scopedPastor, nil)
	if status != http.StatusOK || len(scopedNotes["items"].([]any)) != 1 {
		t.Fatalf("assigned scoped pastor notes status=%d body=%v", status, scopedNotes)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/notes", http.MethodGet, wrongMinistryPastor, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-ministry restricted note status=%d", status)
	}
	status, caseList := integrationJSON(t, server.URL+"/api/chms/v1/care-cases?branchId=branch-1", http.MethodGet, admin, nil)
	encodedList, _ := json.Marshal(caseList)
	if status != http.StatusOK || bytes.Contains(encodedList, []byte(privateContent)) {
		t.Fatalf("care list leaked note status=%d body=%s", status, encodedList)
	}
	status, notes := integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/notes", http.MethodGet, admin, nil)
	if status != http.StatusOK || len(notes["items"].([]any)) != 1 {
		t.Fatalf("notes read status=%d body=%v", status, notes)
	}
	status, _ = integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/notes", http.MethodGet, viewer, nil)
	if status != http.StatusNotFound {
		t.Fatalf("viewer note read status=%d", status)
	}
	status, contact := integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/contact-events", http.MethodPost, admin, map[string]any{"channel": "phone", "purpose": "Pastoral follow-up", "outcome": "reached", "summary": "Meeting agreed."})
	if status != http.StatusCreated || contact["outcome"] != "reached" {
		t.Fatalf("contact status=%d body=%v", status, contact)
	}
	status, closed := integrationJSON(t, server.URL+"/api/chms/v1/care-cases/"+caseID+"/close", http.MethodPost, admin, map[string]any{"expectedVersion": 1, "outcome": "care-completed", "reason": "Care plan completed with consent"})
	if status != http.StatusOK || closed["state"] != "closed" {
		t.Fatalf("close status=%d body=%v", status, closed)
	}
	status, careWorkflow = integrationJSON(t, server.URL+"/api/chms/v1/workflow-instances/"+workflowInstanceID, http.MethodGet, admin, nil)
	if status != http.StatusOK || careWorkflow["instance"].(map[string]any)["state"] != "completed" || careWorkflow["tasks"].([]any)[0].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("closed care workflow status=%d body=%v", status, careWorkflow)
	}
}

func integrationJSON(t *testing.T, url, method, token string, body any) (int, map[string]any) {
	return integrationJSONWithKey(t, url, method, token, "", body)
}
func integrationJSONWithKey(t *testing.T, url, method, token, key string, body any) (int, map[string]any) {
	t.Helper()
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, url, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(response.Body).Decode(&out)
	return response.StatusCode, out
}
