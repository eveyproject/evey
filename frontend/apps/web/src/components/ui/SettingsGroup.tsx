import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { InfoTooltip } from "./InfoTooltip";

interface SettingsGroupProps {
  title: string;
  /** One short sentence under the title. */
  description?: ReactNode;
  /** Longer explanation behind an "i" next to the title. */
  info?: ReactNode;
  /** Action row under the settings (typically the group's Save button). */
  footer?: ReactNode;
  children: ReactNode;
  /** id of the heading, for a control the group title names on its own. */
  titleId?: string;
  "data-testid"?: string;
}

// SettingsGroup is a titled card of SettingRows, separated by hairlines. A
// section of the settings page is a stack of these.
export function SettingsGroup({ title, description, info, footer, children, titleId, "data-testid": testId }: SettingsGroupProps) {
  const { t } = useTranslation();
  return (
    // Not <Card>: its own p-4 and a p-0 override land in the same layer, and
    // the padding that wins doubled the gutter on a phone.
    <section className="bg-[var(--surface)] rounded-[var(--radius-lg)] border border-[var(--border)]" data-testid={testId}>
      <div className="px-4 pt-4 pb-3">
        {/* The "i" sits next to the heading, not inside it, so the heading's
            accessible name stays the title alone. */}
        <div className="flex items-center gap-1.5">
          <h3 id={titleId} className="text-base font-semibold text-[var(--text)]">{title}</h3>
          {info && <InfoTooltip label={t("admin.settingsLayout.moreInfo", { label: title })}>{info}</InfoTooltip>}
        </div>
        {description && <p className="mt-0.5 text-xs text-[var(--text-muted)]">{description}</p>}
      </div>
      <div className="flex flex-col divide-y divide-[var(--border)] border-t border-[var(--border)] px-4 py-4">
        {children}
      </div>
      {footer && (
        <div className="flex flex-wrap items-center justify-end gap-2 border-t border-[var(--border)] px-4 py-3">
          {footer}
        </div>
      )}
    </section>
  );
}
