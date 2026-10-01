package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type playbackHandler struct {
	eveyv1.UnimplementedPlaybackServiceServer
	cmd         *appplayback.CommandService
	query       *appplayback.QueryService
	librarySvc  *applibrary.QueryService
	activitySvc *appactivity.CommandService
	userQuery   *appuser.QueryService
	userAccess  *appuser.AccessService
	translator  *eveyi18n.Translator
	// metadataSvc batch-resolves poster blurhashes for resume/next-up cards
	// (fillPrimaryBlurhashes). Optional: nil skips the stamping.
	metadataSvc artworkBatchGetter
	// hlsManager kills a media's HLS sessions on explicit StopStream. Wired to
	// the fan-out remover (software + hardware) so a stopped software stream is
	// torn down promptly instead of lingering until its idle GC.
	hlsManager HLSSessionRemover
	// keyframeOnDemand fills a missing keyframe index inline at GetStreamInfo
	// (bounded fast-path probe; see apptranscoding.KeyframeOnDemand). Optional.
	keyframeOnDemand *apptranscoding.KeyframeOnDemand
	// remoteAccess reads the operator-wide remote bitrate cap and the LAN list
	// live, per request: a session whose real client IP is classified WAN is
	// clamped to the cap, and a change in Admin -> Remote Access applies to the
	// next playback without a restart. nil or a zero cap = feature off.
	remoteAccess func() (maxKbps int, lanNetworks []string)
	// decider answers GetStreamInfo with the plan's decision (Decide with the
	// request-time pins) - bound by the composition root once the HTTP
	// pipeline that owns the resolver exists, before this server starts.
	decider apptranscoding.PlaybackDecider
	// hours warns a kids profile's player before its opening ends. Optional.
	hours        ProfileHoursReader
	closingWarns closingWarnings
}

func (h *playbackHandler) playbackDecider() apptranscoding.PlaybackDecider {
	if h.decider == nil {
		return apptranscoding.UnwiredDecider()
	}
	return h.decider
}

