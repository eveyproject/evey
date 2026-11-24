import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { V1PlayedState, V1ServedLeg, V1StreamInfo } from "@evey/api";
import { buildStreamInfoModel, decisionBadgeKey, type ServedVariant } from "@evey/client";

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
  const isCopy = decision === "copy" || decision === "direct";
  // "wasm": the AC-3/E-AC-3 audio is NOT transcoded - it is served untouched and
  // decoded in the browser (ac3go WebAssembly). Distinct accent badge so the
  // panel does not mislabel it "transcodé".
  const isWasm = decision === "wasm";
  const badge = decisionBadgeKey(decision);
  return (
    <span
      className="ml-2 px-1.5 py-0.5 rounded-[var(--radius-sm)] text-xs font-semibold uppercase tracking-normal"
      style={{
        backgroundColor: isWasm ? "var(--accent)" : isCopy ? "var(--success)" : "var(--warning)",
        color: "var(--bg)",
        opacity: 0.9,
      }}
    >
      {t(badge.key, { defaultValue: badge.defaultValue })}
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
  // The session's two witnesses: what the server sent (the progress tick's
  // answer) and what this player decodes. What the panel says the session IS
  // comes from them alone.
  witness?: { servedVideo?: V1ServedLeg; servedAudio?: V1ServedLeg; played?: V1PlayedState };
  // AUDIO-relative ordinal of the selected track: describes the source track
  // while neither witness has named one.
  audioTrackIndex?: number | null;
  // The variant hls.js is playing (servedVariant).
  served?: ServedVariant | null;
}

export function StreamInfoPanel({ streamInfo, open, onToggle, witness, audioTrackIndex, served }: StreamInfoPanelProps) {
  const { t } = useTranslation();
  const model = buildStreamInfoModel(streamInfo, {
    audio: { relative: audioTrackIndex },
    served,
    witness,
  });

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

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

      {open && model && (
        <div
          className="absolute bottom-full mb-2 right-0 w-64 rounded-[var(--radius-md)] shadow-xl overflow-y-auto z-badge max-h-[80vh] bg-[var(--surface)] border border-[var(--border)]"
          onClick={(e) => e.stopPropagation()}
        >
          {model.sections.map((section) => (
            <div key={section.kind}>
              <Section title={t(section.titleKey)} decision={section.decision} />
              {section.rows.map((row) => (
                <InfoRow key={`${row.labelKey}:${row.valueKey ?? ""}`} label={t(row.labelKey)} value={row.valueKey ? t(row.valueKey, row.valueParams) : (row.value ?? "")} />
              ))}
            </div>
          ))}
          {model.reasons.length > 0 ? (
            <>
              <Section title={t(model.reasonsHeadingKey)} />
              <ul className="px-3 py-1.5 space-y-1">
                {model.reasons.map((reason) => (
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
          ) : null}
        </div>
      )}
    </div>
  );
}
