package handlers

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"remi-api/internal/chms/platform"
	"remi-api/internal/handlers/httpx"
	"remi-api/internal/models"
)

const (
	memberCommunicationPreferencesCollection = "chms_member_communication_preferences"
	memberInboxStatesCollection              = "chms_member_inbox_states"
	memberGroupMessagesCollection            = "chms_member_group_messages"
)

type memberCommunicationPreferences struct {
	ID              platform.ID `json:"id" bson:"_id"`
	OrganizationID  platform.ID `json:"-" bson:"organizationId"`
	PersonID        platform.ID `json:"-" bson:"personId"`
	QuietEnabled    bool        `json:"quietEnabled" bson:"quietEnabled"`
	QuietStart      string      `json:"quietStart" bson:"quietStart"`
	QuietEnd        string      `json:"quietEnd" bson:"quietEnd"`
	Timezone        string      `json:"timezone" bson:"timezone"`
	DailyCap        int         `json:"dailyCap" bson:"dailyCap"`
	DirectoryFields []string    `json:"directoryFields" bson:"directoryFields"`
	Version         int64       `json:"version" bson:"version"`
	CreatedAt       time.Time   `json:"createdAt" bson:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt" bson:"updatedAt"`
}

type memberCommunicationPreferencesInput struct {
	QuietEnabled    bool     `json:"quietEnabled"`
	QuietStart      string   `json:"quietStart"`
	QuietEnd        string   `json:"quietEnd"`
	Timezone        string   `json:"timezone"`
	DailyCap        int      `json:"dailyCap"`
	DirectoryFields []string `json:"directoryFields"`
	ExpectedVersion int64    `json:"expectedVersion"`
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
var webURLPattern = regexp.MustCompile(`https?://\S+`)

func defaultMemberCommunicationPreferences(org, person platform.ID) memberCommunicationPreferences {
	return memberCommunicationPreferences{OrganizationID: org, PersonID: person, QuietEnabled: true, QuietStart: "21:00", QuietEnd: "07:00", Timezone: "Africa/Accra", DailyCap: 3, DirectoryFields: []string{}, Version: 0}
}

func validClock(value string) bool { _, err := time.Parse("15:04", value); return err == nil }

func normalizeDirectoryFields(values []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "email" && value != "mobile" {
			return nil, false
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out, true
}

func (h *Handler) memberCommunicationPreferencesFor(ctx context.Context, person platform.ID) memberCommunicationPreferences {
	value := defaultMemberCommunicationPreferences(platform.ID(h.Cfg.CHMSOrganizationID), person)
	_ = h.DB.Collection(memberCommunicationPreferencesCollection).FindOne(ctx, bson.M{"organizationId": value.OrganizationID, "personId": person}).Decode(&value)
	if value.Timezone == "" {
		value.Timezone = "Africa/Accra"
	}
	if value.QuietStart == "" {
		value.QuietStart = "21:00"
	}
	if value.QuietEnd == "" {
		value.QuietEnd = "07:00"
	}
	if value.DailyCap < 1 {
		value.DailyCap = 3
	}
	if value.DirectoryFields == nil {
		value.DirectoryFields = []string{}
	}
	return value
}

func personAge(value bson.M, now time.Time) (int, bool) {
	dateValue := ""
	if partial, ok := value["dateOfBirth"].(bson.M); ok {
		dateValue = fmt.Sprint(partial["value"])
	}
	if partial, ok := value["dateOfBirth"].(bson.D); ok {
		for _, item := range partial {
			if item.Key == "value" {
				dateValue = fmt.Sprint(item.Value)
			}
		}
	}
	birth, err := time.Parse("2006-01-02", dateValue)
	if err != nil {
		return 0, false
	}
	age := now.Year() - birth.Year()
	if now.YearDay() < birth.YearDay() {
		age--
	}
	return age, true
}

func personIsVerifiedAdult(value bson.M, now time.Time) bool {
	age, known := personAge(value, now)
	return known && age >= 18
}

func (h *Handler) personHasChildRole(ctx context.Context, person platform.ID) bool {
	count, _ := h.DB.Collection("chms_household_memberships").CountDocuments(ctx, bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": person, "role": bson.M{"$in": bson.A{"child", "minor"}}, "endedAt": nil})
	return count > 0
}

func (h *Handler) memberAdult(ctx context.Context, person platform.ID) bool {
	var value bson.M
	if h.DB.Collection("chms_people").FindOne(ctx, bson.M{"_id": person, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"dateOfBirth": 1})).Decode(&value) != nil {
		return false
	}
	return personIsVerifiedAdult(value, models.Now()) && !h.personHasChildRole(ctx, person)
}

func displayPerson(value bson.M) string {
	names, _ := value["names"].(bson.M)
	if names == nil {
		if doc, ok := value["names"].(bson.D); ok {
			names = bson.M{}
			for _, item := range doc {
				names[item.Key] = item.Value
			}
		}
	}
	return firstNonEmpty(fmt.Sprint(names["preferred"]), strings.TrimSpace(fmt.Sprint(names["given"])+" "+fmt.Sprint(names["family"])), "REMI member")
}

func asDocument(value any) bson.M {
	if document, ok := value.(bson.M); ok {
		return document
	}
	if document, ok := value.(bson.D); ok {
		result := bson.M{}
		for _, item := range document {
			result[item.Key] = item.Value
		}
		return result
	}
	return nil
}

func firstName(value string) string {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return "friend"
	}
	return parts[0]
}

func documentString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (h *Handler) ListMemberDirectory(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok || !h.memberAdult(r.Context(), platform.ID(claims.PersonID)) {
		httpx.Error(w, 403, "adult member access required")
		return
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 80 {
		httpx.Error(w, 400, "search is too long")
		return
	}
	viewer := platform.ID(claims.PersonID)
	personIDs := []platform.ID{}
	visibility := map[platform.ID]bool{}
	if scope == "group" {
		groupID := platform.ID(strings.TrimSpace(r.URL.Query().Get("groupId")))
		var own bson.M
		if !groupID.Valid() || h.DB.Collection("chms_group_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": viewer, "status": "active", "endedAt": nil}).Decode(&own) != nil {
			httpx.Error(w, 404, "group not found")
			return
		}
		cursor, err := h.DB.Collection("chms_group_memberships").Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "status": "active", "endedAt": nil, "directoryVisibility": "members"}, options.Find().SetProjection(bson.M{"personId": 1}).SetLimit(250))
		if err != nil {
			httpx.Error(w, 500, "directory unavailable")
			return
		}
		defer cursor.Close(r.Context())
		var memberships []bson.M
		_ = cursor.All(r.Context(), &memberships)
		for _, item := range memberships {
			id := platform.ID(fmt.Sprint(item["personId"]))
			if id.Valid() {
				personIDs = append(personIDs, id)
				visibility[id] = true
			}
		}
	} else if scope == "branch" {
		var own bson.M
		if h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": viewer, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"homeBranchId": 1})).Decode(&own) != nil {
			httpx.Error(w, 403, "member access required")
			return
		}
		branch := own["homeBranchId"]
		cursor, err := h.DB.Collection("chms_people").Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "homeBranchId": branch, "archivedAt": nil}, options.Find().SetProjection(bson.M{"_id": 1, "customFields": 1}).SetLimit(250))
		if err != nil {
			httpx.Error(w, 500, "directory unavailable")
			return
		}
		defer cursor.Close(r.Context())
		var people []bson.M
		_ = cursor.All(r.Context(), &people)
		for _, item := range people {
			custom := asDocument(item["customFields"])
			if strings.ToLower(fmt.Sprint(custom["member.directoryVisibility"])) != "branch" {
				continue
			}
			id := platform.ID(fmt.Sprint(item["_id"]))
			if id.Valid() {
				personIDs = append(personIDs, id)
				visibility[id] = true
			}
		}
	} else {
		httpx.Error(w, 400, "choose branch or group scope")
		return
	}
	items := []bson.M{}
	if len(personIDs) > 0 {
		cursor, err := h.DB.Collection("chms_people").Find(r.Context(), bson.M{"_id": bson.M{"$in": personIDs}, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": nil}, options.Find().SetProjection(bson.M{"names": 1, "photoAssetId": 1, "dateOfBirth": 1, "contactPoints": 1}).SetLimit(100))
		if err == nil {
			defer cursor.Close(r.Context())
			var people []bson.M
			_ = cursor.All(r.Context(), &people)
			for _, person := range people {
				id := platform.ID(fmt.Sprint(person["_id"]))
				name := displayPerson(person)
				if !visibility[id] || !personIsVerifiedAdult(person, models.Now()) || h.personHasChildRole(r.Context(), id) || (query != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(query))) {
					continue
				}
				prefs := h.memberCommunicationPreferencesFor(r.Context(), id)
				fields := map[string]bool{}
				for _, field := range prefs.DirectoryFields {
					fields[field] = true
				}
				contacts := bson.M{}
				if points, ok := person["contactPoints"].(bson.A); ok {
					for _, raw := range points {
						point := asDocument(raw)
						if point == nil {
							continue
						}
						kind := strings.ToLower(fmt.Sprint(point["type"]))
						if kind == "phone" {
							kind = "mobile"
						}
						if fields[kind] && point["primary"] == true {
							contacts[kind] = point["value"]
						}
					}
				}
				items = append(items, bson.M{"id": id, "name": name, "photoAssetId": person["photoAssetId"], "contacts": contacts, "self": id == viewer})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"]) })
	httpx.JSON(w, 200, bson.M{"scope": scope, "items": models.NormalizeAll(items), "privacy": bson.M{"minors": "excluded", "contacts": "member-selected fields only", "export": false}})
}

func (h *Handler) GetMemberCommunicationWorkspace(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	person := platform.ID(claims.PersonID)
	prefs := h.memberCommunicationPreferencesFor(r.Context(), person)
	states := map[platform.ID]bson.M{}
	if cursor, err := h.DB.Collection(memberInboxStatesCollection).Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": person}); err == nil {
		defer cursor.Close(r.Context())
		var values []bson.M
		_ = cursor.All(r.Context(), &values)
		for _, value := range values {
			states[platform.ID(fmt.Sprint(value["deliveryId"]))] = value
		}
	}
	inbox := []bson.M{}
	if cursor, err := h.DB.Collection("chms_communication_deliveries").Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": person, "state": bson.M{"$in": bson.A{"accepted", "delivered"}}}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(100)); err == nil {
		defer cursor.Close(r.Context())
		var deliveries []bson.M
		_ = cursor.All(r.Context(), &deliveries)
		for _, delivery := range deliveries {
			deliveryID := platform.ID(fmt.Sprint(delivery["_id"]))
			state := states[deliveryID]
			if state["archivedAt"] != nil {
				continue
			}
			subject := documentString(delivery["inboxSubject"])
			bodySource := documentString(delivery["inboxBody"])
			purpose := documentString(delivery["purpose"])
			if subject == "" || bodySource == "" || purpose == "" {
				// Compatibility only for deliveries created before immutable inbox
				// snapshots were introduced. New messages never depend on mutable templates.
				var campaign bson.M
				if h.DB.Collection("chms_communication_campaigns").FindOne(r.Context(), bson.M{"_id": delivery["campaignId"], "organizationId": h.Cfg.CHMSOrganizationID}).Decode(&campaign) != nil {
					continue
				}
				var template bson.M
				if h.DB.Collection("chms_communication_templates").FindOne(r.Context(), bson.M{"_id": campaign["templateId"], "organizationId": h.Cfg.CHMSOrganizationID}).Decode(&template) != nil {
					continue
				}
				subject = firstNonEmpty(subject, fmt.Sprint(template["subject"]), fmt.Sprint(campaign["name"]))
				bodySource = firstNonEmpty(bodySource, fmt.Sprint(template["body"]))
				purpose = firstNonEmpty(purpose, fmt.Sprint(campaign["purpose"]))
			}
			body := html.UnescapeString(htmlTagPattern.ReplaceAllString(bodySource, " "))
			body = webURLPattern.ReplaceAllString(body, "")
			body = strings.NewReplacer("{{first_name}}", firstName(claims.Name), "{{church_name}}", "REMI", "{{unsubscribe_url}}", "").Replace(body)
			body = strings.Join(strings.Fields(body), " ")
			inbox = append(inbox, bson.M{"id": deliveryID, "subject": subject, "preview": body, "purpose": purpose, "channel": delivery["channel"], "state": delivery["state"], "sentAt": delivery["updatedAt"], "readAt": state["readAt"]})
		}
	}
	groups := []bson.M{}
	if cursor, err := h.DB.Collection("chms_group_memberships").Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": person, "status": "active", "endedAt": nil}, options.Find().SetProjection(bson.M{"groupId": 1, "directoryVisibility": 1})); err == nil {
		defer cursor.Close(r.Context())
		var memberships []bson.M
		_ = cursor.All(r.Context(), &memberships)
		for _, membership := range memberships {
			var group bson.M
			if h.DB.Collection("chms_groups").FindOne(r.Context(), bson.M{"_id": membership["groupId"], "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"name": 1})).Decode(&group) == nil {
				groups = append(groups, bson.M{"id": membership["groupId"], "name": group["name"], "canMessage": membership["directoryVisibility"] == "members"})
			}
		}
	}
	httpx.JSON(w, 200, bson.M{"preferences": prefs, "inbox": models.NormalizeAll(inbox), "groups": models.NormalizeAll(groups), "unreadCount": func() int {
		count := 0
		for _, item := range inbox {
			if item["readAt"] == nil {
				count++
			}
		}
		return count
	}()})
}

func (h *Handler) UpdateMemberCommunicationPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	var input memberCommunicationPreferencesInput
	if err := platform.DecodeJSON(w, r, &input, 1<<16); err != nil {
		platform.WriteError(w, r, err)
		return
	}
	fields, valid := normalizeDirectoryFields(input.DirectoryFields)
	if !valid || !validClock(input.QuietStart) || !validClock(input.QuietEnd) || input.QuietStart == input.QuietEnd || input.DailyCap < 1 || input.DailyCap > 5 || input.ExpectedVersion < 0 {
		httpx.Error(w, 400, "choose valid quiet hours, daily cap and directory fields")
		return
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		httpx.Error(w, 400, "timezone must be an IANA timezone")
		return
	}
	person := platform.ID(claims.PersonID)
	org := platform.ID(h.Cfg.CHMSOrganizationID)
	now := models.Now()
	current := h.memberCommunicationPreferencesFor(r.Context(), person)
	if current.Version != input.ExpectedVersion {
		httpx.Error(w, 409, "preferences changed; refresh and try again")
		return
	}
	if !current.ID.Valid() {
		current.ID = platform.ID(bson.NewObjectID().Hex())
		current.CreatedAt = now
	}
	current.QuietEnabled, current.QuietStart, current.QuietEnd, current.Timezone, current.DailyCap, current.DirectoryFields, current.UpdatedAt, current.Version = input.QuietEnabled, input.QuietStart, input.QuietEnd, input.Timezone, input.DailyCap, fields, now, current.Version+1
	store, _ := platform.NewMongoPlatformStore(h.DB)
	err := store.WithTransaction(r.Context(), func(ctx context.Context) error {
		if _, e := h.DB.Collection(memberCommunicationPreferencesCollection).ReplaceOne(ctx, bson.M{"organizationId": org, "personId": person, "version": input.ExpectedVersion}, current, options.Replace().SetUpsert(input.ExpectedVersion == 0)); e != nil {
			return e
		}
		return store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: org, Actor: platform.Actor{Type: platform.ActorMember, ID: person}, Action: "member.communication-preferences.update", ResourceType: "member-communication-preferences", ResourceID: current.ID, SubjectIDs: []platform.ID{person}, ChangedFields: []string{"quietHours", "dailyCap", "directoryFields"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			httpx.Error(w, 409, "preferences changed; refresh and try again")
		} else {
			httpx.Error(w, 500, "preferences unavailable")
		}
		return
	}
	httpx.JSON(w, 200, current)
}

func (h *Handler) UpdateMemberInboxState(w http.ResponseWriter, r *http.Request) {
	claims, ok := memberClaims(r)
	if !ok {
		httpx.Error(w, 403, "member access required")
		return
	}
	deliveryID := platform.ID(chi.URLParam(r, "id"))
	var delivery bson.M
	if !deliveryID.Valid() || h.DB.Collection("chms_communication_deliveries").FindOne(r.Context(), bson.M{"_id": deliveryID, "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "state": bson.M{"$in": bson.A{"accepted", "delivered"}}}).Decode(&delivery) != nil {
		httpx.Error(w, 404, "message not found")
		return
	}
	body, err := httpx.Decode(r)
	if err != nil {
		httpx.Error(w, 400, "invalid JSON body")
		return
	}
	action := strings.ToLower(httpx.Str(body, "action"))
	if action != "read" && action != "unread" && action != "archive" {
		httpx.Error(w, 400, "choose read, unread or archive")
		return
	}
	now := models.Now()
	set := bson.M{"updatedAt": now}
	unset := bson.M{}
	if action == "read" {
		set["readAt"] = now
	} else if action == "archive" {
		set["archivedAt"] = now
	} else {
		unset["readAt"] = ""
	}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"_id": platform.ID(bson.NewObjectID().Hex()), "organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "deliveryId": deliveryID, "createdAt": now}}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	if _, err = h.DB.Collection(memberInboxStatesCollection).UpdateOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "personId": claims.PersonID, "deliveryId": deliveryID}, update, options.UpdateOne().SetUpsert(true)); err != nil {
		httpx.Error(w, 500, "message state unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListMemberGroupMessages(w http.ResponseWriter, r *http.Request) {
	h.memberGroupMessages(w, r, false)
}
func (h *Handler) CreateMemberGroupMessage(w http.ResponseWriter, r *http.Request) {
	h.memberGroupMessages(w, r, true)
}
func (h *Handler) memberGroupMessages(w http.ResponseWriter, r *http.Request, create bool) {
	claims, ok := memberClaims(r)
	if !ok || !h.memberAdult(r.Context(), platform.ID(claims.PersonID)) {
		httpx.Error(w, 403, "adult member access required")
		return
	}
	groupID := platform.ID(chi.URLParam(r, "groupId"))
	var own bson.M
	if !groupID.Valid() || h.DB.Collection("chms_group_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": claims.PersonID, "status": "active", "endedAt": nil}).Decode(&own) != nil {
		httpx.Error(w, 404, "group not found")
		return
	}
	if create {
		if own["directoryVisibility"] != "members" {
			httpx.Error(w, 403, "show your name to this group before posting")
			return
		}
		body, err := httpx.Decode(r)
		if err != nil {
			httpx.Error(w, 400, "invalid JSON body")
			return
		}
		message := strings.TrimSpace(httpx.Str(body, "message"))
		if len(message) < 2 || len(message) > 1200 {
			httpx.Error(w, 400, "message must be 2 to 1200 characters")
			return
		}
		now := models.Now()
		id := platform.ID(bson.NewObjectID().Hex())
		doc := bson.M{"_id": id, "organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": claims.PersonID, "message": message, "state": "published", "createdAt": now}
		store, _ := platform.NewMongoPlatformStore(h.DB)
		err = store.WithTransaction(r.Context(), func(ctx context.Context) error {
			if _, e := h.DB.Collection(memberGroupMessagesCollection).InsertOne(ctx, doc); e != nil {
				return e
			}
			return store.AppendAudit(ctx, platform.AuditEvent{ID: platform.ID(bson.NewObjectID().Hex()), OrganizationID: platform.ID(h.Cfg.CHMSOrganizationID), Actor: platform.Actor{Type: platform.ActorMember, ID: platform.ID(claims.PersonID)}, Action: "member.group-message.create", ResourceType: "member-group-message", ResourceID: id, SubjectIDs: []platform.ID{platform.ID(claims.PersonID)}, ChangedFields: []string{"groupId", "message", "state"}, Outcome: "success", RequestID: platform.RequestIDFrom(r.Context()), OccurredAt: now})
		})
		if err != nil {
			httpx.Error(w, 500, "message unavailable")
			return
		}
		httpx.JSON(w, 201, bson.M{"id": id, "message": message, "createdAt": now})
		return
	}
	cursor, err := h.DB.Collection(memberGroupMessagesCollection).Find(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "state": "published"}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(100))
	if err != nil {
		httpx.Error(w, 500, "messages unavailable")
		return
	}
	defer cursor.Close(r.Context())
	var values []bson.M
	_ = cursor.All(r.Context(), &values)
	items := []bson.M{}
	for index := len(values) - 1; index >= 0; index-- {
		value := values[index]
		authorID := platform.ID(fmt.Sprint(value["personId"]))
		author := "Group member"
		var membership bson.M
		if h.DB.Collection("chms_group_memberships").FindOne(r.Context(), bson.M{"organizationId": h.Cfg.CHMSOrganizationID, "groupId": groupID, "personId": authorID, "status": "active", "endedAt": nil, "directoryVisibility": "members"}).Decode(&membership) == nil && h.memberAdult(r.Context(), authorID) {
			var person bson.M
			if h.DB.Collection("chms_people").FindOne(r.Context(), bson.M{"_id": authorID, "organizationId": h.Cfg.CHMSOrganizationID, "archivedAt": nil}, options.FindOne().SetProjection(bson.M{"names": 1})).Decode(&person) == nil {
				author = displayPerson(person)
			}
		}
		items = append(items, bson.M{"id": value["_id"], "author": author, "message": value["message"], "createdAt": value["createdAt"], "self": authorID == platform.ID(claims.PersonID)})
	}
	httpx.JSON(w, 200, bson.M{"items": models.NormalizeAll(items), "privacy": "Only active adult group members can read. Hidden authors are relabelled."})
}
