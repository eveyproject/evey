import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAdminMediaServiceApplyAudioShift } from "@evey/api";
import { Button } from "@/components/ui/Button";

// Watching a few seconds of the film to judge a desync, and applying what the
// ear found.
//
// This panel exists because nothing the server measures answers the question.
// Comparing a file's two audio tracks says whether they agree with EACH OTHER,
// never which one follows the picture - a whole evening of measurements got
// that backwards on two real films, and the answer came in ten seconds from
// someone watching a clip. So: watch, adjust, watch again, apply.
//
// The clip is rendered server-side, because these files are 4K HEVC in
// Matroska with AC-3 or DTS and no browser plays a frame of that.

// Candidate offsets, in seconds. Coarse on purpose: lip sync is judged in
// tenths, and an operator finds the neighbourhood before the decimal.
const STEPS = [-12, -6, -3, -1, -0.5, 0, 0.5, 1, 3, 6, 12];

export function SyncPreviewPanel({ mediaId, title, onClose }: { mediaId: string; title: string; onClose: () => void }) {
  const { t } = useTranslation();
  const [atSec, setAtSec] = useState(600);
  const [track, setTrack] = useState(0);
  const [shift, setShift] = useState(0);
  // Bumped on every explicit render request: the <video> src must change for
  // the browser to fetch a new clip, and a stale clip would answer the
  // previous question.
  const [nonce, setNonce] = useState(0);
  const applyShift = useAdminMediaServiceApplyAudioShift();

  const src = `/api/v1/admin/media/${encodeURIComponent(mediaId)}/sync-preview?at=${atSec}&len=20&track=${track}&shift=${shift}&n=${nonce}`;

  return (
    <div className="mt-2 rounded border border-[var(--border)] bg-[var(--surface-2)] p-3" data-testid="sync-preview-panel">
      <div className="flex items-center justify-between gap-2">
        <h4 className="text-sm font-semibold text-[var(--text)]">{t("admin.syncPreview.title", { title })}</h4>
        <Button variant="ghost" className="text-xs" onClick={onClose} data-testid="sync-preview-close">
          {t("common.close")}
        </Button>
      </div>
      <p className="mt-1 text-xs text-[var(--text-muted)]">{t("admin.syncPreview.intro")}</p>

      <div className="mt-2 flex flex-wrap items-end gap-3">
        <label className="text-xs text-[var(--text-muted)]">
          {t("admin.syncPreview.at")}
          <input
            type="number"
            min={0}
            step={30}
            value={atSec}
            onChange={(e) => setAtSec(Math.max(0, Number(e.target.value)))}
            className="ml-1.5 w-24 rounded border border-[var(--border)] bg-[var(--surface)] px-1.5 py-0.5 text-[var(--text)]"
            data-testid="sync-preview-at"
          />
        </label>
        <label className="text-xs text-[var(--text-muted)]">
          {t("admin.syncPreview.track")}
          <select
            value={track}
            onChange={(e) => setTrack(Number(e.target.value))}
            className="ml-1.5 rounded border border-[var(--border)] bg-[var(--surface)] px-1.5 py-0.5 text-[var(--text)]"
            data-testid="sync-preview-track"
          >
            {[0, 1, 2, 3].map((n) => (
              <option key={n} value={n}>
                {t("admin.syncPreview.trackN", { n: n + 1 })}
              </option>
            ))}
          </select>
        </label>
        <label className="text-xs text-[var(--text-muted)]">
          {t("admin.syncPreview.shift")}
          <select
            value={shift}
            onChange={(e) => setShift(Number(e.target.value))}
            className="ml-1.5 rounded border border-[var(--border)] bg-[var(--surface)] px-1.5 py-0.5 text-[var(--text)]"
            data-testid="sync-preview-shift"
          >
            {STEPS.map((s) => (
              <option key={s} value={s}>
                {s > 0 ? `+${s}` : `${s}`} s
              </option>
            ))}
          </select>
        </label>
        <Button variant="secondary" className="text-xs" onClick={() => setNonce((n) => n + 1)} data-testid="sync-preview-render">
          {t("admin.syncPreview.render")}
        </Button>
      </div>

      {nonce > 0 ? (
        <video
          key={src}
          src={src}
          controls
          autoPlay
          className="mt-3 w-full max-w-2xl rounded-md bg-black"
          data-testid="sync-preview-video"
        />
      ) : (
        <p className="mt-3 text-xs text-[var(--text-muted)]" data-testid="sync-preview-idle">
          {t("admin.syncPreview.idle")}
        </p>
      )}

      <div className="mt-3 border-t border-[var(--border)] pt-2">
        <Button
          variant="primary"
          className="text-xs"
          disabled={shift === 0}
          loading={applyShift.isPending}
          onClick={() => applyShift.mutate({ mediaId, data: { track: String(track + 1), shiftSec: shift } })}
          data-testid="sync-preview-apply"
        >
          {t("admin.syncPreview.apply", { shift: shift > 0 ? `+${shift}` : `${shift}` })}
        </Button>
        {/* Said out loud because it is the honest description of what this
            does: a person's ear is the evidence here, and the journal records
            it as such - which is better evidence than the file's own claim
            about itself. */}
        <p className="mt-1 text-xs text-[var(--text-muted)]">{t("admin.syncPreview.applyHint")}</p>
      </div>
    </div>
  );
}
