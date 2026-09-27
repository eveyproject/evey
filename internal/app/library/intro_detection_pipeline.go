package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// Two-pass detection pipeline orchestration: pHash + audio confirmation fusion, chapter pass, refinement.

// pickWindowMatchingDuration returns the window whose duration is closest to ref,
// within a 50% tolerance, or nil if none qualifies.
func pickWindowMatchingDuration(windows []matchWindow, refDurationMs int64) *matchWindow {
	const durationToleranceFactor = 2 // accept a window within [ref/2, ref*2] (the 50% tolerance)
	var best *matchWindow
	var bestDelta int64 = -1
	for i, w := range windows {
		dur := w.endMs - w.startMs
		if dur*durationToleranceFactor < refDurationMs || dur > refDurationMs*durationToleranceFactor {
			continue
		}
		delta := dur - refDurationMs
		if delta < 0 {
			delta = -delta
		}
		if bestDelta < 0 || delta < bestDelta {
			bestDelta = delta
			best = &windows[i]
		}
	}
	return best
}

// manifestTotalFrames returns the upper bound on extractable frames for a manifest.
func manifestTotalFrames(m *domainlibrary.TrickplayManifest) int {
	return m.ImageCount * m.TileWidth * m.TileHeight
}

// runPipeline runs the multi-stage intro detection pipeline:
//
//  1. pHash trickplay matching produces a list of candidate windows.
//  2. Audio fingerprint cross-correlation against siblings confirms which
//     candidate is the real recurring intro and rejects coincidental scene
//     transitions that share visual hashes (establishing shots, recurring
//     locations) but not audio content.
//  3. PrecisionPass tunes the boundaries via silence/black detection.
//
// Without sibling audio confirmation the visual-pHash signal alone often picks
// false positives (e.g. recurring landscape inserts), because runs of matched
// frames inside an episode can outscore the real intro when intro frames are
// fragmented by unique transition frames.
func (s *IntroDetectionService) runPipeline(ctx context.Context, item *domainlibrary.MediaItem, hint *domainlibrary.MediaSegment) ([]domainlibrary.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.runPipeline")
	defer span.End()
	// The registry's context: Stop cancels it, and the forks below are charged
	// to the task through it. An admin dry-run is foreground work, so pausing the
	// sweep must not cancel it.
	if !isIntroDryRun(ctx) {
		var done func()
		ctx, done = bgtasks.BeginCtx(ctx, introDetectionTask, string(item.ID))
		defer done()
	}

	mediaItemID := string(item.ID)
	cfg := loadIntroConfig(ctx, s.configRepo)

	// Accumulate the best segment PER TYPE. Passes run in DESCENDING authority;
	// the first pass to fill a type wins, and later passes only supply the types
	// still missing. This replaces the previous early-return design, where a
	// PARTIAL match (a provider that only knows the recap, chapters that only
	// mark the intro) suppressed local detection of every OTHER type. The
	// in-house pipeline must be self-sufficient - external providers are an
	// additive bonus layered on top, never a precondition for local detection.
	found := make(map[string]domainlibrary.MediaSegment, 3)
	add := func(segs []domainlibrary.MediaSegment, decision string) {
		var addedTypes []string
		for _, seg := range segs {
			if seg.Type == "" {
				continue
			}
			if _, ok := found[seg.Type]; ok {
				continue // a higher-authority pass already filled this type
			}
			found[seg.Type] = seg
			addedTypes = append(addedTypes, seg.Type)
		}
		if len(addedTypes) > 0 {
			slog.InfoContext(ctx, "intro_detection.pipeline_decision",
				slog.String("media_id", mediaItemID),
				slog.String("decision", decision),
				slog.String("types", strings.Join(addedTypes, ",")),
			)
		}
	}
	needIntro := func() bool { _, ok := found["intro"]; return !ok }

	// MKV chapters first: authoritative when the uploader labelled them
	// ("Intro" / "Opening" / "Generique" / "Previously" …) - they were encoded
	// into THIS file.
	add(s.chapterPass(ctx, item), "mkv_chapter_match")

	// ---- intro-only passes: each runs only while the intro is still missing ----

	// Season cluster hint: a season-level audio clustering already located this
	// episode's recurring intro at its own offset, widened per episode. Served as
	// is: on 54 dev episodes the silence/black refinement, the subtitle vote and
	// the scene-cut snap each moved a correct window away from the truth, and the
	// refinement erased three intros (2026-09-27). 0.8 keeps it a lone audio witness.
	if needIntro() && hint != nil {
		seg := *hint
		seg.Confidence = 0.8
		add([]domainlibrary.MediaSegment{seg}, "season_cluster_hint")
	}

	// Visual pHash + audio confirmation. classifyConfirmed can yield an intro
	// AND a short preceding recap.
	if needIntro() && cfg.PHashIntroEnabled && s.isTrickplayEnabled(ctx) {
		result, phashErr := s.pHashPass(ctx, item, cfg)
		if errors.Is(phashErr, errTrickplayPending) {
			// Not a failure of this file: the manifest is simply not generated
			// yet. Nothing is stored for this round (a weak fallback answer
			// stored now would stand until the attempts run out) and the
			// sweep retries the episode later, uncounted.
			slog.InfoContext(ctx, "intro_detection: trickplay manifest pending, episode deferred to a later sweep",
				slog.String("media_id", mediaItemID))
			return nil, errTrickplayPending
		}
		if phashErr != nil {
			slog.WarnContext(ctx, "intro_detection: phash pass failed, falling back",
				slog.String("media_id", mediaItemID),
				slog.Any("err", phashErr),
			)
		} else if result != nil && len(result.candidates) > 0 {
			var confirmed []confirmedWindow
			if s.chromaprinter != nil {
				confirmed = s.confirmAudioCandidatesChromaprint(ctx, item, result, cfg)
			}
			if len(confirmed) == 0 {
				confirmed = s.confirmAudioCandidates(ctx, item, result, cfg)
			}
			if len(confirmed) > 0 {
				// pHash boundaries cover only the *matched* frames, which
				// can be a strict subset of the real intro: title-card
				// frames stable across episodes match, but the establishing
				// shots and intertitles around them are unique-per-episode
				// and don't. Use silencedetect+blackdetect on a wider
				// window around each pHash window to widen the boundaries
				// out to the real start/end (silence before/after the
				// theme music). Anchored at the pHash window, so the
				// "first silence in the file" failure mode (studio logo
				// at 0s) doesn't apply.
				refined := s.refineWithPrecisionPass(ctx, item.File.Path, confirmed)
				segs := s.refineIntroEndsBySubtitle(ctx, item.File.Path, classifyConfirmed(refined, cfg))
				add(segs, "phash_confirmed")
			} else {
				slog.InfoContext(ctx, "intro_detection.pipeline_decision",
					slog.String("media_id", mediaItemID),
					slog.Int("candidates_evaluated", len(result.candidates)),
					slog.Int("siblings_total", len(result.siblings)),
					slog.String("decision", "fallback_no_candidate_confirmed"),
				)
			}
		}
	}

	// Leading distributor-logo block: FILMS ONLY. Series label this exact
	// pattern NEGATIVE in the ground-truth set ("HBO distributeur - Pas
	// d'intro") since a logo-on-black at 0:00 is common to every episode -
	// gating on MovieItem is what keeps this pass off series, not an
	// optimisation. Placed BEFORE the metadata provider: community rows are
	// never served (IsCommunityIntroSource), so a provider answer filling
	// "intro" first only starved this pass - prod 2026-08-29: every gold film
	// tidb knew about had NO served intro. And before the audio-primary
	// discovery / silence fallback: those two structurally can't find anything
	// on a film with no sibling episodes to compare against.
	if needIntro() && item.Type == domainlibrary.MovieItem && item.File != nil && item.File.Path != "" {
		durationSec := item.File.Duration.Seconds()
		logoStartMs, logoEndMs, ok, logoErr := s.detector.DetectLeadingLogoBlock(ctx, item.File.Path, durationSec)
		if logoErr != nil {
			slog.WarnContext(ctx, "intro_detection: leading logo block failed",
				slog.String("media_id", mediaItemID), slog.Any("err", logoErr))
		} else if ok {
			add([]domainlibrary.MediaSegment{{
				Type:    "intro",
				StartMs: logoStartMs,
				EndMs:   logoEndMs,
				Source:  "leading_black",
				// Above the serving floor (min_serve_confidence default
				// 0.55) but below a chapter/pHash-confirmed result: this
				// heuristic has no sibling or subtitle corroboration, only
				// the head-of-file black cluster.
				Confidence: 0.7,
			}}, "leading_logo_block")
		}
	}

	// External metadata provider AFTER the file-derived passes: community DBs
	// (tidb/introdb) know nothing about the local file's edition (repack, cut,
	// re-encode), so their timestamps lose to anything derived from the file
	// itself - the eval measured them tens of seconds off where the cluster was
	// within a second. They remain additive for the types still missing.
	// Non-fatal on error - fall through to the remaining heuristics.
	if s.metadataProvider != nil {
		if mdSegs, mdErr := s.metadataProvider.GetSegments(ctx, item); mdErr != nil {
			slog.WarnContext(ctx, "intro_detection: metadata provider failed, falling back",
				slog.String("media_id", mediaItemID), slog.Any("err", mdErr))
		} else {
			add(mdSegs, "metadata_provider_match")
		}
	}

	// Audio-primary DISCOVERY: the visual pass produced nothing confirmable
	// (poor SD trickplay, or a cold-open show whose title card isn't visually
	// stable). Let chromaprint discover a recurring intro directly across
	// siblings at a free per-episode offset - the variable-teaser case the
	// visual+confirm path structurally misses.
	if needIntro() && cfg.AudioDiscoveryEnabled {
		if seg, ok := s.discoverIntroByAudio(ctx, item); ok {
			add([]domainlibrary.MediaSegment{seg}, "chromaprint_discovery")
		}
	}

	// Last resort: the silence pass. Its error is only fatal when nothing else
	// covered the episode - a recap found upstream must still be returned even
	// if the silence pass fails to find the intro. A degenerate sub-second
	// result (the studio-logo silence at 0:00) is dropped downstream by the
	// heuristic-floor guard in detectIntroWithHint.
	//
	// It runs by default even though the serving gate withholds its 0.5 answer:
	// the row is stored for eval. skip_unservable_silence_fallback buys the
	// scan time back - shouldRunSilenceFallback holds that whole decision.
	if needIntro() && shouldRunSilenceFallback(cfg) {
		startMs, endMs, source, confidence, fbErr := s.concurrentFallback(ctx, item)
		if fbErr != nil {
			if len(found) == 0 {
				return nil, fbErr
			}
			slog.WarnContext(ctx, "intro_detection: silence fallback failed",
				slog.String("media_id", mediaItemID), slog.Any("err", fbErr))
		} else if startMs != 0 || endMs != 0 {
			add([]domainlibrary.MediaSegment{{
				Type:       "intro",
				StartMs:    startMs,
				EndMs:      endMs,
				Source:     source,
				Confidence: confidence,
			}}, "silence_fallback")
		}
	}

	out := make([]domainlibrary.MediaSegment, 0, len(found))
	for _, seg := range found {
		out = append(out, seg)
	}
	return out, nil
}

