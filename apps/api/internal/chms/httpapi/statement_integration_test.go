package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/platform"
	"remi-api/internal/middleware"
	"remi-api/internal/services"
	"remi-api/internal/testsupport"
)

type statementHTTPMailer struct{ calls int }

func (m *statementHTTPMailer) SendAttachment(_, _, _, _ string, _ []byte) error {
	m.calls++
	return nil
}

func TestStatementAndReceiptHTTPStaffMemberBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
	if err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	if err = client.Ping(ctx, nil); err != nil {
		testsupport.SkipOrFail(t, "MongoDB", err)
	}
	database := client.Database("remi_statement_http_test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		drop, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = database.Drop(drop)
		_ = client.Disconnect(drop)
	})
	repository, _ := finance.NewRepository(database)
	if err = repository.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	store, _ := platform.NewMongoPlatformStore(database)
	if err = store.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	personID := platform.ID("statement-http-person")
	contributionID := platform.ID("statement-http-contribution")
	fundID := platform.ID("statement-http-fund")
	for collection, document := range map[string]any{
		"chms_people":                bson.M{"_id": personID, "organizationId": "org-test", "homeBranchId": "branch-1", "archivedAt": nil, "names": bson.M{"given": "Ama", "family": "Mensah", "preferred": "Ama"}},
		"chms_finance_funds":         bson.M{"_id": fundID, "organizationId": "org-test", "name": "Tithes", "code": "TITHE", "restrictionType": "unrestricted", "activeFrom": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		"chms_finance_contributions": bson.M{"_id": contributionID, "organizationId": "org-test", "branchId": "branch-1", "schemaVersion": 1, "version": 1, "receiptNumber": "REMI-2026-000001", "receivedAt": time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), "postedAt": now, "donor": bson.M{"type": "person", "personId": personID}, "source": "online", "total": bson.M{"amountMinor": int64(25000), "currency": "GHS"}, "splits": bson.A{bson.M{"fundId": fundID, "amount": bson.M{"amountMinor": int64(25000), "currency": "GHS"}}}, "state": "posted"},
		"chms_member_accounts":       bson.M{"_id": "statement-http-account", "organizationId": "org-test", "personId": personID, "email": "ama@example.com", "status": "active", "emailVerifiedAt": now},
	} {
		if _, err = database.Collection(collection).InsertOne(ctx, document); err != nil {
			t.Fatalf("insert %s: %v", collection, err)
		}
	}
	mailer := &statementHTTPMailer{}
	authorizer := platform.GrantAuthorizer{}
	service := finance.Service{Repository: repository, Platform: store, Authorizer: authorizer, Mailer: mailer, Now: func() time.Time { return now }}
	handler := New(Services{Finance: service, Authorizer: authorizer}, "org-test")
	jwt := services.NewJWTService("statement-http-secret-with-at-least-32-bytes")
	staff, _ := jwt.GenerateMFAAuthenticated("finance-admin", "finance@example.com", "Finance", "super-admin", now)
	member, _ := jwt.GenerateMember("statement-http-account", string(personID), "statement-session", "", "ama@example.com", "Ama")
	other, _ := jwt.GenerateMember("other-account", "other-person", "other-session", "", "other@example.com", "Other")
	router := chi.NewRouter()
	router.With(middleware.Auth(jwt)).Mount("/api/chms/v1", handler.Routes())
	router.With(middleware.Auth(jwt)).Mount("/api/member/chms", handler.MemberRoutes())
	server := httptest.NewServer(router)
	defer server.Close()

	status, statement := statementJSON(t, server.URL+"/api/chms/v1/finance/statements", http.MethodPost, staff, "", map[string]any{"subjectType": "person", "subjectId": personID, "branchId": "branch-1", "year": 2026})
	if status != http.StatusCreated || statement["totalAmountMinor"] != float64(25000) {
		t.Fatalf("staff statement status=%d body=%v", status, statement)
	}
	statementID := statement["id"].(string)
	status, list := statementJSON(t, server.URL+"/api/member/chms/finance/statements", http.MethodGet, member, "", nil)
	if status != http.StatusOK || len(list["items"].([]any)) != 1 {
		t.Fatalf("member statements status=%d body=%v", status, list)
	}
	status, _ = statementJSON(t, server.URL+"/api/member/chms/finance/statements/"+statementID, http.MethodGet, other, "", nil)
	if status != http.StatusNotFound {
		t.Fatalf("other member statement status=%d", status)
	}
	status, headers, content := statementBinary(t, server.URL+"/api/member/chms/finance/statements/"+statementID+"/pdf", member)
	if status != http.StatusOK || headers.Get("Content-Type") != "application/pdf" || headers.Get("Cache-Control") != "private, no-store" || !bytes.HasPrefix(content, []byte("%PDF-1.4")) {
		t.Fatalf("statement PDF status=%d headers=%v prefix=%q", status, headers, content[:min(len(content), 8)])
	}
	status, receipts := statementJSON(t, server.URL+"/api/member/chms/finance/receipts?year=2026", http.MethodGet, member, "", nil)
	if status != http.StatusOK || len(receipts["items"].([]any)) != 1 {
		t.Fatalf("member receipts status=%d body=%v", status, receipts)
	}
	status, _ = statementJSON(t, server.URL+"/api/member/chms/finance/statements/"+statementID+"/deliveries", http.MethodPost, member, "statement-http-delivery", map[string]any{})
	if status != http.StatusOK || mailer.calls != 1 {
		t.Fatalf("statement delivery status=%d calls=%d", status, mailer.calls)
	}
	status, _ = statementJSON(t, server.URL+"/api/member/chms/finance/statements/"+statementID+"/deliveries", http.MethodPost, member, "statement-http-delivery", map[string]any{})
	if status != http.StatusOK || mailer.calls != 1 {
		t.Fatalf("statement delivery replay status=%d calls=%d", status, mailer.calls)
	}
}

func statementJSON(t *testing.T, url, method, token, key string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequest(method, url, reader)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result := map[string]any{}
	_ = json.NewDecoder(response.Body).Decode(&result)
	return response.StatusCode, result
}

func statementBinary(t *testing.T, url, token string) (int, http.Header, []byte) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, url, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, response.Header, content
}
