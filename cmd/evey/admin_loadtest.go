package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"google.golang.org/protobuf/encoding/protojson"
)

// adminLoadTest drives the server's load-test from the CLI. UNLIKE the other
// admin subcommands (which run INSTEAD of the server, against the DB directly),
// this one needs a LIVE server: it is a plain HTTP client of the local gateway.
//
//   - The gateway listens on the gRPC port + 1 (EVEY_SERVER_PORT is the gRPC
//     port; the REST gateway is +1), so we hit 127.0.0.1:<port+1>.
//   - Auth: we read the JWT signing key (same key rotate-jwt-key manages) and
//     mint ourselves a 15-minute admin token - no new server mechanism.
//   - `admission_full` is a VERDICT (the box refused a new stream = its ceiling),
//     not an error: the command still exits 0.
func adminLoadTest(args []string) error {
	fs := adminFlags("load-test")
	scenario := fs.String("scenario", "balanced",
		"transcode | natural | mkvgo | light | balanced | heavy | mkvgo+N (N pinned transcodes, then pure direct-play)")
	maxViewers := fs.Int("max-viewers", 0, "cap the ramp (0 = server default, 12)")
	mode := fs.String("mode", "", "ramp (default) | flash (everyone at once) | seek (viewers jump around) | endurance (hold the tier, default 30min)")
	durationSec := fs.Int("duration", 0, "bound the run in seconds (0 = mode default)")
	coldCache := fs.Bool("cold-cache", false,
		"keep evicting the library from the OS page cache, so the run measures the storage instead of the RAM (a library that fits in memory otherwise overstates the viewer count)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Optional positional preset sizes the run without flag gymnastics:
	//   minimal   smoke test, ~40s (4 viewers)
	//   standard  the server default ramp (12)
	//   max       past any plausible ceiling, stops on the box's own verdict (32)
	// An explicit --max-viewers always wins over the preset.
	if preset := fs.Arg(0); preset != "" {
		presetMax := map[string]int{"minimal": 4, "standard": 0, "max": 32}
		m, ok := presetMax[preset]
		if !ok {
			return fmt.Errorf("unknown preset %q (minimal | standard | max)", preset)
		}
		if *maxViewers == 0 {
			*maxViewers = m
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	token, err := forgeAdminToken(cfg)
	if err != nil {
		return err
	}

	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1/admin/load-test", cfg.Server.Port+1)
	client := &http.Client{Timeout: 15 * time.Second}

	// Ctrl-C during the run offers to cancel + delete it server-side.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start, err := loadTestReq(ctx, client, http.MethodPost, base, token,
		map[string]any{"scenario": *scenario, "max_viewers": *maxViewers, "mode": *mode, "duration_sec": *durationSec, "cold_cache": *coldCache})
	if err != nil {
		return fmt.Errorf("start load-test (is the server running on :%d?): %w", cfg.Server.Port+1, err)
	}
	fmt.Fprintf(os.Stderr, "load-test started · scenario=%s · max-viewers=%s\n\n",
		nonEmpty(*scenario, "balanced"), viewersLabel(*maxViewers))

	last := start
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for last.GetRunning() {
		select {
		case <-ctx.Done():
			// Restore signal handling for a second Ctrl-C (hard quit), then ask.
			stop()
			fmt.Fprintf(os.Stderr, "\n\ninterrupted - cancel and delete this run? [y/N] ")
			if yes := readYes(); yes {
				bg, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				if _, derr := loadTestReq(bg, client, http.MethodDelete, base, token, nil); derr != nil {
					cancel()
					return fmt.Errorf("cancel: %w", derr)
				}
				cancel()
				fmt.Fprintln(os.Stderr, "run cancelled.")
			} else {
				fmt.Fprintln(os.Stderr, "left the run going in the background (`evey admin load-test` polling can resume via the panel).")
			}
			return nil
		case <-ticker.C:
			st, err := loadTestReq(ctx, client, http.MethodGet, base, token, nil)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					continue
				}
				// One failed poll must not lose the run: the gateway drops for
				// ~5s on a dev rebuild (Air) and hiccups happen. Keep trying
				// for a bounded window and only then conclude.
				st, err = repollUntilBack(ctx, client, base, token)
				if err != nil {
					return err
				}
				if st == nil {
					continue // interrupted while waiting; the ctx.Done case handles it
				}
				if !st.GetRunning() && st.GetLastReport() == nil {
					return errors.New("the server restarted during the run (dev rebuild?) - the load test died with it, its state was in memory. Start it again")
				}
			}
			last = st
			printRamp(st)
		}
	}

	fmt.Fprint(os.Stderr, "\r\x1b[K") // clear the live ramp line
	printReport(last.GetLastReport())
	return nil
}

