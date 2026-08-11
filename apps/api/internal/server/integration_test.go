// Integration tests for the REMI API. They run the real router against a
// dedicated remi_test database on a real MongoDB (dropped after the run).
//
// Without MongoDB they skip; with REQUIRE_INFRA=1 they fail instead, so CI
// cannot go green having run nothing. Locally: `make up` first.
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"remi-api/internal/config"
	"remi-api/internal/handlers"
	"remi-api/internal/models"
	"remi-api/internal/server"
	"remi-api/internal/testsupport"
)

const testDBName = "remi_test"

var (
	connOnce  sync.Once
	connErr   error
	testMongo *mongo.Client
	testDB    *mongo.Database
)

func TestMain(m *testing.M) {
	code := m.Run()
	if testMongo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = testDB.Drop(ctx)
		_ = testMongo.Disconnect(ctx)
	}
	os.Exit(code)
}

// setup connects to MongoDB (once per run) and returns the test database and
// a server running the real route table against it.
func setup(t *testing.T) (*mongo.Database, *httptest.Server) {
	t.Helper()
	connOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := mongo.Connect(options.Client().ApplyURI(testsupport.MongoURI()))
		if err != nil {
			connErr = err
			return
		}
		if err := client.Ping(ctx, nil); err != nil {
			connErr = fmt.Errorf("ping mongo: %w", err)
			return
		}
		testMongo = client
		testDB = client.Database(testDBName)
	})
	if connErr != nil {
		testsupport.SkipOrFail(t, "MongoDB", connErr)
	}

	cfg := &config.Config{
		Port:        "0",
		JWTSecret:   "integration-test-secret",
		CORSOrigins: []string{"http://localhost:3010"},
		EmailFrom:   "REMI Test <test@remi.church>",
		NotifyEmail: "pastor@remi.church",
		AdminAppURL: "http://localhost:3011",
		// No Paystack key: giving runs in demo mode.
	}
	h := handlers.New(testDB, cfg)
	srv := httptest.NewServer(server.NewRouter(h, cfg.CORSOrigins))
	t.Cleanup(srv.Close)
	return testDB, srv
}

