package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyBearer(t *testing.T) {
	const secret = "test-secret"
	sign := func(role, subject string) string {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": subject, "role": role, "exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	for _, role := range []string{"UMKM", "MITRA", "ADMIN"} {
		t.Run(role, func(t *testing.T) {
			actor, err := VerifyBearer("bearer  "+sign(role, " account "), secret)
			if err != nil || actor.UserID != " account " || actor.Role != role {
				t.Fatalf("actor=%+v error=%v", actor, err)
			}
		})
	}
	for _, tc := range []struct {
		name, header, secret string
		want                 error
	}{
		{"empty secret", "", "", ErrUnconfigured},
		{"blank secret", "", " \t ", ErrUnconfigured},
		{"missing header", "", secret, ErrAuthorization},
		{"wrong scheme", "Basic token", secret, ErrAuthorization},
		{"malformed token", "Bearer invalid", secret, ErrToken},
		{"blank subject", "Bearer " + sign("UMKM", " "), secret, ErrToken},
		{"invalid role", "Bearer " + sign("OWNER", "account"), secret, ErrRole},
		{"empty role", "Bearer " + sign("", "account"), secret, ErrRole},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor, err := VerifyBearer(tc.header, tc.secret)
			if !errors.Is(err, tc.want) || actor != (Actor{}) {
				t.Fatalf("actor=%+v error=%v, want zero actor and %v", actor, err, tc.want)
			}
		})
	}
}

func TestActorContext(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ctx := WithActor(parent, Actor{UserID: "account", Role: "UMKM"})
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.UserID != "account" || actor.Role != "UMKM" {
		t.Fatalf("actor=%+v ok=%v", actor, ok)
	}
	if _, ok := ActorFromContext(parent); ok {
		t.Fatal("actor leaked into parent")
	}
	wantDeadline, _ := parent.Deadline()
	if deadline, ok := ctx.Deadline(); !ok || deadline != wantDeadline {
		t.Fatal("deadline lost")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("cancellation lost")
	}
	for _, invalid := range []context.Context{
		context.Background(),
		context.WithValue(context.Background(), "user_id", "spoofed"),
		WithActor(context.Background(), Actor{}),
		WithActor(context.Background(), Actor{UserID: " \t ", Role: "UMKM"}),
	} {
		if _, ok := ActorFromContext(invalid); ok {
			t.Fatal("missing/blank actor accepted")
		}
	}
}