func (h *playbackHandler) audit(ctx context.Context, userID, action, resourceType string, resourceID *string, details map[string]any) {
	if h.activitySvc == nil {
		return
	}
	e := &activity.AuditLogEntry{
		UserID:       shared.UserID(userID),
		ProfileID:    auditProfileID(ctx, userID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	if ip := interceptors.ClientIPFromCtx(ctx); ip != "" {
		e.ClientIP = &ip
	}
	if ua := interceptors.UserAgentFromCtx(ctx); ua != "" {
		e.UserAgent = &ua
	}
	if dn := interceptors.DeviceNameFromCtx(ctx); dn != "" {
		e.DeviceName = &dn
	}
	if err := h.activitySvc.CreateAuditEntry(ctx, e); err != nil {
		slog.WarnContext(ctx, "audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

// videoKeyframesMissing reports whether the (first) video stream has no
// keyframe index yet - the gate for the on-demand inline fill. Audio-only
// media (no video stream) never probes.
func videoKeyframesMissing(streams []*library.MediaStream) bool {
	for _, s := range streams {
		if s == nil || s.StreamType != library.StreamTypeVideo {
			continue
		}
		return len(s.Keyframes) == 0
	}
	return false
}

// clientProfileFromRequest layers the client's declared capabilities over the
// conservative web default. Only an explicit signal lifts a default: a client
// that omits HDR support keeps tonemapping rather than direct-playing raw HDR
// to a display that would wash it out.
//
// userAgent drops the codecs a known-broken build declares, the same
// correction the HLS handlers apply to ?audioCodecs, so the stream info and
// the master agree on the audio decision.
func clientProfileFromRequest(req *eveyv1.GetStreamInfoRequest, userAgent string) transcoding.ClientProfile {
	client := transcoding.DefaultWebProfile
	if len(req.SupportedCodecs) > 0 {
		client.VideoCodecs = req.SupportedCodecs
	}
	if codecs := transcoding.WithoutBrokenPlayerCodecs(userAgent, req.SupportedAudioCodecs); len(codecs) > 0 {
		client.AudioCodecs = codecs
	}
	if len(req.SupportedContainers) > 0 {
		client.Containers = req.SupportedContainers
	}
	// Codecs this client takes muxed in fMP4, NOT the ones it can decode (see
	// ClientProfile.FMP4AudioPassthrough). Threaded here so the stream-info
	// panel describes the same bytes the HLS handler will produce: the two read
	// the same eligibility, one from the proto field, one from the query param.
	if len(req.Fmp4AudioPassthrough) > 0 {
		client.FMP4AudioPassthrough = req.Fmp4AudioPassthrough
	}
	if req.MaxWidth > 0 {
		client.MaxWidth = int(req.MaxWidth)
	}
	if req.MaxHeight > 0 {
		client.MaxHeight = int(req.MaxHeight)
	}
	if req.SupportsHdr {
		client.SupportsHDR = true
	}
	// Same level ceiling the HLS master reads from ?maxH264Level=: the
	// stream-info verdict and the manifest must judge the same decoder.
	if req.MaxH264Height > 0 {
		client.MaxH264Height = int(req.MaxH264Height)
	}
	if req.MaxH264Level > 0 {
		client.MaxH264Level = int(req.MaxH264Level)
	}
	return client
}

// resolvePlaybackPrefs builds the transcoding preferences and the effective max
// bitrate for this request: the network type comes from the request, everything
// else from the caller's profile (a tighter profile bitrate wins). It then folds
// in the tracks the client PRE-SELECTED (absolute probed stream indices, the
// same numbering MediaStreams exposes; 0 = "not set") so the decision judges the
// streams that will actually play - a selected bitmap subtitle forces the
// burn-in transcode instead of advertising a direct_play that would silently
// drop it, and a selected audio track is judged instead of the container
// default. Unknown indices are ignored (the historical default-track behaviour).
func (h *playbackHandler) resolvePlaybackPrefs(ctx context.Context, req *eveyv1.GetStreamInfoRequest, streams []*library.MediaStream) (apptranscoding.TranscodingPreferences, int) {
	var maxBitrateKbps int
	if req.MaxBitrate > 0 {
		maxBitrateKbps = int(req.MaxBitrate / 1000)
	}

	prefs := apptranscoding.DefaultTranscodingPreferences()
	prefs.NetworkType = normalizeNetworkType(req.NetworkType)
	if h.userQuery != nil {
		if claims := interceptors.ClaimsFromContext(ctx); claims != nil && claims.ProfileID != "" {
			if prof, err := h.userQuery.GetProfileByID(ctx, claims.ProfileID); err == nil && prof != nil {
				parsed := apptranscoding.ParseTranscodingPreferences(prof.Preferences)
				// Keep network type from request; everything else from profile.
				parsed.NetworkType = prefs.NetworkType
				prefs = parsed
				if prof.MaxBitrateKbps != nil && *prof.MaxBitrateKbps > 0 {
					if maxBitrateKbps == 0 || *prof.MaxBitrateKbps < maxBitrateKbps {
						maxBitrateKbps = *prof.MaxBitrateKbps
					}
				}
			}
		}
		// The account cap (Admin -> Users -> Max Bitrate) too, as the served
		// plan applies it: otherwise this decision could promise a Direct Play
		// above the cap.
		if claims := interceptors.ClaimsFromContext(ctx); claims != nil && claims.UserID != "" {
			if limit, err := h.userQuery.GetUserBitrateLimit(ctx, claims.UserID); err == nil && limit > 0 {
				if maxBitrateKbps == 0 || limit < maxBitrateKbps {
					maxBitrateKbps = limit
				}
			}
		}
	}

	prefs.WasmAudio = req.WasmAudio

	if idx := int(req.AudioStreamIndex); idx > 0 {
		if rel, ok := typeRelativeIndex(streams, library.StreamTypeAudio, idx); ok {
			prefs.AudioTrack = rel
		}
	}
	if idx := int(req.SubtitleStreamIndex); idx > 0 {
		if rel, ok := typeRelativeIndex(streams, library.StreamTypeSubtitle, idx); ok {
			prefs.SubtitleBurnInTrack = rel
		}
	}

	// Operator-wide remote-access bitrate cap. Applied LAST so it can only LOWER
	// the client/profile ceiling, never raise it. Lands only on a client that is
	// positively PUBLIC (parseable, non-RFC1918) and classified WAN by the LAN
	// detector - so it never throttles an obviously-local viewer even if the
	// admin forgot to list a private range in LANNetworks. An unresolved IP fails
	// open (no cap), so an IP-plumbing hiccup can't wrongly throttle a LAN user.
	if h.remoteAccess != nil {
		if remoteCap, lan := h.remoteAccess(); remoteCap > 0 {
			if ip := net.ParseIP(interceptors.ClientIPFromCtx(ctx)); ip != nil &&
				!ip.IsPrivate() && network.NewDetector(lan).DetectIP(ip) == network.WAN {
				if maxBitrateKbps == 0 || remoteCap < maxBitrateKbps {
					maxBitrateKbps = remoteCap
				}
			}
		}
	}
	return prefs, maxBitrateKbps
}

func (h *playbackHandler) GetStreamInfo(ctx context.Context, req *eveyv1.GetStreamInfoRequest) (*eveyv1.StreamInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetStreamInfo")
	defer span.End()
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetStreamInfo: %w", err)
	}
	item, err := h.librarySvc.GetMediaItem(ctx, req.MediaItemId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.WarnContext(ctx, "grpc GetStreamInfo: media item query failed", slog.Any("err", err))
		return nil, status.Error(codes.NotFound, "media item not found")
	}
	// Mirror StartStream: a parental-blocked item must look non-existent. Without
	// this gate a kids profile could call GetStreamInfo by media ID and harvest a
	// blocked adult item's existence, codecs, and playback URLs even though the
	// stream bytes themselves are now 404'd downstream.
	if h.userQuery != nil {
		if claims := interceptors.ClaimsFromContext(ctx); claims != nil {
			if blocked, _ := h.isBlockedByParental(ctx, claims.ProfileID, req.MediaItemId); blocked {
				span.SetStatus(otelcodes.Error, "parental blocked")
				return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
			}
		}
	}
	containerFormat := ""
	if item.File != nil {
		containerFormat = item.File.ContainerFormat
	}
	resp := &eveyv1.StreamInfo{
		MediaItemId: req.MediaItemId,
		Container:   containerFormat,
	}
	streams, err := h.librarySvc.GetMediaStreams(ctx, req.MediaItemId)
	if err != nil {
		// Not recoverable here: every rule that could demote the optimistic
		// direct_play/copy/copy default iterates over these streams, so
		// carrying on with a nil list answers "playable as-is" about a file
		// nobody read. An item with no streams recorded returns an empty list
		// WITHOUT an error and keeps that optimistic answer, deliberately.
		return nil, internalError(ctx, "get media streams", err)
	}
	// On-demand keyframe fill: when the paced background backfill hasn't
	// indexed this file yet, try the bounded fast-path (container seek index,
	// milliseconds) right here so THIS playback's decision can already take
	// the copy rung instead of transcoding the whole viewing. Cue-less files
	// exceed the budget and stay on the worker's slow path - unchanged.
	if h.keyframeOnDemand != nil && item.File != nil && videoKeyframesMissing(streams) {
		if h.keyframeOnDemand.TryIndex(ctx, req.MediaItemId, item.File.Path) {
			if fresh, freshErr := h.librarySvc.GetMediaStreams(ctx, req.MediaItemId); freshErr == nil {
				streams = fresh
			}
		}
	}
	for _, s := range streams {
		resp.MediaStreams = append(resp.MediaStreams, mediaStreamToProto(s))
	}

	client := clientProfileFromRequest(req, interceptors.UserAgentFromCtx(ctx))
	prefs, maxBitrateKbps := h.resolvePlaybackPrefs(ctx, req, streams)

	preq := apptranscoding.PlaybackRequest{
		MediaID:             req.MediaItemId,
		Container:           containerFormat,
		Streams:             streams,
		Client:              client,
		Prefs:               prefs,
		MaxBitrateKbps:      maxBitrateKbps,
		AudioTrack:          prefs.AudioTrack,
		SubtitleBurnInTrack: prefs.SubtitleBurnInTrack,
		WasmAudio:           prefs.WasmAudio,
	}
	if item.File != nil {
		preq.SourcePath = item.File.Path
		preq.SourceBytes = item.File.Size
		// Same bitrate fallback the HLS plan applies, so the verdict here and the
		// master agree on a cap when the video stream carries no bitrate.
		preq.SourceBitrateKbps = item.File.BitrateBps() / 1000
		// A file stored in one-track blocks is refused everywhere; the decision
		// routes it to the HLS master (never the raw file) so every client
		// meets the typed 503 (file_needs_remux) it already knows how to show.
		preq.SourceNeedsRemux = item.File.NeedsRemux()
	}
	// The plan's decision, the one the master this info announces is served
	// from: the core plus the request-time pins and the route's tone-map
	// (Reported), never the bare core.
	decided := h.playbackDecider().Reported(ctx, preq)
	decision := &decided
	if apptranscoding.TranscodeDisallowed(decision, prefs) {
		err := status.Errorf(codes.FailedPrecondition, "transcoding disabled by user preferences")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	resp.TranscodeReasons = decision.TranscodeReasons
	resp.VideoDecision = decision.VideoDecision
	resp.AudioDecision = decision.AudioDecision
	// Target (served) codecs + resolution so the player can echo what's actually
	// served back onto the session for the admin panel. VideoCodec/AudioCodec are
	// only set by the decision when transcoding (empty == copy, i.e. source==served).
	resp.VideoCodec = decision.VideoCodec
	resp.AudioCodec = decision.AudioCodec
	resp.Resolution = servedResolution(decision, streams)

	switch decision.PlayMethod {
	case "direct_play":
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY
		resp.DirectPlayUrl = "/api/v1/media/" + req.MediaItemId + "/stream"
	case "direct_stream":
		// Container-only mismatch (e.g. h264/aac inside MKV a browser can't
		// demux): the bytes are copy-eligible but the wrapper isn't. Point the
		// client at the HLS master, NOT the raw /stream file - the VOD handler's
		// copy rung repackages to fMP4 segments with `-c:v copy` (no re-encode).
		// Returning DirectPlayUrl here byte-served the original container, which
		// the client then couldn't play (the direct_stream branch was a no-op).
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM
		resp.TranscodeUrl = "/api/v1/media/" + req.MediaItemId + "/hls/master.m3u8"
	default:
		resp.PlayMethod = eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE
		resp.TranscodeUrl = "/api/v1/media/" + req.MediaItemId + "/hls/master.m3u8"
	}

	// Carry the bitrate the client will actually pull, not the source bitrate.
	// resp.Bitrate was never populated, so the player's "total bitrate" row was
	// dead (0 is falsy). For a transcode the decision capped the video target
	// (and a default audio target); for direct play/stream nothing is re-encoded
	// so the source bitrate is what's served. A transcode that only copies video
	// (VideoBitrate == 0) also falls back to source - video dominates the total.
	resp.Bitrate = servedBitrateBps(decision.PlayMethod, decision.VideoBitrate, decision.AudioBitrate, sourceBitrateBps(item))
	resp.AudioBitrate = servedAudioBitrateBps(decision, streams)

	return resp, nil
}

// typeRelativeIndex converts an ABSOLUTE probed stream index (what the proto
// carries - MediaStream.StreamIndex as exposed to clients) into the
// type-relative ordinal the decision prefs consume (ffmpeg's 0:a:N / 0:s:N).
// Returns ok=false when no stream of that type has this absolute index.
func typeRelativeIndex(streams []*library.MediaStream, streamType library.StreamType, absIdx int) (int, bool) {
	rel := 0
	for _, s := range streams {
		if s == nil || s.StreamType != streamType {
			continue
		}
		if s.StreamIndex == absIdx {
			return rel, true
		}
		rel++
	}
	return 0, false
}

// sourceBitrateBps returns the media's source bitrate in bits/s, deriving it
// from size/duration when the probed bitrate is absent.
func sourceBitrateBps(item *library.MediaItem) int64 {
	if item == nil || item.File == nil {
		return 0
	}
	if bps := item.File.BitrateBps(); bps > 0 {
		return bps
	}
	return 0
}

// servedBitrateBps is the bitrate actually delivered to the client: the
// decision's capped video+audio targets for a transcode, the source bitrate for
// direct play/stream (nothing re-encoded) or a video-copy transcode (videoKbps
// == 0), where the source rate is what goes out.
func servedBitrateBps(playMethod string, videoKbps, audioKbps, sourceBps int64) int64 {
	if playMethod == "direct_play" || playMethod == "direct_stream" || videoKbps == 0 {
		return sourceBps
	}
	return (videoKbps + audioKbps) * 1000
}

// servedAudioBitrateBps is the audio bitrate actually delivered. Matroska rarely
// declares one per track, so the panel read "-" - yet on a transcode we choose the
// figure ourselves. Two paths, two bases: video copied -> the standalone rendition's
// base; video transcoded -> the ladder rung's. Both scaled by the same domain rule,
// so the panel quotes what the encoder is handed. A copied track keeps the source's
// rate; 0 = unknown, and the row hides.
func servedAudioBitrateBps(d *transcoding.PlaybackDecision, streams []*library.MediaStream) int64 {
	if d.AudioDecision != "transcode" {
		return sourceAudioBitrateBps(streams)
	}

	base := d.AudioBitrate
	if d.VideoDecision == "copy" {
		base = transcoding.AudioRenditionBaseKbps
	}
	if base <= 0 {
		return sourceAudioBitrateBps(streams)
	}
	return transcoding.ServedAudioBitrateKbps(base, servedAudioChannels(d, streams)) * 1000
}

// servedAudioChannels is the channel count the encoder will actually target: the
// source's own, capped by what the client accepts (a 7.1 source served as 5.1).
func servedAudioChannels(d *transcoding.PlaybackDecision, streams []*library.MediaStream) int {
	ch := 0
	for _, s := range streams {
		if s == nil || s.StreamType != library.Audio || s.Channels == nil {
			continue
		}
		ch = *s.Channels
		break
	}
	if d.MaxAudioChannels > 0 && (ch == 0 || ch > d.MaxAudioChannels) {
		return d.MaxAudioChannels
	}
	return ch
}

func sourceAudioBitrateBps(streams []*library.MediaStream) int64 {
	for _, s := range streams {
		if s == nil || s.StreamType != library.Audio {
			continue
		}
		if s.Bitrate != nil && *s.Bitrate > 0 {
			return *s.Bitrate
		}
	}
	return 0
}

// clampSessionField bounds a client-supplied descriptive session field (codec
// label, decision) so a misbehaving client can't stuff the play_sessions row.
func clampSessionField(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 48 {
		return s[:48]
	}
	return s
}

// servedResolution names the served frame the way every client does
// (library.ResolutionLabel): the source, scaled down to the transcode's height
// cap when there is one. A 1920x818 source is "1080p", not "818p".
func servedResolution(d *transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	w, h := 0, 0
	for _, s := range streams {
		if s.StreamType == library.StreamTypeVideo && s.Height != nil && *s.Height > 0 {
			h = *s.Height
			if s.Width != nil {
				w = *s.Width
			}
			break
		}
	}
	if d.MaxHeight > 0 && (h <= 0 || d.MaxHeight < h) {
		if h > 0 && w > 0 {
			w = w * d.MaxHeight / h
		}
		h = d.MaxHeight
	}
	return library.ResolutionLabel(w, h)
}

func (h *playbackHandler) StartStream(ctx context.Context, req *eveyv1.StartStreamRequest) (*eveyv1.PlaySession, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.StartStream")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StartStream: %w", err)
	}
	if h.userQuery != nil {
		if blocked, _ := h.isBlockedByParental(ctx, profileID, req.MediaItemId); blocked {
			// NotFound, not PermissionDenied: starting a stream by media ID must
			// not confirm to a kids profile that a parental-blocked adult item
			// exists. Matches the library/aggregate paths and the HTTP 404
			// decision ("hide existence"). The span keeps the real reason.
			span.SetStatus(otelcodes.Error, "parental blocked")
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
		}
	}

	// Resolve the item once: its source bitrate is the fallback bandwidth stamp
	// and its title lands in the audit entry below.
	var itemTitle string
	var sourceBps int64
	if item, itemErr := h.librarySvc.GetMediaItem(ctx, req.MediaItemId); itemErr == nil {
		itemTitle = item.Title
		sourceBps = sourceBitrateBps(item)
	}
	// Prefer the served bitrate the client reports (from the StreamInfo it just
	// fetched) so the dashboard shows what's actually streamed; fall back to the
	// source bitrate for clients that don't send it yet.
	bandwidthBps := sourceBps
	if req.ServedBitrateBps > 0 {
		bandwidthBps = req.ServedBitrateBps
	}

	session, err := playback.NewPlaySession(
		id.New("ps-"),
		shared.ProfileID(profileID),
		shared.MediaItemID(req.MediaItemId),
		playback.WithPlaySessionMethod(playback.PlayMethod(playMethodFromProto[req.PlayMethod])),
		playback.WithPlaySessionDevice(req.DeviceName, req.ClientName),
		playback.WithPlaySessionBandwidth(bandwidthBps),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	// Per-track serving detail the player copied from the StreamInfo it resolved.
	// Purely descriptive (admin "Now Playing" panel); clamp to keep a spoofing
	// client from stuffing the column.
	session.VideoDecision = clampSessionField(req.VideoDecision)
	session.AudioDecision = clampSessionField(req.AudioDecision)
	session.VideoCodec = clampSessionField(req.VideoCodec)
	session.AudioCodec = clampSessionField(req.AudioCodec)
	session.Resolution = clampSessionField(req.Resolution)
	if err := h.cmd.StartStream(ctx, session, shared.UserID(claims.UserID)); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if errors.Is(err, appplayback.ErrMaxStreams) {
			tErr := h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.playback.max_streams_reached")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if errors.Is(err, appplayback.ErrMaxDevicesPerProfile) {
			tErr := h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.playback.max_devices_reached")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		// Starting playback is the most user-visible failure a media server
		// has. Logged bare, it said only that "start stream" failed somewhere,
		// for someone, on something - unattributable, and unreachable from the
		// request id the caller was handed back.
		slog.ErrorContext(ctx, "start stream",
			slog.String("media_id", req.MediaItemId),
			slog.String("play_method", req.PlayMethod.String()),
			slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	details := map[string]any{"media_id": req.MediaItemId, "method": req.PlayMethod.String()}
	if itemTitle != "" {
		details["title"] = itemTitle
	}
	h.audit(ctx, claims.UserID, "start_stream", "media", &req.MediaItemId, details)
	return playSessionToProto(session), nil
}

func (h *playbackHandler) StopStream(ctx context.Context, req *eveyv1.StopStreamRequest) (*eveyv1.StopStreamResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.StopStream")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.StopStream: %w", err)
	}
	sess, sessErr := h.cmd.GetSession(ctx, req.SessionId)
	if !claims.IsAdmin {
		if sessErr != nil {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if string(sess.ProfileID) != claims.ProfileID {
			tErr := h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.cannot_stop_other_session")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
	}
	if err := h.cmd.StopStream(ctx, req.SessionId); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrSessionNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "stop stream", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	if sessErr == nil && sess != nil && h.hlsManager != nil {
		// Tear down the transcode session ONLY when this was the last active
		// stream for the media. Two tabs / two devices on the same film share one
		// transcode (identical DecisionHash); closing one must not kill the
		// encoder the other is still consuming. The command StopStream above has
		// already marked this session ended, so it's excluded from the active set.
		// Admin force-kill is a separate, forceful path (admin_audit.go) and is
		// unaffected by this gentle gate.
		if h.lastActiveStreamForMedia(ctx, string(sess.MediaItemID), req.SessionId) {
			h.hlsManager.RemoveSession(string(sess.MediaItemID))
		}
	}
	sid := req.SessionId
	h.audit(ctx, claims.UserID, "stop_stream", "session", &sid, nil)
	return &eveyv1.StopStreamResponse{}, nil
}

// lastActiveStreamForMedia reports whether NO other active playback session
// remains for mediaItemID (excludeSessionID is the session just stopped, guarded
// against in case its ended-at hasn't propagated). It exists so closing one of
// two tabs on the same film doesn't tear down the shared transcode the other tab
// is still consuming. On a missing/erroring active-session query it returns
// false - leaning toward NOT killing, since a spurious teardown interrupts a
// live viewer whereas an orphaned transcode is reclaimed by the producer's idle
// GC anyway.
func (h *playbackHandler) lastActiveStreamForMedia(ctx context.Context, mediaItemID, excludeSessionID string) bool {
	if h.query == nil {
		return false
	}
	active, err := h.query.GetActiveSessions(ctx)
	if err != nil {
		slog.WarnContext(ctx, "StopStream: GetActiveSessions failed; leaving transcode to idle GC",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	for _, s := range active {
		if s == nil || s.SessionID == excludeSessionID {
			continue
		}
		if string(s.MediaItemID) == mediaItemID {
			return false // another tab/device is still streaming this media
		}
	}
	return true
}

func (h *playbackHandler) ReportProgress(ctx context.Context, req *eveyv1.ReportProgressRequest) (*eveyv1.ReportProgressResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.ReportProgress")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.ReportProgress: %w", err)
	}
	if !claims.IsAdmin {
		session, err := h.cmd.GetSession(ctx, req.SessionId)
		if err != nil {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.playback.session_not_found")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		if string(session.ProfileID) != claims.ProfileID {
			tErr := h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.playback.cannot_report_other_session")
			span.RecordError(tErr)
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
	}
	// Consume remote command BEFORE updating the session (Update overwrites the
	// row). NOT while casting: the TV consumes its own commands via the alive
	// beacon poll, and the phone's 3s tick used to win that race and apply
	// "pause" to its muted LOCAL player - the admin's button then visibly did
	// nothing on the TV (live repro 2026-07-29).
	resp := &eveyv1.ReportProgressResponse{}
	if req.CastDeviceName == "" {
		if cmd, cmdErr := h.cmd.ConsumeRemoteCommand(ctx, req.SessionId); cmdErr == nil && cmd != "" {
			resp.RemoteCommand = cmd
		}
	}
	if resp.RemoteCommand == "" && claims != nil {
		resp.RemoteCommand = closingSoonMessage(ctx, h.hours, &h.closingWarns, h.translator, claims.ProfileID, req.SessionId, time.Now())
	}
	if err := h.cmd.ReportProgressBySession(ctx, req.SessionId, req.PositionMs, req.IsPaused, req.CastDeviceName); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "report progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return resp, nil
}

func (h *playbackHandler) GetWatchProgress(ctx context.Context, req *eveyv1.GetWatchProgressRequest) (*eveyv1.WatchProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetWatchProgress")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchProgress: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchProgress: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		return &eveyv1.WatchProgress{}, nil
	}
	wp, err := h.query.GetWatchProgress(ctx, profileID, req.MediaItemId)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return &eveyv1.WatchProgress{}, nil
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "get watch progress", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	pb := watchProgressToProto(wp)
	if mi, miErr := h.librarySvc.GetMediaItem(ctx, req.MediaItemId); miErr == nil {
		applyMediaDuration(pb, mi)
	}
	return pb, nil
}

// applyMediaDuration fills the two fields only the media item knows: the
// declared length, and the share of it the position represents. A progress
// row stores where the viewer stopped - how long the film is was never its to
// know, and leaving these at zero is how every client's history rendered
// "0:00 / 0:00" lines and empty progress bars.
func applyMediaDuration(pb *eveyv1.WatchProgress, mi *library.MediaItem) {
	if mi == nil || mi.File == nil {
		return
	}
	d := mi.File.Duration.Milliseconds()
	if d <= 0 {
		return
	}
	pb.DurationMs = d
	// Clamped: a position past the declared length (short-header files exist
	// in the wild) must not hand clients a bar wider than its container.
	pb.PercentComplete = min(float64(pb.PositionMs)/float64(d)*100, 100)
}

func (h *playbackHandler) GetWatchHistory(ctx context.Context, req *eveyv1.GetWatchHistoryRequest) (*eveyv1.GetWatchHistoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetWatchHistory")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchHistory: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetWatchHistory: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	} else if limit > maxPageSize {
		limit = maxPageSize
	}
	offset := 0
	var cursor *playback.WatchHistoryCursor
	if req.Pagination != nil {
		limit, offset = clampPagination(req.Pagination.PageSize, req.Pagination.Page, limit)
		if req.Pagination.Cursor != "" {
			c, err := decodeTimeCursor(req.Pagination.Cursor)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(otelcodes.Error, err.Error())
				slog.WarnContext(ctx, "grpc GetWatchHistory: invalid cursor", slog.Any("err", err))
				return nil, status.Error(codes.InvalidArgument, "invalid cursor")
			}
			cursor = &playback.WatchHistoryCursor{LastPlayedAt: c.t, MediaItemID: c.id}
		}
	}
	var items []*playback.WatchProgress
	if cursor != nil {
		items, err = h.query.GetWatchHistoryCursor(ctx, shared.ProfileID(profileID), cursor, limit)
	} else {
		items, err = h.query.GetWatchHistory(ctx, shared.ProfileID(profileID), limit, offset)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get watch history", err)
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	resp := &eveyv1.GetWatchHistoryResponse{Pagination: &eveyv1.PaginationResponse{}}
	resp.Items = h.enrichWatchHistory(ctx, items, allowedIDs)
	if len(items) > 0 && len(items) >= limit {
		last := items[len(items)-1]
		resp.Pagination.NextCursor = encodeTimeCursor(last.LastPlayedAt, string(last.MediaItemID))
	}
	return resp, nil
}

// enrichWatchHistory turns raw progress rows into display-ready items: it
// batch-resolves each media item (title, type, episode number) plus, for
// episodes, the parent season (season number) and grandparent series (title),
// so clients render a real label instead of the bare media_item_id. It also
// enforces library access: a restricted user only sees items in an allowed
// library, and an item whose media no longer exists is hidden from them.
// Best-effort - a failed lookup just leaves the affected fields empty.
func (h *playbackHandler) enrichWatchHistory(ctx context.Context, items []*playback.WatchProgress, allowedIDs []string) []*eveyv1.WatchProgress {
	ids := make([]string, 0, len(items))
	for _, wp := range items {
		ids = append(ids, string(wp.MediaItemID))
	}
	byID := h.mediaItemMap(ctx, ids)
	seasonByID, seriesByID := h.episodeAncestors(ctx, byID)

	out := make([]*eveyv1.WatchProgress, 0, len(items))
	for _, wp := range items {
		mi := byID[string(wp.MediaItemID)]
		if allowedIDs != nil {
			// Restricted user: skip items outside an allowed library, and skip
			// unresolved (deleted) media since access can't be verified.
			if mi == nil || !slices.Contains(allowedIDs, string(mi.LibraryID)) {
				continue
			}
		}
		pb := watchProgressToProto(wp)
		applyMediaDuration(pb, mi)
		stampProgressDisplay(pb, mi, seasonByID, seriesByID)
		out = append(out, pb)
	}
	return out
}

// mediaItemMap batch-resolves media items by ID into a lookup map. Best-effort:
// a fetch error logs and yields an empty map rather than failing the caller.
func (h *playbackHandler) mediaItemMap(ctx context.Context, ids []string) map[string]*library.MediaItem {
	return fetchItemMap(ctx, h.librarySvc.GetMediaItemsByIDs, ids)
}

// episodeAncestors resolves the season and series levels above the given
// items, one batch fetch each.
func (h *playbackHandler) episodeAncestors(ctx context.Context, byID map[string]*library.MediaItem) (seasonByID, seriesByID map[string]*library.MediaItem) {
	seasonIDs := make([]string, 0)
	for _, mi := range byID {
		if mi.Type == library.Episode && mi.ParentID != "" {
			seasonIDs = append(seasonIDs, mi.ParentID)
		}
	}
	seasonByID = h.mediaItemMap(ctx, seasonIDs)
	seriesIDs := make([]string, 0)
	for _, s := range seasonByID {
		if s.ParentID != "" {
			seriesIDs = append(seriesIDs, s.ParentID)
		}
	}
	seriesByID = h.mediaItemMap(ctx, seriesIDs)
	return seasonByID, seriesByID
}

// stampProgressDisplay copies the display metadata clients render instead of
// the bare media_item_id: title, type, episode number, plus season number and
// series title for episodes.
func stampProgressDisplay(pb *eveyv1.WatchProgress, mi *library.MediaItem, seasonByID, seriesByID map[string]*library.MediaItem) {
	if mi == nil {
		return
	}
	pb.Title = mi.Title
	pb.MediaType = string(mi.Type)
	if mi.IndexNumber != nil {
		pb.IndexNumber = int32(*mi.IndexNumber)
	}
	if mi.Type == library.Episode {
		if season := seasonByID[mi.ParentID]; season != nil {
			if season.IndexNumber != nil {
				pb.ParentIndexNumber = int32(*season.IndexNumber)
			}
			if series := seriesByID[season.ParentID]; series != nil {
				pb.SeriesTitle = series.Title
			}
		}
	}
}

func (h *playbackHandler) GetResumeItems(ctx context.Context, req *eveyv1.GetResumeItemsRequest) (*eveyv1.GetResumeItemsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetResumeItems")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetResumeItems: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetResumeItems: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	} else if limit > maxPageSize {
		limit = maxPageSize
	}
	items, err := h.query.GetResumeItems(ctx, shared.ProfileID(profileID), limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "get resume items", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	// Parental gate alongside the library-access gate: a kids profile must not
	// see adult resume entries it played before the profile was switched to
	// kids. A kid can have allowedIDs==nil (no library restriction) yet still
	// need the parental filter, so we fetch+filter the media items whenever
	// EITHER gate is active. Mirrors the feed handlers.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	parentalActive := pf.maxAge >= 0 || len(pf.blockedTags) > 0
	resp := &eveyv1.GetResumeItemsResponse{}
	// The media items are fetched unconditionally now: the gates need them
	// when active, and the declared duration comes from them always.
	ids := make([]string, len(items))
	for i, wp := range items {
		ids[i] = string(wp.MediaItemID)
	}
	miByID := h.mediaItemMap(ctx, ids)
	seasonByID, seriesByID := h.episodeAncestors(ctx, miByID)
	for _, wp := range items {
		mi := miByID[string(wp.MediaItemID)]
		if allowedIDs != nil || parentalActive {
			// Gated profile: an unresolvable item stays hidden (access can't
			// be verified), exactly as before.
			if mi == nil {
				continue
			}
			if allowedIDs != nil && !slices.Contains(allowedIDs, string(mi.LibraryID)) {
				continue
			}
			if mediaItemBlockedByParental(pf, mi) {
				continue
			}
		}
		pb := watchProgressToProto(wp)
		applyMediaDuration(pb, mi)
		stampProgressDisplay(pb, mi, seasonByID, seriesByID)
		resp.Progress = append(resp.Progress, pb)
		// The declared response shape has always carried the items alongside
		// the progress rows, but nothing filled them: the TV home row reads
		// resp.items and rendered an empty shelf. Deleted media keeps its
		// progress row without an item - clients match the two by media id.
		if mi != nil {
			resp.Items = append(resp.Items, mediaItemToProto(mi, ctxIsAdmin(ctx)))
		}
	}
	// Reuse the season/series maps already resolved for stampProgressDisplay
	// instead of re-fetching them through fillAncestorTitles.
	stampAncestorTitles(resp.Items, seasonByID, seriesByID)
	fillPrimaryBlurhashes(ctx, h.metadataSvc, resp.Items)
	return resp, nil
}

func (h *playbackHandler) GetNextUp(ctx context.Context, req *eveyv1.GetNextUpRequest) (*eveyv1.GetNextUpResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.GetNextUp")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetNextUp: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.GetNextUp: %w", err)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	} else if limit > maxPageSize {
		limit = maxPageSize
	}
	progresses, err := h.query.GetNextUp(ctx, shared.ProfileID(profileID), limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get next up", err)
	}
	ids := make([]string, len(progresses))
	for i, wp := range progresses {
		ids[i] = string(wp.MediaItemID)
	}
	mediaItems, err := h.librarySvc.GetMediaItemsByIDs(ctx, ids)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "get media items for next up", err)
	}
	allowedIDs, _ := h.getAllowedLibraryIDs(ctx)
	// Parental gate: GetNextUp returns media-item protos (title/overview), so a
	// kids profile must not see adult items it previously played before the
	// profile was switched to kids. Mirrors the feed handlers.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	byID := make(map[string]*library.MediaItem, len(mediaItems))
	for _, mi := range mediaItems {
		if allowedIDs != nil && !slices.Contains(allowedIDs, string(mi.LibraryID)) {
			continue
		}
		if mediaItemBlockedByParental(pf, mi) {
			continue
		}
		byID[string(mi.ID)] = mi
	}
	resp := &eveyv1.GetNextUpResponse{}
	for _, wp := range progresses {
		if item, ok := byID[string(wp.MediaItemID)]; ok {
			resp.Items = append(resp.Items, mediaItemToProto(item, ctxIsAdmin(ctx)))
		}
	}
	fillAncestorTitles(ctx, h.librarySvc.GetMediaItemsByIDs, resp.Items)
	fillPrimaryBlurhashes(ctx, h.metadataSvc, resp.Items)
	return resp, nil
}

func (h *playbackHandler) MarkAsWatched(ctx context.Context, req *eveyv1.MarkAsWatchedRequest) (*eveyv1.MarkAsWatchedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.MarkAsWatched")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsWatched: %w", err)
	}
	if err := h.cmd.MarkAsWatched(ctx, profileID, req.MediaItemId); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "mark as watched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsWatchedResponse{}, nil
}

