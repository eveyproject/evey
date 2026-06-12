// Package netmeter counts the bytes actually written to streaming HTTP
// responses (HLS segments, direct-play, downloads) so the admin dashboard can
// show real served bandwidth instead of a guess. One process-wide cumulative
// counter; rates are derived by whoever samples it (dashboard poll, metrics
// sampler) from two reads over an interval.
package netmeter

import (
	"io"
	"net/http"
	"sync/atomic"
)

// Meter is a cumulative byte counter. The zero value is ready to use.
type Meter struct {
	total atomic.Int64
}

// Add records n bytes served.
func (m *Meter) Add(n int64) { m.total.Add(n) }

// Total returns the bytes served since process start.
func (m *Meter) Total() int64 { return m.total.Load() }

// Middleware wraps a streaming handler and counts every byte written to the
// response. The wrapper preserves the optional interfaces streaming relies on:
// http.Flusher (HLS/SSE progressive writes) and io.ReaderFrom (the sendfile
// path http.ServeContent takes via io.Copy).
func (m *Meter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	if m == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		next(&countingWriter{ResponseWriter: w, meter: m}, r)
	}
}

type countingWriter struct {
	http.ResponseWriter
	meter *Meter
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.meter.Add(int64(n))
	return n, err
}

// ReadFrom keeps io.Copy on the efficient path (sendfile) when the underlying
// ResponseWriter supports it, while still counting.
func (c *countingWriter) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := c.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(src)
		c.meter.Add(n)
		return n, err
	}
	n, err := io.Copy(struct{ io.Writer }{c}, src) // c.Write counts
	return n, err
}

func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer's deadline
// and hijack controls through the wrapper.
func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
