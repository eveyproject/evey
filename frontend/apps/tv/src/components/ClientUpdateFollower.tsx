import { useClientUpdates } from "@evey/client/native/useClientUpdates";
import { clientUpdateBinding } from "../lib/clientUpdates";

/**
 * Points the app at its server's bundles and nothing more: a downloaded
 * bundle starts with the next launch. A television gets no restart offer -
 * reloading the JS runtime in place is the heaviest thing the app can ask of
 * a small box, and the one a viewer had to unplug theirs after.
 */
export function ClientUpdateFollower() {
  useClientUpdates(clientUpdateBinding, false);
  return null;
}
