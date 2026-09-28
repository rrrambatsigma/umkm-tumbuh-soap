package soap

import (
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type authenticatedActor struct {
	UserID string
	Role   string
}

type accessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type authenticationError struct {
	serverMisconfigured bool
}

func (e *authenticationError) Error() string {
	if e.serverMisconfigured {
		return "JWT secret is not configured"
	}
	return "invalid authentication credentials"
}

func authenticateBearer(authorizationHeader, jwtSecret string) (authenticatedActor, error) {
	if strings.TrimSpace(jwtSecret) == "" {
		return authenticatedActor{}, &authenticationError{serverMisconfigured: true}
	}

	parts := strings.Fields(authorizationHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return authenticatedActor{}, &authenticationError{}
	}

	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(
		parts[1],
		claims,
		func(token *jwt.Token) (any, error) {
			return []byte(jwtSecret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || token == nil || !token.Valid || strings.TrimSpace(claims.Subject) == "" {
		return authenticatedActor{}, &authenticationError{}
	}

	switch claims.Role {
	case "ADMIN", "UMKM", "MITRA":
		return authenticatedActor{UserID: claims.Subject, Role: claims.Role}, nil
	default:
		return authenticatedActor{}, &authenticationError{}
	}
}