func (h *playbackHandler) MarkAsUnwatched(ctx context.Context, req *eveyv1.MarkAsUnwatchedRequest) (*eveyv1.MarkAsUnwatchedResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.MarkAsUnwatched")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	if err := h.requireMediaAccess(ctx, req.MediaItemId); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.MarkAsUnwatched: %w", err)
	}
	if err := h.cmd.MarkAsUnwatched(ctx, profileID, req.MediaItemId); err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "mark as unwatched", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.MarkAsUnwatchedResponse{}, nil
}

func (h *playbackHandler) ClearWatchHistory(ctx context.Context, req *eveyv1.ClearWatchHistoryRequest) (*eveyv1.ClearWatchHistoryResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.playback.ClearWatchHistory")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.ClearWatchHistory: %w", err)
	}
	// resolveProfileID pins the call to the caller's own profile (admins too),
	// so nobody clears another profile's history through this RPC.
	profileID, err := resolveProfileID(claims, req.ProfileId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("playbackHandler.ClearWatchHistory: %w", err)
	}
	deleted, err := h.cmd.ClearWatchHistory(ctx, shared.ProfileID(profileID))
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, appplayback.ErrProfileNotFound) {
			tErr := h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
			span.SetStatus(otelcodes.Error, tErr.Error())
			return nil, tErr
		}
		span.SetStatus(otelcodes.Error, err.Error())
		slog.ErrorContext(ctx, "clear watch history", slog.Any("err", err))
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.ClearWatchHistoryResponse{Deleted: int32(deleted)}, nil
}

