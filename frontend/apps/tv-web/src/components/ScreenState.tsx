import { useTranslation } from "react-i18next";
import { FocusRoot } from "../tv/focus";
import { goBack } from "../app/router";
import { Pressable, Spinner } from "./ui";

/** The Android television's non-content state: a spinner, or the failure with Retry and Back. */
export function ScreenState({ loading, error, onRetry, message }: {
  loading?: boolean;
  error?: boolean;
  onRetry?: () => void;
  message?: string;
}) {
  const { t } = useTranslation();
  if (loading) return <div className="screen-state"><Spinner /></div>;
  if (!error) return null;
  return (
    <FocusRoot autoFocus className="screen-state">
      <p className="state-title">{message ?? t("common.somethingWentWrong", "Something went wrong")}</p>
      <p className="state-hint">{t("common.offline.description", "Check your internet connection and try again.")}</p>
      <div className="state-actions">
        {onRetry ? (
          <Pressable className="tv-card state-btn primary" autoFocus onPress={onRetry}>{t("common.retry", "Retry")}</Pressable>
        ) : null}
        <Pressable className="tv-card state-btn" autoFocus={!onRetry} onPress={goBack}>{t("common.back", "Back")}</Pressable>
      </div>
    </FocusRoot>
  );
}
