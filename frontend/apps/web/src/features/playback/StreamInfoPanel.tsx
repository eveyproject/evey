import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  formatChannelLayout,
  effectivePlayMethod,
  playMethodI18nKey,
  reasonsHeadingI18nKey,
  visibleTranscodeReasons,
  type V1StreamInfo,
  type V1MediaStream,
} from "@evey/api";

function formatBitrate(bitrate?: string): string {
  if (!bitrate) return "-";
  const bps = Number(bitrate);
  if (!Number.isFinite(bps) || bps <= 0) return "-";
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  if (bps >= 1_000) return `${Math.round(bps / 1_000)} Kbps`;
  return `${bps} bps`;
}

// 23.976023976023978 is a true value and a useless one to read. Show what a human
// says: 23.976, 24, 25, 30.
function formatFrameRate(fps?: number): string | null {
  if (!fps || !Number.isFinite(fps) || fps <= 0) return null;
  const rounded = Math.round(fps * 1000) / 1000;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(3)} fps`;
}

// Per-stream bitrate is often absent from the probe (the container only carries a
// total). Deriving video ≈ total − audio beats showing a bare "-": it is the number
// the viewer actually wants, and it is honest about being an estimate.
function deriveVideoBitrate(total?: string, audio?: string): { value: string; estimated: boolean } | null {
  const t = Number(total);
  if (!Number.isFinite(t) || t <= 0) return null;
  const a = Number(audio);
  if (Number.isFinite(a) && a > 0 && a < t) {
    return { value: formatBitrate(String(t - a)), estimated: true };
  }
  return { value: formatBitrate(String(t)), estimated: true };
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-1.5 border-b border-[var(--border)] last:border-b-0">
      <span className="text-xs text-[var(--text-muted)]">{label}</span>
      <span className="text-xs font-medium text-[var(--text)] text-right truncate">{value}</span>
    </div>
  );
}

// The decision belongs to its stream, not to a lonely row at the bottom of the
// section - you could not tell at a glance whether "transcode" applied to the video
// or the audio. It rides the section header now, colour-coded: copied = untouched
// (good), transcoded = re-encoded (costly).
function DecisionBadge({ decision }: { decision?: string }) {
  const { t } = useTranslation();
  if (!decision) return null;
  const isCopy = decision === "copy";
  // "wasm": the AC-3/E-AC-3 audio is NOT transcoded - it is served untouched and
  // decoded in the browser (ac3go WebAssembly). Distinct accent badge so the
  // panel does not mislabel it "transcodé".
  const isWasm = decision === "wasm";
  return (
    <span
      className="ml-2 px-1.5 py-0.5 rounded-[var(--radius-sm)] text-xs font-semibold uppercase tracking-normal"
      style={{
        backgroundColor: isWasm ? "var(--accent)" : isCopy ? "var(--success)" : "var(--warning)",
        color: "var(--bg)",
        opacity: 0.9,
      }}
    >
      {isWasm
        ? t("player.streamInfo.decisionWasm", { defaultValue: "décodé client" })
        : isCopy
          ? t("player.streamInfo.decisionCopy", { defaultValue: "copié" })
          : t("player.streamInfo.decisionTranscode", { defaultValue: "transcodé" })}
    </span>
  );
}

function Section({ title, decision }: { title: string; decision?: string }) {
  return (
    <div className="flex items-center px-3 pt-2.5 pb-1 text-xs font-semibold uppercase tracking-wider text-[var(--accent)]">
      <span>{title}</span>
      <DecisionBadge decision={decision} />
    </div>
  );
}

interface StreamInfoPanelProps {
  streamInfo?: V1StreamInfo;
  open: boolean;
  onToggle: () => void;
  // wasmAudioActive: the audio is decoded client-side (ac3go WebAssembly), not
  // transcoded server-side, so the audio section shows a "décodé client" badge.
  wasmAudioActive?: boolean;
}

// StreamInfoPanel surfaces the playback decision the backend already returns
// (play method, codecs, transcode reasons) - visibility the mobile app has but the
// web player lacked. Mirrors the SettingsPanel popover chrome.
export function StreamInfoPanel({ streamInfo, open, onToggle, wasmAudioActive }: StreamInfoPanelProps) {
  const { t } = useTranslation();

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

  const videoStream = streamInfo?.mediaStreams?.find(
    (s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO",
  );
  const audioStream = streamInfo?.mediaStreams?.find(
    (s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_AUDIO",
  );

  const decisions = {
    playMethod: streamInfo?.playMethod,
    videoDecision: streamInfo?.videoDecision,
    audioDecision: streamInfo?.audioDecision,
    wasmAudio: wasmAudioActive,
  };
  const methodKey = playMethodI18nKey(effectivePlayMethod(decisions));

  return (
    <div className="relative">
      <button
        onClick={toggle}
        aria-label={t("player.streamInfo.title")}
        aria-expanded={open}
        aria-haspopup="true"
        title={t("player.streamInfo.title")}
        className="text-white hover:text-[var(--accent)] transition-colors-fast p-1"
      >
        <Info size={20} aria-hidden="true" />
      </button>

      {open && streamInfo && (
        <div
          className="absolute bottom-full mb-2 right-0 w-64 rounded-[var(--radius-md)] shadow-xl overflow-y-auto z-badge max-h-[80vh] bg-[var(--surface)] border border-[var(--border)]"
          onClick={(e) => e.stopPropagation()}
        >
          <Section title={t("player.streamInfo.playback")} />
          {methodKey && <InfoRow label={t("player.streamInfo.method")} value={t(methodKey)} />}
          {/* Source, not served: anything but direct play repackages to fMP4. */}
          {streamInfo.container && (
            <InfoRow label={t("player.streamInfo.containerSource")} value={streamInfo.container.toUpperCase()} />
          )}
          {streamInfo.bitrate && (
            <InfoRow label={t("player.streamInfo.totalBitrate")} value={formatBitrate(streamInfo.bitrate)} />
          )}

          {videoStream && (
            <>
              <Section title={t("player.streamInfo.video")} decision={streamInfo.videoDecision} />
              <InfoRow label={t("player.streamInfo.codec")} value={(videoStream.codec ?? "-").toUpperCase()} />
              {videoStream.width && videoStream.height && (
                <InfoRow label={t("player.streamInfo.resolution")} value={`${videoStream.width}×${videoStream.height}`} />
              )}
              {(() => {
                const explicit = videoStream.bitrate ? formatBitrate(videoStream.bitrate) : null;
                if (explicit && explicit !== "-") {
                  return <InfoRow label={t("player.streamInfo.bitrate")} value={explicit} />;
                }
                const derived = deriveVideoBitrate(streamInfo.bitrate, audioStream?.bitrate);
                return derived ? (
                  <InfoRow label={t("player.streamInfo.bitrate")} value={`≈ ${derived.value}`} />
                ) : null;
              })()}
              {formatFrameRate(videoStream.frameRate) && (
                <InfoRow label={t("player.streamInfo.framerate")} value={formatFrameRate(videoStream.frameRate)!} />
              )}
              {videoStream.profile && (
                <InfoRow label={t("player.streamInfo.profile")} value={videoStream.profile} />
              )}
              {videoStream.pixelFormat && (
                <InfoRow label={t("player.streamInfo.pixelFormat")} value={videoStream.pixelFormat} />
              )}
            </>
          )}

          {audioStream && (
            <>
              <Section title={t("player.streamInfo.audio")} decision={wasmAudioActive ? "wasm" : streamInfo.audioDecision} />
              <InfoRow label={t("player.streamInfo.codec")} value={(audioStream.codec ?? "-").toUpperCase()} />
              {audioStream.channels && (
                <InfoRow label={t("player.streamInfo.channels")} value={formatChannelLayout(audioStream.channels)} />
              )}
              {audioStream.language && (
                <InfoRow label={t("player.streamInfo.language")} value={audioStream.language} />
              )}
              {/* The SERVED rate, not the source's: a Matroska track rarely declares
                  one, so this row used to sit at "-" on every re-encoded track even
                  though we picked the figure ourselves. Falls back to the source when
                  the track is copied. */}
              {(streamInfo.audioBitrate ?? audioStream.bitrate) && (
                <InfoRow
                  label={t("player.streamInfo.bitrate")}
                  value={formatBitrate(streamInfo.audioBitrate ?? audioStream.bitrate)}
                />
              )}
              {audioStream.sampleRate && (
                <InfoRow label={t("player.streamInfo.sampleRate")} value={`${audioStream.sampleRate} Hz`} />
              )}
            </>
          )}

          {(() => {
            const reasons = visibleTranscodeReasons(streamInfo.transcodeReasons, wasmAudioActive);
            return reasons.length > 0 ? (
            <>
              <Section title={t(reasonsHeadingI18nKey(decisions))} />
              <ul className="px-3 py-1.5 space-y-1">
                {reasons.map((reason) => (
                  <li key={reason} className="flex gap-1.5 text-xs text-[var(--text-muted)]">
                    <span aria-hidden className="text-[var(--warning)]">•</span>
                    {/* Raw reasons are snake_case enum values: translate, and fall back to
                        a readable form rather than leaking `hdr_tonemapping_needed`. */}
                    <span>
                      {t(`player.streamInfo.reason.${reason}`, {
                        defaultValue: reason.replace(/_/g, " "),
                      })}
                    </span>
                  </li>
                ))}
              </ul>
            </>
            ) : null;
          })()}
        </div>
      )}
    </div>
  );
}