// do issues one JSON request and returns the status and decoded body.
func do(t *testing.T, method, url, token string, body any) (int, bson.M) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	out := bson.M{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// doList issues a GET and decodes the bare-array response the list endpoints
// return.
func doList(t *testing.T, url, token string) (int, []bson.M) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	out := []bson.M{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestHealth(t *testing.T) {
	_, srv := setup(t)
	status, body := do(t, http.MethodGet, srv.URL+"/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("health: got %d, want 200", status)
	}
	if body["status"] != "ok" {
		t.Fatalf("health: got body %v", body)
	}
}

// seedUser upserts a login with a known password.
func seedUser(t *testing.T, db *mongo.Database, email, password, role string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	c, cancel := ctx()
	defer cancel()
	_, err = db.Collection("users").UpdateOne(c,
		bson.M{"email": email},
		bson.M{"$set": bson.M{
			"email": email, "name": "Integration " + role,
			"role": role, "passwordHash": string(hash),
		}, "$setOnInsert": bson.M{"createdAt": models.Now()}},
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func login(t *testing.T, srv *httptest.Server, email, password string) string {
	t.Helper()
	status, body := do(t, http.MethodPost, srv.URL+"/api/auth/login", "",
		bson.M{"email": email, "password": password})
	if status != http.StatusOK {
		t.Fatalf("login %s: got %d (%v), want 200", email, status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("login %s: no token in response %v", email, body)
	}
	return token
}

func TestLogin(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "editor@test.remi", "correct-horse", "editor")

	t.Run("good credentials issue a token", func(t *testing.T) {
		token := login(t, srv, "editor@test.remi", "correct-horse")
		status, body := do(t, http.MethodGet, srv.URL+"/api/auth/me", token, nil)
		if status != http.StatusOK {
			t.Fatalf("me: got %d, want 200", status)
		}
		user, _ := body["user"].(map[string]any)
		if user["email"] != "editor@test.remi" || user["role"] != "editor" {
			t.Fatalf("me: unexpected user %v", user)
		}
	})

	t.Run("bad credentials are rejected", func(t *testing.T) {
		status, body := do(t, http.MethodPost, srv.URL+"/api/auth/login", "",
			bson.M{"email": "editor@test.remi", "password": "wrong"})
		if status != http.StatusUnauthorized {
			t.Fatalf("bad password: got %d (%v), want 401", status, body)
		}
		status, _ = do(t, http.MethodPost, srv.URL+"/api/auth/login", "",
			bson.M{"email": "nobody@test.remi", "password": "whatever"})
		if status != http.StatusUnauthorized {
			t.Fatalf("unknown user: got %d, want 401", status)
		}
	})
}

func TestAdminInvitationRoundTrip(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "inviter@test.remi", "inviter-pass", "super-admin")
	adminToken := login(t, srv, "inviter@test.remi", "inviter-pass")

	status, invited := do(t, http.MethodPost, srv.URL+"/api/admin/users", adminToken, bson.M{
		"email": "new-teammate@test.remi", "role": "editor",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: got %d (%v), want 201", status, invited)
	}
	inviteURL, _ := invited["demoInvitationUrl"].(string)
	parts := strings.Split(inviteURL, "/")
	inviteToken := parts[len(parts)-1]
	if inviteToken == "" {
		t.Fatalf("invite: missing demo invitation URL in %v", invited)
	}

	status, invitation := do(t, http.MethodGet, srv.URL+"/api/auth/invitations/"+inviteToken, "", nil)
	if status != http.StatusOK || invitation["email"] != "new-teammate@test.remi" {
		t.Fatalf("inspect invitation: got %d (%v), want 200", status, invitation)
	}

	status, accepted := do(t, http.MethodPost, srv.URL+"/api/auth/invitations/"+inviteToken+"/accept", "", bson.M{
		"name": "New Teammate", "password": "a-secure-passphrase-2026",
	})
	if status != http.StatusOK || accepted["token"] == "" {
		t.Fatalf("accept invitation: got %d (%v), want session", status, accepted)
	}
	status, _ = do(t, http.MethodGet, srv.URL+"/api/auth/invitations/"+inviteToken, "", nil)
	if status != http.StatusGone {
		t.Fatalf("reused invitation: got %d, want 410", status)
	}
	login(t, srv, "new-teammate@test.remi", "a-secure-passphrase-2026")

	status, users := doList(t, srv.URL+"/api/admin/users", adminToken)
	if status != http.StatusOK {
		t.Fatalf("list users: got %d, want 200", status)
	}
	for _, user := range users {
		if _, leaked := user["passwordHash"]; leaked {
			t.Fatal("users API leaked passwordHash")
		}
		if _, leaked := user["invitationTokenHash"]; leaked {
			t.Fatal("users API leaked invitationTokenHash")
		}
	}
}

func insertSermon(t *testing.T, db *mongo.Database, slug, series, status string) {
	t.Helper()
	c, cancel := ctx()
	defer cancel()
	_, err := db.Collection("sermons").InsertOne(c, bson.M{
		"title": "Sermon " + slug, "slug": slug, "series": series,
		"preacher": "Test Preacher", "date": models.Now(),
		"contentStatus": status, "createdAt": models.Now(), "updatedAt": models.Now(),
	})
	if err != nil {
		t.Fatalf("insert sermon: %v", err)
	}
}

func TestSermonsListAndSeriesFilter(t *testing.T) {
	db, srv := setup(t)
	insertSermon(t, db, "it-hearing-1", "IT Hearing God", models.StatusPublished)
	insertSermon(t, db, "it-hearing-2", "IT Hearing God", models.StatusPublished)
	insertSermon(t, db, "it-prayer-1", "IT Prayer Life", models.StatusPublished)

	status, body := do(t, http.MethodGet, srv.URL+"/api/sermons", "", nil)
	if status != http.StatusOK {
		t.Fatalf("list sermons: got %d, want 200", status)
	}
	if total, _ := body["total"].(float64); total < 3 {
		t.Fatalf("list sermons: total %v, want >= 3", body["total"])
	}

	status, body = do(t, http.MethodGet, srv.URL+"/api/sermons?series=IT+Hearing+God", "", nil)
	if status != http.StatusOK {
		t.Fatalf("series filter: got %d, want 200", status)
	}
	if total, _ := body["total"].(float64); total != 2 {
		t.Fatalf("series filter: total %v, want exactly 2", body["total"])
	}
	items, _ := body["items"].([]any)
	for _, item := range items {
		m, _ := item.(map[string]any)
		if m["series"] != "IT Hearing God" {
			t.Fatalf("series filter leaked a sermon from %v", m["series"])
		}
	}
}

func TestPublicContentFiltering(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	for _, m := range []bson.M{
		{"title": "IT Visible Ministry", "slug": "it-visible-ministry", "name": "IT Visible Ministry",
			"contentStatus": models.StatusPublished, "createdAt": models.Now(), "updatedAt": models.Now()},
		{"title": "IT Draft Ministry", "slug": "it-draft-ministry", "name": "IT Draft Ministry",
			"contentStatus": models.StatusAIDraft, "createdAt": models.Now(), "updatedAt": models.Now()},
	} {
		if _, err := db.Collection("ministries").InsertOne(c, m); err != nil {
			t.Fatalf("insert ministry: %v", err)
		}
	}

	status, list := doList(t, srv.URL+"/api/ministries", "")
	if status != http.StatusOK {
		t.Fatalf("list ministries: got %d, want 200", status)
	}
	var visible, draft bool
	for _, m := range list {
		switch m["slug"] {
		case "it-visible-ministry":
			visible = true
		case "it-draft-ministry":
			draft = true
		}
	}
	if !visible {
		t.Fatal("published ministry missing from public list")
	}
	if draft {
		t.Fatal("ai-draft ministry leaked into the public list")
	}

	// The draft is also unreachable by slug.
	status, _ = do(t, http.MethodGet, srv.URL+"/api/ministries/it-draft-ministry", "", nil)
	if status != http.StatusNotFound {
		t.Fatalf("draft by slug: got %d, want 404", status)
	}
}

func TestAdminContentCRUDRoundTrip(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "crud@test.remi", "crud-pass", "editor")
	token := login(t, srv, "crud@test.remi", "crud-pass")

	// Create
	status, body := do(t, http.MethodPost, srv.URL+"/api/admin/announcements", token, bson.M{
		"title":     "IT Announcement",
		"body":      "Created by the integration test.",
		"publishAt": models.Now().Add(-time.Hour),
	})
	if status != http.StatusCreated {
		t.Fatalf("create: got %d (%v), want 201", status, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("create: no id in %v", body)
	}

	// Update
	status, body = do(t, http.MethodPut, srv.URL+"/api/admin/announcements/"+id, token, bson.M{
		"title": "IT Announcement (updated)",
	})
	if status != http.StatusOK || body["title"] != "IT Announcement (updated)" {
		t.Fatalf("update: got %d (%v), want 200 with new title", status, body)
	}

	// Status transition
	status, body = do(t, http.MethodPatch, srv.URL+"/api/admin/content/announcements/"+id+"/status", token, bson.M{
		"contentStatus": models.StatusInReview,
	})
	if status != http.StatusOK || body["contentStatus"] != models.StatusInReview {
		t.Fatalf("status transition: got %d (%v), want 200 in-review", status, body)
	}

	// While in-review it must not appear publicly.
	status, publicList := doList(t, srv.URL+"/api/announcements", "")
	if status != http.StatusOK {
		t.Fatalf("public announcements: got %d, want 200", status)
	}
	for _, m := range publicList {
		if title, _ := m["title"].(string); title == "IT Announcement (updated)" {
			t.Fatal("in-review announcement leaked into the public list")
		}
	}

	// Delete
	status, _ = do(t, http.MethodDelete, srv.URL+"/api/admin/announcements/"+id, token, nil)
	if status != http.StatusOK {
		t.Fatalf("delete: got %d, want 200", status)
	}
	c, cancel := ctx()
	defer cancel()
	count, err := db.Collection("announcements").CountDocuments(c, bson.M{"title": "IT Announcement (updated)"})
	if err != nil || count != 0 {
		t.Fatalf("delete: count %d err %v, want 0", count, err)
	}
}

func TestPrayerFormAndHoneypot(t *testing.T) {
	db, srv := setup(t)
	c, cancel := ctx()
	defer cancel()
	before, err := db.Collection("prayer_requests").CountDocuments(c, bson.M{})
	if err != nil {
		t.Fatalf("count prayer_requests: %v", err)
	}

	// A genuine submission is stored.
	status, _ := do(t, http.MethodPost, srv.URL+"/api/forms/prayer", "", bson.M{
		"name": "IT Member", "email": "member@test.remi", "request": "Please pray for the integration test.",
	})
	if status != http.StatusCreated {
		t.Fatalf("prayer submit: got %d, want 201", status)
	}

	// A filled honeypot is silently accepted but stores nothing.
	status, _ = do(t, http.MethodPost, srv.URL+"/api/forms/prayer", "", bson.M{
		"request": "spam", "website": "http://spam.example",
	})
	if status != http.StatusCreated {
		t.Fatalf("honeypot submit: got %d, want 201", status)
	}

	after, err := db.Collection("prayer_requests").CountDocuments(c, bson.M{})
	if err != nil {
		t.Fatalf("count prayer_requests: %v", err)
	}
	if after-before != 1 {
		t.Fatalf("prayer_requests grew by %d, want exactly 1 (honeypot must store nothing)", after-before)
	}
}

func TestGivingInitializeDemoMode(t *testing.T) {
	_, srv := setup(t)
	status, body := do(t, http.MethodPost, srv.URL+"/api/giving/initialize", "", bson.M{
		"email": "giver@test.remi", "amount": 5000, "category": "Tithe",
	})
	if status != http.StatusOK {
		t.Fatalf("giving initialize: got %d (%v), want 200", status, body)
	}
	if body["demo"] != true {
		t.Fatalf("giving initialize: demo %v, want true (no Paystack key configured)", body["demo"])
	}
	if ref, _ := body["reference"].(string); ref == "" {
		t.Fatalf("giving initialize: no reference in %v", body)
	}
}

func TestRBACViewerCannotWrite(t *testing.T) {
	db, srv := setup(t)
	seedUser(t, db, "viewer@test.remi", "viewer-pass", "viewer")
	token := login(t, srv, "viewer@test.remi", "viewer-pass")

	// Viewers may read admin lists...
	status, _ := do(t, http.MethodGet, srv.URL+"/api/admin/announcements", token, nil)
	if status != http.StatusOK {
		t.Fatalf("viewer read: got %d, want 200", status)
	}

	// ...but every write is forbidden.
	for _, attempt := range []struct{ method, path string }{
		{http.MethodPost, "/api/admin/announcements"},
		{http.MethodPut, "/api/admin/settings"},
		{http.MethodPatch, "/api/admin/content/announcements/000000000000000000000000/status"},
	} {
		status, body := do(t, attempt.method, srv.URL+attempt.path, token, bson.M{"title": "x"})
		if status != http.StatusForbidden {
			t.Fatalf("viewer %s %s: got %d (%v), want 403", attempt.method, attempt.path, status, body)
		}
	}

	// And no token at all is unauthorized.
	status, _ = do(t, http.MethodPost, srv.URL+"/api/admin/announcements", "", bson.M{"title": "x"})
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous write: got %d, want 401", status)
	}
}