// repollUntilBack retries the status poll for up to 30s after a transport
// failure (the gateway drops ~5s on a dev rebuild). Returns the first status
// it gets, nil if the ctx was interrupted, or an error once the window is
// exhausted with the server still unreachable.
func repollUntilBack(ctx context.Context, client *http.Client, base, token string) (*eveyv1.LoadTestStatus, error) {
	fmt.Fprint(os.Stderr, "\r\x1b[K  server unreachable, retrying...")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(2 * time.Second):
		}
		if st, err := loadTestReq(ctx, client, http.MethodGet, base, token, nil); err == nil {
			return st, nil
		}
	}
	return nil, errors.New("server unreachable for 30s - it went down during the run; check `docker logs` and start the test again")
}

// forgeAdminToken reads the JWT private key and mints a 15-minute admin token.
// The server's token validator is epoch-guarded (it rejects tokens for unknown
// users), so we mint AS a real admin: pull one admin's id + current token_epoch
// straight from the DB (the CLI already has DB access, like reset-password) and
// stamp them into the claims so the token validates.
func forgeAdminToken(cfg *config.Config) (string, error) {
	path := cfg.JWT.PrivateKeyPath
	if path == "" {
		return "", errors.New("EVEY_JWT_PRIVATE_KEY_PATH is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no JWT key at %s - is the server set up and running? (%w)", path, err)
	}
	key, err := auth.LoadOrGenerateRSAKey(path)
	if err != nil {
		return "", fmt.Errorf("load JWT key: %w", err)
	}

	db, err := openDBReadOnly(cfg)
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var uid, uname string
	var epoch int64
	err = db.QueryRowContext(ctx,
		`SELECT id, username, token_epoch FROM users WHERE is_admin = true LIMIT 1`).
		Scan(&uid, &uname, &epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("no admin user found - set one up first")
	}
	if err != nil {
		return "", fmt.Errorf("find admin user: %w", err)
	}

	gen := auth.NewJWTGenerator(key, 15*time.Minute)
	return gen.GenerateAccessToken(user.Claims{
		UserID:     uid,
		Username:   uname,
		IsAdmin:    true,
		TokenEpoch: epoch,
	})
}

// loadTestReq issues one request and decodes the LoadTestStatus reply (protojson,
// matching the gateway's camelCase output). A non-2xx returns the body as error.
func loadTestReq(ctx context.Context, c *http.Client, method, url, token string, body any) (*eveyv1.LoadTestStatus, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		raw = nil // status line still tells the story below
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	var st eveyv1.LoadTestStatus
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &st, nil
}

// printRamp draws a single, overwriting live line while the ramp climbs.
func printRamp(st *eveyv1.LoadTestStatus) {
	secs := st.GetElapsedMs() / 1000
	extra := ""
	if r := st.GetLastReport(); r != nil && r.GetSegmentMsP95() > 0 {
		extra = fmt.Sprintf("  seg p95 %dms/%dms", r.GetSegmentMsP95(), r.GetSegmentSloMs())
	}
	fmt.Fprintf(os.Stderr, "\r\033[K  %-8s viewers %-3d  %3ds%s", st.GetPhase(), st.GetViewers(), secs, extra)
}

