package mkvgo

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// WithMaxBuilds replaces the default cap; zero or less keeps it.
func TestOnDemandPlan_WithMaxBuildsSetsTheCap(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	if got := p.MaxBuilds(); got != onDemandMaxBuilds {
		t.Fatalf("default cap = %d, want %d", got, onDemandMaxBuilds)
	}
	if got := p.WithMaxBuilds(0).MaxBuilds(); got != onDemandMaxBuilds {
		t.Fatalf("a zero cap replaced the default: %d", got)
	}

	p.WithMaxBuilds(3)
	var running, peak atomic.Int32
	gate := make(chan struct{})
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		n := running.Add(1)
		for {
			m := peak.Load()
			if n <= m || peak.CompareAndSwap(m, n) {
				break
			}
		}
		<-gate
		running.Add(-1)
		return &stubPlan{}, nil
	}
	var wg sync.WaitGroup
	for range 10 {
		src := stampedeSrc(t)
		wg.Go(func() {
			_, _ = p.Plan(context.Background(), src, apptranscoding.OnDemandOptions{})
		})
	}
	time.Sleep(100 * time.Millisecond)
	if got := peak.Load(); got != 3 {
		t.Fatalf("%d builds ran at once, want the cap of 3", got)
	}
	close(gate)
	wg.Wait()
}

// Every build lands in the histogram under its result, the fast ones too.
func TestOnDemandPlan_EveryBuildIsTimed(t *testing.T) {
	p := NewOnDemandPackager()
	defer p.Close()
	count := func(result string) int {
		var m dto.Metric
		if err := metrics.MkvgoPlanBuildSeconds.WithLabelValues(result).(prometheus.Histogram).Write(&m); err != nil {
			t.Fatal(err)
		}
		return int(m.GetHistogram().GetSampleCount())
	}
	okBefore, failedBefore := count("ok"), count("failed")

	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		return &stubPlan{}, nil
	}
	_, _ = p.Plan(context.Background(), stampedeSrc(t), apptranscoding.OnDemandOptions{})
	p.build = func(context.Context, string, apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
		return nil, errors.New("unreadable")
	}
	_, _ = p.Plan(context.Background(), stampedeSrc(t), apptranscoding.OnDemandOptions{})

	if got := count("ok") - okBefore; got != 1 {
		t.Fatalf("ok builds timed = %d, want 1", got)
	}
	if got := count("failed") - failedBefore; got != 1 {
		t.Fatalf("failed builds timed = %d, want 1", got)
	}
}

func TestBuildResult(t *testing.T) {
	for err, want := range map[error]string{
		nil:                      "ok",
		context.Canceled:         "cancelled",
		context.DeadlineExceeded: "cancelled",
		errors.New("x"):          "failed",
	} {
		if got := buildResult(err); got != want {
			t.Errorf("buildResult(%v) = %q, want %q", err, got, want)
		}
	}
}
