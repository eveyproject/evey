import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { goBack } from "../app/router";

// Nobody answered "still watching?": the player is gone (its session with it)
// and the screen stays black until the television sleeps on its own - a web
// app cannot put it to sleep. The first key or click returns to where the
// player was opened from, as leaving the player does.
export function SleepScreen() {
  const { t } = useTranslation();
  const wokeRef = useRef(false);
  useEffect(() => {
    const wake = (e: Event) => {
      // Captured first and stopped: the app's global back must not go back a
      // second time.
      e.preventDefault();
      e.stopImmediatePropagation();
      if (wokeRef.current) return;
      wokeRef.current = true;
      goBack();
    };
    window.addEventListener("keydown", wake, true);
    window.addEventListener("pointerdown", wake, true);
    return () => {
      window.removeEventListener("keydown", wake, true);
      window.removeEventListener("pointerdown", wake, true);
    };
  }, []);
  return <div className="sleep-screen" data-testid="sleep-screen" aria-label={t("player.nextEpisodeOverlay.stillWatching", "Still watching?")} />;
}
