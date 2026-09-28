package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/auth"
)

func TestAuthResponseCompatibility(t *testing.T) {
	const secret = "test-secret"
	invalidRole, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "account", "role": "OWNER", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, secret, header, message string
		status                        int
	}{
		{"configuration first", "", "", "JWT secret belum dikonfigurasi", 500},
		{"blank configuration", " \t ", "", "JWT secret belum dikonfigurasi", 500},
		{"authorization", secret, "", "Authorization header tidak valid", 401},
		{"token", secret, "Bearer invalid", "Token tidak valid atau sudah kedaluwarsa", 401},
		{"role", secret, "Bearer " + invalidRole, "Peran pada token tidak valid", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", tc.header)
			rr := httptest.NewRecorder()
			AuthMiddleware(tc.secret)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid credentials reached handler") })).ServeHTTP(rr, req)
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rr.Code != tc.status || rr.Header().Get("Content-Type") != "application/json" || len(body) != 2 || body["success"] != false || body["message"] != tc.message {
				t.Fatalf("status=%d headers=%v body=%s", rr.Code, rr.Header(), rr.Body.String())
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	const secret = "test-only-jwt-secret"
	sign := func(method jwt.SigningMethod, key any, changes map[string]any) string {
		t.Helper()
		claims := jwt.MapClaims{"sub": "ACCOUNT_A", "role": "UMKM", "exp": time.Now().Add(time.Hour).Unix()}
		for name, value := range changes {
			if value == nil {
				delete(claims, name)
			} else {
				claims[name] = value
			}
		}
		token, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	valid := sign(jwt.SigningMethodHS256, []byte(secret), nil)
	cases := []struct {
		name   string
		header string
		secret string
		want   int
	}{
		{"valid token ignores spoofed role header", "Bearer " + valid, secret, http.StatusNoContent},
		{"lowercase bearer scheme", "bearer " + valid, secret, http.StatusNoContent},
		{"anonymous with role header", "", secret, http.StatusUnauthorized},
		{"wrong scheme", "Basic " + valid, secret, http.StatusUnauthorized},
		{"extra header field", "Bearer " + valid + " extra", secret, http.StatusUnauthorized},
		{"malformed token", "Bearer invalid", secret, http.StatusUnauthorized},
		{"forged signature", "Bearer " + sign(jwt.SigningMethodHS256, []byte("wrong-key"), nil), secret, http.StatusUnauthorized},
		{"unsigned token", "Bearer " + sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, nil), secret, http.StatusUnauthorized},
		{"wrong algorithm", "Bearer " + sign(jwt.SigningMethodHS384, []byte(secret), nil), secret, http.StatusUnauthorized},
		{"expired token", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}), secret, http.StatusUnauthorized},
		{"missing expiry", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"exp": nil}), secret, http.StatusUnauthorized},
		{"future not-before", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"nbf": time.Now().Add(time.Hour).Unix()}), secret, http.StatusUnauthorized},
		{"blank subject", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"sub": " "}), secret, http.StatusUnauthorized},
		{"missing subject", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"sub": nil}), secret, http.StatusUnauthorized},
		{"missing role", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"role": nil}), secret, http.StatusUnauthorized},
		{"unknown role", "Bearer " + sign(jwt.SigningMethodHS256, []byte(secret), map[string]any{"role": "OWNER"}), secret, http.StatusUnauthorized},
		{"empty server secret", "Bearer " + valid, "", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				actor, actorOK := auth.ActorFromContext(r.Context())
				if !actorOK || actor.UserID != "ACCOUNT_A" || actor.Role != "UMKM" {
					t.Fatal("verified identity did not reach neutral actor context")
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("parent deadline lost")
				}
				id, ok := GetUserID(r.Context())
				role, roleOK := GetUserRole(r.Context())
				if !ok || !roleOK || id != "ACCOUNT_A" || role != "UMKM" {
					t.Fatal("identity must come from verified claims")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(ctx)
			req.Header.Set("Authorization", tc.header)
			req.Header.Set("X-User-Role", "ADMIN")
			req.Header.Set("X-User-ID", "VICTIM")
			rr := httptest.NewRecorder()
			AuthMiddleware(tc.secret)(next).ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
			if called != (tc.want == http.StatusNoContent) {
				t.Fatal("invalid token reached protected handler")
			}
		})
	}
}
