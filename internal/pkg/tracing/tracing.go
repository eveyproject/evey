// Package tracing provides a minimal opt-in OpenTelemetry tracing scaffold.
//
// The package is intentionally thin: it exposes Init() to wire up a global
// TracerProvider when an OTLP endpoint is configured, plus small helpers
// (Tracer, StartSpan) so call sites can emit spans without importing the
// OpenTelemetry libraries directly.
//
// When tracing is disabled (Config.Disabled or Config.Endpoint == ""),
// Init returns a noop provider and a no-op shutdown so existing call sites
// pay zero runtime cost.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// defaultTracerName is used by StartSpan when the caller does not need a
// dedicated tracer scope.
const defaultTracerName = "evey"

// Config controls how the tracing module initialises the global provider.
//
// All fields are optional. When Disabled is true or Endpoint is empty,
// Init becomes a no-op and returns a noop TracerProvider.
type Config struct {
	// Endpoint is the OTLP gRPC endpoint, e.g. "localhost:4317".
	// Empty disables tracing.
	Endpoint string

	// ServiceName is reported as service.name in the OTLP resource.
	// Defaults to "evey" when empty.
	ServiceName string

	// Disabled forces a noop provider regardless of Endpoint.
	Disabled bool

	// SampleRatio is the head-based sampling ratio in [0, 1].
	// 0 (default) means never sample; 1 means always sample.
	// Values outside [0, 1] are clamped.
	SampleRatio float64
}

// ShutdownFunc is the cleanup function returned by Init.
// It flushes pending spans and tears down the exporter.
// It always honours its context deadline.
type ShutdownFunc = func(context.Context) error

// noopShutdown is returned when tracing is disabled. It satisfies
// ShutdownFunc without doing any work.
func noopShutdown(context.Context) error { return nil }

// Init initialises the global OpenTelemetry tracer provider.
//
// When cfg.Disabled or cfg.Endpoint is empty, Init installs a noop provider
// and returns a no-op shutdown.
//
// Otherwise, Init creates an OTLP gRPC exporter, wraps it in a batch span
// processor, and registers the resulting TracerProvider as global.
// The exporter is created with WithInsecure for local self-hosted setups.
//
// The returned shutdown function should be invoked during process shutdown.
// Callers MUST pass a context with a finite deadline; otherwise a slow
// collector can block indefinitely.
func Init(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	if cfg.Disabled || cfg.Endpoint == "" {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		return noopShutdown, nil
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "evey"
	}

	ratio := cfg.SampleRatio
	switch {
	case ratio < 0:
		ratio = 0
	case ratio > 1:
		ratio = 1
	}

	// Build a resource with our service.name attribute. We avoid
	// resource.Merge with semconv-specific schema URLs because the SDK's
	// Default resource carries its own (often newer) schema URL and a merge
	// across mismatched schemas fails. An empty schema URL on our side is
	// always compatible, and the SDK still adds its default attributes.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			attribute.String("service.name", serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: build resource: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: create OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(shutdownCtx context.Context) error {
		// Always cap shutdown time so a stalled collector cannot hang the
		// process. Callers can still pass a shorter deadline.
		ctxTimeout, cancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer cancel()

		var errs []error
		if err := tp.Shutdown(ctxTimeout); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
		}
		return errors.Join(errs...)
	}

	return shutdown, nil
}

// Tracer returns a named tracer from the global provider.
// Call sites use this to avoid importing go.opentelemetry.io/otel directly.
func Tracer(name string) trace.Tracer {
	if name == "" {
		name = defaultTracerName
	}
	return otel.Tracer(name)
}

// StartSpan is a convenience wrapper around the default Evey tracer.
// It returns the new context (carrying the span) and the span itself.
// When tracing is disabled, the returned span is a noop.
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer(defaultTracerName).Start(ctx, name)
}
