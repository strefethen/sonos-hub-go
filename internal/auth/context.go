package auth

import "context"

type contextKey string

const userKey contextKey = "authUser"

// User represents an authenticated device.
type User struct {
	Sub        string
	DeviceName string
	Type       TokenType
}

// WithUser stores an authenticated user in the context.
func WithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

// UserFromContext returns the authenticated user, if present.
func UserFromContext(ctx context.Context) (User, bool) {
	if ctx == nil {
		return User{}, false
	}
	value := ctx.Value(userKey)
	if value == nil {
		return User{}, false
	}
	user, ok := value.(User)
	return user, ok
}
