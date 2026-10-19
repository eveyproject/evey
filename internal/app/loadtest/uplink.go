package loadtest

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// uplink.go - how fast the line in front of the server sends to the internet.
//
// The audience the run climbs is the server's own: its viewers reach it from
// inside, so the upload of the line real viewers come through is never on
// their path. The run measures it apart, by sending to a speed-test service
// served from a content delivery network: the nearest of its points of
// presence answers wherever the server stands, so the figure is the line's,
// not the distance to one far city's.

// UplinkProbeURL is the speed-test upload endpoint the server measures its
// line against: speed.cloudflare.com answers it from the nearest of its points
// of presence worldwide.
const UplinkProbeURL = "https://speed.cloudflare.com/__up"

const (
	// uplinkStreams is how many uploads run at once: one TCP stream does not
	// fill a fast line across any distance, a few do.
	uplinkStreams = 6
	// uplinkChunkBytes is the body of one upload; each stream sends one after
	// the other until the probe's time is up.
	uplinkChunkBytes = 8 << 20
	// uplinkProbeDuration is how long the line is measured. The first second
	// of every stream is its TCP ramp, and is counted: a probe long enough
	// makes it small.
	uplinkProbeDuration = 10 * time.Second
	// uplinkMinBytes is the least a probe must have sent to be a measure: less
	// is a service that refused or a line that did not carry, not a speed.
	uplinkMinBytes = 4 << 20
)

// Uplink is what the line in front of the server sent during the probe.
type Uplink struct {
	BytesPerSec int64
	Measured    bool
	// Service names the host measured against, for the report.
	Service string
}

// countingZeros is an upload body of n zero bytes that adds what is actually
// read off it to sent: a stream cut off by the deadline still counts what it
// had sent.
type countingZeros struct {
	left int64
	sent *atomic.Int64
}

func (z *countingZeros) Read(p []byte) (int, error) {
	if z.left <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), z.left)
	clear(p[:n])
	z.left -= n
	z.sent.Add(n)
	return int(n), nil
}

// measureUplink sends to probeURL on uplinkStreams streams for d and returns
// the rate. A probe that sent too little to be a measure (no network, a
// refusal) comes back unmeasured: the report then says so instead of naming
// a line speed.
func measureUplink(ctx context.Context, client *http.Client, probeURL string, d time.Duration) Uplink {
	u := Uplink{}
	if parsed, err := url.Parse(probeURL); err == nil {
		u.Service = parsed.Host
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var sent atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for range uplinkStreams {
		wg.Go(func() {
			for ctx.Err() == nil {
				// The client reads the whole body even from a service that has
				// already refused it: an upload counts once the service accepted
				// it, or when the probe's own deadline cut it short.
				var chunk atomic.Int64
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, probeURL, &countingZeros{left: uplinkChunkBytes, sent: &chunk})
				if err != nil {
					return
				}
				req.ContentLength = uplinkChunkBytes
				req.Header.Set("Content-Type", "application/octet-stream")
				resp, err := client.Do(req)
				if err != nil {
					if ctx.Err() != nil {
						sent.Add(chunk.Load())
					}
					return
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode >= 300 {
					return
				}
				sent.Add(chunk.Load())
			}
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	if n := sent.Load(); n >= uplinkMinBytes && elapsed > 0 {
		u.BytesPerSec = int64(float64(n) / elapsed.Seconds())
		u.Measured = true
	}
	return u
}

// remoteHeadroom is the share of the line the audience may fill: a line run
// flat out queues every viewer's next segment behind the others, and the
// start-up times break before the bytes run out.
const remoteHeadroom = 0.8

// remoteAudience is how many viewers outside can be served: the server's own
// figure, or as many as the line carries at what one viewer of this run
// pulled, whichever is smaller. Zero when either side is unknown. byLine is
// true when the line is the smaller.
func remoteAudience(sustained int, servedBytesPerSec int64, up Uplink) (viewers int, byLine bool) {
	if sustained <= 0 || servedBytesPerSec <= 0 || !up.Measured {
		return 0, false
	}
	perViewer := float64(servedBytesPerSec) / float64(sustained)
	line := int(float64(up.BytesPerSec) * remoteHeadroom / perViewer)
	if line < sustained {
		return line, true
	}
	return sustained, false
}
