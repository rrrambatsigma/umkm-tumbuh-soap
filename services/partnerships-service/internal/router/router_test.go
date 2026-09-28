package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/rest"
)

func TestEveryPartnershipRouteRequiresAuthentication(t *testing.T) {
	router := NewRouter(rest.NewHandler(nil), "http://localhost:5173", "test-secret")
	for _, tc := range []struct{ method, path string }{
		{"POST", "/partnerships"}, {"GET", "/partnerships/status"}, {"GET", "/partnerships/summary"},
		{"GET", "/partnerships/incoming"}, {"GET", "/partnerships/incoming/summary"}, {"GET", "/partnerships/P1"},
		{"POST", "/partnerships/P1/sign"}, {"PATCH", "/partnerships/P1/read"}, {"PATCH", "/partnerships/P1/approve"},
		{"PATCH", "/partnerships/P1/reject"}, {"PATCH", "/partnerships/P1/cancel"},
		{"GET", "/umkm"}, {"GET", "/umkm/U1"}, {"GET", "/mitra"}, {"GET", "/mitra/M1"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1"+tc.path, nil)
			req.Header.Set("X-User-Role", "MITRA")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rr.Code)
			}
		})
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest("GET", "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rr.Code)
	}
}

func TestDirectoryRoleComesFromToken(t *testing.T) {
	const secret = "test-secret"
	router := NewRouter(rest.NewHandler(nil), "http://localhost:5173", secret)
	for _, tc := range []struct{ role, spoof, path string }{
		{"UMKM", "MITRA", "/umkm"}, {"UMKM", "MITRA", "/umkm/U1"},
		{"MITRA", "UMKM", "/mitra"}, {"MITRA", "UMKM", "/mitra/M1"},
		{"ADMIN", "UMKM", "/partnerships/status"},
	} {
		t.Run(tc.role+tc.path, func(t *testing.T) {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": "account", "role": tc.role, "exp": time.Now().Add(time.Hour).Unix(),
			}).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "/api/v1"+tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-User-Role", tc.spoof)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rr.Code)
			}
		})
	}
}
