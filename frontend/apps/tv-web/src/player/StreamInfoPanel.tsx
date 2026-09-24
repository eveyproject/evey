import { useTranslation } from "react-i18next";
import type { V1StreamInfo } from "@evey/api";
import { buildStreamInfoModel, decisionBadgeKey } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button } from "../components/ui";

function Row({ label, value }: { label: string; value: string }) {
  return <div className="info-row"><span className="muted">{label}</span><span>{value}</span></div>;
}

/** What is actually being served: the method, the codecs, and why it is transcoding. */
export function StreamInfoPanel({ streamInfo, audioStreamIndex, onClose }: {
  streamInfo: V1StreamInfo | null;
  audioStreamIndex: number | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const model = buildStreamInfoModel(streamInfo, { audio: { absolute: audioStreamIndex } });
  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus className="modal-sheet">
        <h2 className="row-title">{t("player.streamInfo.title")}</h2>
        {model?.sections.map((section) => {
          const badge = section.decision ? decisionBadgeKey(section.decision) : null;
          return (
            <div key={section.kind}>
              {section.kind !== "playback" ? (
                <h3 className="row-title">{t(section.titleKey, section.title)}{badge ? ` · ${t(badge.key, badge.defaultValue)}` : ""}</h3>
              ) : null}
              {section.rows.map((row) => (
                <Row key={row.labelKey} label={t(row.labelKey, row.label)} value={row.valueKey ? t(row.valueKey) : (row.value ?? "")} />
              ))}
            </div>
          );
        })}
        {model && model.reasons.length > 0 ? (
          <>
            <h3 className="row-title">{t(model.reasonsHeadingKey)}</h3>
            {model.reasons.map((r) => <p key={r} className="muted">· {t(`player.streamInfo.reason.${r}`, r.replace(/_/g, " "))}</p>)}
          </>
        ) : null}
        <div className="actions">
          <Button variant="primary" onPress={onClose}>{t("common.close")}</Button>
        </div>
      </FocusRoot>
    </div>
  );
}
