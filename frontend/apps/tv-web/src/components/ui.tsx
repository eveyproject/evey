import { memo, useCallback, useEffect, useRef, type ReactNode } from "react";
import { useFocusable } from "../tv/focus";
import { tvKeyOf } from "../tv/keys";

interface ButtonProps {
  children: ReactNode;
  onPress: () => void;
  variant?: "primary" | "ghost" | "default";
  focusKey?: string;
  autoFocus?: boolean;
  disabled?: boolean;
  /** The remote landed here: a paged list fetches its next page from this. */
  onFocus?: () => void;
}

/** A remote-driven button: focus is its hover, "select" its click. */
export function Button({ children, onPress, variant = "default", focusKey, autoFocus, disabled, onFocus }: ButtonProps) {
  const { ref, focused, focusSelf } = useFocusable({
    focusKey,
    onEnterPress: () => { if (!disabled) onPress(); },
    focusable: !disabled,
    onFocus: (layout) => {
      layout.node.scrollIntoView({ block: "nearest", inline: "nearest" });
      onFocus?.();
    },
  });
  useEffect(() => {
    if (autoFocus) focusSelf();
  }, [autoFocus, focusSelf]);
  return (
    <button
      ref={ref}
      type="button"
      className={`btn focusable ${variant} ${focused ? "focused" : ""}`}
      onClick={onPress}
      disabled={disabled}
      style={disabled ? { opacity: 0.5 } : undefined}
    >
      {children}
    </button>
  );
}

interface TextFieldProps {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  type?: "text" | "password" | "url";
  focusKey?: string;
  autoFocus?: boolean;
  onSubmit?: () => void;
}

/**
 * A text field the remote can reach. Focusing it (select) hands the keys to
 * the browser's own on-screen keyboard; Enter submits, back leaves it.
 */
export function TextField({ value, onChange, placeholder, type = "text", focusKey, autoFocus, onSubmit }: TextFieldProps) {
  const { ref, focused, focusSelf } = useFocusable({
    focusKey,
    onEnterPress: () => {
      const el = ref.current as HTMLInputElement | null;
      if (el && document.activeElement !== el) el.focus();
      else onSubmit?.();
    },
  });
  useEffect(() => {
    if (autoFocus) focusSelf();
  }, [autoFocus, focusSelf]);
  return (
    <input
      ref={ref}
      className={`field focusable ${focused ? "focused" : ""}`}
      type={type}
      value={value}
      placeholder={placeholder}
      autoCapitalize="none"
      autoCorrect="off"
      spellCheck={false}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={(e) => {
        const key = tvKeyOf(e);
        if (key === "select") {
          // Enter from the on-screen keyboard: leave the field and submit.
          // Stopped here, or the spatial library would see the same Enter
          // on the field it still marks focused and hand the keys straight
          // back to it - the keyboard reopened on every Done (LG simulator,
          // 2026-09-22).
          e.preventDefault();
          e.stopPropagation();
          (e.target as HTMLInputElement).blur();
          onSubmit?.();
        } else if (key === "back" && e.key !== "Backspace") {
          // Back leaves the field, and only the field: the screen's own
          // back handling must not fire and walk out of the form.
          e.preventDefault();
          e.stopPropagation();
          (e.target as HTMLInputElement).blur();
        }
      }}
    />
  );
}

interface CardProps {
  title: string;
  imageUrl?: string;
  wide?: boolean;
  onPress: () => void;
  focusKey?: string;
  autoFocus?: boolean;
  /** The remote landed on this card: a paged grid fetches its next page from here. */
  onFocus?: () => void;
  /** Already seen: the tick the other clients draw in the corner. */
  watched?: boolean;
  /** How far in, 0 to 100: the bar along the bottom of the picture. */
  percent?: number;
}

function CardImpl({ title, imageUrl, wide, onPress, focusKey, autoFocus, onFocus, watched, percent }: CardProps) {
  const { ref, focused, focusSelf } = useFocusable({ focusKey, onEnterPress: onPress, onFocus: (layout) => {
    // Keep the focused card in view: the row scrolls, the screen scrolls.
    layout.node.scrollIntoView({ block: "nearest", inline: "nearest" });
    onFocus?.();
  } });
  useEffect(() => {
    if (autoFocus) focusSelf();
  }, [autoFocus, focusSelf]);
  return (
    <div ref={ref} className={`card focusable ${wide ? "wide" : ""} ${focused ? "focused" : ""}`} onClick={onPress}>
      {imageUrl ? <img src={imageUrl} alt="" loading="lazy" /> : <div style={{ aspectRatio: wide ? "16 / 9" : "2 / 3", background: "var(--surface-raised)" }} />}
      {watched ? <span className="watched-badge" aria-hidden="true">✓</span> : null}
      {percent != null && percent > 0 && percent < 100 ? (
        <span className="card-progress"><i style={{ width: `${Math.min(100, percent)}%` }} /></span>
      ) : null}
      <div className="label">{title}</div>
    </div>
  );
}

/**
 * A card only re-renders when its OWN props change. A library grid appends a
 * page of thirty every time the remote reaches the last rows, and without
 * this every card already on screen was rebuilt with it - measured at nine
 * style recalculations per key press on a three hundred title library.
 * Callers must keep their handlers stable for this to hold (useCardHandlers).
 */
export const Card = memo(CardImpl);

export function Spinner() {
  return <div className="spinner" role="progressbar" />;
}

/**
 * One stable callback per item id, kept for the life of the screen: a new
 * arrow function per render would defeat the memo above.
 */
export function useCardHandlers<A extends unknown[]>(fn: (...args: A) => void): (key: string, ...args: A) => () => void {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const cache = useRef(new Map<string, () => void>());
  return useCallback((key: string, ...args: A) => {
    const existing = cache.current.get(key);
    if (existing) return existing;
    const handler = () => fnRef.current(...args);
    cache.current.set(key, handler);
    return handler;
  }, []);
}