// refineWithPrecisionPass widens each pHash window to the actual intro
// boundaries. The pHash window is the anchor (it survived audio
// confirmation, so we trust it points at a real intro), but its raw
// boundaries only cover the matched frames - the actual intro can extend
// before the first match and after the last (frames unique per episode
// don't match across siblings).
//
// Strategy:
//  1. Run silencedetect+blackdetect on a +/-padSec window around the pHash
//     anchor. If a usable transition is found, use it (most accurate).
//  2. Otherwise apply a fixed +/-fallbackPadSec extension to the pHash
//     window. Series with continuous music (Robin Hood, action shows)
//     don't have detectable silence around their intro, so silencedetect
//     finds nothing - but the actual intro is still wider than the
//     matched-frame window. The fallback pad gives the user a usable
//     skip button rather than a too-tight 16s window.
//
// PrecisionPass used to be called on the whole episode and would lock onto
// the first studio-logo silence at 0s - that failure mode doesn't apply
// here because we anchor on the pHash window position.
func (s *IntroDetectionService) refineWithPrecisionPass(ctx context.Context, filePath string, windows []confirmedWindow) []confirmedWindow {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.refineWithPrecisionPass")
	defer span.End()

	if filePath == "" || len(windows) == 0 {
		return windows
	}
	const (
		padSec = 15.0
		// Asymmetric fallback pad: pHash matches the visually-stable parts of
		// an intro (title cards, logos), which typically sit at the END of
		// the intro. The START is more often a cold-open transition unique
		// per episode that doesn't match. So expand more on the start side
		// than the end side to cover the real intro span without
		// over-shooting into the post-intro content.
		fallbackPadStartMs = 10_000
		fallbackPadEndMs   = 2_000
	)
	out := make([]confirmedWindow, 0, len(windows))
	for _, w := range windows {
		startSec := float64(w.startMs) / 1000
		endSec := float64(w.endMs) / 1000
		refStart, refEnd, err := s.detector.PrecisionPass(ctx, filePath, startSec, endSec, padSec)

		// A refined boundary is only plausible within this margin of the pHash anchor.
		sanityMarginMs := int64(2 * padSec * 1000)
		usePrecision := err == nil &&
			refStart > 0 && refEnd > refStart &&
			refEnd >= w.startMs-sanityMarginMs &&
			refStart <= w.endMs+sanityMarginMs &&
			// PrecisionPass returns the *input* (startMs, endMs) when it
			// finds nothing - detect the no-op to fall through to padding.
			(refStart != w.startMs || refEnd != w.endMs)

		var newStart, newEnd int64
		var source string
		if usePrecision {
			newStart, newEnd, source = refStart, refEnd, "silencedetect"
		} else {
			newStart = max(w.startMs-fallbackPadStartMs, 0)
			newEnd = w.endMs + fallbackPadEndMs
			source = "fixed_pad"
		}

		slog.DebugContext(ctx, "intro_detection.refined",
			slog.Int64("phash_start_ms", w.startMs),
			slog.Int64("phash_end_ms", w.endMs),
			slog.Int64("refined_start_ms", newStart),
			slog.Int64("refined_end_ms", newEnd),
			slog.String("refine_source", source),
		)
		out = append(out, confirmedWindow{
			startMs:    newStart,
			endMs:      newEnd,
			matchCount: w.matchCount,
			confidence: w.confidence,
		})
	}
	return out
}

