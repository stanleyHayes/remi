// Package server wires the HTTP router for the REMI API.
//
// The route table lives here rather than in cmd/server so that integration
// tests exercise the exact routes the binary serves — a test-only copy of the
// table would drift from the real one and prove nothing.
package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	chmshttp "remi-api/internal/chms/httpapi"
	chmsplatform "remi-api/internal/chms/platform"
	"remi-api/internal/handlers"
	"remi-api/internal/middleware"
)

// NewRouter builds the API router around the shared handlers.
func NewRouter(h *handlers.Handler, corsOrigins []string) http.Handler {
	return NewRouterWithCHMS(h, nil, corsOrigins)
}

// NewRouterWithCHMS mounts the versioned church-management API without
// changing the existing public/CMS contract.
func NewRouterWithCHMS(h *handlers.Handler, chms *chmshttp.Handler, corsOrigins []string) http.Handler {
	rateLimitForms := middleware.RateLimit(10, time.Minute)
	rateLimitAuth := middleware.RateLimit(20, time.Minute)

	r := chi.NewRouter()
	r.Use(chimw.Logger, chimw.Recoverer, chimw.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key", "X-Paystack-Signature", "X-REMI-Communication-Signature", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", h.Health)

	r.Route("/api", func(r chi.Router) {
		if chms != nil {
			r.With(chmsplatform.RequestIDMiddleware).Get("/fundraising/campaigns", chms.ListPublicCampaigns)
			r.With(chmsplatform.RequestIDMiddleware).Get("/fundraising/campaigns/{slug}", chms.GetPublicCampaign)
			r.With(rateLimitForms, chmsplatform.RequestIDMiddleware).Post("/payments/paystack/intents", chms.CreatePaystackIntent)
			r.With(rateLimitForms, chmsplatform.RequestIDMiddleware).Post("/giving/initialize", chms.CreatePaystackIntent)
			r.With(chmsplatform.RequestIDMiddleware).Post("/webhooks/paystack", chms.ReceivePaystackWebhook)
			r.With(rateLimitForms, chmsplatform.RequestIDMiddleware).Post("/communications/opt-outs", chms.ReceiveCommunicationOptOut)
			r.With(chmsplatform.RequestIDMiddleware).Post("/webhooks/communications/{provider}", chms.ReceiveCommunicationEvent)
			r.With(middleware.StaffAuth(h.JWT, h.DB), middleware.RequireRole("viewer")).Mount("/chms/v1", chms.Routes())
		}
		// Public read endpoints
		r.Get("/settings", h.GetSettings)
		r.Get("/pages/{pageKey}", h.GetPage)
		r.Get("/leadership", h.ListLeadership)
		r.Get("/ministries", h.ListMinistries)
		r.Get("/ministries/{slug}", h.GetMinistry)
		r.Get("/branches", h.ListBranches)
		r.Get("/sermons", h.ListSermons)
		r.Get("/sermons/{slug}", h.GetSermon)
		r.Get("/events", h.ListEvents)
		r.Get("/events/{slug}", h.GetEvent)
		r.Get("/announcements", h.ListAnnouncements)
		r.Get("/testimonies", h.ListTestimonies)

		// Public write endpoints (rate limited)
		r.With(rateLimitForms).Post("/events/{id}/register", h.RegisterForEvent)
		r.With(rateLimitForms).Post("/forms/prayer", h.SubmitPrayer)
		r.With(rateLimitForms).Post("/forms/contact", h.SubmitContact)
		r.With(rateLimitForms).Post("/forms/testimony", h.SubmitTestimony)
		r.With(rateLimitForms).Post("/forms/visit", h.SubmitVisit)
		r.With(rateLimitForms).Post("/subscribe", h.Subscribe)
		if chms == nil {
			// Compatibility for isolated legacy-handler tests. Production mounts
			// the durable ChMS payment-intent route above.
			r.With(rateLimitForms).Post("/giving/initialize", h.GivingInitialize)
		}

		// Auth
		r.With(rateLimitAuth).Post("/auth/login", h.Login)
		r.With(rateLimitAuth).Post("/auth/mfa/verify", h.VerifyMFA)
		r.With(rateLimitAuth).Get("/auth/invitations/{token}", h.GetInvitation)
		r.With(rateLimitAuth).Post("/auth/invitations/{token}/accept", h.AcceptInvitation)
		r.With(rateLimitAuth).Get("/member-auth/invitations/{token}", h.GetMemberInvitation)
		r.With(rateLimitAuth).Post("/member-auth/invitations/{token}/redeem", h.RedeemMemberInvitation)
		r.With(rateLimitAuth).Post("/member-auth/otp/request", h.RequestMemberOTP)
		r.With(rateLimitAuth).Post("/member-auth/otp/verify", h.VerifyMemberOTP)
		r.With(rateLimitAuth).Post("/member-auth/mfa/verify", h.VerifyMemberMFA)
		r.With(rateLimitAuth).Post("/member-auth/refresh", h.RefreshMemberSession)
		r.With(rateLimitAuth).Post("/member-auth/logout", h.LogoutMemberSession)
		r.Route("/member", func(r chi.Router) {
			r.Use(middleware.Auth(h.JWT), chmsplatform.RequestIDMiddleware)
			if chms != nil {
				r.Mount("/chms", chms.MemberRoutes())
			}
			r.Get("/me", h.MemberMe)
			r.Get("/home", h.MemberHome)
			r.Get("/profile", h.GetMemberProfile)
			r.Patch("/profile", h.UpdateMemberProfile)
			r.Get("/household", h.GetMemberHousehold)
			r.Patch("/household", h.UpdateMemberHousehold)
			r.Get("/data-requests", h.ListMemberDataRequests)
			r.Post("/data-requests", h.CreateMemberDataRequest)
			r.Post("/data-requests/{requestId}/withdraw", h.WithdrawMemberDataRequest)
			r.Post("/prayer-requests", h.SubmitMemberPrayer)
			r.Get("/care-content", h.GetMemberCareContent)
			r.Post("/care-requests", h.CreateMemberCareRequest)
			r.Post("/pastoral-appointments", h.CreateMemberPastoralAppointment)
			r.Put("/saved-content/{type}/{id}", h.SaveMemberContent)
			r.Get("/consents", h.ListMemberConsents)
			r.Put("/consents", h.UpdateMemberConsent)
			r.Get("/communication-eligibility", h.GetMemberCommunicationEligibility)
			r.Post("/client-events", h.RecordMemberClientEvent)
			r.Get("/communication-workspace", h.GetMemberCommunicationWorkspace)
			r.Put("/communication-preferences", h.UpdateMemberCommunicationPreferences)
			r.Get("/directory", h.ListMemberDirectory)
			r.Patch("/inbox/{id}", h.UpdateMemberInboxState)
			r.Post("/uploads/signature", h.MemberUploadSignature)
			r.Get("/sessions", h.ListMemberSessions)
			r.Get("/households", h.ListMemberHouseholds)
			r.Delete("/sessions/{sessionId}", h.RevokeMemberSession)
			r.Patch("/session/household", h.SwitchMemberHousehold)
			r.Get("/household-delegations", h.ListMemberHouseholdDelegations)
			r.Post("/household-delegations", h.CreateMemberHouseholdDelegation)
			r.Delete("/household-delegations/{delegationId}", h.RevokeMemberHouseholdDelegation)
			r.Get("/mfa", h.GetMemberMFA)
			r.Post("/mfa/setup", h.StartMemberMFA)
			r.Post("/mfa/confirm", h.ConfirmMemberMFA)
			r.Post("/mfa/disable/request", h.RequestDisableMemberMFA)
			r.Delete("/mfa", h.DisableMemberMFA)
			r.Get("/participation", h.GetMemberParticipation)
			r.Post("/registrations", h.CreateMemberRegistration)
			r.Delete("/registrations/{registrationId}", h.CancelMemberRegistration)
			r.Get("/registrations/{registrationId}/calendar.ics", h.DownloadMemberRegistrationCalendar)
			r.Post("/registrations/{registrationId}/payment", h.StartMemberRegistrationPayment)
			r.Post("/registration-payments/{paymentId}/confirm", h.ConfirmMemberRegistrationPayment)
			r.Post("/pathway-requests", h.CreateMemberPathwayRequest)
			r.Post("/child-prechecks", h.CreateMemberChildPrecheck)
			r.Post("/groups/{groupId}/membership", h.JoinMemberGroup)
			r.Delete("/groups/{groupId}/membership", h.LeaveMemberGroup)
			r.Get("/groups/{groupId}/community", h.GetMemberCommunityGroup)
			r.Put("/groups/{groupId}/directory-visibility", h.UpdateMemberGroupVisibility)
			r.Put("/groups/{groupId}/meetings/{meetingId}/response", h.RespondMemberGroupMeeting)
			r.Post("/groups/{groupId}/leader-messages", h.MessageMemberGroupLeaders)
			r.Get("/groups/{groupId}/messages", h.ListMemberGroupMessages)
			r.Post("/groups/{groupId}/messages", h.CreateMemberGroupMessage)
			r.Get("/serving-workspace", h.GetMemberServingWorkspace)
			r.Put("/serving-workspace/preferences", h.UpdateMemberServingPreferences)
			r.Post("/serving-workspace/availability", h.AddMemberServingAvailability)
			r.Delete("/serving-workspace/availability/{availabilityId}", h.CancelMemberServingAvailability)
			r.Post("/serving-workspace/assignments/{assignmentId}/substitute-request", h.RequestMemberServingSubstitute)
			r.Post("/serving-workspace/assignments/{assignmentId}/check-in", h.CheckInMemberServingAssignment)
		})
		r.With(middleware.StaffAuth(h.JWT, h.DB)).Get("/auth/me", h.Me)
		r.Route("/account", func(r chi.Router) {
			r.Use(middleware.StaffAuth(h.JWT, h.DB))
			r.Put("/profile", h.UpdateProfile)
			r.Put("/password", h.UpdatePassword)
			r.Put("/preferences", h.UpdatePreferences)
			r.Post("/mfa/setup", h.StartMFA)
			r.Post("/mfa/confirm", h.ConfirmMFA)
			r.Delete("/mfa", h.DisableMFA)
		})

		// Admin
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.StaffAuth(h.JWT, h.DB), chmsplatform.RequestIDMiddleware)

			// Read-only (viewer+)
			for name, res := range handlers.ContentResources() {
				res := res
				r.Get("/"+name, func(w http.ResponseWriter, req *http.Request) {
					h.AdminList(w, req, res)
				})
			}
			r.Get("/settings", h.GetSettings)
			r.Get("/forms/{formType}", h.AdminListForms)
			r.Get("/registrations", h.AdminListRegistrations)
			r.Get("/stats", h.AdminStats)
			r.Get("/people/{personId}/consents", h.AdminListPersonConsents)
			r.Get("/people/{personId}/communication-eligibility", h.AdminEvaluateCommunication)

			// Writes (editor+)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("editor"))
				r.Post("/member-invitations", h.AdminInviteMember)
				r.Post("/people/{personId}/suppressions", h.AdminApplySuppression)
				for name, res := range handlers.ContentResources() {
					res := res
					r.Post("/"+name, func(w http.ResponseWriter, req *http.Request) {
						h.AdminCreate(w, req, res)
					})
					r.Put("/"+name+"/{id}", func(w http.ResponseWriter, req *http.Request) {
						h.AdminUpdate(w, req, res)
					})
					if name != "pages" { // contract: no DELETE for pages
						r.Delete("/"+name+"/{id}", func(w http.ResponseWriter, req *http.Request) {
							h.AdminDelete(w, req, res)
						})
					}
				}
				r.Put("/settings", h.AdminPutSettings)
				r.Patch("/content/{type}/{id}/status", h.AdminUpdateStatus)
				r.Patch("/forms/{formType}/{id}", h.AdminUpdateFormStatus)
				r.Post("/uploads/signature", h.AdminUploadSignature)
			})

			// User management (super-admin only)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("super-admin"))
				r.Get("/users", h.AdminListUsers)
				r.Post("/users", h.AdminCreateUser)
				r.Patch("/users/{id}/scopes", h.AdminUpdateUserScopes)
			})
		})
	})

	return r
}
