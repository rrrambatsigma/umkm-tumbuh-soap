package auth

import (
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrUnconfigured  = errors.New("JWT secret is not configured")
	ErrAuthorization = errors.New("invalid authorization scheme")
	ErrToken         = errors.New("invalid token")
	ErrRole          = errors.New("invalid token role")
)

type accessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// VerifyBearer preserves the existing REST JWT policy without selecting an HTTP
// response. The secret is supplied explicitly; this package has no fallback.
func VerifyBearer(authorization, secret string) (Actor, error) {
	if strings.TrimSpace(secret) == "" {
		return Actor{}, ErrUnconfigured
	}
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return Actor{}, ErrAuthorization
	}
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid || strings.TrimSpace(claims.Subject) == "" {
		return Actor{}, ErrToken
	}
	switch claims.Role {
	case "ADMIN", "UMKM", "MITRA":
		return Actor{UserID: claims.Subject, Role: claims.Role}, nil
	default:
		return Actor{}, ErrRole
	}
}