func (h *playbackHandler) requireMediaAccess(ctx context.Context, mediaItemID string) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return status.Errorf(codes.Unauthenticated, "unauthenticated")
	}
	if claims.IsAdmin {
		return nil
	}
	if h.userAccess == nil {
		return errUserAccessNotConfigured(ctx, "playbackHandler.requireMediaAccess")
	}
	item, err := h.librarySvc.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	ok, accErr := h.userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accErr != nil {
		// Fail closed, but say why. Discarded, a database outage is
		// indistinguishable from a legitimate denial: everyone is refused and
		// nothing explains it.
		slog.ErrorContext(ctx, "playback: library access check failed",
			slog.String("user_id", claims.UserID),
			slog.String("media_item_id", string(item.ID)),
			slog.Any("err", accErr))
	}
	if !ok {
		return h.translator.TranslatedError(ctx, codes.NotFound, "errors.media.not_found")
	}
	return nil
}

func (h *playbackHandler) getAllowedLibraryIDs(ctx context.Context) ([]string, error) {
	// Admin bypass is a claims check, independent of whether userAccess is
	// wired, so it is resolved before the wiring check below - an unwired
	// port must never turn an admin request into a denial.
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return nil, nil
	}
	if h.userAccess == nil {
		// nil reads as "unrestricted" below; a wiring regression must deny
		// instead, like a real zero-access user would get.
		return []string{}, nil
	}
	ids, err := h.userAccess.GetLibraryAccessByUserID(ctx, claims.UserID)
	if err != nil {
		// Deny-shaped, NOT nil. nil reads as "unrestricted" everywhere below, so
		// returning it here made a grant-store outage WIDEN what the caller sees -
		// the opposite of the unwired-port branch above, which already denies on
		// purpose. Callers that discard this error (three of them did) must still
		// end up denying.
		slog.ErrorContext(ctx, "playback: library grant lookup failed, denying",
			slog.String("user_id", claims.UserID), slog.Any("err", err))
		return []string{}, err
	}
	if len(ids) == 0 {
		return nil, nil // zero grants = unrestricted (opt-out default)
	}
	return ids, nil
}

