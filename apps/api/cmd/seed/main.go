package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"remi-api/internal/config"
	"remi-api/internal/db"
	"remi-api/internal/models"
)

const videoPlaceholder = "https://www.youtube.com/embed/dQw4w9WgXcQ"

// seededCollections are the collections this command writes — and therefore
// the only ones -reset is allowed to drop.
var seededCollections = []string{
	"users", "settings", "pages", "leaders", "ministries",
	"branches", "sermons", "events", "announcements", "testimonies",
	"chms_people", "chms_member_accounts",
	"chms_people_segments", "chms_consent_projections", "chms_saved_audiences", "chms_audience_exports",
	"chms_occurrences", "chms_attendance", "chms_engagement_rules", "chms_engagement_signals", "chms_engagement_review_events",
	"chms_finance_funds", "chms_finance_payment_methods", "chms_finance_campuses",
	"chms_finance_periods", "chms_finance_receipt_sequences",
}

func img(slug string) string {
	return "https://picsum.photos/seed/" + slug + "/1200/800"
}

func main() {
	var (
		reset = flag.Bool("reset", false, "drop the seeded collections before seeding")
		force = flag.Bool("i-know-what-im-doing", false,
			"permit seeding an environment that looks like production")
	)
	flag.Parse()

	// The guard. Seeding writes an admin login with a published password and
	// demo content; the failure mode of running it against production is a
	// live church site full of fabricated sermons and a known password.
	if isProductionEnv() && !*force {
		fmt.Fprintln(os.Stderr, "seed: APP_ENV/ENV indicates production — refusing to run.")
		fmt.Fprintln(os.Stderr, "This writes demo content and an admin login with a known password.")
		fmt.Fprintln(os.Stderr, "If that is genuinely what you want, pass -i-know-what-im-doing")
		os.Exit(1)
	}

	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, database, err := db.Connect(ctx, cfg.MongoURI)
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer client.Disconnect(context.Background())

	if *reset {
		for _, coll := range seededCollections {
			if err := database.Collection(coll).Drop(ctx); err != nil {
				log.Fatalf("drop %s: %v", coll, err)
			}
		}
		log.Printf("dropped %d seeded collections", len(seededCollections))
	}

	s := &seeder{db: database}
	s.users(cfg)
	s.members(cfg)
	s.settings()
	s.pages()
	s.leaders()
	s.ministries()
	s.branches()
	s.retentionDemo(cfg)
	s.finance(cfg)
	s.sermons()
	s.events()
	s.announcements()
	s.testimonies()

	log.Println("seed complete")
}

// isProductionEnv reports whether an APP_ENV/ENV-style variable marks this
// environment as production.
func isProductionEnv() bool {
	for _, key := range []string{"APP_ENV", "ENV"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "production", "prod":
			return true
		}
	}
	return false
}

type seeder struct{ db *mongo.Database }

// upsertContent upserts a content document by slug, filling common fields.
func (s *seeder) upsertContent(coll string, doc bson.M) {
	ctx := context.Background()
	now := models.Now()
	doc["createdBy"] = "seed"
	doc["updatedBy"] = "seed"
	doc["updatedAt"] = now

	_, err := s.db.Collection(coll).UpdateOne(ctx,
		bson.M{"slug": doc["slug"]},
		bson.M{"$set": doc, "$setOnInsert": bson.M{"createdAt": now}},
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed %s/%v: %v", coll, doc["slug"], err)
	}
}

func (s *seeder) users(cfg *config.Config) {
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.SeedAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	_, err = s.db.Collection("users").UpdateOne(context.Background(),
		bson.M{"email": cfg.SeedAdminEmail},
		bson.M{"$set": bson.M{
			"email": cfg.SeedAdminEmail, "name": "REMI Administrator",
			"role": "super-admin", "passwordHash": string(hash),
		}, "$setOnInsert": bson.M{"createdAt": models.Now()}},
		options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatal(err)
	}
	// Helpful unique index for future user creation.
	_, _ = s.db.Collection("users").Indexes().CreateOne(context.Background(),
		mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)})
	log.Printf("seeded admin user %s", cfg.SeedAdminEmail)
}

