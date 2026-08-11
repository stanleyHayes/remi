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
	s.settings()
	s.pages()
	s.leaders()
	s.ministries()
	s.branches()
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
			"registrationEnabled": true, "capacity": 500, "registeredCount": 0,
			"contentStatus": models.StatusPublished},
		{"title": "Prophetic Equipping School", "slug": "prophetic-equipping-school",
			"description": "A four-week Saturday school with Dr. Awudu on hearing God, journaling, and ministering prophecy with humility and accuracy.",
			"startAt":     now.AddDate(0, 0, 21), "endAt": now.AddDate(0, 0, 42),
			"location": "REMI Kumasi & online", "image": img("event-equipping"),
			"registrationEnabled": true, "capacity": 120, "registeredCount": 0,
			"contentStatus": models.StatusPublished},
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
