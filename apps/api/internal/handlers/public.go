package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"remi-api/internal/config"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/models"
	"remi-api/internal/services"
)

// Handler bundles the dependencies shared by all HTTP handlers.
type Handler struct {
	DB       *mongo.Database
	Cfg      *config.Config
	JWT      *services.JWTService
	Email    *services.EmailService
	SMS      *services.SMSService
	Paystack *services.PaystackService
	Cloud    *services.CloudinaryService
}

func New(db *mongo.Database, cfg *config.Config) *Handler {
	return &Handler{
		DB:       db,
		Cfg:      cfg,
		JWT:      services.NewJWTService(cfg.JWTSecret),
		Email:    services.NewEmailService(cfg.ResendAPIKey, cfg.EmailFrom),
		SMS:      services.NewSMSService(cfg.ArkeselAPIKey, cfg.SMSSender),
		Paystack: services.NewPaystackService(cfg.PaystackSecretKey),
		Cloud:    services.NewCloudinaryService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret),
	}
}

func publicContentFilter() bson.M {
	return bson.M{"contentStatus": bson.M{"$in": models.PublicStatuses}}
}

func (h *Handler) findOne(w http.ResponseWriter, r *http.Request, coll string, filter bson.M) (bson.M, bool) {
	var doc bson.M
	err := h.DB.Collection(coll).FindOne(r.Context(), filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		httpx.Error(w, http.StatusNotFound, "not found")
		return nil, false
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	return doc, true
}

func (h *Handler) findAll(w http.ResponseWriter, r *http.Request, coll string, filter bson.M, opts ...options.Lister[options.FindOptions]) ([]bson.M, bool) {
	cur, err := h.DB.Collection(coll).Find(r.Context(), filter, opts...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	defer cur.Close(r.Context())
	var docs []bson.M
	if err := cur.All(r.Context(), &docs); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	return docs, true
}

// ── Health & settings ─────────────────────────────────────────────

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, bson.M{"status": "ok"})
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	err := h.DB.Collection("settings").FindOne(r.Context(), bson.M{}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		httpx.JSON(w, http.StatusOK, bson.M{})
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(doc))
}

// ── Pages ─────────────────────────────────────────────────────────

func (h *Handler) GetPage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "pageKey")
	filter := publicContentFilter()
	filter["pageKey"] = key
	doc, ok := h.findOne(w, r, "pages", filter)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(doc))
}

// ── Leadership ────────────────────────────────────────────────────