// refineEndBySubtitleOnset pulls an intro END to where dialogue resumes. A
// title sequence carries no dialogue, so the first subtitle cue after the intro
// start (past a short guard that skips a cold-open line lingering into the theme)
// marks content resumption ≈ the visual intro end. This is the one signal that
// nails the end on "music-bleed" shows, where the recurring AUDIO run overshoots
// the visual cut (theme music continues under the next scene) - the eval showed
// audio_cluster ends ~+30s late there. Returns endMs unchanged (a no-op) when
// the file has no subtitles or no plausible cue is found, so media without subs
// is unaffected.
func (s *IntroDetectionService) refineEndBySubtitleOnset(ctx context.Context, filePath string, startMs, endMs int64) int64 {
	if s.detector == nil || filePath == "" {
		return endMs
	}
	cues, err := s.detector.SubtitleCueStarts(ctx, filePath)
	if err != nil || len(cues) == 0 {
		return endMs
	}
	const (
		guardMs = 5_000 // skip a cold-open line lingering into the theme onset
		slackMs = 8_000 // tolerate a little past the audio end (grid jitter)
	)
	// Upper bound = the audio end. The recurring-audio run length OVERSHOOTS the
	// visual cut (theme bleeds into the next scene), so the true dialogue-resume
	// sits at or before it. A first cue PAST the audio end means dialogue didn't
	// resume right after the intro (a silent post-intro action scene) - that cue
	// is not the intro's end, so we keep the audio end rather than overshoot far.
	for _, c := range cues {
		if c <= startMs+guardMs {
			continue
		}
		if c > endMs+slackMs {
			break // first plausible cue is past the audio end → don't trust it
		}
		return c // dialogue resumes within the intro window ≈ the visual end
	}
	return endMs
}

