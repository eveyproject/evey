import { useTranslation } from "react-i18next";
import { Modal } from "./Modal";

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent);

interface Props {
  open: boolean;
  onClose: () => void;
}

export function KeyboardShortcutsHelp({ open, onClose }: Props) {
  const { t } = useTranslation();

  const shortcuts = [
    { key: "/", desc: t("common:shortcuts.focusSearch") },
    { key: isMac ? "⌘K" : "Ctrl+K", desc: t("common:shortcuts.focusSearch") },
    { key: "Esc", desc: t("common:shortcuts.closeModal") },
    { key: "h", desc: t("common:shortcuts.goHome") },
    { key: "?", desc: t("common:shortcuts.toggleHelp") },
  ];

  return (
    <Modal open={open} onClose={onClose} title={t("common:shortcuts.title")}>
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
        {t("common:shortcuts.disabledHint")}
      </p>
    </Modal>
  );
}
