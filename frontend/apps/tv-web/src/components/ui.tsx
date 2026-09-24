import { useCallback, useRef, type ReactNode } from "react";
import { useArrivalFocus, useFocusable } from "../tv/focus";
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
  /** What the button says when its face is an icon: read by a screen reader. */
  label?: string;
}

/** A remote-driven button: focus is its hover, "select" its click. */
export function Button({ children, onPress, variant = "default", focusKey, autoFocus, disabled, onFocus, label }: ButtonProps) {
  const { ref, focused, focusSelf } = useFocusable({
    focusKey,
    onEnterPress: () => { if (!disabled) onPress(); },
    focusable: !disabled,
    onFocus: (layout) => {
      layout.node.scrollIntoView({ block: "nearest", inline: "nearest" });
      onFocus?.();
    },
  });
  useArrivalFocus(focusKey, autoFocus, focusSelf);
  return (
    <button
      ref={ref}
      type="button"
      className={`btn focusable ${variant} ${focused ? "focused" : ""}`}
      onClick={onPress}
      aria-label={label}
      disabled={disabled}
      style={disabled ? { opacity: 0.5 } : undefined}
    >
      {children}
    </button>
  );
}

interface PressableProps {
  children: ReactNode;
  onPress: () => void;
  className?: string;
  focusKey?: string;
  autoFocus?: boolean;
  label?: string;
  onFocus?: () => void;
  onBlur?: () => void;
}

/** A focusable box with its own look: what the Android television's Pressable is. */
export function Pressable({ children, onPress, className = "", focusKey, autoFocus, label, onFocus, onBlur }: PressableProps) {
  const { ref, focused, focusSelf } = useFocusable({
    focusKey,
    onEnterPress: onPress,
    onFocus: (layout) => {
      layout.node.scrollIntoView({ block: "nearest", inline: "nearest" });
      onFocus?.();
    },
    onBlur: () => onBlur?.(),
  });
  useArrivalFocus(focusKey, autoFocus, focusSelf);
  return (
    <div ref={ref} role="button" aria-label={label} className={`${className} ${focused ? "focused" : ""}`} onClick={onPress}>
      {children}
    </div>
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
  useArrivalFocus(focusKey, autoFocus, focusSelf);
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

/** A filter chip: the Android television's FocusableCard with a label. */
export function Chip({ label, active, onPress, focusKey }: { label: string; active: boolean; onPress: () => void; focusKey?: string }) {
  return (
    <Pressable className={`tv-card chip ${active ? "active" : ""}`} onPress={onPress} focusKey={focusKey} label={label}>
      {label}
    </Pressable>
  );
}
