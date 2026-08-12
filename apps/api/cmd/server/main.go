package main

import (
	"context"
	"crypto/sha256"
	"log"
	"net/http"
	"strings"
	"time"

	"remi-api/internal/chms/care"
	"remi-api/internal/chms/communications"
	"remi-api/internal/chms/community"
	"remi-api/internal/chms/consent"
	"remi-api/internal/chms/engagement"
	"remi-api/internal/chms/finance"
	"remi-api/internal/chms/governance"
	chmshttp "remi-api/internal/chms/httpapi"
	chmsimports "remi-api/internal/chms/imports"
	"remi-api/internal/chms/participation"
	"remi-api/internal/chms/people"
	"remi-api/internal/chms/platform"
	"remi-api/internal/chms/reporting"
	"remi-api/internal/config"
	"remi-api/internal/db"
	"remi-api/internal/handlers"
	"remi-api/internal/server"
	"remi-api/internal/services"
)

func main() {
	cfg := config.Load()
	if err := cfg.ValidateProduction(); err != nil {
		log.Fatal(err)
	}

	// Atlas can need more than ten seconds to establish a cold TLS connection
	// and reconcile every ChMS index. Keep startup bounded, but do not turn a
	// healthy cold database into a deployment crash loop.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, database, err := db.Connect(ctx, cfg.MongoURI)
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer client.Disconnect(context.Background())
	environment := strings.ToLower(strings.TrimSpace(cfg.Environment))
	isProduction := environment == "production" || environment == "prod"
	if isProduction {
		if err := db.RequireTransactions(ctx, database); err != nil {
			log.Fatalf("mongo transaction safety: %v", err)
		}
	}
	initialized, err := db.EnsureInitialAdmin(ctx, database, cfg.SeedAdminEmail, cfg.SeedAdminPassword)
	if err != nil {
		log.Fatalf("initial admin: %v", err)
	}
	if initialized {
		log.Printf("initialized administrator %s", cfg.SeedAdminEmail)
	}
	if err := db.EnsureMemberAuthIndexes(ctx, database); err != nil {
		log.Fatalf("member auth indexes: %v", err)
	}
	chmsPlatform, err := platform.NewMongoPlatformStore(database)
	if err != nil {
		log.Fatalf("chms platform: %v", err)
	}
	if err := chmsPlatform.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms platform indexes: %v", err)
	}
	peopleRepository, err := people.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms people: %v", err)
	}
	if err := peopleRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms people indexes: %v", err)
	}
	householdRepository, err := people.NewHouseholdRepository(database)
	if err != nil {
		log.Fatalf("chms households: %v", err)
	}
	if err := householdRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms household indexes: %v", err)
	}
	importRepository, err := chmsimports.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms imports: %v", err)
	}
	if err := importRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms import indexes: %v", err)
	}
	participationRepository, err := participation.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms participation: %v", err)
	}
	if err := participationRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms participation indexes: %v", err)
	}
	communityRepository, err := community.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms community: %v", err)
	}
	if err := communityRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms community indexes: %v", err)
	}
	engagementRepository, err := engagement.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms engagement: %v", err)
	}
	if err := engagementRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms engagement indexes: %v", err)
	}
	consentRepository, err := consent.NewRepository(database)
	if err != nil {
		log.Fatalf("chms consent: %v", err)
	}
	if err := consentRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms consent indexes: %v", err)
	}
	communicationsRepository, err := communications.NewRepository(database)
	if err != nil {
		log.Fatalf("chms communications: %v", err)
	}
	if err := communicationsRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms communications indexes: %v", err)
	}
	financeRepository, err := finance.NewRepository(database)
	if err != nil {
		log.Fatalf("chms finance: %v", err)
	}
	if err := financeRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms finance indexes: %v", err)
	}
	careRepository, err := care.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms care: %v", err)
	}
	if err := careRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms care indexes: %v", err)
	}
	if err := careRepository.EnsureCaseIndexes(ctx); err != nil {
		log.Fatalf("chms care-case indexes: %v", err)
	}
	reportingRepository, err := reporting.NewMongoRepository(database)
	if err != nil {
		log.Fatalf("chms reporting: %v", err)
	}
	if err := reportingRepository.EnsureIndexes(ctx); err != nil {
		log.Fatalf("chms reporting indexes: %v", err)
	}

	h := handlers.New(database, cfg)
	cursorKey := sha256.Sum256([]byte(cfg.JWTSecret + ":chms-cursors:v1"))
	cursors, err := platform.NewCursorCodec(cursorKey[:])
	if err != nil {
		log.Fatalf("chms cursors: %v", err)
	}
	authorizer := platform.GrantAuthorizer{RecentMFAWindow: 10 * time.Minute}
	featureFlags := platform.NewFeatureFlags(1, map[string]bool{"retention-individual-signals": cfg.RetentionSignalsEnabled})
	pickupKey := sha256.Sum256([]byte(cfg.JWTSecret + ":chms-pickup-codes:v1"))
	pickupCodes, err := participation.NewPickupCodeManager(pickupKey[:])
	if err != nil {
		log.Fatalf("chms pickup codes: %v", err)
	}
	careKey := sha256.Sum256([]byte(cfg.JWTSecret + ":chms-care-notes:v1"))
	careCipher, err := platform.NewEnvelopeCipher("care-notes-v1", careKey[:])
	if err != nil {
		log.Fatalf("chms care cipher: %v", err)
	}
	consentService := consent.Service{Repository: consentRepository, Platform: chmsPlatform, Authorizer: authorizer}
	reminderEligibility := func(ctx context.Context, principal platform.Principal, branchID, personID platform.ID, purpose, channel string) (bool, string, error) {
		decision, evaluateErr := consentService.Evaluate(ctx, principal, branchID, personID, purpose, channel)
		if evaluateErr != nil {
			return false, "evaluation-error", evaluateErr
		}
		return decision.Eligible, decision.Decision, nil
	}
	engagementConsent := func(ctx context.Context, principal platform.Principal, branchID, personID platform.ID, purpose, channel string) (engagement.ConsentDecision, error) {
		decision, evaluateErr := consentService.Evaluate(ctx, principal, branchID, personID, purpose, channel)
		if evaluateErr != nil {
			return engagement.ConsentDecision{}, evaluateErr
		}
		result := engagement.ConsentDecision{Eligible: decision.Eligible, Decision: decision.Decision, EvaluatedAt: decision.EvaluatedAt}
		if decision.Consent != nil {
			result.ProjectionID, result.ProjectionVersion = decision.Consent.ID, decision.Consent.Version
		}
		for _, suppression := range decision.BlockingReasons {
			result.BlockingSuppressionIDs = append(result.BlockingSuppressionIDs, suppression.ID)
		}
		return result, nil
	}
	optOutKey := sha256.Sum256([]byte(cfg.JWTSecret + ":communication-opt-outs:v1"))
	optOutCodec, err := communications.NewOptOutCodec(optOutKey[:])
	if err != nil {
		log.Fatalf("communication opt-outs: %v", err)
	}
	communicationService := communications.Service{Repository: communicationsRepository, Platform: chmsPlatform, Authorizer: authorizer, Sender: services.CommunicationSender{Email: h.Email, SMS: h.SMS, WhatsApp: services.NewWhatsAppService(cfg.WhatsAppAccessToken, cfg.WhatsAppPhoneNumberID, cfg.WhatsAppGraphVersion)}, OptOuts: optOutCodec, PublicWebURL: cfg.PublicWebURL, WebhookSecret: cfg.CommunicationWebhookSecret}
	financeKey := sha256.Sum256([]byte(cfg.JWTSecret + ":chms-member-payment-methods:v1"))
	financeCipher, err := platform.NewEnvelopeCipher("member-payment-methods-v1", financeKey[:])
	if err != nil {
		log.Fatalf("chms finance cipher: %v", err)
	}
	financeService := finance.Service{Repository: financeRepository, Platform: chmsPlatform, Authorizer: authorizer, Provider: h.Paystack, AllowProviderDemo: !isProduction, ReminderEligibility: reminderEligibility, Mailer: h.Email, Cipher: financeCipher, MemberAppURL: cfg.MemberAppURL}
	reportingService := reporting.Service{Authorizer: authorizer, Repository: reportingRepository, Evidence: chmsPlatform, Sender: h.Email, AdminAppURL: cfg.AdminAppURL, Principals: reporting.PrincipalResolverFunc(func(resolveCtx context.Context, organizationID, actorID platform.ID) (platform.Principal, bool, error) {
		role, branches, ministries, assignments, active, resolveErr := reportingRepository.FindStaffRole(resolveCtx, actorID)
		if resolveErr != nil || !active {
			return platform.Principal{}, false, resolveErr
		}
		principal, supported := chmshttp.StaffPrincipalForRoleScoped(role, actorID, organizationID, branches, ministries, assignments)
		return principal, supported, nil
	})}
	chmsHandler := chmshttp.New(chmshttp.Services{
		Search:           people.SearchService{Store: peopleRepository, Authorizer: authorizer, Cursors: cursors},
		People:           people.Service{Repository: peopleRepository, Platform: chmsPlatform},
		HouseholdService: people.HouseholdService{Repository: householdRepository, Platform: chmsPlatform},
		Segments:         people.SegmentService{Store: peopleRepository, Platform: chmsPlatform, Authorizer: authorizer},
		Merge:            people.MergeService{Store: peopleRepository, Platform: chmsPlatform, Authorizer: authorizer},
		Bulk:             people.BulkService{Store: peopleRepository, Platform: chmsPlatform, Authorizer: authorizer},
		Imports:          chmsimports.Service{Repository: importRepository, Platform: chmsPlatform, Limits: chmsimports.DefaultParseLimits()},
		Participation:    participation.Service{Store: participationRepository, Platform: chmsPlatform, People: peopleRepository, PickupCodes: pickupCodes, Authorizer: authorizer},
		Community:        community.Service{Store: communityRepository, Platform: chmsPlatform, People: peopleRepository, VolunteerPeople: peopleRepository, Occurrences: participationRepository, Authorizer: authorizer},
		Communications:   communicationService,
		Engagement:       engagement.Service{Store: engagementRepository, Platform: chmsPlatform, Authorizer: authorizer, Flags: featureFlags, Consent: engagementConsent},
		Care:             care.Service{Store: careRepository, Cases: careRepository, Platform: chmsPlatform, Attendance: participationRepository, Cipher: careCipher, Authorizer: authorizer},
		Finance:          financeService,
		Reporting:        reportingService,
		Audit:            platform.AuditService{Store: chmsPlatform, Evidence: chmsPlatform, Authorizer: authorizer},
		Governance:       governance.Service{Store: governance.MongoStore{DB: database}, Evidence: chmsPlatform, Authorizer: authorizer},
		Repository:       peopleRepository,
		Households:       householdRepository,
		Journey:          peopleRepository,
		Authorizer:       authorizer,
	}, platform.ID(cfg.CHMSOrganizationID))
	r := server.NewRouterWithCHMS(h, chmsHandler, cfg.CORSOrigins)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			workerCtx, workerCancel := context.WithTimeout(context.Background(), 25*time.Second)
			processed, runErr := communicationService.RunDue(workerCtx)
			workerCancel()
			if runErr != nil {
				log.Printf("communication scheduler: %v", runErr)
			} else if processed > 0 {
				log.Printf("communication scheduler dispatched %d campaign(s)", processed)
			}
			workerCtx, workerCancel = context.WithTimeout(context.Background(), 25*time.Second)
			recurringProcessed, recurringErr := financeService.RunDueRecurring(workerCtx)
			workerCancel()
			if recurringErr != nil {
				log.Printf("recurring giving scheduler: %v", recurringErr)
			} else if recurringProcessed > 0 {
				log.Printf("recurring giving scheduler submitted %d charge(s)", recurringProcessed)
			}
			workerCtx, workerCancel = context.WithTimeout(context.Background(), 25*time.Second)
			reportProcessed, reportErr := reportingService.ProcessPendingExport(workerCtx)
			workerCancel()
			if reportErr != nil {
				log.Printf("report export worker: %v", reportErr)
			} else if reportProcessed {
				log.Printf("report export worker completed one run")
			}
			workerCtx, workerCancel = context.WithTimeout(context.Background(), 25*time.Second)
			scheduleProcessed, scheduleErr := chmsHandler.ProcessDueReportingSchedule(workerCtx)
			workerCancel()
			if scheduleErr != nil {
				log.Printf("scheduled report worker: %v", scheduleErr)
			} else if scheduleProcessed {
				log.Printf("scheduled report worker completed one run")
			}
		}
	}()

	addr := ":" + cfg.Port
	log.Printf("REMI API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
