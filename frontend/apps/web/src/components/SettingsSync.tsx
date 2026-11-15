import { useProfileSettingsFollower } from "@evey/client";
import { useDeviceRegistration } from "@evey/client/web";
import { useLocaleStore } from "@evey/store";

const noop = () => {};
const browserName = () => "Browser";

/**
 * Keeps this tab on the settings of the profile it runs as. It listens on the
 * devices socket for a change made elsewhere (listen-only: a browser tab is
 * not a Play On target) and adopts the profile's theme and language as soon
 * as the reloaded profile carries them.
 */
export function SettingsSync() {
  useDeviceRegistration({
    deviceType: "web",
    listenOnly: true,
    deviceName: browserName,
    serverUrl: window.location.origin,
    // A hidden tab drops the socket; coming back reloads the settings anyway.
    pauseInBackground: true,
    onLoadAndPlay: noop,
  });
  useProfileSettingsFollower((_profile, locale) => {
    // Empty is "automatic": the browser keeps the language it already shows.
    if (locale) useLocaleStore.getState().setLocale(locale, "profile");
  });
  return null;
}
