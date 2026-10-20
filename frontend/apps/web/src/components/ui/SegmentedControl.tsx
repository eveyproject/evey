import { useRef, type KeyboardEvent } from "react";

export interface SegmentedOption {
  value: string;
  label: string;
  disabled?: boolean;
  "data-testid"?: string;
}

interface SegmentedControlProps {
  /** Accessible name of the group (the setting's label). */
  label: string;
  options: SegmentedOption[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  id?: string;
  "data-testid"?: string;
}

// The narrowest an option may get before the group wraps to another row.
const MIN_OPTION_REM = 7;

// SegmentedControl is the radio group of the settings pages: a row of options
// where exactly one is picked. Every option gets the same width (a CSS grid
// of equal columns), whatever its label, and the row wraps onto several lines
// of equal cells on a narrow screen instead of squeezing the labels. Native
// radios with labels of different lengths made ragged rows of uneven targets.
//
// It follows the ARIA radio group pattern: one tab stop (the checked option),
// arrow keys move and select, Home/End jump to the ends.
export function SegmentedControl({
  label,
  options,
  value,
  onChange,
  disabled = false,
  id,
  "data-testid": testId,
}: SegmentedControlProps) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const enabled = options.map((o, i) => ({ o, i })).filter(({ o }) => !o.disabled && !disabled);
  const checkedIndex = options.findIndex((o) => o.value === value);
  // The tab stop is the checked option, or the first usable one when none is.
  const tabStop = checkedIndex >= 0 ? checkedIndex : (enabled[0]?.i ?? 0);

  function move(e: KeyboardEvent<HTMLButtonElement>, from: number) {
    if (enabled.length === 0) return;
    const pos = enabled.findIndex(({ i }) => i === from);
    let next: number | undefined;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = enabled[(pos + 1) % enabled.length].i;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = enabled[(pos - 1 + enabled.length) % enabled.length].i;
    else if (e.key === "Home") next = enabled[0].i;
    else if (e.key === "End") next = enabled[enabled.length - 1].i;
    if (next === undefined) return;
    e.preventDefault();
    refs.current[next]?.focus();
    if (options[next].value !== value) onChange(options[next].value);
  }

  return (
    <div
      id={id}
      role="radiogroup"
      aria-label={label}
      aria-disabled={disabled || undefined}
      data-testid={testId}
      className="grid w-full gap-1 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] p-1"
      style={{
        gridTemplateColumns: `repeat(auto-fit, minmax(min(100%, ${MIN_OPTION_REM}rem), 1fr))`,
        maxWidth: `${options.length * 10}rem`,
      }}
    >
      {options.map((o, i) => {
        const checked = o.value === value;
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={i === tabStop ? 0 : -1}
            disabled={disabled || o.disabled}
            data-testid={o["data-testid"]}
            data-value={o.value}
            onClick={() => {
              if (!checked) onChange(o.value);
            }}
            onKeyDown={(e) => move(e, i)}
            className={`min-w-0 rounded-[var(--radius-sm)] px-2 py-1.5 text-center text-sm leading-tight transition-colors-fast focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] disabled:cursor-not-allowed disabled:opacity-50 ${
              checked
                ? "bg-[var(--accent)] font-medium text-[var(--filter-active-text)]"
                : "cursor-pointer text-[var(--text-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--text)]"
            }`}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
