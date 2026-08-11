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

	"remi-api/internal/handlers"
	"remi-api/internal/middleware"
)

// NewRouter builds the API router around the shared handlers.
func NewRouter(h *handlers.Handler, corsOrigins []string) http.Handler {
	rateLimitForms := middleware.RateLimit(10, time.Minute)
	rateLimitAuth := middleware.RateLimit(20, time.Minute)

	r := chi.NewRouter()
	r.Use(chimw.Logger, chimw.Recoverer, chimw.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", h.Health)

	r.Route("/api", func(r chi.Router) {
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
		r.With(rateLimitForms).Post("/giving/initialize", h.GivingInitialize)

		// Auth
		r.With(rateLimitAuth).Post("/auth/login", h.Login)
		r.With(rateLimitAuth).Post("/auth/mfa/verify", h.VerifyMFA)
		r.With(rateLimitAuth).Get("/auth/invitations/{token}", h.GetInvitation)
		r.With(rateLimitAuth).Post("/auth/invitations/{token}/accept", h.AcceptInvitation)
		r.With(middleware.Auth(h.JWT)).Get("/auth/me", h.Me)
		r.Route("/account", func(r chi.Router) {
			r.Use(middleware.Auth(h.JWT))
			r.Put("/profile", h.UpdateProfile)
			r.Put("/password", h.UpdatePassword)
			r.Put("/preferences", h.UpdatePreferences)
			r.Post("/mfa/setup", h.StartMFA)
			r.Post("/mfa/confirm", h.ConfirmMFA)
			r.Delete("/mfa", h.DisableMFA)
		})

		// Admin
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.Auth(h.JWT))

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

			// Writes (editor+)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("editor"))
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
			})
		})
	})

	return r
}
