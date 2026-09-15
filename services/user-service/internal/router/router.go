package router

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/savitar393/umkm-tumbuh/services/user-service/internal/adminprofiles"
	"github.com/savitar393/umkm-tumbuh/services/user-service/internal/dashboard"
	"github.com/savitar393/umkm-tumbuh/services/user-service/internal/documents"
	"github.com/savitar393/umkm-tumbuh/services/user-service/internal/health"
	"github.com/savitar393/umkm-tumbuh/services/user-service/internal/middleware"
	"github.com/savitar393/umkm-tumbuh/services/user-service/internal/profiles"
)

func New(db *pgxpool.Pool, frontendURL string, jwtSecret string, uploadDir string) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{frontendURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Use(timeoutMiddleware(10 * time.Second))

	healthHandler := health.NewHandler(db)
	profileHandler := profiles.NewHandler(db)
	dashboardHandler := dashboard.NewHandler(db)
	adminProfileHandler := adminprofiles.NewHandler(db)
	docHandler := documents.NewHandler(db, uploadDir)

		r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", healthHandler.ServiceHealth)
		r.Get("/health/db", healthHandler.DatabaseHealth)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(jwtSecret))

			r.Route("/profiles", func(r chi.Router) {
				r.Get("/me", profileHandler.GetMe)
				r.Put("/me", profileHandler.UpsertMe)
				r.Get("/mitra", profileHandler.ListMitra)
				r.Get("/umkm", profileHandler.ListUMKM)
			})

			r.Route("/register", func(r chi.Router) {
				r.Get("/status", profileHandler.GetRegistrationStatus)
				r.Post("/submit", profileHandler.SubmitRegistration)
			})

			r.Route("/dashboard", func(r chi.Router) {
				r.Get("/umkm/summary", dashboardHandler.UMKMSummary)
				r.Get("/umkm", dashboardHandler.GetUMKMDashboard)
				r.Get("/mitra", dashboardHandler.GetMitraDashboard)
			})

			r.Route("/documents", func(r chi.Router) {
				r.Get("/", docHandler.GetDocuments)
				r.Get("/checklist", docHandler.GetDocumentChecklist)
				r.Get("/{docID}/view", docHandler.ViewDocument)
				r.Get("/{docID}/download", docHandler.DownloadDocument)
				r.Delete("/{docID}", docHandler.DeleteDocument)
			})
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.Auth(jwtSecret))
			r.Get("/profiles/{userID}", adminProfileHandler.GetProfileByUserID)
			r.Get("/users/{userID}/documents", docHandler.AdminGetUserDocuments)
		})

	})

	return r
}

func timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, `{"error":"request timeout"}`)
	}
}
