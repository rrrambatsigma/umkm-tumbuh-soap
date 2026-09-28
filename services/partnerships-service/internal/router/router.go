package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/middleware"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/rest"
)

func NewRouter(
	partnershipHandler *rest.Handler,
	frontendURL string,
	jwtSecret string,
) *chi.Mux {
	r := chi.NewRouter()

	// CORS configuration - allow frontend to access
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins:   []string{frontendURL, "http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	r.Use(corsMiddleware.Handler)

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(jwtSecret))
		r.Use(middleware.RequireRoles("UMKM", "MITRA"))
		// ============================================================
		// EXISTING ENDPOINTS (untuk pengajuan kemitraan)
		// ============================================================
		r.Post("/partnerships", partnershipHandler.CreatePartnership)
		r.Get("/partnerships/status", partnershipHandler.GetPartnershipStatus)
		r.Get("/partnerships/summary", partnershipHandler.GetPartnershipSummary)
		r.Get("/partnerships/incoming", partnershipHandler.GetIncomingPartnerships)
		r.Get("/partnerships/incoming/summary", partnershipHandler.GetIncomingPartnershipSummary)
		r.Get("/partnerships/{id}", partnershipHandler.GetPartnershipDetail)
		r.Post("/partnerships/{id}/sign", partnershipHandler.SignPartnership)
		r.Patch("/partnerships/{id}/read", partnershipHandler.MarkAsRead)
		r.Patch("/partnerships/{id}/approve", partnershipHandler.ApprovePartnership)
		r.Patch("/partnerships/{id}/reject", partnershipHandler.RejectPartnership)
		r.Patch("/partnerships/{id}/cancel", partnershipHandler.CancelPartnership)

		// ============================================================
		// NEW ENDPOINTS (untuk menampilkan list UMKM dan Mitra)
		// ============================================================
		// GET /api/v1/umkm - daftar UMKM (diakses oleh MITRA)
		r.With(middleware.RequireRoles("MITRA")).Get("/umkm", partnershipHandler.GetUMKMList)
		// GET /api/v1/umkm/{id} - detail UMKM (diakses oleh MITRA)
		r.With(middleware.RequireRoles("MITRA")).Get("/umkm/{id}", partnershipHandler.GetUMKMDetail)

		// GET /api/v1/mitra - daftar Mitra (diakses oleh UMKM)
		r.With(middleware.RequireRoles("UMKM")).Get("/mitra", partnershipHandler.GetMitraList)
		// GET /api/v1/mitra/{id} - detail Mitra (diakses oleh UMKM)
		r.With(middleware.RequireRoles("UMKM")).Get("/mitra/{id}", partnershipHandler.GetMitraDetail)
	})

	return r
}
