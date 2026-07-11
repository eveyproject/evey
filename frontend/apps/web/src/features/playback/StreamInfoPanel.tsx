import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { V1StreamInfo, V1MediaStream } from "@evey/api";

function formatBitrate(bitrate?: string): string {
  if (!bitrate) return "-";
  const bps = Number(bitrate);
  if (!Number.isFinite(bps) || bps <= 0) return "-";
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  if (bps >= 1_000) return `${Math.round(bps / 1_000)} Kbps`;
  return `${bps} bps`;
}

function formatPlayMethod(method?: string): string {
  switch (method) {
    case "PLAY_METHOD_DIRECT_PLAY":
      return "Direct Play";
    case "PLAY_METHOD_DIRECT_STREAM":
      return "Direct Stream";
    case "PLAY_METHOD_TRANSCODE":
      return "Transcode";
    default:
      return method ?? "-";
  }
}

function formatChannels(ch?: number): string {
  if (!ch) return "";
  if (ch === 1) return "Mono";
  if (ch === 2) return "Stereo";
  if (ch === 6) return "5.1";
  if (ch === 8) return "7.1";
  return `${ch}ch`;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-1.5 border-b border-[var(--border)] last:border-b-0">
      <span className="text-xs text-[var(--text-muted)]">{label}</span>
      <span className="text-xs font-medium text-[var(--text)] text-right truncate">{value}</span>
    </div>
  );
}

function Section({ title }: { title: string }) {
  return (
    <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider text-[var(--accent)]">
      {title}
    </div>
  );
}

interface StreamInfoPanelProps {
  streamInfo?: V1StreamInfo;
  open: boolean;
  onToggle: () => void;
}

// StreamInfoPanel surfaces the playback decision the backend already returns
// (play method, codecs, transcode reasons) - visibility the mobile app has but the
// web player lacked. Mirrors the SettingsPanel popover chrome.
export function StreamInfoPanel({ streamInfo, open, onToggle }: StreamInfoPanelProps) {
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
          <InfoRow label={t("player.streamInfo.method")} value={formatPlayMethod(streamInfo.playMethod)} />
          {streamInfo.container && (
            <InfoRow label={t("player.streamInfo.container")} value={streamInfo.container.toUpperCase()} />
          )}
          {streamInfo.bitrate && (
            <InfoRow label={t("player.streamInfo.totalBitrate")} value={formatBitrate(streamInfo.bitrate)} />
          )}

          {videoStream && (
            <>
              <Section title={t("player.streamInfo.video")} />
              <InfoRow label={t("player.streamInfo.codec")} value={(videoStream.codec ?? "-").toUpperCase()} />
              {videoStream.width && videoStream.height && (
                <InfoRow label={t("player.streamInfo.resolution")} value={`${videoStream.width}×${videoStream.height}`} />
              )}
              {videoStream.bitrate && (
                <InfoRow label={t("player.streamInfo.bitrate")} value={formatBitrate(videoStream.bitrate)} />
              )}
              {videoStream.frameRate && (
                <InfoRow label={t("player.streamInfo.framerate")} value={`${videoStream.frameRate} fps`} />
              )}
              {videoStream.profile && (
                <InfoRow label={t("player.streamInfo.profile")} value={videoStream.profile} />
              )}
              {videoStream.pixelFormat && (
                <InfoRow label={t("player.streamInfo.pixelFormat")} value={videoStream.pixelFormat} />
              )}
              {streamInfo.videoDecision && (
                <InfoRow label={t("player.streamInfo.decision")} value={streamInfo.videoDecision} />
              )}
            </>
          )}

          {audioStream && (
            <>
              <Section title={t("player.streamInfo.audio")} />
              <InfoRow label={t("player.streamInfo.codec")} value={(audioStream.codec ?? "-").toUpperCase()} />
              {audioStream.channels && (
                <InfoRow label={t("player.streamInfo.channels")} value={formatChannels(audioStream.channels)} />
              )}
              {audioStream.language && (
                <InfoRow label={t("player.streamInfo.language")} value={audioStream.language} />
              )}
              {audioStream.bitrate && (
                <InfoRow label={t("player.streamInfo.bitrate")} value={formatBitrate(audioStream.bitrate)} />
              )}
              {audioStream.sampleRate && (
                <InfoRow label={t("player.streamInfo.sampleRate")} value={`${audioStream.sampleRate} Hz`} />
              )}
              {streamInfo.audioDecision && (
                <InfoRow label={t("player.streamInfo.decision")} value={streamInfo.audioDecision} />
              )}
            </>
          )}

          {streamInfo.transcodeReasons && streamInfo.transcodeReasons.length > 0 && (
            <>
              <Section title={t("player.streamInfo.transcodeReasons")} />
              <div className="px-3 py-1.5 text-xs text-[var(--text-muted)]">
                {streamInfo.transcodeReasons.join(", ")}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
