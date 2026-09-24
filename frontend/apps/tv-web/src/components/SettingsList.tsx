import type { ReactNode } from "react";
import { FocusRoot } from "../tv/focus";
import { Pressable } from "./ui";
import { Icon } from "./icons";

/** A titled block of rows, the Android television's settings section. */
export function SettingsGroup({ title, hint, label, children }: { title?: string; hint?: string; label?: string; children: ReactNode }) {
  return (
    <>
      {title ? <h2 className="set-section-title">{title}</h2> : null}
      {label ? <p className="set-toggle-label">{label}</p> : null}
      {hint ? <p className="set-hint">{hint}</p> : null}
      <FocusRoot className="set-section">{children}</FocusRoot>
    </>
  );
}

export function SettingsRow({ label, onPress, checked, muted, autoFocus, children }: {
  label: string;
  onPress: () => void;
  checked?: boolean;
  muted?: boolean;
  autoFocus?: boolean;
  children?: ReactNode;
}) {
  return (
    <Pressable className="set-row" label={label} autoFocus={autoFocus} onPress={onPress}>
      {children ?? <span className={muted ? "set-muted" : "set-label"}>{label}</span>}
      {checked ? <span className="set-check"><Icon name="checkmark" size={20} /></span> : null}
    </Pressable>
  );
}
