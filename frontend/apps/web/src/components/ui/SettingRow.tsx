import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { InfoTooltip } from "./InfoTooltip";
import { Badge } from "./Badge";

export interface SettingRowReset {
  /** True when the admin chose a value (an override); false while the default applies. */
  overridden: boolean;
  /** The default, as shown in "Using default: X" while no override is set. */
  defaultText?: string;
  onReset: () => void;
  disabled?: boolean;
}

interface SettingRowProps {
  label: string;
  /** id of the control, so the label is a real <label for>. Leave it out for
      controls that carry their own accessible name (Switch, SegmentedControl). */
  htmlFor?: string;
  /** id put on the label, for a control named through aria-labelledby (SegmentedControl). */
  labelId?: string;
  /** Plain-language explanation, shown behind the "i" button next to the label. */
  info?: ReactNode;
  /** One short line under the label, always visible. */
  hint?: ReactNode;
  /** The value is read when the server starts: shows an "applied on restart" badge. */
  restart?: boolean;
  /** Default / reset line under the label. */
  reset?: SettingRowReset;
  /** A validation message, shown under the label in the danger colour. */
  error?: string;
  /**
   * How the control sits next to the label:
   * - "toggle": a switch, right-aligned on wide screens;
   * - "field": an input or a select, a fixed column on wide screens;
   * - "wide": the control takes the full width, under the label.
   */
  kind?: "toggle" | "field" | "wide";
  children: ReactNode;
  "data-testid"?: string;
}

const CONTROL_CLASS: Record<NonNullable<SettingRowProps["kind"]>, string> = {
  toggle: "flex shrink-0 sm:justify-end sm:pt-0.5",
  field: "w-full sm:w-72 sm:shrink-0",
  wide: "w-full",
};

// SettingRow is the one layout every admin setting uses: label, its "i"
// explanation and badges on the left, the control on the right (or under it
// for wide controls), an optional hint and the default/reset line below the
// label. Before it, each card hand-rolled its own label/control/hint stack
// and no two settings lined up.
export function SettingRow({
  label,
  htmlFor,
  labelId,
  info,
  hint,
  restart = false,
  reset,
  error,
  kind = "field",
  children,
  "data-testid": testId,
}: SettingRowProps) {
  const { t } = useTranslation();
  const wide = kind === "wide";
  const LabelTag = htmlFor ? "label" : "span";

  return (
    <div
      data-testid={testId}
      data-setting-row=""
      className={`flex flex-col gap-2 py-3 first:pt-0 last:pb-0 ${wide ? "" : "sm:flex-row sm:items-start sm:justify-between sm:gap-6"}`}
    >
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
          <LabelTag {...(htmlFor ? { htmlFor } : {})} id={labelId} className="text-sm font-medium text-[var(--text)]">
            {label}
          </LabelTag>
          {info && (
            <InfoTooltip label={t("admin.settingsLayout.moreInfo", { label })}>
              {info}
            </InfoTooltip>
          )}
          {restart && (
            <Badge variant="warning" data-testid="setting-restart-badge">
              {t("admin.serverConfig.appliedOnRestart")}
            </Badge>
          )}
        </div>
        {hint && <p className="mt-0.5 text-xs text-[var(--text-muted)]">{hint}</p>}
        {error && (
          <p className="mt-0.5 text-xs text-[var(--color-danger)]" role="alert" data-testid="setting-error">
            {error}
          </p>
        )}
        {reset && (
          <p className="mt-0.5 text-xs">
            {reset.overridden ? (
              <button
                type="button"
                onClick={reset.onReset}
                disabled={reset.disabled}
                className="text-[var(--accent)] hover:underline cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
              >
                {t("admin.serverSettings.reset")}
              </button>
            ) : (
              <span className="text-[var(--text-muted)]" data-testid="setting-using-default">
                {t("admin.serverSettings.usingDefault")} {reset.defaultText}
              </span>
            )}
          </p>
        )}
      </div>
      <div className={CONTROL_CLASS[kind]}>{children}</div>
    </div>
  );
}
