package tenant

import (
	"context"

	"capcom/internal/domain"
)

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func PrincipalFrom(ctx context.Context) (domain.Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(domain.Principal)
	return principal, ok
}

func OrganizationID(ctx context.Context) string {
	principal, ok := PrincipalFrom(ctx)
	if !ok || principal.PlatformAdmin {
		return ""
	}
	return principal.OrganizationID
}
