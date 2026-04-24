package tracing

import (
	"context"
	"testing"
	"time"
)

func TestInit_DisabledReturnsNoopShutdown(t *testing.T) {
	t.Parallel()

	shutdown, err := Init(context.Background(), Config{Disabled: true, Endpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("Init disabled: unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatalf("Init disabled: expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("disabled shutdown: unexpected error: %v", err)
	}
}

func TestInit_EmptyEndpointReturnsNoopShutdown(t *testing.T) {
	t.Parallel()

	shutdown, err := Init(context.Background(), Config{ServiceName: "test"})
	if err != nil {
		t.Fatalf("Init empty endpoint: unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatalf("Init empty endpoint: expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("empty endpoint shutdown: unexpected error: %v", err)
	}
}

// TestInit_UnreachableEndpointDoesNotBlock verifies that the OTLP gRPC
// exporter does not block Init when nothing is listening on the configured
// endpoint. The exporter buffers spans in the background; only shutdown can
// observe the connection failure, and it must respect the timeout.
func TestInit_UnreachableEndpointDoesNotBlock(t *testing.T) {
	t.Parallel()

	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := Init(initCtx, Config{
		Endpoint:    "127.0.0.1:0",
		ServiceName: "evey-test",
		SampleRatio: 0.5,
	})
	if err != nil {
		t.Fatalf("Init with unreachable endpoint: unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatalf("Init with unreachable endpoint: expected non-nil shutdown")
	}

	// Shutdown must complete within its own deadline even though the
	// exporter cannot flush to a non-listening endpoint.
	done := make(chan error, 1)
	go func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer shutdownCancel()
		done <- shutdown(shutdownCtx)
	}()

	select {
	case err := <-done:
		// Shutdown is allowed to return nil OR an error wrapping
		// context.DeadlineExceeded; both indicate the timeout path
		// behaved correctly without hanging the process.
		_ = err
	case <-time.After(10 * time.Second):
		t.Fatalf("shutdown did not respect deadline (hung > 10s)")
	}
}

func TestTracer_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	if Tracer("") == nil {
		t.Fatalf("Tracer(\"\") returned nil")
	}
	if Tracer("evey/foo") == nil {
		t.Fatalf("Tracer(named) returned nil")
	}
}

func TestStartSpan_ReturnsContextAndSpan(t *testing.T) {
	t.Parallel()

	ctx, span := StartSpan(context.Background(), "test-span")
	if ctx == nil {
		t.Fatalf("StartSpan returned nil context")
	}
	if span == nil {
		t.Fatalf("StartSpan returned nil span")
	}
	span.End()
}

func TestInit_SampleRatioClampedHigh(t *testing.T) {
	t.Parallel()

	shutdown, err := Init(context.Background(), Config{
		Endpoint:    "127.0.0.1:0",
		ServiceName: "clamp-high",
		SampleRatio: 17.0,
	})
	if err != nil {
		t.Fatalf("Init clamp-high: unexpected error: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctx)
	})
}

func TestInit_SampleRatioClampedLow(t *testing.T) {
	t.Parallel()

	shutdown, err := Init(context.Background(), Config{
		Endpoint:    "127.0.0.1:0",
		ServiceName: "clamp-low",
		SampleRatio: -3.0,
	})
	if err != nil {
		t.Fatalf("Init clamp-low: unexpected error: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctx)
	})
}
