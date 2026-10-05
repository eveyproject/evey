import i18n from "i18next";

// The download engine and the audio pass post their notifications from native
// code, which knows nothing of the app's language: JS hands them the words.
function tr(key: string, fallback: string, options?: Record<string, unknown>): string {
  try {
    const v = i18n.t(key, fallback, options);
    return typeof v === "string" && v !== "" ? v : fallback;
  } catch {
    return fallback;
  }
}

export function downloadNotificationTexts() {
  return {
    downloadTitle: tr("downloads.notification.title", "Download"),
    downloadStarting: tr("downloads.notification.starting", "Starting download..."),
    // {progress} is the engine's own placeholder, filled natively.
    downloadProgress: tr("downloads.notification.progress", "Downloading... {{percent}}%", { percent: "{progress}" })
      .replace("{{percent}}", "{progress}"),
    downloadPaused: tr("downloads.notification.paused", "Paused"),
    downloadFinished: tr("downloads.notification.finished", "Download complete"),
  };
}

export function audioPassNotificationTitle(): string {
  return tr("downloads.notification.audioTitle", "Optimizing audio");
}
