import { isMeteredNetwork } from "./networkHint";

/**
 * The "download started" toast, with a Wi-Fi note appended when the phone is on
 * mobile data. A movie is gigabytes: worth a heads-up, not worth a refusal.
 *
 * Deliberately NOT a gate. Downloading on cellular is a legitimate choice - on
 * a train, or when home is exactly where you are not - so this only names the
 * cost and lets the download run. The probe never blocks the toast either: if
 * the network state cannot be read, the plain message goes out.
 */
export async function downloadStartedMessage(
  t: (key: string, fallback: string) => string,
): Promise<string> {
  const started = t("media.detail.downloadStarted", "Download started");
  if (!(await isMeteredNetwork())) return started;
  return `${started} - ${t("media.detail.downloadOnMobileData", "on mobile data, Wi-Fi recommended")}`;
}
