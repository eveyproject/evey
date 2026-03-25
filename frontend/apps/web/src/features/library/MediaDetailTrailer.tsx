import { useState } from "react";
import { Play } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { V1Trailer } from "@evey/api";

function TrailerPlayer({ trailer, userLang }: { trailer: V1Trailer; userLang: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

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
        className="flex items-center gap-2 px-4 py-2 rounded text-sm font-medium transition-colors"
        style={{ backgroundColor: "var(--overlay-border-subtle)", color: "var(--text)", border: "1px solid var(--border)" }}
      >
        <Play size={16} fill="currentColor" />
        {t("media.detail.trailer")}
        {needsSubtitles && (
          <span className="text-xs px-1 rounded" style={{ backgroundColor: "var(--overlay-btn-bg)", color: "var(--text-muted)" }}>
            CC
          </span>
        )}
      </button>

      {open && (
        <div
          className="fixed inset-0 z-overlay flex items-center justify-center p-4"
          style={{ backgroundColor: "var(--overlay-scrim)" }}
          onClick={() => setOpen(false)}
        >
          <div
            className="relative w-full max-w-4xl"
            style={{ aspectRatio: "16/9" }}
            onClick={(e) => e.stopPropagation()}
          >
            {iframeSrc ? (
              <iframe
                src={iframeSrc}
                className="w-full h-full rounded"
                allow="autoplay; fullscreen"
                allowFullScreen
              />
            ) : (
              <div className="w-full h-full flex items-center justify-center" style={{ backgroundColor: "var(--surface)", borderRadius: "var(--radius-md)", color: "var(--text-muted)" }}>
                {t("media.detail.unsupportedPlatform", { platform: trailer.site })}
              </div>
            )}
            <button
              onClick={() => setOpen(false)}
              className="absolute -top-10 right-0 text-white/70 hover:text-white text-sm"
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