// fuseIntroEnd combines every available END witness into a consensus boundary,
// then snaps it to a frame-accurate scene cut. No single signal is reliable, so
// they VOTE: the audio run end (always present), the subtitle dialogue-resume
// (when subs exist), and a silence/black boundary (when present). Where they
// AGREE we are confident; stacking them is what keeps the end robust on series
// with NO subtitles. Returns the fused end and a confidence in [0,1] (the share
// of evidence weight that agreed). All votes are bounded by the audio end + a
// little slack - the recurring-audio run overshoots, so the true resume is at or
// before it.
func (s *IntroDetectionService) fuseIntroEnd(ctx context.Context, filePath string, startMs, audioEnd int64) (int64, float64) {
	// Relative weights of the three end witnesses: the subtitle dialogue-resume
	// is the most trustworthy, the recurring-audio run end always exists but
	// overshoots, silence/black sits in between.
	const (
		audioVoteWeight      = 0.4
		subtitleVoteWeight   = 1.0
		silenceVoteWeight    = 0.6
		maxSilenceDeltaMs    = 20_000 // reject a silence boundary too far from the audio end
		fuseAgreeToleranceMs = 4_000
		sceneCutTailSec      = 6.0 // extra window after the end for the scene-cut scan
		sceneCutThreshold    = 0.3
		snapToleranceMs      = 2_500
	)
	if s.detector == nil || filePath == "" {
		return audioEnd, 0.5
	}
	votes := []boundaryVote{{ms: audioEnd, weight: audioVoteWeight, source: "audio"}}

	// Subtitle dialogue-resume (already capped at the audio end inside the helper).
	if subEnd := s.refineEndBySubtitleOnset(ctx, filePath, startMs, audioEnd); subEnd != audioEnd {
		votes = append(votes, boundaryVote{ms: subEnd, weight: subtitleVoteWeight, source: "subtitle"})
	}
	// Silence/black boundary near the audio end (PrecisionPass returns the input
	// unchanged when it finds nothing, so a differing end means a real boundary).
	if _, refEnd, err := s.detector.PrecisionPass(ctx, filePath, float64(startMs)/1000, float64(audioEnd)/1000, sceneCutTailSec); err == nil {
		if d := refEnd - audioEnd; refEnd > startMs && refEnd != audioEnd && d < maxSilenceDeltaMs && d > -maxSilenceDeltaMs {
			votes = append(votes, boundaryVote{ms: refEnd, weight: silenceVoteWeight, source: "silence"})
		}
	}

	fb, ok := fuseBoundary(votes, fuseAgreeToleranceMs)
	if !ok {
		return audioEnd, 0.5
	}
	end := fb.ms
	// Snap the consensus to a frame-accurate content cut so the skip lands on a
	// scene boundary, not mid-shot. Bounded window → cheap.
	dur := float64(end-startMs)/1000 + sceneCutTailSec
	if cuts, cErr := s.detector.SceneCutTimestamps(ctx, filePath, float64(startMs)/1000, dur, sceneCutThreshold); cErr == nil {
		if snapped, did := snapToNearest(end, cuts, snapToleranceMs); did {
			end = snapped
		}
	}
	// Every witness and both stages, so an eval can replay the end without re-decoding.
	slog.DebugContext(ctx, "intro_detection.fuse_end",
		slog.Int64("audio_end_ms", audioEnd), slog.String("votes", fmt.Sprint(votes)),
		slog.Int64("fused_ms", fb.ms), slog.Int64("snapped_ms", end), slog.Float64("confidence", fb.confidence))
	return end, fb.confidence
}

