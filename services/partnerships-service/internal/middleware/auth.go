package middleware

import (
	"context"
	"net/http"

	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/auth"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/response"
)

func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, err := auth.VerifyBearer(r.Header.Get("Authorization"), jwtSecret)
			if err != nil {
				switch err {
				case auth.ErrUnconfigured:
					response.Error(w, http.StatusInternalServerError, "JWT secret belum dikonfigurasi")
				case auth.ErrAuthorization:
					response.Error(w, http.StatusUnauthorized, "Authorization header tidak valid")
				case auth.ErrRole:
					response.Error(w, http.StatusUnauthorized, "Peran pada token tidak valid")
				default:
					response.Error(w, http.StatusUnauthorized, "Token tidak valid atau sudah kedaluwarsa")
				}
				return
			}

			ctx := auth.WithActor(r.Context(), actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := GetUserID(r.Context()); !ok {
				response.Error(w, http.StatusUnauthorized, "User belum terautentikasi")
				return
			}
			role, _ := GetUserRole(r.Context())
			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.Error(w, http.StatusForbidden, "Anda tidak memiliki izin untuk tindakan ini")
		})
	}
}

func GetUserID(ctx context.Context) (string, bool) {
	actor, ok := auth.ActorFromContext(ctx)
	return actor.UserID, ok
}

func GetUserRole(ctx context.Context) (string, bool) {
	actor, ok := auth.ActorFromContext(ctx)
	return actor.Role, ok && actor.Role != ""
}
