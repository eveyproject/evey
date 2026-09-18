package transcoding

import (
	"context"
	"log/slog"
)

// resolver_line.go - the client's line as a plan input.
//
// A cast receiver measures its own throughput before asking for a master
// (?maxKbps=) and reloads on the floor when it starves (?abr=floor): old
// cast runtimes never dive onto an advertised rung by themselves (Bouygtel4K,
// 2026-08-03, a variant pinned through nine minutes of freeze), so the
// descent is the server's, deterministic and mono-variant. This used to be
// the HTTP adapter re-pinning the request to a transcode on the receiver's
// behalf and serving that second plan; here it is what the plan itself says.
//
// The rules are unchanged: copy-first (a line that carries the copy peak
// with a 1.2x margin leaves the copy alone), the HIGHEST rung the line
// carries (else the lowest), never a sidegrade (a rung at or above the copy
// peak), and a floor only when it halves the bitrate (a lean source keeps
// its copy). A transcode ladder is the player's to pick from: the line
// judges the copy rung only.

// Reasons the fitted plan's transcode carries (TranscodeReasons).
const (
	reasonLineExceeded = "client_line_exceeded"
	reasonFloorRequest = "abr_floor_requested"
)

// resolveLineConstrained is Resolve for a request carrying a line constraint.
// The natural plan first; when it is a copy the line cannot carry (or a
// rescue), the same request pinned to a transcode is resolved and its ladder
// judged: the fitting rung becomes the plan's Fit, or the copy stays.
//
// A request whose video decision is already pinned (vd=, every child URL of
// a served master) is never re-judged: the master's verdict rides the pin,
// so a child resolves ONE plan like any other request.
func (pr *PlanResolver) resolveLineConstrained(ctx context.Context, req PlaybackRequest) PlaybackPlan {
	natural := req
	natural.LineKbps, natural.ABRFloor = 0, false
	plan := pr.resolveDecided(ctx, natural, pr.Decide(ctx, natural))
	if req.PinnedVideoDecision != "" || !videoCopied(plan) {
		return plan
	}
	peak := req.Source.PeakKbps
	line, floor := req.LineKbps, req.ABRFloor
	if line > 0 && lineCarries(peak, line) {
		line = 0 // copy-first: the measure carries the copy; the floor, if asked, still applies
	}
	if line == 0 && !floor {
		return plan
	}
	fitted := natural
	fitted.PinnedVideoDecision = "transcode"
	fitted.fitReason = reasonFloorRequest
	if line > 0 {
		fitted.fitReason = reasonLineExceeded
	}
	tp := pr.resolveDecided(ctx, fitted, pr.Decide(ctx, fitted))
	if videoCopied(tp) || len(tp.Ladder) == 0 {
		slog.WarnContext(ctx, "hls-vod: line constraint unresolvable, the copy plan serves",
			slog.String("media_id", req.MediaID), slog.Int64("line_kbps", line), slog.Bool("floor", floor))
		return plan
	}
	idx := 0
	if line > 0 {
		for i, p := range tp.Ladder {
			if lineCarries(p.VideoBitrate+p.AudioBitrate, line) {
				idx = i
			}
		}
	}
	rung := tp.Ladder[idx]
	total := rung.VideoBitrate + rung.AudioBitrate
	switch {
	case line > 0 && total >= peak:
		return plan // no real drop over the copy: a pointless sidegrade
	case line == 0 && total*2 > peak+128:
		slog.InfoContext(ctx, "hls-vod: floor refused, source already lean",
			slog.String("media_id", req.MediaID), slog.Int64("rung_kbps", total), slog.Int64("copy_peak_kbps", peak+128))
		return plan
	}
	tp.Fit = LineFit{Single: true, Rung: idx}
	slog.InfoContext(ctx, "hls-vod: plan fitted to the client's line",
		slog.String("media_id", req.MediaID), slog.String("rung", rung.Name), slog.Int64("rung_kbps", total),
		slog.Int64("line_kbps", line), slog.Bool("floor", line == 0), slog.String("fp", tp.FingerprintHash))
	return tp
}

// videoCopied: the plan pushes the source video down the line verbatim - the
// ffmpeg copy rung or the packager's presentation (a host without the copy
// producer still copies through mkvgo).
func videoCopied(plan PlaybackPlan) bool {
	return plan.Route.Copy || plan.Delivery.Video == EngineMkvgo
}

// lineCarries: the line carries kbps with the 1.2x margin the starvation
// watchdog stands behind (an optimistic call has a net).
func lineCarries(kbps, lineKbps int64) bool {
	return kbps*12 <= lineKbps*10
}
