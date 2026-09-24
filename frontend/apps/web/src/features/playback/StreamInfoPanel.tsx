import { Info } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { V1StreamInfo } from "@evey/api";
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
  const isCopy = decision === "copy";
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
  // wasmAudioActive: the audio is decoded client-side (ac3go WebAssembly), not
  // transcoded server-side, so the audio section shows a "décodé client" badge.
  wasmAudioActive?: boolean;
  // wasmAudioLeg: which leg of the bi-source relay is actually feeding the
  // speakers - "aac" while the decoder boots, "wasm" once it carries,
  // "aac-latched" when it starved and the server's AAC rendition finishes the
  // film. Undefined when the host does not know (mobile, cast, no engine).
  // Both the negotiated decision and the client flag say the audio is decoded
  // here; only this says whether it still IS. Prod, 2026-09-20: a latch at 4:25
  // left the panel announcing "décodé client" for the rest of the film.
  wasmAudioLeg?: string | null;
  // audioTrackIndex: the SUBTITLE-relative... no: the AUDIO-relative ordinal of
  // the track the viewer selected (first audio track 0). Without it this panel
  // reported `mediaStreams.find(type === AUDIO)` - the file's FIRST audio track,
  // whatever was playing. Prod, 2026-09-09: a release whose first track is a
  // TrueHD Atmos 7.1 (eng) showed "TRUEHD / 7.1 / eng" while the viewer was on
  // the French AC-3, and contradicted the "decoded client" badge, which is
  // computed from the real selection.
  audioTrackIndex?: number | null;
  // The variant playing right now (auto or picked quality); null before one is known.
  served?: ServedVariant | null;
}

// StreamInfoPanel surfaces the playback decision the backend already returns
// (play method, codecs, transcode reasons) - visibility the mobile app has but the
// web player lacked. Mirrors the SettingsPanel popover chrome.
export function StreamInfoPanel({ streamInfo, open, onToggle, wasmAudioActive, wasmAudioLeg, audioTrackIndex, served }: StreamInfoPanelProps) {
  const { t } = useTranslation();
  const model = buildStreamInfoModel(streamInfo, {
    audio: { relative: audioTrackIndex },
    wasmAudioActive,
    wasmAudioLeg,
    served,
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
                <InfoRow key={row.labelKey} label={t(row.labelKey)} value={row.valueKey ? t(row.valueKey) : (row.value ?? "")} />
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
