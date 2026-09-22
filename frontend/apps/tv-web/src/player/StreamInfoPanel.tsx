import { useTranslation } from "react-i18next";
import {
  effectivePlayMethod,
  reasonsHeadingI18nKey,
  formatChannelLayout,
  pickAudioStream,
  playMethodI18nKey,
  visibleTranscodeReasons,
  type V1MediaStream,
  type V1StreamInfo,
} from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Button } from "../components/ui";

function bitrate(bps?: number | string): string {
  const n = typeof bps === "string" ? Number(bps) : bps ?? 0;
  if (!n) return "";
  return n >= 1_000_000 ? `${(n / 1_000_000).toFixed(1)} Mb/s` : `${Math.round(n / 1000)} kb/s`;
}

function Row({ label, value }: { label: string; value?: string | number }) {
  if (value === undefined || value === "" || value === 0) return null;
  return <div className="info-row"><span className="muted">{label}</span><span>{value}</span></div>;
}

/** What is actually being served: the method, the codecs, and why it is transcoding. */
export function StreamInfoPanel({ streamInfo, audioStreamIndex, onClose }: {
  streamInfo: V1StreamInfo | null;
  audioStreamIndex: number | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const video = streamInfo?.mediaStreams?.find((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const audio = pickAudioStream(streamInfo?.mediaStreams, { absolute: audioStreamIndex ?? undefined });
  const methodKey = playMethodI18nKey(effectivePlayMethod({
    playMethod: streamInfo?.playMethod,
    videoDecision: streamInfo?.videoDecision,
    audioDecision: streamInfo?.audioDecision,
  }));
  const reasons = visibleTranscodeReasons(streamInfo?.transcodeReasons);
  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus className="modal-sheet">
        <h2 className="row-title">{t("player.streamInfo.title")}</h2>
        <Row label={t("player.streamInfo.method", "Method")} value={methodKey ? t(methodKey) : ""} />
        <Row label={t("player.streamInfo.containerSource", "Source container")} value={streamInfo?.container?.toUpperCase()} />
        <Row label={t("player.streamInfo.totalBitrate", "Total Bitrate")} value={bitrate(streamInfo?.bitrate)} />
        {video ? (
          <>
            <h3 className="row-title">{t("player.streamInfo.video", "Video")}{streamInfo?.videoDecision ? ` · ${streamInfo.videoDecision}` : ""}</h3>
            <Row label={t("player.streamInfo.codec", "Codec")} value={video.codec?.toUpperCase()} />
            <Row label={t("player.streamInfo.resolution", "Resolution")} value={video.width && video.height ? `${video.width}×${video.height}` : ""} />
            <Row label={t("player.streamInfo.bitrate", "Bitrate")} value={bitrate(video.bitrate)} />
          </>
        ) : null}
        {audio ? (
          <>
            <h3 className="row-title">{t("player.streamInfo.audio", "Audio")}{streamInfo?.audioDecision ? ` · ${streamInfo.audioDecision}` : ""}</h3>
            <Row label={t("player.streamInfo.codec", "Codec")} value={audio.codec?.toUpperCase()} />
            <Row label={t("player.streamInfo.channels", "Channels")} value={formatChannelLayout(audio.channels)} />
            <Row label={t("player.streamInfo.language", "Language")} value={audio.language} />
          </>
        ) : null}
        {reasons.length > 0 ? (
          <>
            <h3 className="row-title">{t(reasonsHeadingI18nKey({ playMethod: streamInfo?.playMethod, videoDecision: streamInfo?.videoDecision, audioDecision: streamInfo?.audioDecision }))}</h3>
            {reasons.map((r) => <p key={r} className="muted">· {t(`player.streamInfo.reason.${r}`, r.replace(/_/g, " "))}</p>)}
          </>
        ) : null}
        <div className="actions">
          <Button variant="primary" onPress={onClose}>{t("common.close")}</Button>
        </div>
      </FocusRoot>
    </div>
  );
}
