package platform

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMoneyRejectsCurrencyMismatchAndOverflow(t *testing.T) {
	ghs, _ := NewMoney(10, "ghs")
	usd, _ := NewMoney(10, "USD")
	if _, err := ghs.Add(usd); err == nil {
		t.Fatal("expected currency mismatch")
	}
	max, _ := NewMoney(int64(^uint64(0)>>1), "GHS")
	if _, err := max.Add(ghs); err == nil {
		t.Fatal("expected overflow")
	}
}

func TestGrantAuthorizerScopesFieldsAndRecentMFA(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	mfa := now.Add(-2 * time.Minute)
	p := Principal{OrganizationID: "org-1", MFAConfirmedAt: &mfa, Grants: []Grant{{Action: "read", Resource: "person", BranchIDs: []ID{"branch-1"}, FieldClasses: []FieldClass{FieldPersonal}}}}
	a := GrantAuthorizer{RecentMFAWindow: 5 * time.Minute}
	allowed := a.Authorize(p, AccessRequest{Action: "read", ResourceType: "person", OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []FieldClass{FieldPersonal}, RequireRecentMFA: true, Now: now})
	if !allowed.Allowed {
		t.Fatalf("expected allowed, got %s", allowed.Code)
	}
	for name, request := range map[string]AccessRequest{
		"other organization": {Action: "read", ResourceType: "person", OrganizationID: "org-2", BranchID: "branch-1", FieldClasses: []FieldClass{FieldPersonal}, Now: now},
		"other branch":       {Action: "read", ResourceType: "person", OrganizationID: "org-1", BranchID: "branch-2", FieldClasses: []FieldClass{FieldPersonal}, Now: now},
		"finance field":      {Action: "read", ResourceType: "person", OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []FieldClass{FieldFinancial}, Now: now},
		"secret field":       {Action: "read", ResourceType: "person", OrganizationID: "org-1", BranchID: "branch-1", FieldClasses: []FieldClass{FieldSecret}, Now: now},
	} {
		if decision := a.Authorize(p, request); decision.Allowed {
			t.Fatalf("%s unexpectedly allowed", name)
		}
	}
}

func TestGrantWithoutMinistryRestrictionCanAuthorizeTaggedResource(t *testing.T) {
	authorizer := GrantAuthorizer{}
	principal := Principal{OrganizationID: "org-1", Grants: []Grant{{Action: "read", Resource: "group", BranchIDs: []ID{"branch-1"}, FieldClasses: []FieldClass{FieldOperational}}}}
	decision := authorizer.Authorize(principal, AccessRequest{Action: "read", ResourceType: "group", OrganizationID: "org-1", BranchID: "branch-1", MinistryID: "youth", FieldClasses: []FieldClass{FieldOperational}})
	if !decision.Allowed {
		t.Fatalf("unrestricted ministry grant denied tagged resource: %s", decision.Code)
	}
}

func TestGrantAuthorizerFailsClosedForOmittedAndAssignedScopes(t *testing.T) {
	p := Principal{OrganizationID: "org-1", AssignedResourceIDs: []ID{"case-1"}, Grants: []Grant{{Action: "read", Resource: "care-case", BranchIDs: []ID{"branch-1"}, MinistryIDs: []ID{"pastoral-care"}, FieldClasses: []FieldClass{FieldSensitiveMinistry}}}}
	a := GrantAuthorizer{}
	base := AccessRequest{Action: "read", ResourceType: "care-case", OrganizationID: "org-1", BranchID: "branch-1", MinistryID: "pastoral-care", ResourceID: "case-1", FieldClasses: []FieldClass{FieldSensitiveMinistry}, RequireAssignment: true}
	if decision := a.Authorize(p, base); !decision.Allowed {
		t.Fatalf("fully scoped request denied: %s", decision.Code)
	}
	requests := map[string]AccessRequest{
		"omitted branch":   base,
		"omitted ministry": base,
		"other assignment": base,
	}
	request := requests["omitted branch"]
	request.BranchID = ""
	requests["omitted branch"] = request
	request = requests["omitted ministry"]
	request.MinistryID = ""
	requests["omitted ministry"] = request
	request = requests["other assignment"]
	request.ResourceID = "case-2"
	requests["other assignment"] = request
	for name, candidate := range requests {
		if decision := a.Authorize(p, candidate); decision.Allowed {
			t.Fatalf("%s unexpectedly allowed", name)
		}
	}
}

