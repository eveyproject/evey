import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { formatTime } from "@evey/api";
import { Icon } from "../components/icons";

// The Android television's "More" sheet: up/down walk it, OK opens a picker or
// runs an action, back climbs one level. Outside the spatial library: a linear
// list is what a remote reads best over a full-screen player.
export interface PanelChoice {
  key: string;
  label: string;
  selected?: boolean;
}

export interface PanelSection {
  key: "audio" | "subtitles" | "quality" | "speed" | "chapters" | "sleep" | "streamInfo" | "subtitleStyle" | "volume" | "previous" | "next" | "autoplay";
  label: string;
  value: string;
  choices: PanelChoice[];
  onPick: (key: string) => void;
  /** An entry that acts at once instead of opening a picker. */
  action?: () => void;
}

interface Props {
  sections: PanelSection[];
  onClose: () => void;
}

export function SettingsPanel({ sections, onClose }: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState<PanelSection["key"] | null>(null);
  const [row, setRow] = useState(0);
  const section = sections.find((s) => s.key === open) ?? null;
  const count = section ? section.choices.length : sections.length;

  // The row is set in the same update that opens or closes a list, not in
  // an effect after it: a down press arriving right after OK used to land
  // before the effect ran and be wiped by it (measured in the LG simulator).
  const openList = (target: PanelSection) => {
    const idx = target.choices.findIndex((c) => c.selected);
    setRow(idx >= 0 ? idx : 0);
    setOpen(target.key);
  };
  const closeList = useCallback(() => {
    setRow(Math.max(0, sections.findIndex((s) => s.key === open)));
    setOpen(null);
  }, [sections, open]);

  // A long list (chapters, subtitle style) keeps the row the remote is on in view.
  useEffect(() => {
    document.querySelector(".player-sheet-row.focused")?.scrollIntoView?.({ block: "nearest" });
  }, [row, open]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const code = e.keyCode;
      const isBack = e.key === "Escape" || e.key === "GoBack" || e.key === "BrowserBack" || code === 27 || code === 461 || code === 10009 || code === 8;
      if (isBack) {
        e.preventDefault();
        e.stopImmediatePropagation();
        if (section) closeList();
        else onClose();
        return;
      }
      if (e.key === "ArrowDown" || code === 40) {
        e.preventDefault();
        e.stopImmediatePropagation();
        setRow((r) => Math.min(count - 1, r + 1));
      } else if (e.key === "ArrowUp" || code === 38) {
        e.preventDefault();
        e.stopImmediatePropagation();
        setRow((r) => Math.max(0, r - 1));
      } else if (e.key === "Enter" || code === 13) {
        e.preventDefault();
        e.stopImmediatePropagation();
        if (section) {
          const choice = section.choices[row];
          if (choice) {
            section.onPick(choice.key);
            onClose();
          }
        } else {
          const target = sections[row];
          if (target?.action) {
            target.action();
            onClose();
          } else if (target) openList(target);
        }
      } else if (e.key === "ArrowLeft" || e.key === "ArrowRight" || code === 37 || code === 39) {
        // The panel owns the remote while open: a stray left/right must not
        // seek behind it.
        e.preventDefault();
        e.stopImmediatePropagation();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [section, sections, row, count, onClose, closeList]);

  return (
    <div className="modal-backdrop player-sheet-backdrop">
      <div className={`player-sheet ${section ? "picker" : ""}`} role="menu" aria-label={section ? section.label : t("player.controls.more", "More")}>
        <div className="player-sheet-title">{section ? section.label : t("player.controls.more", "More")}</div>
        <div className="player-sheet-list">
          {section
            ? section.choices.map((c, i) => (
                <div key={c.key} className={`player-sheet-row tv-card ${i === row ? "focused" : ""} ${c.selected ? "selected" : ""}`} role="menuitemradio" aria-checked={!!c.selected}>
                  <span className="player-sheet-label">{c.label}</span>
                  {c.selected ? <span className="player-sheet-check"><Icon name="checkmark" size={20} /></span> : null}
                </div>
              ))
            : sections.map((s, i) => (
                <div key={s.key} className={`player-sheet-row tv-card ${i === row ? "focused" : ""}`} role="menuitem">
                  <span className="player-sheet-label">{s.label}</span>
                  {s.value ? <span className="player-sheet-value">{s.value}</span> : null}
                </div>
              ))}
        </div>
      </div>
    </div>
  );
}

export function chapterLabel(title: string, startMs: number): string {
  return `${formatTime(startMs / 1000)}  ${title}`;
}
