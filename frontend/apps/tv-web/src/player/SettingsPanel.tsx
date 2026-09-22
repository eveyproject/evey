import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { formatTime } from "@evey/api";

// One menu the remote walks with up/down, OK opens a list or picks an entry,
// back closes the list then the menu. Kept outside the spatial library: the
// player owns the whole screen and a linear list is what a remote reads best.
export interface PanelChoice {
  key: string;
  label: string;
  selected?: boolean;
}

export interface PanelSection {
  key: "audio" | "subtitles" | "quality" | "speed" | "chapters" | "sleep" | "streamInfo" | "subtitleStyle" | "volume";
  label: string;
  value: string;
  choices: PanelChoice[];
  onPick: (key: string) => void;
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
  const closeList = () => {
    setRow(Math.max(0, sections.findIndex((s) => s.key === open)));
    setOpen(null);
  };

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
          if (target) openList(target);
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
  }, [section, sections, row, count, onClose]);

  return (
    <div className="panel" role="menu" aria-label={t("player.controls.settings", "Settings")}>
      <div className="panel-title">{section ? section.label : t("player.controls.settings", "Settings")}</div>
      {section
        ? section.choices.map((c, i) => (
            <div key={c.key} className={`panel-row ${i === row ? "focused" : ""} ${c.selected ? "selected" : ""}`} role="menuitemradio" aria-checked={!!c.selected}>
              <span>{c.label}</span>
              {c.selected ? <span className="check">✓</span> : null}
            </div>
          ))
        : sections.map((s, i) => (
            <div key={s.key} className={`panel-row ${i === row ? "focused" : ""}`} role="menuitem">
              <span>{s.label}</span>
              <span className="muted">{s.value}</span>
            </div>
          ))}
    </div>
  );
}

export function chapterLabel(title: string, startMs: number): string {
  return `${formatTime(startMs / 1000)}  ${title}`;
}
