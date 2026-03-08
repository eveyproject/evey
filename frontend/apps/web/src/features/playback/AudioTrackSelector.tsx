import { useState } from "react";

export type AudioStream = {
  index: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
};

interface AudioTrackSelectorProps {
  streams: AudioStream[];
  currentIndex: number;
  onChange: (index: number) => void;
}

export function AudioTrackSelector({ streams, currentIndex, onChange }: AudioTrackSelectorProps) {
  const [open, setOpen] = useState(false);

  if (streams.length === 0) return null;

  const current = streams.find((s) => s.index === currentIndex) ?? streams[0];
  const label = current?.title ?? current?.language ?? `Track ${current?.index}`;

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className="px-3 py-1 text-sm rounded-[var(--radius-sm)] bg-[var(--surface)] text-[var(--text)] hover:bg-[var(--surface-raised)] transition-colors"
        aria-label="Audio track"
      >
        Audio: {label}
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 right-0 min-w-max rounded-[var(--radius-md)] bg-[var(--surface)] border border-[var(--border)] shadow-lg overflow-hidden z-10">
          {streams.map((s) => {
            const trackLabel = s.title ?? s.language ?? `Track ${s.index}`;
            return (
              <button
                key={s.index}
                onClick={() => {
                  onChange(s.index);
                  setOpen(false);
                }}
                className={`block w-full text-left px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors ${s.index === currentIndex ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
              >
                {trackLabel}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