// refineIntroEndsBySubtitle fuses the END of every intro segment in segs (in
// place), leaving recap/other types untouched. Used by the heuristic paths
// (cluster hint, pHash) whose ends overshoot on music-bleed shows; authoritative
// paths (chapter/provider) are never routed through here.
func (s *IntroDetectionService) refineIntroEndsBySubtitle(ctx context.Context, filePath string, segs []domainlibrary.MediaSegment) []domainlibrary.MediaSegment {
	for i := range segs {
		if segs[i].Type == "intro" {
			segs[i].EndMs, _ = s.fuseIntroEnd(ctx, filePath, segs[i].StartMs, segs[i].EndMs)
		}
	}
	return segs
}

// chapterPass looks for Matroska chapters whose title matches a known
// intro/recap pattern (Intro, Opening, Theme, Generique, Title Sequence,
// Previously, Recap, ...). Returns one MediaSegment per matched chapter
// with source="mkv_chapter", confidence 1.0 - chapters are authoritative
// since they were encoded by the uploader.
//
// Special cases:
//   - "Intro End" alone is not an intro; it's the END marker. Pair with a
//     preceding chapter (typically the cold open or studio logo) by using
//     [start_of_episode, intro_end_chapter.start_ms] as the intro window.
//     Heuristic: if "Intro End" is found and no plain "Intro" chapter
//     exists, treat the previous chapter's start (or 0) as the intro start.
func (s *IntroDetectionService) chapterPass(ctx context.Context, item *domainlibrary.MediaItem) []domainlibrary.MediaSegment {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.chapterPass")
	defer span.End()

	if s.chapterRepo == nil {
		return nil
	}
	chapters, err := s.chapterRepo.GetByMediaItemID(ctx, item.ID)
	if err != nil || len(chapters) == 0 {
		return nil
	}
	// Sort by start in case the repo doesn't guarantee order.
	sort.Slice(chapters, func(i, j int) bool { return chapters[i].StartTimeMs < chapters[j].StartTimeMs })

	// MKV chapters often record only the start timestamp; end_ms is then 0
	// (or equal to start). Standard convention: a chapter ends where the
	// next one begins. Fill missing ends in-place so the segment builders
	// below can rely on EndTimeMs > StartTimeMs.
	durationMs := int64(0)
	if item.File != nil {
		durationMs = item.File.Duration.Milliseconds()
	}
	for i := range chapters {
		if chapters[i].EndTimeMs > chapters[i].StartTimeMs {
			continue
		}
		if i+1 < len(chapters) && chapters[i+1].StartTimeMs > chapters[i].StartTimeMs {
			chapters[i].EndTimeMs = chapters[i+1].StartTimeMs
		} else if durationMs > chapters[i].StartTimeMs {
			// Last chapter with no recorded end: it runs to the end of the file.
			// Without this, a final "Générique" chapter stored end_ms=0 - an
			// inverted segment persisted as-is.
			chapters[i].EndTimeMs = durationMs
		}
	}

	classify := func(title string, startMs int64) string {
		t := strings.ToLower(strings.TrimSpace(title))
		// "Générique" in French names BOTH the title sequence and the end
		// credits. An explicit "de fin" wins; otherwise the position decides:
		// a "générique" (or bare "credits") past the midpoint of the file is
		// the credits roll, not an intro - a movie stored its end-credits
		// chapter at 2h35 as an "intro" before this guard.
		inBackHalf := durationMs > 0 && startMs > durationMs/2
		switch {
		case t == "intro end" || t == "end of intro" || t == "intro ends":
			return "intro_end_marker"
		case strings.Contains(t, "generique de fin") || strings.Contains(t, "générique de fin"):
			return "end_credits"
		case strings.Contains(t, "end credits") ||
			strings.Contains(t, "closing credits"):
			return "end_credits"
		case strings.Contains(t, "ending"):
			// Bare "ending" is weak evidence ("Ending Theme" can open an
			// anime episode, and Contains also matches pending/ascending):
			// only trust it in the back half of the file.
			if inBackHalf {
				return "end_credits"
			}
			return ""
		case strings.Contains(t, "intro") ||
			strings.Contains(t, "opening") ||
			strings.Contains(t, "theme") ||
			strings.Contains(t, "title sequence") ||
			strings.Contains(t, "generique") || strings.Contains(t, "générique"):
			if inBackHalf {
				return "end_credits"
			}
			return "intro"
		case strings.Contains(t, "previously") ||
			strings.Contains(t, "recap") ||
			strings.HasPrefix(t, "previously on") ||
			strings.Contains(t, "résumé") || strings.Contains(t, "resume des"):
			return "recap"
		}
		return ""
	}

	var (
		out         []domainlibrary.MediaSegment
		hasIntro    bool
		introEndIdx = -1
	)
	for i, ch := range chapters {
		kind := classify(ch.Title, ch.StartTimeMs)
		// A chapter whose end could not be resolved (last chapter, unknown file
		// duration) would emit an inverted segment - skip it, except the
		// intro_end_marker which only uses its start.
		if kind != "" && kind != "intro_end_marker" && ch.EndTimeMs <= ch.StartTimeMs {
			continue
		}
		switch kind {
		case "intro":
			out = append(out, domainlibrary.MediaSegment{
				Type: "intro", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: ch.StartTimeMs, EndMs: ch.EndTimeMs,
			})
			hasIntro = true
		case "recap":
			out = append(out, domainlibrary.MediaSegment{
				Type: "recap", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: ch.StartTimeMs, EndMs: ch.EndTimeMs,
			})
		case "end_credits":
			out = append(out, domainlibrary.MediaSegment{
				Type: "end_credits", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: ch.StartTimeMs, EndMs: ch.EndTimeMs,
			})
		case "intro_end_marker":
			introEndIdx = i
		}
	}

	// "Intro End" without a paired "Intro" chapter: synthesise an intro
	// segment from [previous chapter start, intro_end start]. Skip if a
	// plain Intro chapter was already added (avoid duplicates).
	if introEndIdx >= 0 && !hasIntro {
		var startMs int64
		if introEndIdx > 0 {
			startMs = chapters[introEndIdx-1].StartTimeMs
		}
		endMs := chapters[introEndIdx].StartTimeMs
		if endMs > startMs {
			out = append(out, domainlibrary.MediaSegment{
				Type: "intro", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: startMs, EndMs: endMs,
			})
		}
	}
	return out
}