func (s *seeder) members(cfg *config.Config) {
	now := models.Now()
	members := []struct {
		accountID string
		personID  string
		personNo  string
		given     string
		family    string
		email     string
		phone     string
		birth     string
		stage     string
	}{
		{"66b8f9500000000000000001", "seed-member-ama", "P-DEMO-001", "Ama", "Mensah", "ama.member@remi.church", "+233244000001", "1990-04-18", "member"},
		{"66b8f9500000000000000002", "seed-member-kwame", "P-DEMO-002", "Kwame", "Asante", "kwame.member@remi.church", "+233244000002", "1988-09-03", "member"},
		{"66b8f9500000000000000003", "seed-member-esi", "P-DEMO-003", "Esi", "Owusu", "esi.member@remi.church", "+233244000003", "1995-12-21", "newcomer"},
	}
	for _, member := range members {
		accountID, err := bson.ObjectIDFromHex(member.accountID)
		if err != nil {
			log.Fatalf("seed member object id: %v", err)
		}
		contacts := bson.A{
			bson.M{"type": "email", "value": member.email, "normalized": member.email, "primary": true, "verifiedAt": now},
			bson.M{"type": "phone", "value": member.phone, "normalized": member.phone, "primary": true, "verifiedAt": now},
		}
		_, err = s.db.Collection("chms_people").UpdateOne(context.Background(),
			bson.M{"_id": member.personID},
			bson.M{"$set": bson.M{
				"organizationId": cfg.CHMSOrganizationID, "personNumber": member.personNo, "homeBranchId": "accra-headquarters",
				"names":         bson.M{"given": member.given, "family": member.family, "preferred": member.given},
				"dateOfBirth":   bson.M{"value": member.birth, "precision": "day"},
				"contactPoints": contacts, "membershipStage": member.stage, "tags": bson.A{"demo-member"}, "archivedAt": nil, "updatedAt": now,
				"customFields": bson.M{"member.directoryVisibility": "branch"},
			}, "$setOnInsert": bson.M{"createdAt": now}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed member person %s: %v", member.email, err)
		}
		_, err = s.db.Collection("chms_member_accounts").UpdateOne(context.Background(),
			bson.M{"organizationId": cfg.CHMSOrganizationID, "personId": member.personID},
			bson.M{"$set": bson.M{
				"name": member.given + " " + member.family, "email": member.email,
				"emailNormalized": member.email, "phoneNormalized": member.phone,
				"status": "active", "demoAccount": true, "updatedAt": now,
			}, "$setOnInsert": bson.M{"_id": accountID, "createdAt": now, "activatedAt": now, "emailVerifiedAt": now}},
			options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed member account %s: %v", member.email, err)
		}
	}
	_, err := s.db.Collection("chms_people").UpdateOne(context.Background(), bson.M{"_id": "seed-member-child"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "personNumber": "P-DEMO-004", "homeBranchId": "accra-headquarters", "names": bson.M{"given": "Nhyira", "family": "Mensah", "preferred": "Nhyira"}, "dateOfBirth": bson.M{"value": "2017-06-12", "precision": "day"}, "membershipStage": "member", "tags": bson.A{"demo-member", "child"}, "archivedAt": nil, "updatedAt": now}, "$setOnInsert": bson.M{"createdAt": now}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member child: %v", err)
	}
	_, err = s.db.Collection("chms_households").UpdateOne(context.Background(), bson.M{"_id": "seed-member-household"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "name": "Mensah household", "primaryContactPersonId": "seed-member-ama", "schemaVersion": 1, "version": int64(1), "updatedAt": now}, "$setOnInsert": bson.M{"createdAt": now}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member household: %v", err)
	}
	for _, entry := range []bson.M{{"_id": "seed-household-ama", "personId": "seed-member-ama", "role": "primary"}, {"_id": "seed-household-child", "personId": "seed-member-child", "role": "child"}} {
		_, err = s.db.Collection("chms_household_memberships").UpdateOne(context.Background(), bson.M{"_id": entry["_id"]}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "householdId": "seed-member-household", "personId": entry["personId"], "role": entry["role"], "startedAt": now.Add(-365 * 24 * time.Hour), "endedAt": nil, "schemaVersion": 1, "version": int64(1), "updatedAt": now}, "$setOnInsert": bson.M{"createdAt": now}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed household member: %v", err)
		}
	}
	actor := bson.M{"type": "system", "id": "seed"}
	_, err = s.db.Collection("chms_people_segments").UpdateOne(context.Background(), bson.M{"_id": "seed-demo-members", "organizationId": cfg.CHMSOrganizationID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "schemaVersion": 1, "version": int64(1), "name": "Demo members", "description": "Verified demo member accounts", "filter": bson.M{"branchId": "accra-headquarters", "membershipStages": bson.A{"member"}, "tags": bson.A{}, "archived": false}, "visibility": "organization", "ownerId": "seed", "archivedAt": nil, "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed communication segment: %v", err)
	}
	for _, member := range members {
		_, err = s.db.Collection("chms_consent_projections").UpdateOne(context.Background(), bson.M{"organizationId": cfg.CHMSOrganizationID, "personId": member.personID, "purpose": "church-updates", "channel": "email"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "personId": member.personID, "purpose": "church-updates", "channel": "email", "state": "granted", "noticeVersion": "communications-2026-01", "source": "demo-seed", "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"_id": "seed-consent-" + member.personID, "createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed communication consent: %v", err)
		}
	}
	for _, member := range members {
		_, err = s.db.Collection("chms_member_communication_preferences").UpdateOne(context.Background(), bson.M{"organizationId": cfg.CHMSOrganizationID, "personId": member.personID}, bson.M{"$set": bson.M{"quietEnabled": true, "quietStart": "21:00", "quietEnd": "07:00", "timezone": "Africa/Accra", "dailyCap": 3, "directoryFields": bson.A{"email", "mobile"}, "version": int64(1), "updatedAt": now}, "$setOnInsert": bson.M{"_id": "seed-communication-preference-" + member.personID, "createdAt": now}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed member communication preference: %v", err)
		}
	}
	communityID := "seed-member-community"
	_, err = s.db.Collection("chms_groups").UpdateOne(context.Background(), bson.M{"_id": communityID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "homeBranchId": "accra-headquarters", "name": "The Table", "type": "community", "description": "A warm weekly circle for prayer, honest conversation and everyday discipleship.", "leaderPersonIds": bson.A{"seed-member-kwame"}, "capacity": 18, "activeMemberCount": 3, "meetingPattern": bson.M{"frequency": "weekly", "weekday": "thursday", "localStart": "18:30", "durationMinutes": 90, "timezone": "Africa/Accra", "location": "REMI Community Room"}, "privacy": "open", "discoverability": "members", "status": "active", "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member community: %v", err)
	}
	for index, member := range members {
		visibility := "members"
		if index == 2 {
			visibility = "hidden"
		}
		role := "member"
		if member.personID == "seed-member-kwame" {
			role = "facilitator"
		}
		_, err = s.db.Collection("chms_group_memberships").UpdateOne(context.Background(), bson.M{"_id": "seed-member-community-" + member.personID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "groupId": communityID, "personId": member.personID, "role": role, "status": "active", "source": "demo-seed", "directoryVisibility": visibility, "joinedAt": now.Add(-45 * 24 * time.Hour), "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed member community membership: %v", err)
		}
	}
	_, err = s.db.Collection("chms_member_group_messages").UpdateOne(context.Background(), bson.M{"_id": "seed-member-group-message-welcome"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "groupId": communityID, "personId": "seed-member-kwame", "message": "Welcome to The Table. Share one thing we can celebrate with you this week.", "state": "published", "createdAt": now.Add(-2 * time.Hour)}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member group message: %v", err)
	}
	for index, member := range members {
		deliveryID := fmt.Sprintf("66c8f950000000000000000%d", index+1)
		_, err = s.db.Collection("chms_communication_deliveries").UpdateOne(context.Background(), bson.M{"_id": deliveryID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "campaignId": "seed-member-welcome-campaign", "personId": member.personID, "channel": "email", "destinationHint": "***@remi.church", "inboxSubject": "Your week at REMI", "inboxBody": "Hello " + member.given + ". The Table gathers Thursday at 6:30 PM. We would love to see you there.", "purpose": "church-updates", "state": "accepted", "attemptCount": 1, "createdAt": now.Add(-4 * time.Hour), "updatedAt": now.Add(-4 * time.Hour)}}, options.UpdateOne().SetUpsert(true))
		if err != nil {
			log.Fatalf("seed member inbox delivery: %v", err)
		}
	}
	nextMeeting := now.AddDate(0, 0, 7)
	_, err = s.db.Collection("chms_group_meetings").UpdateOne(context.Background(), bson.M{"_id": "seed-member-community-meeting"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "groupId": communityID, "topic": "Stories around the table", "startsAt": nextMeeting, "endsAt": nextMeeting.Add(90 * time.Minute), "timezone": "Africa/Accra", "location": "REMI Community Room", "status": "scheduled", "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member community meeting: %v", err)
	}
	_, err = s.db.Collection("chms_volunteer_teams").UpdateOne(context.Background(), bson.M{"_id": "seed-member-serving-team"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "homeBranchId": "accra-headquarters", "name": "Welcome & Hospitality", "description": "Create a generous first impression from the gate to the gathering.", "leaderPersonIds": bson.A{"seed-member-kwame"}, "status": "active", "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member serving team: %v", err)
	}
	_, err = s.db.Collection("chms_volunteer_positions").UpdateOne(context.Background(), bson.M{"_id": "seed-member-welcome-host"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "teamId": "seed-member-serving-team", "name": "Welcome host", "description": "Welcome people and help them find their next step.", "eligibility": bson.M{"minimumAgeYears": 16, "membershipStages": bson.A{"member", "serving-member", "leader"}, "requiredSkills": bson.A{"hospitality"}}, "status": "active", "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member serving position: %v", err)
	}
	servingStart := now.Add(72 * time.Hour)
	_, err = s.db.Collection("chms_service_plans").UpdateOne(context.Background(), bson.M{"_id": "seed-member-serving-plan"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "occurrenceId": "seed-member-serving-occurrence", "name": "Sunday Celebration", "startsAt": servingStart, "endsAt": servingStart.Add(2 * time.Hour), "needs": bson.A{bson.M{"positionId": "seed-member-welcome-host", "slots": 2}}, "status": "published", "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member serving plan: %v", err)
	}
	_, err = s.db.Collection("chms_assignments").UpdateOne(context.Background(), bson.M{"_id": "seed-member-serving-assignment"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "planId": "seed-member-serving-plan", "occurrenceId": "seed-member-serving-occurrence", "teamId": "seed-member-serving-team", "positionId": "seed-member-welcome-host", "personId": "seed-member-ama", "slot": 1, "startsAt": servingStart, "endsAt": servingStart.Add(2 * time.Hour), "status": "invited", "reminder": bson.M{"sentCount": 1, "lastSentAt": now}, "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member serving assignment: %v", err)
	}
	_, err = s.db.Collection("chms_volunteer_profiles").UpdateOne(context.Background(), bson.M{"_id": "seed-member-serving-profile"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "personId": "seed-member-ama", "skills": bson.A{"hospitality", "prayer"}, "preferredTeamIds": bson.A{"seed-member-serving-team"}, "preferredPositionIds": bson.A{"seed-member-welcome-host"}, "status": "active", "eligibility": bson.M{"backgroundCheckStatus": "not-required"}, "schemaVersion": 1, "version": int64(1), "updatedAt": now, "updatedBy": actor}, "$setOnInsert": bson.M{"createdAt": now, "createdBy": actor}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed member serving profile: %v", err)
	}
	if err := db.EnsureMemberAuthIndexes(context.Background(), s.db); err != nil {
		log.Fatalf("seed member indexes: %v", err)
	}
	log.Printf("seeded %d passwordless member accounts", len(members))
}

func (s *seeder) settings() {
	doc := bson.M{
		"churchName": "Ruach Elohim Ministries International",
		"tagline":    "Raising a prophetic people, filled with the Spirit, transforming nations.",
		"serviceTimes": bson.A{
			bson.M{"name": "First Service", "day": "Sunday", "time": "08:00 GMT", "location": "Accra Headquarters"},
			bson.M{"name": "Second Service", "day": "Sunday", "time": "10:30 GMT", "location": "Accra Headquarters"},
			bson.M{"name": "Midweek Service", "day": "Wednesday", "time": "18:30 GMT", "location": "Accra Headquarters"},
		},
		"socials": bson.M{
			"facebook":  "https://facebook.com/ruachelohimministries",
			"instagram": "https://instagram.com/ruachelohimministries",
			"youtube":   "https://youtube.com/@ruachelohimministries",
			"tiktok":    "https://tiktok.com/@ruachelohimministries",
		},
		"whatsapp":      "+233 24 000 0000",
		"phone":         "+233 30 000 0000",
		"email":         "info@remi.church",
		"address":       "REMI Auditorium, Spintex Road, Accra, Ghana",
		"livestreamUrl": "https://youtube.com/@ruachelohimministries/live",
		"isLive":        false,
		"givingCategories": bson.A{
			"Tithe", "Offering", "Seed", "Missions", "Building Fund", "Welfare",
		},
		"nextServiceOverride": nil,
	}
	_, err := s.db.Collection("settings").UpdateOne(context.Background(),
		bson.M{}, bson.M{"$set": doc}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatal(err)
	}
	log.Println("seeded settings")
}

func (s *seeder) pages() {
	pages := []bson.M{
		{
			"title": "Our History", "slug": "history", "pageKey": "history",
			"subtitle":  "From a living-room prayer meeting to a prophetic movement.",
			"heroImage": img("remi-history"),
			"body": `<p>Ruach Elohim Ministries International began in 2003 in the living room of Dr. Ismaila Hans Awudu, where a small band of believers gathered every Friday night to seek the face of God. What started as twelve people praying for the nation of Ghana quickly became a furnace of the Holy Spirit, marked by prophecy, healing and an unusual hunger for the Word.</p>
<p>By 2007 the fellowship had outgrown three rented venues, and the Lord spoke clearly about establishing an apostolic hub in Accra. The first auditorium on Spintex Road was dedicated with a three-day outpouring that drew believers from across West Africa.</p>
<p>Today REMI is a family of worshippers across two branches, with missions partnerships reaching into the nations. Our story is still being written — and every chapter testifies that the Spirit of God still builds His church.</p>`,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "Our Mission", "slug": "mission", "pageKey": "mission",
			"subtitle":  "Raising a prophetic people for the nations.",
			"heroImage": img("remi-mission"),
			"body": `<p>Our mission is simple and burning: to raise a people who carry the presence of God into every sphere of society. We exist to make disciples who hear God's voice, walk in holiness, and demonstrate the power of the Kingdom with compassion.</p>
<p>We pursue this through fervent prayer, sound apostolic teaching, prophetic equipping, and practical love for our communities. Every ministry, every service, and every outreach at REMI is measured against one question — does it form Christ in people?</p>
<p>We believe the local church is God's answer for the city. From Accra to Kumasi and beyond, we are building altars of worship and sending labourers into the harvest.</p>`,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "What We Believe", "slug": "beliefs", "pageKey": "beliefs",
			"subtitle":  "Anchored in Scripture, alive in the Spirit.",
			"heroImage": img("remi-beliefs"),
			"body": `<p>We believe the Bible is the inspired and authoritative Word of God. We believe in one God, eternally existing as Father, Son and Holy Spirit, and in the full deity and humanity of Jesus Christ — His virgin birth, sinless life, atoning death, bodily resurrection and coming return.</p>
<p>We believe salvation is by grace through faith in Christ alone, and that the Holy Spirit fills, empowers and gifts every believer for service. We honour the present ministry of the five-fold gifts — apostles, prophets, evangelists, pastors and teachers — for the equipping of the saints.</p>
<p>We believe in water baptism, the Lord's Table, divine healing, the resurrection of the dead, and the blessed hope of Christ's return. Above all, we believe the Church is called to be a house of prayer for all nations.</p>`,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "Plan Your Visit", "slug": "visit", "pageKey": "visit",
			"subtitle":  "You are family already — come as you are.",
			"heroImage": img("remi-visit"),
			"body": `<p>We would love to welcome you home. Our Sunday services run at 8:00 and 10:30 GMT, and our Wednesday midweek service begins at 18:30. Come early for pre-service prayer — it is where the atmosphere is set.</p>
<p>When you arrive, our welcome team will greet you at the gate, help you find a seat, and answer any questions. There is no dress code; come as you are. Children's church runs during both Sunday services, led by trained and vetted volunteers.</p>
<p>After the service, stop by the New Here lounge. We would be honoured to meet you, pray with you, and help you find your place in the family. Fill in the visit form and we will be expecting you.</p>`,
			"contentStatus": models.StatusAIDraft, // left in the review queue
		},
	}
	for _, p := range pages {
		s.upsertContent("pages", p)
	}
	log.Printf("seeded %d pages", len(pages))
}

func (s *seeder) leaders() {
	leaders := []bson.M{
		{
			"title": "Dr. Ismaila Hans Awudu", "slug": "dr-ismaila-hans-awudu",
			"name": "Dr. Ismaila Hans Awudu", "position": "Founder & Apostolic Overseer",
			"bio":   "Dr. Ismaila Hans Awudu is the founder of Ruach Elohim Ministries International. A prophet and teacher of the Word, he carries a mandate to raise a praying, prophetic people for the nations. For over two decades he has pastored the REMI family in Accra with a message of holiness, prayer and the manifested presence of God.",
			"photo": img("leader-awudu"), "order": 1, "isFounder": true,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "Rev. Abena Serwaa", "slug": "rev-abena-serwaa",
			"name": "Rev. Abena Serwaa", "position": "Resident Pastor, Accra HQ",
			"bio":   "Rev. Abena oversees the day-to-day pastoral life of the Accra headquarters. She is a gifted shepherd with a deep passion for discipleship, counselling and raising strong families in the Word.",
			"photo": img("leader-abena"), "order": 2, "isFounder": false,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "Ps. Kwame Osei-Bonsu", "slug": "ps-kwame-osei-bonsu",
			"name": "Ps. Kwame Osei-Bonsu", "position": "Branch Pastor, Kumasi",
			"bio":   "Ps. Kwame leads the Kumasi branch with a heart for evangelism and community transformation. Under his leadership the branch has grown into a vibrant centre of worship and outreach in the Ashanti Region.",
			"photo": img("leader-kwame"), "order": 3, "isFounder": false,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "Ps. Efua Mensimah", "slug": "ps-efua-mensimah",
			"name": "Ps. Efua Mensimah", "position": "Worship & Intercession Director",
			"bio":   "Ps. Efua leads REMI's worship teams and the house of prayer. Her ministry carries a tangible anointing that ushers congregations into deep, Spirit-led worship and intercession.",
			"photo": img("leader-efua"), "order": 4, "isFounder": false,
			"contentStatus": models.StatusPublished,
		},
		{
			"title": "Min. Yaw Darko", "slug": "min-yaw-darko",
			"name": "Min. Yaw Darko", "position": "Youth & Missions Pastor",
			"bio":   "Min. Yaw pastors the youth ministry and coordinates REMI's missions partnerships. He is raising a generation of young believers who are bold in witness and grounded in Scripture.",
			"photo": img("leader-yaw"), "order": 5, "isFounder": false,
			"contentStatus": models.StatusPublished,
		},
	}
	for _, l := range leaders {
		s.upsertContent("leaders", l)
	}
	log.Printf("seeded %d leaders", len(leaders))
}

func (s *seeder) ministries() {
	ministries := []bson.M{
		{"title": "Intercessory Prayer", "slug": "intercessory-prayer", "name": "Intercessory Prayer",
			"description": "The engine room of REMI. Our intercessors stand in the gap for the church, the city and the nations through weekly all-night prayer and daily altars.",
			"leaderName":  "Ps. Efua Mensimah", "meetingTime": "Fridays 22:00 GMT",
			"image": img("min-prayer"), "contentStatus": models.StatusPublished},
		{"title": "Worship & Music", "slug": "worship-music", "name": "Worship & Music",
			"description": "A team of musicians and singers committed to leading the congregation into the presence of God with excellence and spontaneity.",
			"leaderName":  "Ps. Efua Mensimah", "meetingTime": "Rehearsals Thursdays 18:00 GMT",
			"image": img("min-worship"), "contentStatus": models.StatusPublished},
		{"title": "Children's Church", "slug": "childrens-church", "name": "Children's Church",
			"description": "Raising boys and girls who know God's voice early. Bible teaching, worship and fun in a safe environment during both Sunday services.",
			"leaderName":  "Mrs. Akosua Frimpong", "meetingTime": "Sundays 08:00 & 10:30 GMT",
			"image": img("min-children"), "contentStatus": models.StatusPublished},
		{"title": "Youth Ablaze", "slug": "youth-ablaze", "name": "Youth Ablaze",
			"description": "A generation on fire. Youth Ablaze gathers students and young professionals for Word, worship and authentic community.",
			"leaderName":  "Min. Yaw Darko", "meetingTime": "Saturdays 16:00 GMT",
			"image": img("min-youth"), "contentStatus": models.StatusPublished},
		{"title": "Women of Virtue", "slug": "women-of-virtue", "name": "Women of Virtue",
			"description": "Equipping women to flourish in God, family and purpose through fellowship, mentoring and outreach.",
			"leaderName":  "Rev. Abena Serwaa", "meetingTime": "2nd Saturdays 09:00 GMT",
			"image": img("min-women"), "contentStatus": models.StatusPublished},
		{"title": "Men of Valour", "slug": "men-of-valour", "name": "Men of Valour",
			"description": "Calling men to godly leadership at home, at work and in the church — through breakfast meetings, accountability and service.",
			"leaderName":  "Ps. Kwame Osei-Bonsu", "meetingTime": "1st Saturdays 07:30 GMT",
			"image": img("min-men"), "contentStatus": models.StatusPublished},
	}
	for _, m := range ministries {
		s.upsertContent("ministries", m)
	}
	log.Printf("seeded %d ministries", len(ministries))
}

func (s *seeder) branches() {
	branches := []bson.M{
		{"title": "REMI Accra (Headquarters)", "slug": "accra-headquarters",
			"name": "REMI Accra (Headquarters)", "address": "REMI Auditorium, Spintex Road",
			"city": "Accra", "phone": "+233 30 000 0000", "email": "accra@remi.church",
			"pastorName":   "Rev. Abena Serwaa",
			"serviceTimes": "Sun 08:00 & 10:30 GMT, Wed 18:30 GMT",
			"mapQuery":     "Spintex Road, Accra, Ghana",
			"image":        img("branch-accra"), "contentStatus": models.StatusPublished},
		{"title": "REMI Kumasi", "slug": "kumasi",
			"name": "REMI Kumasi", "address": "12 Harper Road, Adum",
			"city": "Kumasi", "phone": "+233 32 000 0000", "email": "kumasi@remi.church",
			"pastorName":   "Ps. Kwame Osei-Bonsu",
			"serviceTimes": "Sun 09:00 GMT, Thu 18:00 GMT",
			"mapQuery":     "Adum, Kumasi, Ghana",
			"image":        img("branch-kumasi"), "contentStatus": models.StatusPublished},
	}
	for _, b := range branches {
		s.upsertContent("branches", b)
	}
	log.Printf("seeded %d branches", len(branches))
}

func (s *seeder) retentionDemo(cfg *config.Config) {
	now := models.Now()
	visitAt := now.AddDate(0, 0, -35)
	actor := bson.M{"type": "system", "id": "seed"}
	approvalAt := now.AddDate(0, 0, -60)
	_, err := s.db.Collection("chms_occurrences").UpdateOne(context.Background(), bson.M{"_id": "seed-retention-occurrence"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "homeBranchId": "accra-headquarters", "occurrenceKey": "seed-retention-occurrence", "startsAt": visitAt, "endsAt": visitAt.Add(2 * time.Hour), "status": "scheduled", "updatedAt": now}, "$setOnInsert": bson.M{"createdAt": visitAt}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed retention occurrence: %v", err)
	}
	_, err = s.db.Collection("chms_attendance").UpdateOne(context.Background(), bson.M{"_id": "seed-retention-attendance"}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "occurrenceId": "seed-retention-occurrence", "personId": "seed-member-esi", "status": "present", "guest": true, "recordedAt": visitAt.Add(2 * time.Hour), "updatedAt": visitAt.Add(2 * time.Hour)}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed retention attendance: %v", err)
	}
	// Five mature weekly cohorts make the privacy-safe aggregate dashboard
	// useful in local demos. Reserved seed IDs keep reruns deterministic and
	// never overwrite operator-created records.
	for cohort := 0; cohort < 5; cohort++ {
		firstAt := now.AddDate(0, 0, -(155 - cohort*7))
		firstOccurrenceID := fmt.Sprintf("seed-cohort-%d-first", cohort)
		return30OccurrenceID := fmt.Sprintf("seed-cohort-%d-return-30", cohort)
		return60OccurrenceID := fmt.Sprintf("seed-cohort-%d-return-60", cohort)
		for occurrenceID, startsAt := range map[string]time.Time{firstOccurrenceID: firstAt, return30OccurrenceID: firstAt.AddDate(0, 0, 20), return60OccurrenceID: firstAt.AddDate(0, 0, 45)} {
			_, err = s.db.Collection("chms_occurrences").UpdateOne(context.Background(), bson.M{"_id": occurrenceID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "homeBranchId": "accra-headquarters", "occurrenceKey": occurrenceID, "name": "REMI Sunday Gathering", "startsAt": startsAt, "endsAt": startsAt.Add(2 * time.Hour), "status": "scheduled", "updatedAt": startsAt.Add(2 * time.Hour)}, "$setOnInsert": bson.M{"createdAt": startsAt}}, options.UpdateOne().SetUpsert(true))
			if err != nil {
				log.Fatalf("seed retention cohort occurrence: %v", err)
			}
		}
		return30Count := 2 + cohort
		return60Count := return30Count + 2
		if return60Count > 6 {
			return60Count = 6
		}
		for member := 0; member < 6; member++ {
			personID := fmt.Sprintf("seed-cohort-%d-person-%d", cohort, member)
			_, err = s.db.Collection("chms_people").UpdateOne(context.Background(), bson.M{"_id": personID, "organizationId": cfg.CHMSOrganizationID}, bson.M{"$setOnInsert": bson.M{"organizationId": cfg.CHMSOrganizationID, "homeBranchId": "accra-headquarters", "schemaVersion": 1, "version": int64(1), "personNumber": fmt.Sprintf("DEMO-%02d%02d", cohort+1, member+1), "names": bson.M{"given": fmt.Sprintf("Demo %d", cohort+1), "family": fmt.Sprintf("Guest %d", member+1)}, "membershipStage": "guest", "contactPoints": bson.A{}, "addresses": bson.A{}, "tags": bson.A{"retention-demo"}, "archivedAt": nil, "createdAt": firstAt, "updatedAt": firstAt}}, options.UpdateOne().SetUpsert(true))
			if err != nil {
				log.Fatalf("seed retention cohort person: %v", err)
			}
			seedAttendance := func(suffix, occurrenceID string, guest bool, recordedAt time.Time) {
				attendanceID := fmt.Sprintf("seed-cohort-%d-person-%d-%s", cohort, member, suffix)
				_, attendanceErr := s.db.Collection("chms_attendance").UpdateOne(context.Background(), bson.M{"_id": attendanceID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "occurrenceId": occurrenceID, "personId": personID, "status": "present", "guest": guest, "recordedAt": recordedAt, "updatedAt": recordedAt}}, options.UpdateOne().SetUpsert(true))
				if attendanceErr != nil {
					log.Fatalf("seed retention cohort attendance: %v", attendanceErr)
				}
			}
			seedAttendance("first", firstOccurrenceID, true, firstAt.Add(2*time.Hour))
			if member < return30Count {
				seedAttendance("return-30", return30OccurrenceID, false, firstAt.AddDate(0, 0, 20).Add(2*time.Hour))
			} else if member < return60Count {
				seedAttendance("return-60", return60OccurrenceID, false, firstAt.AddDate(0, 0, 45).Add(2*time.Hour))
			}
			if member < cohort+1 {
				joinedAt := firstAt.AddDate(0, 0, 35)
				membershipID := fmt.Sprintf("seed-cohort-%d-group-%d", cohort, member)
				_, err = s.db.Collection("chms_group_memberships").UpdateOne(context.Background(), bson.M{"_id": membershipID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "personId": personID, "groupId": "seed-retention-demo-group", "status": "active", "joinedAt": joinedAt, "updatedAt": joinedAt}}, options.UpdateOne().SetUpsert(true))
				if err != nil {
					log.Fatalf("seed retention group connection: %v", err)
				}
			}
			if member >= 3-cohort/2 {
				startsAt := firstAt.AddDate(0, 0, 55)
				assignmentID := fmt.Sprintf("seed-cohort-%d-serving-%d", cohort, member)
				_, err = s.db.Collection("chms_assignments").UpdateOne(context.Background(), bson.M{"_id": assignmentID}, bson.M{"$set": bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "personId": personID, "teamId": "seed-retention-demo-team", "planId": fmt.Sprintf("seed-retention-plan-%d", cohort), "positionId": "seed-retention-host", "slot": member + 1, "status": "accepted", "startsAt": startsAt, "endsAt": startsAt.Add(2 * time.Hour), "updatedAt": startsAt}}, options.UpdateOne().SetUpsert(true))
				if err != nil {
					log.Fatalf("seed retention serving connection: %v", err)
				}
			}
		}
	}
	rule := bson.M{"_id": "seed-retention-rule", "organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "schemaVersion": 1, "version": int64(1), "createdAt": approvalAt, "createdBy": actor, "updatedAt": approvalAt, "updatedBy": actor, "name": "First visit return review", "kind": "first-visit-no-return", "description": "A mature first visit with no later qualifying local date. Human context is required.", "timezone": "Africa/Accra", "windowDays": 30, "lookbackDays": 180, "expiresAfterDays": 14, "status": "published", "metricVersion": "retention-v1", "approval": bson.M{"productOwnerId": "seed-product-owner", "pastoralApproverId": "seed-pastoral-owner", "privacyApproverId": "seed-privacy-owner", "approvedAt": approvalAt, "reviewCadenceDays": 30}, "publishedAt": approvalAt}
	_, err = s.db.Collection("chms_engagement_rules").UpdateOne(context.Background(), bson.M{"_id": "seed-retention-rule", "organizationId": cfg.CHMSOrganizationID}, bson.M{"$set": rule}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed retention rule: %v", err)
	}
	signal := bson.M{"_id": "seed-retention-signal", "organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters", "schemaVersion": 1, "version": int64(1), "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor, "ruleId": "seed-retention-rule", "ruleVersion": int64(1), "ruleName": "First visit return review", "ruleKind": "first-visit-no-return", "personId": "seed-member-esi", "observedFrom": visitAt, "observedThrough": now, "evidence": bson.A{bson.M{"sourceType": "attendance", "sourceId": "seed-retention-attendance", "occurredAt": visitAt, "fact": "first qualifying guest visit"}}, "caveats": bson.A{"This is an observed participation gap, not a judgment of faith, intent, worth or pastoral need.", "A human reviewer must verify context and consent before any action."}, "state": "open", "sourceKey": "seed-retention-rule:1:seed-member-esi:seed-retention-attendance", "expiresAt": now.AddDate(0, 0, 14)}
	if _, err = s.db.Collection("chms_engagement_review_events").DeleteMany(context.Background(), bson.M{"organizationId": cfg.CHMSOrganizationID, "signalId": "seed-retention-signal"}); err != nil {
		log.Fatalf("reset demo retention review history: %v", err)
	}
	_, err = s.db.Collection("chms_engagement_signals").UpdateOne(context.Background(), bson.M{"_id": "seed-retention-signal", "organizationId": cfg.CHMSOrganizationID}, bson.M{"$set": signal, "$unset": bson.M{"assigneeId": "", "snoozedUntil": "", "resolutionOutcome": "", "falsePositive": "", "resolvedAt": ""}}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		log.Fatalf("seed retention signal: %v", err)
	}
	log.Println("seeded one local retention review observation")
}

func (s *seeder) finance(cfg *config.Config) {
	now := models.Now()
	actor := bson.M{"type": "system", "id": "seed"}
	envelope := func(id, branch string) bson.M {
		return bson.M{"_id": id, "organizationId": cfg.CHMSOrganizationID, "branchId": branch, "schemaVersion": 1, "version": int64(1), "createdAt": now, "createdBy": actor, "updatedAt": now, "updatedBy": actor, "archivedAt": nil}
	}
	upsert := func(collection string, filter, set bson.M) {
		// Finance fixtures are bootstrap-only. Re-running the seed must never
		// reset receipt counters, lifecycle state or operator changes.
		if _, err := s.db.Collection(collection).UpdateOne(context.Background(), filter, bson.M{"$setOnInsert": set}, options.UpdateOne().SetUpsert(true)); err != nil {
			log.Fatalf("seed finance %s: %v", collection, err)
		}
	}
	campus := envelope("seed-finance-campus-accra", "accra-headquarters")
	campus["name"], campus["timezone"], campus["currency"] = "REMI Accra (Headquarters)", "Africa/Accra", "GHS"
	upsert("chms_finance_campuses", bson.M{"organizationId": cfg.CHMSOrganizationID, "branchId": "accra-headquarters"}, campus)
	method := envelope("seed-finance-method-paystack", "")
	method["code"], method["name"], method["kind"], method["provider"], method["active"] = "PAYSTACK", "Paystack online", "card", "Paystack", true
	upsert("chms_finance_payment_methods", bson.M{"organizationId": cfg.CHMSOrganizationID, "code": "PAYSTACK"}, method)
	for _, item := range []struct{ id, code, name, restriction string }{
		{"seed-finance-fund-tithe", "TITHE", "Tithe", "unrestricted"},
		{"seed-finance-fund-offering", "OFFERING", "Offering", "unrestricted"},
		{"seed-finance-fund-seed", "SEED", "Seed", "unrestricted"},
		{"seed-finance-fund-missions", "MISSIONS", "Missions", "temporarily-restricted"},
		{"seed-finance-fund-building", "BUILDING_FUND", "Building Fund", "temporarily-restricted"},
		{"seed-finance-fund-welfare", "WELFARE", "Welfare", "board-designated"},
	} {
		fund := envelope(item.id, "")
		fund["code"], fund["name"], fund["restrictionType"], fund["activeFrom"], fund["activeUntil"] = item.code, item.name, item.restriction, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil
		upsert("chms_finance_funds", bson.M{"organizationId": cfg.CHMSOrganizationID, "code": item.code}, fund)
	}
	campaign := envelope("seed-campaign-community-centre", "")
	campaign["slug"], campaign["title"] = "build-the-community-centre", "Build the REMI Community Centre"
	campaign["summary"] = "Help create a welcoming home for discipleship, youth mentoring, counselling and practical community care."
	campaign["story"] = "We are building a flexible community centre where children can learn, families can find support and our city can encounter the practical love of Christ throughout the week. Every gift moves the project from plans to a place people can call home."
	campaign["coverImageUrl"], campaign["fundId"] = img("branch-accra"), "seed-finance-fund-building"
	campaign["goal"] = bson.M{"amountMinor": int64(2_500_000_00), "currency": "GHS"}
	campaign["startsAt"], campaign["endsAt"] = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	campaign["status"], campaign["featured"] = "published", true
	upsert("chms_finance_campaigns", bson.M{"organizationId": cfg.CHMSOrganizationID, "slug": campaign["slug"]}, campaign)
	period := envelope("seed-finance-period-2026", "")
	period["code"], period["name"], period["startsAt"], period["endsAt"], period["status"] = "FY2026", "2026 fiscal year", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "open"
	upsert("chms_finance_periods", bson.M{"organizationId": cfg.CHMSOrganizationID, "code": "FY2026"}, period)
	sequence := envelope("seed-finance-receipts-2026", "")
	sequence["code"], sequence["prefix"], sequence["fiscalYear"], sequence["padding"], sequence["nextNumber"] = "MAIN2026", "REMI", 2026, 6, int64(1)
	upsert("chms_finance_receipt_sequences", bson.M{"organizationId": cfg.CHMSOrganizationID, "code": "MAIN2026"}, sequence)
	log.Println("seeded online giving finance configuration")
}

func (s *seeder) sermons() {
	d := func(daysAgo int) time.Time { return models.Now().AddDate(0, 0, -daysAgo) }
	sermons := []bson.M{
		{"title": "The God Who Speaks", "slug": "the-god-who-speaks",
			"preacher": "Dr. Ismaila Hans Awudu", "series": "Hearing God", "topic": "Prophetic",
			"bibleRefs": bson.A{"1 Samuel 3:1-10", "John 10:27"}, "date": d(7),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-god-speaks"),
			"description":   "God is still speaking. Learn how to recognise His voice, test what you hear, and respond in obedience.",
			"contentStatus": models.StatusPublished},
		{"title": "Tuning Your Ear", "slug": "tuning-your-ear",
			"preacher": "Dr. Ismaila Hans Awudu", "series": "Hearing God", "topic": "Prophetic",
			"bibleRefs": bson.A{"1 Kings 19:11-13", "Psalm 46:10"}, "date": d(14),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-tuning-ear"),
			"description":   "The still small voice is learned, not luck. Practical disciplines for a listening life.",
			"contentStatus": models.StatusPublished},
		{"title": "When God Is Silent", "slug": "when-god-is-silent",
			"preacher": "Rev. Abena Serwaa", "series": "Hearing God", "topic": "Faith",
			"bibleRefs": bson.A{"Psalm 13:1-6", "Habakkuk 2:1-3"}, "date": d(21),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-silent"),
			"description":   "Silence is not absence. How to trust the character of God in the seasons when heaven seems quiet.",
			"contentStatus": models.StatusPublished},
		{"title": "Altars of Fire", "slug": "altars-of-fire",
			"preacher": "Dr. Ismaila Hans Awudu", "series": "The Prayer Life", "topic": "Prayer",
			"bibleRefs": bson.A{"1 Kings 18:36-39", "James 5:16-18"}, "date": d(28),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-altars"),
			"description":   "Elijah prayed and fire fell. A call to rebuild personal and family altars of prayer.",
			"contentStatus": models.StatusPublished},
		{"title": "Watching in the Night", "slug": "watching-in-the-night",
			"preacher": "Ps. Efua Mensimah", "series": "The Prayer Life", "topic": "Prayer",
			"bibleRefs": bson.A{"Luke 6:12", "Psalm 63:6"}, "date": d(35),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-watching"),
			"description":   "The ministry of night prayer — why the watches matter and how to keep them without striving.",
			"contentStatus": models.StatusAIDraft}, // in review queue
		{"title": "Prevailing Prayer", "slug": "prevailing-prayer",
			"preacher": "Dr. Ismaila Hans Awudu", "series": "The Prayer Life", "topic": "Prayer",
			"bibleRefs": bson.A{"Genesis 32:24-28", "Colossians 4:12"}, "date": d(42),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-prevailing"),
			"description":   "Jacob wrestled and would not let go. Prayer that prevails until the blessing breaks through.",
			"contentStatus": models.StatusPublished},
		{"title": "Built to Last", "slug": "built-to-last",
			"preacher": "Min. Yaw Darko", "series": "Kingdom Foundations", "topic": "Discipleship",
			"bibleRefs": bson.A{"Matthew 7:24-27", "1 Corinthians 3:10-15"}, "date": d(49),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-built"),
			"description":   "Foundations determine futures. Building a life that survives storms on the rock of obedience.",
			"contentStatus": models.StatusPublished},
		{"title": "The Sent Ones", "slug": "the-sent-ones",
			"preacher": "Ps. Kwame Osei-Bonsu", "series": "Kingdom Foundations", "topic": "Missions",
			"bibleRefs": bson.A{"Isaiah 6:8", "Romans 10:14-15"}, "date": d(56),
			"videoUrl": videoPlaceholder, "audioUrl": "", "notesUrl": "",
			"image":         img("sermon-sent"),
			"description":   "Every believer is sent. Rediscovering the missionary call of the local church.",
			"contentStatus": models.StatusAIDraft}, // in review queue
	}
	for _, sdoc := range sermons {
		s.upsertContent("sermons", sdoc)
	}
	log.Printf("seeded %d sermons", len(sermons))
}

func (s *seeder) events() {
	now := models.Now()
	events := []bson.M{
		{"title": "Night of Glory 2026", "slug": "night-of-glory-2026",
			"description": "Our annual all-night worship and prayer encounter. Six hours of unbroken worship, prophecy and intercession for Ghana and the nations.",
			"startAt":     now.AddDate(0, 0, 10), "endAt": now.AddDate(0, 0, 11),
			"location": "REMI Auditorium, Spintex Road, Accra", "image": img("event-night-glory"),
			"registrationEnabled": true, "capacity": 500, "registeredCount": 0, "waitlistEnabled": true, "childPrecheckEnabled": true,
			"registrationQuestions": bson.A{bson.M{"id": "arrival", "label": "How will you attend?", "type": "choice", "required": true, "options": bson.A{"In person", "Online"}}, bson.M{"id": "accessibility", "label": "Do you need accessibility support?", "type": "boolean", "required": false}},
			"contentStatus":         models.StatusPublished},
		{"title": "Prophetic Equipping School", "slug": "prophetic-equipping-school",
			"description": "A four-week Saturday school with Dr. Awudu on hearing God, journaling, and ministering prophecy with humility and accuracy.",
			"startAt":     now.AddDate(0, 0, 21), "endAt": now.AddDate(0, 0, 42),
			"location": "REMI Kumasi & online", "image": img("event-equipping"),
			"registrationEnabled": true, "capacity": 120, "registeredCount": 0, "paymentRequired": true, "priceMinor": int64(15000), "currency": "GHS", "waitlistEnabled": true,
			"registrationQuestions": bson.A{bson.M{"id": "campus", "label": "Choose your learning room", "type": "choice", "required": true, "options": bson.A{"Kumasi classroom", "Online room"}}},
			"contentStatus":         models.StatusPublished},
		{"title": "Easter Convention 2026", "slug": "easter-convention-2026",
			"description": "Three days of resurrection power — crusade nights, healing services and the sunrise celebration.",
			"startAt":     now.AddDate(0, 0, -40), "endAt": now.AddDate(0, 0, -37),
			"location": "REMI Auditorium, Accra", "image": img("event-easter"),
			"registrationEnabled": false, "capacity": 0, "registeredCount": 0,
			"contentStatus": models.StatusPublished},
		{"title": "Community Outreach: Nima", "slug": "community-outreach-nima",
			"description": "Free medical screening, food distribution and an open-air gospel rally with the Nima community.",
			"startAt":     now.AddDate(0, 0, -15), "endAt": now.AddDate(0, 0, -15),
			"location": "Nima Community Park, Accra", "image": img("event-outreach"),
			"registrationEnabled": false, "capacity": 0, "registeredCount": 0,
			"contentStatus": models.StatusPublished},
	}
	for _, e := range events {
		s.upsertContent("events", e)
	}
	log.Printf("seeded %d events", len(events))
}

func (s *seeder) announcements() {
	now := models.Now()
	announcements := []bson.M{
		{"title": "Midweek service moves to 18:30", "slug": "midweek-service-time",
			"body":      "From this month, our Wednesday midweek service begins at 18:30 GMT prompt. Pre-service prayer starts at 18:00.",
			"publishAt": now.AddDate(0, 0, -5), "expiresAt": now.AddDate(0, 1, 0),
			"contentStatus": models.StatusPublished},
		{"title": "Night of Glory registration is open", "slug": "night-of-glory-registration",
			"body":      "Seats are limited for Night of Glory 2026. Register online to secure your place — entry is free.",
			"publishAt": now.AddDate(0, 0, -3), "expiresAt": now.AddDate(0, 0, 12),
			"contentStatus": models.StatusPublished},
		{"title": "New believers class begins", "slug": "new-believers-class",
			"body":      "Foundations class for new believers starts this Sunday after second service in Classroom 2. All are welcome.",
			"publishAt": now.AddDate(0, 0, -1), "expiresAt": nil,
			"contentStatus": models.StatusPublished},
	}
	for _, a := range announcements {
		s.upsertContent("announcements", a)
	}
	log.Printf("seeded %d announcements", len(announcements))
}

func (s *seeder) testimonies() {
	testimonies := []bson.M{
		{"title": "Healed after seven years", "slug": "healed-after-seven-years",
			"author": "Gifty A.", "image": img("testimony-gifty"),
			"body":          "I suffered chronic back pain for seven years. During the Easter Convention, Pastor called out my condition and the power of God touched me. I have been completely pain-free since that night. To God be the glory!",
			"contentStatus": models.StatusPublished},
		{"title": "A job against all odds", "slug": "a-job-against-all-odds",
			"author": "Kwabena M.", "image": img("testimony-kwabena"),
			"body":          "After two years of unemployment, I sowed my last seed in faith as Dr. Awudu taught. Three weeks later I received an offer I had not even applied for, with double my previous salary. God is faithful.",
			"contentStatus": models.StatusPublished},
		{"title": "My marriage restored", "slug": "my-marriage-restored",
			"author": "Anonymous", "image": img("testimony-marriage"),
			"body":          "My husband and I were weeks from signing divorce papers. The intercessory team prayed with us for a month. Today we are not only together — we serve God side by side. Nothing is too hard for the Lord.",
			"contentStatus": models.StatusPublished},
		{"title": "Delivered from addiction", "slug": "delivered-from-addiction",
			"author": "Samuel T.", "image": img("testimony-samuel"),
			"body":          "Alcohol controlled me for a decade. One encounter at the altar during Night of Glory broke what rehab could not. Two years clean, and now I mentor other young men walking the same road.",
			"contentStatus": models.StatusPublished},
	}
	for _, t := range testimonies {
		s.upsertContent("testimonies", t)
	}
	log.Printf("seeded %d testimonies", len(testimonies))
}
