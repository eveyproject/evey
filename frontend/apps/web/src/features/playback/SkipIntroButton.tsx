import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { mKVServiceListChapters } from "@evey/api";

const INTRO_PATTERNS = /^(intro|introduction|opening|op$|op\s)/i;

interface SkipIntroButtonProps {
  filePath?: string;
  currentTime: number;
  onSkip: (toSec: number) => void;
}

export function SkipIntroButton({ filePath, currentTime, onSkip }: SkipIntroButtonProps) {
  const { t } = useTranslation();
  const { data } = useQuery({
    queryKey: ["/api/v1/mkv/chapters", filePath],
    queryFn: ({ signal }) => mKVServiceListChapters({ path: filePath }, signal),
    enabled: !!filePath,
    staleTime: Infinity,
  });

  const introChapter = data?.chapters?.find((ch) =>
    ch.title && INTRO_PATTERNS.test(ch.title.trim())
  );

  if (!introChapter) return null;

  const startSec = Number(introChapter.startMs ?? 0) / 1000;
  const endSec = Number(introChapter.endMs ?? 0) / 1000;

  if (endSec <= startSec) return null;
  if (currentTime < startSec || currentTime >= endSec) return null;

  return (
    <button
      onClick={() => onSkip(endSec)}
      className="absolute bottom-28 right-4 z-30 px-5 py-2.5 text-sm font-semibold text-white rounded-md transition-all duration-200 hover:scale-105 animate-fade-in"
      style={{
        backgroundColor: "var(--overlay-btn-bg)",
        border: "1px solid var(--overlay-dot-inactive)",
        backdropFilter: "blur(8px)",
      }}
    >
      {t("player.skipIntro")}
    </button>
  );
}
