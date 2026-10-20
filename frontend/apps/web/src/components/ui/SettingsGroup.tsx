import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "./Card";
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
  "data-testid"?: string;
}

// SettingsGroup is a titled card of SettingRows, separated by hairlines. A
// section of the settings page is a stack of these.
export function SettingsGroup({ title, description, info, footer, children, "data-testid": testId }: SettingsGroupProps) {
  const { t } = useTranslation();
  return (
    <Card className="p-0" data-testid={testId}>
      <div className="px-4 pt-4 pb-3">
        <h3 className="flex items-center gap-1.5 text-base font-semibold text-[var(--text)]">
          {title}
          {info && <InfoTooltip label={t("admin.settingsLayout.moreInfo", { label: title })}>{info}</InfoTooltip>}
        </h3>
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
    </Card>
  );
}
