import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { mKVServiceListChapters } from "@evey/api";
import { useMediaSegments } from "@/hooks/useMediaSegments";

const INTRO_PATTERNS = /^(intro|introduction|opening|op$|op\s)/i;

interface SkipIntroButtonProps {
  mediaId?: string;   // used for the segments API (priority)
  filePath?: string;  // fallback via MKV chapters
  currentTime: number;
  onSkip: (toSec: number) => void;
}

export function SkipIntroButton({ mediaId, filePath, currentTime, onSkip }: SkipIntroButtonProps) {
  const { t } = useTranslation();

  const { data: segments } = useMediaSegments(mediaId, ["intro"]);

  const { data: chaptersData } = useQuery({
    queryKey: ["/api/v1/mkv/chapters", filePath],
    queryFn: ({ signal }) => mKVServiceListChapters({ path: filePath }, signal),
    // only fetch MKV chapters when no API segments are available
    enabled: !!filePath && (!segments || segments.length === 0),
    staleTime: Infinity,
  });

  // Resolve intro window: API segments take priority over MKV chapters
  let startSec: number | null = null;
  let endSec: number | null = null;

  const apiIntro = segments?.find((s) => s.type === "intro");
  if (apiIntro) {
    startSec = apiIntro.startMs / 1000;
    endSec = apiIntro.endMs / 1000;
  } else {
    const introChapter = chaptersData?.chapters?.find(
      (ch) => ch.title && INTRO_PATTERNS.test(ch.title.trim())
    );
    if (introChapter) {
      startSec = Number(introChapter.startMs ?? 0) / 1000;
      endSec = Number(introChapter.endMs ?? 0) / 1000;
    }
  }

  if (startSec === null || endSec === null) return null;
  if (endSec <= startSec) return null;
  if (currentTime < startSec || currentTime >= endSec) return null;

  return (
    <button
      onClick={() => onSkip(endSec as number)}
      className="absolute bottom-28 right-4 z-float px-5 py-2.5 text-sm font-semibold text-white rounded-md transition-all duration-200 hover:scale-105 animate-fade-in bg-[var(--overlay-btn-bg)] border border-[var(--overlay-dot-inactive)] backdrop-blur-[8px]"
      aria-label={t("player.skipIntro")}
    >
      {t("player.skipIntro")}
    </button>
  );
}
