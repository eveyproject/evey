import { useRef, type KeyboardEvent } from "react";

export interface SegmentedOption {
  value: string;
  label: string;
  disabled?: boolean;
  "data-testid"?: string;
}

interface SegmentedControlProps {
  /** Accessible name of the group, used when no visible label is linked. */
  label: string;
  /** id of the visible label naming the group (preferred over `label`). */
  labelledBy?: string;
  options: SegmentedOption[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  /**
   * "auto": the arrow keys pick the option they land on (the ARIA default).
   * "manual": the arrows only move the focus and Space/Enter picks. Use it
   * wherever a pick is saved at once, or walking the options would write
   * every value on the way.
   */
  activation?: "auto" | "manual";
  /** Label of a value the server reports that is not among the options. */
  formatUnknown?: (value: string) => string;
  id?: string;
  "data-testid"?: string;
}

// SegmentedControl is the radio group of the settings pages: a row of options
// where exactly one is picked. On a wide screen every option takes the width
// of the longest label of the group (equal columns sized to their content),
// so the labels stay on one line and the cells line up; on a phone the
// options stack, one per row at full width, instead of a ragged 2+1 grid.
//
// It follows the ARIA radio group pattern: one tab stop (the checked option),
// arrow keys move, Home/End jump to the ends. While disabled it stays
// focusable (aria-disabled), so a save in flight never throws the focus out.
export function SegmentedControl({
  label,
  labelledBy,
  options: given,
  value,
  onChange,
  disabled = false,
  activation = "auto",
  formatUnknown,
  id,
  "data-testid": testId,
}: SegmentedControlProps) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  // A value the server holds that no option offers is shown, not hidden: an
  // admin must see what is in force, even an off-ladder value.
  const options = given.some((o) => o.value === value)
    ? given
    : [...given, { value, label: formatUnknown ? formatUnknown(value) : value }];
  const usable = (o: SegmentedOption) => !o.disabled && !disabled;
  const enabled = options.map((o, i) => ({ o, i })).filter(({ o }) => !o.disabled);
  const checkedIndex = options.findIndex((o) => o.value === value);
  // The tab stop is the checked option, or the first usable one when none is.
  const tabStop = checkedIndex >= 0 ? checkedIndex : (enabled[0]?.i ?? 0);

  function pick(i: number) {
    const o = options[i];
    if (!usable(o) || o.value === value) return;
    onChange(o.value);
  }

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
    if (activation === "auto") pick(next);
  }

  return (
    <div
      id={id}
      role="radiogroup"
      aria-label={labelledBy ? undefined : label}
      aria-labelledby={labelledBy}
      aria-disabled={disabled || undefined}
      data-testid={testId}
      className="grid w-full max-w-full grid-flow-row gap-1 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] p-1 sm:inline-grid sm:w-auto sm:grid-flow-col sm:auto-cols-fr"
    >
      {options.map((o, i) => {
        const checked = o.value === value;
        const off = !usable(o);
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            aria-disabled={off || undefined}
            tabIndex={i === tabStop ? 0 : -1}
            data-testid={o["data-testid"]}
            data-value={o.value}
            onClick={() => pick(i)}
            onKeyDown={(e) => move(e, i)}
            className={`min-w-0 rounded-[var(--radius-sm)] px-3 py-1.5 text-center text-sm leading-tight transition-colors-fast focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] aria-disabled:cursor-not-allowed aria-disabled:opacity-50 ${
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
