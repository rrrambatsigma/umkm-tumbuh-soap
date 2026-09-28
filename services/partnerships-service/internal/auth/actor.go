// Package auth provides transport-neutral token verification and actor context.
package auth

import (
	"context"
	"strings"
)

// Actor identifies an authenticated account. Roles are authorized by the consumer.
type Actor struct {
	UserID string
	Role   string
}

type actorKey struct{}

// WithActor preserves the parent context, including its deadline and cancellation.
// Trusted callers must supply an actor from VerifyBearer, never request fields.
// This function stores identity; it does not authenticate it.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFromContext returns only an actor stored by trusted application code.
// A missing or blank account ID is not an authenticated identity.
func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(Actor)
	return actor, ok && strings.TrimSpace(actor.UserID) != ""
}
