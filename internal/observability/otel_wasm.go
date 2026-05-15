//go:build js

package observability

import (
	"context"
	"net/http"
)

// Config controls OTEL resource attributes for this process.
type Config struct {
	ServiceName      string
	ServiceNamespace string
	ServiceVersion   string
	Environment      string
}

// Enabled reports whether OTLP export is configured.
func Enabled() bool { return false }

// SentryTraceSampleRate disables Sentry performance tracing when OTEL is active.
func SentryTraceSampleRate() float64 { return 0.2 }

// Init is a no-op in js builds to keep OTEL exporters out of the wasm bundle.
func Init(context.Context, Config) (func(context.Context) error, bool, error) {
	return func(context.Context) error { return nil }, false, nil
}

// NewHTTPHandler is a no-op wrapper in js builds.
func NewHTTPHandler(next http.Handler) http.Handler { return next }

// NewTransport is a no-op wrapper in js builds.
func NewTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		return http.DefaultTransport
	}
	return base
}

// WrapClient returns client unchanged in js builds.
func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		return &http.Client{}
	}
	return client
}

// TraceIDs extracts the current trace/span IDs from context.
func TraceIDs(context.Context) (string, string) { return "", "" }
