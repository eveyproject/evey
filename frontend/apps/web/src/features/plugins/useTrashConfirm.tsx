import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch, formatBytes } from "@evey/api";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";

// The host's answer to evey.media.trash: Evey's own confirmation, built from
// the server's record of each item (title, path, size, quality), never from
// what the plugin sent. Nothing moves until the administrator says yes here,
// and the move runs on the administrator's own session through the admin
// trash endpoint, one item at a time.

interface StreamInfo {
  type?: string;
  codec?: string;
  width?: number;
  height?: number;
  colorTransfer?: string;
  channels?: number;
  isDefault?: boolean;
}

export interface TrashCandidate {
  id: string;
  title?: string;
  year?: number;
  filePath?: string;
  fileSize?: string | number;
  bitrate?: string | number;
  streams?: StreamInfo[];
}

export interface TrashResult {
  /** pending: the file is being copied to another disk in the background. */
  trashed: { id: string; trashedPath: string; pending: boolean }[];
  failed: { id: string; message: string }[];
}

/** "2160p HEVC HDR10 · 25.0 Mb/s · EAC3 5.1", from the server's streams. */
export function qualityLine(item: TrashCandidate): string {
  const streams = item.streams ?? [];
  const isType = (s: StreamInfo, t: string) => String(s.type ?? "").toLowerCase().includes(t);
  const video = streams.find((s) => isType(s, "video"));
  const audios = streams.filter((s) => isType(s, "audio"));
  const audio = audios.find((s) => s.isDefault) ?? audios[0];
  const parts: string[] = [];
  if (video) {
    const hdr = video.colorTransfer === "smpte2084" ? "HDR" : video.colorTransfer === "arib-std-b67" ? "HLG" : "";
    const v = [video.height ? `${video.height}p` : "", (video.codec ?? "").toUpperCase(), hdr].filter(Boolean).join(" ");
    if (v) parts.push(v);
  }
  const bps = Number(item.bitrate) || 0;
  if (bps > 0) parts.push(`${(bps / 1e6).toFixed(1)} Mb/s`);
  if (audio) {
    const ch = audio.channels ? (audio.channels >= 6 ? `${audio.channels - 1}.1` : audio.channels === 2 ? "2.0" : `${audio.channels}.0`) : "";
    parts.push([(audio.codec ?? "").toUpperCase(), ch].filter(Boolean).join(" "));
  }
  return parts.join(" · ");
}

async function loadCandidates(ids: string[]): Promise<TrashCandidate[]> {
  const out: TrashCandidate[] = [];
  for (const id of ids) {
    const item = await eveyFetch<TrashCandidate>({ url: `/api/v1/media/${encodeURIComponent(id)}`, method: "GET" });
    out.push({ ...item, id });
  }
  return out;
}

export interface RestoreResult {
  restored: { trashedPath: string; restoredPath: string }[];
  failed: { trashedPath: string; message: string }[];
}

/**
 * Undoes a trash: each file goes back where it was through the admin restore
 * endpoint, on the administrator's session. Nothing is destroyed by it, so no
 * confirmation; the server only ever moves a file OUT of a library trash.
 */
export async function restoreTrashed(paths: string[]): Promise<RestoreResult> {
  const result: RestoreResult = { restored: [], failed: [] };
  for (const p of paths) {
    try {
      const res = await eveyFetch<{ restoredPath?: string }>({
        url: "/api/v1/admin/media/trash/restore",
        method: "POST",
        data: { trashedPath: p },
      });
      result.restored.push({ trashedPath: p, restoredPath: res.restoredPath ?? "" });
    } catch (err) {
      result.failed.push({ trashedPath: p, message: err instanceof Error ? err.message : String(err) });
    }
  }
  return result;
}

export function useTrashConfirm() {
  const { t } = useTranslation();
  const [candidates, setCandidates] = useState<TrashCandidate[] | null>(null);
  const [busy, setBusy] = useState(false);
  const pending = useRef<{ resolve: (r: TrashResult) => void; reject: (e: Error) => void } | null>(null);

  const request = useCallback(async (ids: string[]): Promise<TrashResult> => {
    if (pending.current) throw new Error("a trash confirmation is already open");
    const items = await loadCandidates(ids);
    return new Promise<TrashResult>((resolve, reject) => {
      pending.current = { resolve, reject };
      setCandidates(items);
    });
  }, []);

  const cancel = useCallback(() => {
    if (busy) return;
    pending.current?.reject(new Error("cancelled"));
    pending.current = null;
    setCandidates(null);
  }, [busy]);

  const confirm = useCallback(async () => {
    if (!candidates) return;
    setBusy(true);
    const result: TrashResult = { trashed: [], failed: [] };
    for (const c of candidates) {
      try {
        const res = await eveyFetch<{ trashedPath?: string; pending?: boolean }>({ url: `/api/v1/admin/media/${encodeURIComponent(c.id)}/trash`, method: "POST" });
        result.trashed.push({ id: c.id, trashedPath: res.trashedPath ?? "", pending: !!res.pending });
      } catch (err) {
        result.failed.push({ id: c.id, message: err instanceof Error ? err.message : String(err) });
      }
    }
    setBusy(false);
    pending.current?.resolve(result);
    pending.current = null;
    setCandidates(null);
  }, [candidates]);

  const dialog = (
    <Modal open={candidates !== null} onClose={cancel} title={t("admin.plugins.trash.title", { count: candidates?.length ?? 0 })} size="lg">
      <p className="text-sm text-[var(--text-muted)] mb-3">{t("admin.plugins.trash.explain")}</p>
      <ul className="flex flex-col gap-2 mb-4" data-testid="trash-confirm-list">
        {(candidates ?? []).map((c) => (
          <li key={c.id} className="rounded-[var(--radius-md)] border border-[var(--border)] px-3 py-2">
            <div className="text-sm font-medium">
              {c.title}
              {c.year ? ` (${c.year})` : ""}
              {Number(c.fileSize) > 0 ? ` - ${formatBytes(c.fileSize)}` : ""}
            </div>
            <div className="text-xs text-[var(--text-muted)]">{qualityLine(c)}</div>
            <div className="text-xs font-mono text-[var(--text-muted)] break-all">{c.filePath}</div>
          </li>
        ))}
      </ul>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={cancel} disabled={busy}>{t("common.cancel")}</Button>
        <Button variant="danger" onClick={() => void confirm()} disabled={busy} data-testid="trash-confirm-btn">
          {t("admin.plugins.trash.confirm", { count: candidates?.length ?? 0 })}
        </Button>
      </div>
    </Modal>
  );

  return { request, dialog };
}
