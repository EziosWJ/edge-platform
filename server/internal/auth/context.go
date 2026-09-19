package auth

import "context"

type principalContextKey struct{}

// ContextWithPrincipal lets HTTP middleware pass an authenticated identity to
// later handlers through the standard Go context.
func ContextWithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}