// printReport prints the final verdict.
func printReport(r *eveyv1.LoadTestReport) {
	if r == nil {
		fmt.Fprintln(os.Stderr, "no report.")
		return
	}
	fmt.Fprintf(os.Stderr, "\n  scenario           %s\n", r.GetScenario())
	if r.GetColdCache() {
		fmt.Fprintf(os.Stderr, "  page cache         evicted throughout - the figures measure the STORAGE, not the RAM\n")
	}
	fmt.Fprintf(os.Stderr, "  sustained viewers  %d\n", r.GetSustainedViewers())
	fmt.Fprintf(os.Stderr, "  stopped because    %s", r.GetStopReason())
	if d := r.GetStopDetail(); d != "" {
		fmt.Fprintf(os.Stderr, " - %s", d)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  TTFF   p50 %dms · p95 %dms\n", r.GetTtffMsP50(), r.GetTtffMsP95())
	fmt.Fprintf(os.Stderr, "  segment p50 %dms · p95 %dms  (SLO %dms)\n",
		r.GetSegmentMsP50(), r.GetSegmentMsP95(), r.GetSegmentSloMs())
	if mix := r.GetViewerMix(); len(mix) > 0 {
		fmt.Fprintf(os.Stderr, "  mix at ceiling    ")
		first := true
		for _, k := range []string{"mkvgo", "transcode", "natural"} {
			if v, ok := mix[k]; ok {
				if !first {
					fmt.Fprint(os.Stderr, " · ")
				}
				fmt.Fprintf(os.Stderr, "%s %d", k, v)
				first = false
			}
		}
		fmt.Fprintln(os.Stderr)
	}
	printSourceMix(r)
	if r.GetStopReason() == "admission_full" {
		fmt.Fprintln(os.Stderr, "\n  → admission_full is the ceiling verdict, not a failure.")
	}
}

// printSourceMix names what the run decoded. The viewer count alone is not a
// verdict: the candidate pool is drawn by duration, so the same host reports a
// very different ceiling depending on the codec draw (measured 2.12x realtime
// on 4K HEVC against 8.43x on 1080p H.264, same output rung). A run whose mix
// is unknown says so rather than letting the number pass for more than it is.
func printSourceMix(r *eveyv1.LoadTestReport) {
	mix := r.GetSourceMix()
	if len(mix) == 0 {
		return
	}
	total := 0
	named := 0
	for _, m := range mix {
		total += int(m.GetCount())
		if m.GetCodec() != "" {
			named += int(m.GetCount())
		}
	}
	fmt.Fprintf(os.Stderr, "  served            ")
	first := true
	for _, m := range mix {
		if m.GetCodec() == "" {
			continue
		}
		if !first {
			fmt.Fprint(os.Stderr, " · ")
		}
		pct := 0
		if total > 0 {
			pct = int(m.GetCount()) * 100 / total
		}
		fmt.Fprintf(os.Stderr, "%s %dp x%d (%d%%)", strings.ToUpper(m.GetCodec()), m.GetHeight(), m.GetCount(), pct)
		first = false
	}
	if named == 0 {
		fmt.Fprint(os.Stderr, "unknown - the candidates' codecs could not be resolved")
	}
	fmt.Fprintln(os.Stderr)
	if named < total {
		fmt.Fprintf(os.Stderr, "  → %d of %d titles had no probed codec: read the ceiling with that in mind.\n", total-named, total)
	}
}

func readYes() bool {
	var s string
	if _, err := fmt.Fscanln(os.Stdin, &s); err != nil {
		return false // EOF or unreadable stdin reads as "no"
	}
	return s == "y" || s == "Y" || s == "yes"
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func viewersLabel(n int) string {
	if n <= 0 {
		return "server default (12)"
	}
	return fmt.Sprintf("%d", n)
}