// classifyConfirmed converts audio-confirmed windows into typed MediaSegments.
// At most one intro and one preceding recap (<60s, occurs before intro) are
// emitted. Source is set to "phash" with confidence inherited from the
// confirmation stage (typically 0.95 for audio-confirmed, 0.7 for visual-only
// fallback).
func classifyConfirmed(windows []confirmedWindow, cfg introConfig) []domainlibrary.MediaSegment {
	if len(windows) == 0 {
		return nil
	}
	// Pick the longest as intro.
	intro := windows[0]
	for _, w := range windows[1:] {
		if (w.endMs - w.startMs) > (intro.endMs - intro.startMs) {
			intro = w
		}
	}
	out := []domainlibrary.MediaSegment{{
		Type:       "intro",
		StartMs:    intro.startMs,
		EndMs:      intro.endMs,
		Source:     "phash",
		Confidence: intro.confidence,
	}}
	// Look for the LAST short window (< RecapMaxMs) ending before the intro
	// start: that's the recap (a brief "previously on…" right before the title
	// sequence). Taking the first chronological match instead would mislabel an
	// earlier recurring element (a confirmed network bumper) as the recap and
	// never consider the true one sitting closest to the intro.
	var recap *confirmedWindow
	for i := range windows {
		w := windows[i]
		if w.endMs > intro.startMs {
			continue
		}
		if (w.endMs - w.startMs) >= cfg.RecapMaxMs {
			continue
		}
		if recap == nil || w.endMs > recap.endMs {
			recap = &windows[i]
		}
	}
	if recap != nil {
		out = append([]domainlibrary.MediaSegment{{
			Type:       "recap",
			StartMs:    recap.startMs,
			EndMs:      recap.endMs,
			Source:     "phash",
			Confidence: 0.7,
		}}, out...)
	}
	return out
}