// normalizeNetworkType accepts "wifi" or "cellular" and defaults unknown/empty to "wifi".
func normalizeNetworkType(raw string) string {
	if raw == "cellular" {
		return "cellular"
	}
	return "wifi"
}

func (h *playbackHandler) isBlockedByParental(ctx context.Context, _ string, mediaItemID string) (bool, string) {
	// Delegate to the ONE resolver the browse/feed paths use instead of
	// rebuilding the filter here. This used to be a hand-copied pipeline
	// (profile lookup, MaxAge, region.Resolve, allowedRatings), and the copy
	// silently missed the admin exemption: b3ad4ac43 added it to
	// resolveParentalFilterFromUserQuery and never crossed over, so an admin
	// whose profile carries IsKids=true could browse the library but was
	// refused playback of everything in it.
	//
	// The resolver reads the caller's claims from ctx; both call sites pass the
	// caller's own profile (resolveProfileID refuses any other), so nothing
	// changes for a normal profile. It stays fail-closed on a failed profile
	// lookup, and keeps using the worst-case mediaItemBlockedByParental rather
	// than the permissive single-country check - otherwise a title hidden from
	// browse could still be played directly by media_item_id.
	pf := resolveParentalFilterFromUserQuery(ctx, h.userQuery)
	if !pf.enforced() {
		return false, ""
	}
	item, err := h.librarySvc.GetMediaItem(ctx, mediaItemID)
	if err != nil {
		// Parental enforcement is active for this profile and we cannot rate
		// the item - fail closed rather than play unrated content.
		return true, "parental status unavailable"
	}
	if mediaItemBlockedByParental(pf, item) {
		return true, "content blocked by parental controls"
	}
	return false, ""
}