func (h *Handler) ListLeadership(w http.ResponseWriter, r *http.Request) {
	docs, ok := h.findAll(w, r, "leaders", publicContentFilter(),
		options.Find().SetSort(bson.D{{Key: "order", Value: 1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

// ── Ministries ────────────────────────────────────────────────────

func (h *Handler) ListMinistries(w http.ResponseWriter, r *http.Request) {
	docs, ok := h.findAll(w, r, "ministries", publicContentFilter(),
		options.Find().SetSort(bson.D{{Key: "title", Value: 1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

func (h *Handler) GetMinistry(w http.ResponseWriter, r *http.Request) {
	filter := publicContentFilter()
	filter["slug"] = chi.URLParam(r, "slug")
	doc, ok := h.findOne(w, r, "ministries", filter)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(doc))
}

// ── Branches ──────────────────────────────────────────────────────

func (h *Handler) ListBranches(w http.ResponseWriter, r *http.Request) {
	docs, ok := h.findAll(w, r, "branches", publicContentFilter())
	if !ok {
		return
	}
	branches := models.NormalizeAll(docs)
	// Branch slugs are the stable operational identifiers used by CHMS records.
	// Preserve the CMS document id separately so public content editing and
	// operational filtering cannot silently drift onto different identifiers.
	for _, branch := range branches {
		branch["contentId"] = branch["id"]
		if slug, valid := branch["slug"].(string); valid && strings.TrimSpace(slug) != "" {
			branch["id"] = slug
			branch["operationalId"] = slug
		}
	}
	httpx.JSON(w, http.StatusOK, branches)
}

// ── Sermons ───────────────────────────────────────────────────────

func (h *Handler) ListSermons(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := publicContentFilter()
	if v := q.Get("preacher"); v != "" {
		filter["preacher"] = v
	}
	if v := q.Get("series"); v != "" {
		filter["series"] = v
	}
	if v := q.Get("topic"); v != "" {
		filter["topic"] = v
	}
	if v := q.Get("q"); v != "" {
		rx := bson.M{"$regex": v, "$options": "i"}
		filter["$or"] = bson.A{
			bson.M{"title": rx}, bson.M{"description": rx},
			bson.M{"topic": rx}, bson.M{"preacher": rx},
		}
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 12
	}

	ctx := r.Context()
	coll := h.DB.Collection("sermons")
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	cur, err := coll.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "date", Value: -1}}).
			SetSkip(int64((page-1)*pageSize)).SetLimit(int64(pageSize)))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	defer cur.Close(ctx)
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	facets := bson.M{
		"preachers": h.distinctStrings(ctx, "sermons", "preacher"),
		"series":    h.distinctStrings(ctx, "sermons", "series"),
		"topics":    h.distinctStrings(ctx, "sermons", "topic"),
	}

	httpx.JSON(w, http.StatusOK, bson.M{
		"items": models.NormalizeAll(docs), "total": total,
		"page": page, "pageSize": pageSize, "facets": facets,
	})
}

func (h *Handler) distinctStrings(ctx context.Context, coll, field string) []string {
	var vals []string
	out := []string{}
	if err := h.DB.Collection(coll).Distinct(ctx, field, publicContentFilter()).Decode(&vals); err != nil {
		return out
	}
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (h *Handler) GetSermon(w http.ResponseWriter, r *http.Request) {
	filter := publicContentFilter()
	filter["slug"] = chi.URLParam(r, "slug")
	doc, ok := h.findOne(w, r, "sermons", filter)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(doc))
}

// ── Events ────────────────────────────────────────────────────────

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	now := models.Now()
	filter := publicContentFilter()
	sort := bson.D{{Key: "startAt", Value: -1}}
	switch r.URL.Query().Get("when") {
	case "upcoming":
		filter["startAt"] = bson.M{"$gte": now}
		sort = bson.D{{Key: "startAt", Value: 1}}
	case "past":
		filter["startAt"] = bson.M{"$lt": now}
	}
	docs, ok := h.findAll(w, r, "events", filter, options.Find().SetSort(sort))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	filter := publicContentFilter()
	filter["slug"] = chi.URLParam(r, "slug")
	doc, ok := h.findOne(w, r, "events", filter)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.Normalize(doc))
}

func (h *Handler) RegisterForEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.ObjectID(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid event id")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body["website"] != nil && httpx.Str(body, "website") != "" { // honeypot
		httpx.JSON(w, http.StatusCreated, bson.M{"message": "registration received"})
		return
	}
	name, email := httpx.Str(body, "name"), httpx.Str(body, "email")
	if name == "" || email == "" {
		httpx.Error(w, http.StatusBadRequest, "name and email are required")
		return
	}

	ctx := r.Context()
	var event bson.M
	if err := h.DB.Collection("events").FindOne(ctx, bson.M{"_id": id}).Decode(&event); err != nil {
		httpx.Error(w, http.StatusNotFound, "event not found")
		return
	}
	if enabled, _ := event["registrationEnabled"].(bool); !enabled {
		httpx.Error(w, http.StatusBadRequest, "registration is not enabled for this event")
		return
	}

	// Atomically bump registeredCount only while under capacity.
	incFilter := bson.M{"_id": id}
	if cap, _ := event["capacity"].(int32); cap > 0 {
		incFilter["registeredCount"] = bson.M{"$lt": cap}
	}
	res, err := h.DB.Collection("events").UpdateOne(ctx, incFilter,
		bson.M{"$inc": bson.M{"registeredCount": 1}})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if res.MatchedCount == 0 {
		httpx.Error(w, http.StatusConflict, "event is at full capacity")
		return
	}

	reg := bson.M{
		"eventId": id.Hex(), "name": name, "email": email,
		"normalizedEmail": strings.ToLower(strings.TrimSpace(email)),
		"phone":           httpx.Str(body, "phone"), "createdAt": models.Now(),
	}
	if _, err := h.DB.Collection("event_registrations").InsertOne(ctx, reg); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}

	go h.Email.Send(h.Cfg.NotifyEmail,
		fmt.Sprintf("New event registration: %s", httpx.Str(event, "title")),
		fmt.Sprintf("<p><b>%s</b> (%s, %s) registered for <b>%s</b>.</p>",
			name, email, httpx.Str(body, "phone"), httpx.Str(event, "title")))

	httpx.JSON(w, http.StatusCreated, bson.M{"message": "registration received"})
}

// ── Announcements ─────────────────────────────────────────────────

func (h *Handler) ListAnnouncements(w http.ResponseWriter, r *http.Request) {
	now := models.Now()
	filter := publicContentFilter()
	filter["publishAt"] = bson.M{"$lte": now}
	filter["$or"] = bson.A{
		bson.M{"expiresAt": nil},
		bson.M{"expiresAt": bson.M{"$gt": now}},
	}
	docs, ok := h.findAll(w, r, "announcements", filter,
		options.Find().SetSort(bson.D{{Key: "publishAt", Value: -1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

// ── Testimonies (public) ──────────────────────────────────────────

func (h *Handler) ListTestimonies(w http.ResponseWriter, r *http.Request) {
	docs, ok := h.findAll(w, r, "testimonies", publicContentFilter(),
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, models.NormalizeAll(docs))
}

// ── Forms ─────────────────────────────────────────────────────────

// submitForm handles honeypot + insert for a public form collection.
func (h *Handler) submitForm(w http.ResponseWriter, r *http.Request, coll string, required []string, build func(bson.M) bson.M, notifySubject, notifyHTML string) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if s := httpx.Str(body, "website"); s != "" { // honeypot — silently accept
		httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
		return
	}
	for _, f := range required {
		if httpx.Str(body, f) == "" {
			httpx.Error(w, http.StatusBadRequest, f+" is required")
			return
		}
	}
	doc := build(body)
	doc["createdAt"] = models.Now()
	if _, err := h.DB.Collection(coll).InsertOne(r.Context(), doc); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if notifySubject != "" {
		go h.Email.Send(h.Cfg.NotifyEmail, notifySubject, notifyHTML)
	}
	httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
}

func (h *Handler) SubmitPrayer(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if httpx.Str(body, "website") != "" {
		httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
		return
	}
	request := strings.TrimSpace(httpx.Str(body, "request"))
	if request == "" || len(request) > 10000 {
		httpx.Error(w, http.StatusBadRequest, "request is required and must be under 10000 characters")
		return
	}
	identityMode := strings.ToLower(strings.TrimSpace(httpx.Str(body, "identityMode")))
	if identityMode == "" {
		identityMode = "identified"
	}
	visibility := strings.ToLower(strings.TrimSpace(httpx.Str(body, "visibility")))
	if visibility == "" {
		if body["isPrivate"] == true {
			visibility = "pastors-only"
		} else {
			visibility = "prayer-team"
		}
	}
	if (identityMode != "identified" && identityMode != "anonymous") || (visibility != "pastors-only" && visibility != "prayer-team") {
		httpx.Error(w, http.StatusBadRequest, "choose anonymous or identified, and pastors-only or prayer-team visibility")
		return
	}
	doc := bson.M{"request": request, "identityMode": identityMode, "visibility": visibility, "status": "new", "createdAt": models.Now(), "profileLinkStatus": "not-requested"}
	if identityMode == "identified" {
		doc["name"], doc["email"] = strings.TrimSpace(httpx.Str(body, "name")), strings.ToLower(strings.TrimSpace(httpx.Str(body, "email")))
		if body["linkToProfile"] == true && body["linkConsent"] == true && doc["email"] != "" {
			doc["profileLinkStatus"] = "pending-verification"
			doc["profileLinkConsent"] = bson.M{"state": "granted", "source": "public-prayer-form", "noticeVersion": "prayer-link-2026-01", "capturedAt": models.Now()}
		}
	}
	if _, err = h.DB.Collection("prayer_requests").InsertOne(r.Context(), doc); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	go h.Email.Send(h.Cfg.NotifyEmail, "New prayer request", "<p>A new prayer request was submitted on the REMI website.</p>")
	httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
}

func (h *Handler) SubmitContact(w http.ResponseWriter, r *http.Request) {
	name := ""
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	name = httpx.Str(body, "name")
	if s := httpx.Str(body, "website"); s != "" {
		httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
		return
	}
	for _, f := range []string{"name", "email", "subject", "message"} {
		if httpx.Str(body, f) == "" {
			httpx.Error(w, http.StatusBadRequest, f+" is required")
			return
		}
	}
	doc := bson.M{
		"name": name, "email": httpx.Str(body, "email"), "phone": httpx.Str(body, "phone"),
		"subject": httpx.Str(body, "subject"), "message": httpx.Str(body, "message"),
		"status": "new", "createdAt": models.Now(),
	}
	if _, err := h.DB.Collection("contact_submissions").InsertOne(r.Context(), doc); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	go h.Email.Send(h.Cfg.NotifyEmail,
		"New contact message: "+httpx.Str(body, "subject"),
		fmt.Sprintf("<p><b>%s</b> (%s) wrote: %s</p>", name, httpx.Str(body, "email"), httpx.Str(body, "message")))
	httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
}

func (h *Handler) SubmitTestimony(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if s := httpx.Str(body, "website"); s != "" {
		httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
		return
	}
	author, text := httpx.Str(body, "author"), httpx.Str(body, "body")
	if author == "" || text == "" {
		httpx.Error(w, http.StatusBadRequest, "author and body are required")
		return
	}
	slug := models.Slugify(author) + "-" + randHex(4)
	doc := bson.M{
		"title": "Testimony from " + author, "slug": slug,
		"author": author, "body": text, "image": "",
		"contentStatus": models.StatusInReview,
		"createdBy":     "website", "updatedBy": "website",
		"createdAt": models.Now(), "updatedAt": models.Now(),
	}
	if _, err := h.DB.Collection("testimonies").InsertOne(r.Context(), doc); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	go h.Email.Send(h.Cfg.NotifyEmail,
		"New testimony submitted", fmt.Sprintf("<p><b>%s</b> shared a testimony.</p>", author))
	httpx.JSON(w, http.StatusCreated, bson.M{"message": "received"})
}

func (h *Handler) SubmitVisit(w http.ResponseWriter, r *http.Request) {
	h.submitForm(w, r, "visit_submissions", []string{"name", "email", "visitDate"},
		func(b bson.M) bson.M {
			return bson.M{
				"name": httpx.Str(b, "name"), "email": httpx.Str(b, "email"),
				"phone": httpx.Str(b, "phone"), "visitDate": httpx.Str(b, "visitDate"),
				"branch": httpx.Str(b, "branch"), "notes": httpx.Str(b, "notes"),
				"status": "new",
			}
		},
		"", "")
}

// ── Newsletter ────────────────────────────────────────────────────

func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(httpx.Str(body, "email")))
	if email == "" || !strings.Contains(email, "@") {
		httpx.Error(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	// Demo: auto-confirm instead of double opt-in.
	_, err = h.DB.Collection("subscribers").UpdateOne(r.Context(),
		bson.M{"email": email},
		bson.M{"$set": bson.M{"email": email, "status": "confirmed", "token": randHex(16)},
			"$setOnInsert": bson.M{"createdAt": models.Now()}},
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, bson.M{"message": "subscribed"})
}

// ── Giving ────────────────────────────────────────────────────────

func (h *Handler) GivingInitialize(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := httpx.Str(body, "email")
	if email == "" {
		httpx.Error(w, http.StatusBadRequest, "email is required")
		return
	}
	var amount int64
	switch v := body["amount"].(type) {
	case float64:
		amount = int64(v)
	case int64:
		amount = v
	case int32:
		amount = int64(v)
	}
	if amount <= 0 {
		httpx.Error(w, http.StatusBadRequest, "amount (pesewas) must be positive")
		return
	}
	reference := "REMI-" + time.Now().UTC().Format("20060102") + "-" + randHex(6)

	if !h.Paystack.Configured() {
		httpx.JSON(w, http.StatusOK, bson.M{
			"authorizationUrl": "/give/demo-success?ref=" + reference,
			"reference":        reference,
			"demo":             true,
		})
		return
	}
	res, err := h.Paystack.Initialize(amount, email, reference, httpx.Str(body, "category"))
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "payment provider error: "+err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, bson.M{
		"authorizationUrl": res.AuthorizationURL, "reference": res.Reference,
	})
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
