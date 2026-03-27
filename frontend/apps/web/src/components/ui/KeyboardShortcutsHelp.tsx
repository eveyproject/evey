import { Modal } from "./Modal";

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent);

const shortcuts = [
  { key: "/", desc: "Focus search" },
  { key: isMac ? "⌘K" : "Ctrl+K", desc: "Focus search" },
  { key: "Esc", desc: "Close modal / blur input" },
  { key: "h", desc: "Go to Home" },
  { key: "?", desc: "Toggle this help" },
];

interface Props {
  open: boolean;
  onClose: () => void;
}

export function KeyboardShortcutsHelp({ open, onClose }: Props) {
  return (
    <Modal open={open} onClose={onClose} title="Keyboard Shortcuts">
      <div className="flex flex-col gap-2">
        {shortcuts.map(({ key, desc }) => (
          <div key={key} className="flex items-center justify-between py-1.5">
            <span className="text-sm text-[var(--text-muted)]">
              {desc}
            </span>
            <kbd className="inline-flex items-center px-2 py-0.5 rounded text-xs font-mono font-medium bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]">
              {key}
            </kbd>
          </div>
        ))}
      </div>
      <p className="text-xs mt-4 text-[var(--text-muted)]">
        Shortcuts are disabled while typing in an input field.
      </p>
    </Modal>
  );
}
