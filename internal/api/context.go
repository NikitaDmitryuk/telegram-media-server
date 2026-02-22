package api

import "context"

type contextKey string

const requestIDContextKey contextKey = "request_id"

// RequestIDFromContext returns the request ID from the context, or empty string.
func RequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(requestIDContextKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithRequestID returns a copy of ctx with the request ID set.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, requestID)
}
