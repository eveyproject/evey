import { Check } from "lucide-react";
import { List } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";

export interface EpisodeNavItem { id: string; title?: string; indexNumber?: number }
export interface SeasonNavItem { id: string; title?: string; indexNumber?: number }

export interface EpisodesPanelProps {
  seasons: SeasonNavItem[];
  currentSeasonId: string;
  episodes: EpisodeNavItem[];
  currentEpisodeId: string;
  onSeasonChange: (id: string) => void;
  onEpisodeChange: (id: string) => void;
  open: boolean;
  onToggle: () => void;
}

export function EpisodesPanel({ seasons, currentSeasonId, episodes, currentEpisodeId, onSeasonChange, onEpisodeChange, open, onToggle }: EpisodesPanelProps) {
  const { t } = useTranslation();
  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

  return (
    <div className="relative">
      <button onClick={toggle} aria-label={t("player.controls.episodes")} title={t("player.controls.episodes")} className="text-white hover:text-[var(--accent)] transition-colors-fast-fast p-1">
        <List size={20} />
      </button>
      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-72 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-badge"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
          onClick={(e) => e.stopPropagation()}
        >
          {seasons.length > 1 && (
            <div className="px-3 py-2" style={{ borderBottom: "1px solid var(--border)" }}>
              <Select
                value={currentSeasonId}
                onChange={onSeasonChange}
                options={seasons.map((s) => ({
                  value: s.id,
                  label: s.title ?? t("media.season", { number: s.indexNumber }),
                }))}
              />
            </div>
          )}
          <div className="max-h-72 overflow-y-auto">
            {episodes.map((ep) => (
              <button
                key={ep.id}
                onClick={() => { onEpisodeChange(ep.id); onToggle(); }}
                className="flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast-fast text-left"
                style={{ color: ep.id === currentEpisodeId ? "var(--accent)" : "var(--text)" }}
              >
                <span className="w-8 flex-shrink-0 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                  E{ep.indexNumber ?? "?"}
                </span>
                <span className="flex-1 truncate">{ep.title}</span>
                {ep.id === currentEpisodeId && (
                  <Check size={12} className="flex-shrink-0" />
                )}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