func TestEnvelopeCipherAuthenticatesAssociatedData(t *testing.T) {
	cipher, err := NewEnvelopeCipher("v1", bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	value, err := cipher.Encrypt([]byte("restricted note"), []byte("case-1"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := cipher.Decrypt(value, []byte("case-1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "restricted note" {
		t.Fatalf("unexpected plaintext %q", plain)
	}
	if _, err := cipher.Decrypt(value, []byte("case-2")); err == nil {
		t.Fatal("expected associated-data authentication failure")
	}
}

func TestFeatureFlagsRejectStaleSnapshotAndCopyMaps(t *testing.T) {
	input := map[string]bool{"chms": true}
	flags := NewFeatureFlags(2, input)
	input["chms"] = false
	if !flags.Enabled("chms") {
		t.Fatal("input map mutated flags")
	}
	if flags.Replace(1, map[string]bool{"chms": false}) {
		t.Fatal("accepted stale version")
	}
	if !flags.Replace(3, map[string]bool{"chms": false}) || flags.Enabled("chms") {
		t.Fatal("failed to apply new version")
	}
}

func TestIdempotencyKeyAndHash(t *testing.T) {
	if err := ValidateIdempotencyKey("short"); err == nil {
		t.Fatal("expected short key error")
	}
	if err := ValidateIdempotencyKey("request-1234"); err != nil {
		t.Fatal(err)
	}
	if RequestHash([]byte("same")) != RequestHash([]byte("same")) || RequestHash([]byte("same")) == RequestHash([]byte("different")) {
		t.Fatal("request hash is not deterministic")
	}
}

func TestCursorCodecSignsAndRejectsTampering(t *testing.T) {
	codec, err := NewCursorCodec(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	input := struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}{"Mensah", "person-1"}
	encoded, err := codec.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := codec.Decode(encoded, &decoded); err != nil || decoded != input {
		t.Fatalf("cursor roundtrip: decoded=%+v err=%v", decoded, err)
	}
	tampered := encoded[:len(encoded)-1] + "A"
	if err := codec.Decode(tampered, &decoded); err == nil {
		t.Fatal("tampered cursor accepted")
	}
}

func TestDecodeJSONIsStrictAndBounded(t *testing.T) {
	for name, body := range map[string]string{"unknown field": `{"known":"yes","other":1}`, "multiple values": `{"known":"yes"} {}`, "too large": strings.Repeat("x", 64)} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			response := httptest.NewRecorder()
			var dst struct {
				Known string `json:"known"`
			}
			if err := DecodeJSON(response, request, &dst, 32); err == nil {
				t.Fatal("expected strict decode error")
			}
		})
	}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"known":"yes"}`))
	response := httptest.NewRecorder()
	var dst struct {
		Known string `json:"known"`
	}
	if err := DecodeJSON(response, request, &dst, 32); err != nil || dst.Known != "yes" {
		t.Fatalf("valid decode: dst=%+v err=%v", dst, err)
	}
}

func TestRequestIDAndSafeErrorEnvelope(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, ValidationError(FieldError{Path: "name", Code: "required", Message: "Required."}))
	}))
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("X-Request-ID", "request-1234")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Header().Get("X-Request-ID") != "request-1234" || !strings.Contains(response.Body.String(), `"requestId":"request-1234"`) {
		t.Fatalf("unexpected response: %d %s %s", response.Code, response.Header().Get("X-Request-ID"), response.Body.String())
	}
}

type fakeJobStore struct {
	job                      *Job
	completed, retried, dead bool
}

func (s *fakeJobStore) Claim(context.Context, string, time.Time, time.Duration) (*Job, error) {
	job := s.job
	s.job = nil
	return job, nil
}
func (s *fakeJobStore) Complete(context.Context, ID, string, time.Time) error {
	s.completed = true
	return nil
}
func (s *fakeJobStore) Retry(context.Context, ID, string, string, time.Time) error {
	s.retried = true
	return nil
}
func (s *fakeJobStore) DeadLetter(context.Context, ID, string, string, time.Time) error {
	s.dead = true
	return nil
}

func TestJobRunnerCompletesRetriesAndDeadLetters(t *testing.T) {
	base := Job{ID: "j1", Type: "known", Attempts: 1, MaxAttempts: 3}
	store := &fakeJobStore{job: &base}
	runner := JobRunner{Store: store, WorkerID: "w", Handlers: map[string]JobHandler{"known": func(context.Context, Job) error { return nil }}}
	if ran, err := runner.RunOne(context.Background()); err != nil || !ran || !store.completed {
		t.Fatalf("complete: ran=%v err=%v", ran, err)
	}
	base.ID = "j2"
	store = &fakeJobStore{job: &base}
	runner.Store = store
	runner.Handlers["known"] = func(context.Context, Job) error { return errors.New("boom") }
	if _, err := runner.RunOne(context.Background()); err != nil || !store.retried {
		t.Fatalf("retry: %v", err)
	}
	base.ID = "j3"
	base.Type = "missing"
	store = &fakeJobStore{job: &base}
	runner.Store = store
	if _, err := runner.RunOne(context.Background()); err != nil || !store.dead {
		t.Fatalf("dead letter: %v", err)
	}
}
