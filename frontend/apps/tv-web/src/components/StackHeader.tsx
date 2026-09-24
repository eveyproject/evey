import { useTranslation } from "react-i18next";
import { goBack } from "../app/router";
import { Icon } from "./icons";

// The header's Customize reaches the home through this event: the header is
// the shell's, drawn above the screen.
export const HOME_CUSTOMIZE_EVENT = "evey:home-customize";

/** The stack header a few Android deep screens show: back and the title. */
export function StackHeader({ title }: { title: string }) {
  const { t } = useTranslation();
  return (
    <div className="stack-header">
      <button type="button" className="stack-back" aria-label={t("common.back", "Back")} onClick={goBack} tabIndex={-1}>
        <Icon name="arrow-back" size={24} />
      </button>
      <span className="stack-title">{title}</span>
    </div>
  );
}
