import { useEffect, type ReactNode } from "react";
import { useFocusable } from "../tv/focus";

interface ButtonProps {
  children: ReactNode;
  onPress: () => void;
  variant?: "primary" | "ghost" | "default";
  focusKey?: string;
  autoFocus?: boolean;
  disabled?: boolean;
}

/** A remote-driven button: focus is its hover, "select" its click. */
export function Button({ children, onPress, variant = "default", focusKey, autoFocus, disabled }: ButtonProps) {
  const { ref, focused, focusSelf } = useFocusable({ focusKey, onEnterPress: () => { if (!disabled) onPress(); }, focusable: !disabled });
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
        if (e.key === "Enter") {
          e.preventDefault();
          (e.target as HTMLInputElement).blur();
          onSubmit?.();
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
}

export function Card({ title, imageUrl, wide, onPress, focusKey, autoFocus }: CardProps) {
  const { ref, focused, focusSelf } = useFocusable({ focusKey, onEnterPress: onPress, onFocus: (layout) => {
    // Keep the focused card in view: the row scrolls, the screen scrolls.
    layout.node.scrollIntoView({ block: "nearest", inline: "nearest" });
  } });
  useEffect(() => {
    if (autoFocus) focusSelf();
  }, [autoFocus, focusSelf]);
  return (
    <div ref={ref} className={`card focusable ${wide ? "wide" : ""} ${focused ? "focused" : ""}`} onClick={onPress}>
      {imageUrl ? <img src={imageUrl} alt="" loading="lazy" /> : <div style={{ aspectRatio: wide ? "16 / 9" : "2 / 3", background: "var(--surface-raised)" }} />}
      <div className="label">{title}</div>
    </div>
  );
}

export function Spinner() {
  return <div className="spinner" role="progressbar" />;
}
