import { useEffect, useRef, useState } from "react";
import { Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useMusicPlayerStore } from "@evey/store";
import type { V1Trailer } from "@evey/api";
import { useFocusTrap } from "@/hooks/useFocusTrap";

function TrailerPlayer({ trailer, userLang }: { trailer: V1Trailer; userLang: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  useFocusTrap(dialogRef, open);
  const pauseMusic = useMusicPlayerStore((s) => s.pauseExternal);
  const resumeMusic = useMusicPlayerStore((s) => s.resumeExternal);

  // Pause the global mini-player while a trailer modal is open so we don't
  // have music competing with trailer audio. Resume on close.
  useEffect(() => {
    if (!open) return;
    pauseMusic();
    return () => { resumeMusic(); };
  }, [open, pauseMusic, resumeMusic]);

  // Close the trailer dialog on Escape (keyboard/a11y parity with shared Modal).
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  if (!trailer.key || !trailer.site) return null;

  const needsSubtitles = (trailer.language || "en") !== userLang;

  const iframeSrc = (() => {
    switch (trailer.site?.toLowerCase()) {
      case "youtube": {
        const params = new URLSearchParams({ autoplay: "1", rel: "0" });
        if (needsSubtitles) {
          params.set("cc_load_policy", "1");
          params.set("cc_lang_pref", userLang);
        }
        return `https://www.youtube.com/embed/${trailer.key}?${params}`;
      }
      case "vimeo":
        return `https://player.vimeo.com/video/${trailer.key}?autoplay=1`;
      default:
        return null;
    }
  })();

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="flex items-center gap-2.5 px-5 py-2.5 rounded text-lg font-medium transition-colors-fast bg-[var(--overlay-border-subtle)] text-[var(--text)] border border-[var(--border)]"
      >
        <Play size={20} fill="currentColor" />
        {t("media.detail.trailer")}
        {needsSubtitles && (
          <span className="text-sm px-1 rounded bg-[var(--overlay-btn-bg)] text-[var(--text-muted)]">
            CC
          </span>
        )}
      </button>

      {open && (
        <div
          className="fixed inset-0 z-overlay flex items-center justify-center p-4 bg-[var(--overlay-scrim)]"
          onClick={() => setOpen(false)}
        >
          <div
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-label={t("media.detail.trailer")}
            className="relative w-full max-w-4xl aspect-video"
            onClick={(e) => e.stopPropagation()}
          >
            {iframeSrc ? (
              <iframe
                src={iframeSrc}
                title={t("media.detail.trailer")}
                className="w-full h-full rounded"
                allow="autoplay; fullscreen"
                allowFullScreen
              />
            ) : (
              <div className="w-full h-full flex items-center justify-center bg-[var(--surface)] rounded-[var(--radius-md)] text-[var(--text-muted)]">
                {t("media.detail.unsupportedPlatform", { platform: trailer.site })}
              </div>
            )}
            <button
              onClick={() => setOpen(false)}
              className="absolute -top-10 right-0 text-white/70 hover:text-white text-sm"
              aria-label={t("media.detail.close")}
            >
              {t("media.detail.close")} ✕
            </button>
          </div>
        </div>
      )}
    </>
  );
}

export function TrailerSection({ trailers }: { trailers?: V1Trailer[] }) {
  const { i18n } = useTranslation();
  if (!trailers || trailers.length === 0) return null;

  const userLang = i18n.language.split("-")[0];

  const sorted = [...trailers].sort((a, b) => {
    const aLang = a.language || "en";
    const bLang = b.language || "en";
    if (aLang === userLang && bLang !== userLang) return -1;
    if (bLang === userLang && aLang !== userLang) return 1;
    if (aLang === "en" && bLang !== "en") return -1;
    if (bLang === "en" && aLang !== "en") return 1;
    if (a.official && !b.official) return -1;
    if (!a.official && b.official) return 1;
    return 0;
  });

  const display = sorted.slice(0, 3);

  return (
    <div className="flex flex-wrap gap-2 pt-1">
      {display.map((t, i) => (
        <TrailerPlayer key={`${t.key}-${i}`} trailer={t} userLang={userLang} />
      ))}
    </div>
  );
}
